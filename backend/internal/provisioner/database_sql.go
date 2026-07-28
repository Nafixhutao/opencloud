package provisioner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/nazxf/opencloud/backend/internal/model"
)

const databaseOperationTimeout = 30 * time.Second

var (
	physicalDatabasePattern = regexp.MustCompile(`^ocdb_[0-9a-f]{32}$`)
	physicalUsernamePattern = regexp.MustCompile(`^ocu_[0-9a-f]{32}$`)
)

// SQLDatabaseProvisioner manages scoped users/databases on dedicated
// PostgreSQL and MariaDB data-plane targets. It never connects to the
// control-plane database.
type SQLDatabaseProvisioner struct {
	postgres         *sql.DB
	mariaDB          *sql.DB
	postgresEndpoint DatabaseEndpoint
	mariaDBEndpoint  DatabaseEndpoint
}

// NewSQLDatabaseProvisioner opens and verifies both configured targets.
func NewSQLDatabaseProvisioner(
	ctx context.Context,
	postgresAdminURL string,
	postgresEndpoint DatabaseEndpoint,
	mariaDBAdminDSN string,
	mariaDBEndpoint DatabaseEndpoint,
) (*SQLDatabaseProvisioner, error) {
	pgConfig, err := pgx.ParseConfig(postgresAdminURL)
	if err != nil {
		return nil, errors.New("invalid customer PostgreSQL admin URL")
	}
	pgDB := stdlib.OpenDB(*pgConfig)
	configureDatabasePool(pgDB)

	if _, err := mysql.ParseDSN(mariaDBAdminDSN); err != nil {
		_ = pgDB.Close()
		return nil, errors.New("invalid customer MariaDB admin DSN")
	}
	mariaDB, err := sql.Open("mysql", mariaDBAdminDSN)
	if err != nil {
		_ = pgDB.Close()
		return nil, fmt.Errorf("open customer MariaDB target: %w", err)
	}
	configureDatabasePool(mariaDB)

	p := &SQLDatabaseProvisioner{
		postgres:         pgDB,
		mariaDB:          mariaDB,
		postgresEndpoint: postgresEndpoint,
		mariaDBEndpoint:  mariaDBEndpoint,
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pgDB.PingContext(pingCtx); err != nil {
		p.Close()
		return nil, fmt.Errorf("ping customer PostgreSQL target: %w", err)
	}
	if err := mariaDB.PingContext(pingCtx); err != nil {
		p.Close()
		return nil, fmt.Errorf("ping customer MariaDB target: %w", err)
	}
	return p, nil
}

// Close releases both target pools.
func (p *SQLDatabaseProvisioner) Close() {
	if p == nil {
		return
	}
	if p.postgres != nil {
		_ = p.postgres.Close()
	}
	if p.mariaDB != nil {
		_ = p.mariaDB.Close()
	}
}

// CreateDatabase creates or converges a scoped database and rotates its login
// password on every retry. Provider work is idempotent because physical names
// are deterministic and ownership is checked before mutation.
func (p *SQLDatabaseProvisioner) CreateDatabase(
	ctx context.Context,
	spec DatabaseSpec,
) (DatabaseCredentials, error) {
	if err := validateDatabaseSpec(spec); err != nil {
		return DatabaseCredentials{}, err
	}
	password, err := randomDatabasePassword()
	if err != nil {
		return DatabaseCredentials{}, fmt.Errorf("generate database credential: %w", err)
	}
	opCtx, cancel := context.WithTimeout(ctx, databaseOperationTimeout)
	defer cancel()

	switch spec.Engine {
	case model.DatabaseEnginePostgres:
		if err := p.createPostgres(opCtx, spec, password); err != nil {
			return DatabaseCredentials{}, err
		}
		return credentialsFor(spec, password, p.postgresEndpoint), nil
	case model.DatabaseEngineMariaDB:
		if err := p.createMariaDB(opCtx, spec, password); err != nil {
			return DatabaseCredentials{}, err
		}
		return credentialsFor(spec, password, p.mariaDBEndpoint), nil
	default:
		return DatabaseCredentials{}, fmt.Errorf("unsupported database engine %q", spec.Engine)
	}
}

// DeleteDatabase idempotently removes the database and its scoped login.
func (p *SQLDatabaseProvisioner) DeleteDatabase(ctx context.Context, ref DatabaseRef) error {
	if err := validateDatabaseRef(ref); err != nil {
		return err
	}
	opCtx, cancel := context.WithTimeout(ctx, databaseOperationTimeout)
	defer cancel()
	switch ref.Engine {
	case model.DatabaseEnginePostgres:
		return p.deletePostgres(opCtx, ref)
	case model.DatabaseEngineMariaDB:
		return p.deleteMariaDB(opCtx, ref)
	default:
		return fmt.Errorf("unsupported database engine %q", ref.Engine)
	}
}

func (p *SQLDatabaseProvisioner) createPostgres(
	ctx context.Context,
	spec DatabaseSpec,
	password string,
) error {
	createRole, err := postgresFormat(
		ctx,
		p.postgres,
		`SELECT format(
			'CREATE ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION',
			$1::text, $2::text
		)`,
		spec.Username,
		password,
	)
	if err != nil {
		return safeDatabaseProvisionerError("format PostgreSQL role creation", err)
	}
	if _, err := p.postgres.ExecContext(ctx, createRole); err != nil && !postgresCode(err, "42710") {
		return safeDatabaseProvisionerError("create PostgreSQL role", err)
	}
	alterRole, err := postgresFormat(
		ctx,
		p.postgres,
		`SELECT format(
			'ALTER ROLE %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOREPLICATION',
			$1::text, $2::text
		)`,
		spec.Username,
		password,
	)
	if err != nil {
		return safeDatabaseProvisionerError("format PostgreSQL role update", err)
	}
	if _, err := p.postgres.ExecContext(ctx, alterRole); err != nil {
		return safeDatabaseProvisionerError("update PostgreSQL role", err)
	}

	createDatabase, err := postgresFormat(
		ctx,
		p.postgres,
		`SELECT format('CREATE DATABASE %I OWNER %I', $1::text, $2::text)`,
		spec.DatabaseName,
		spec.Username,
	)
	if err != nil {
		return fmt.Errorf("format PostgreSQL database creation: %w", err)
	}
	if _, err := p.postgres.ExecContext(ctx, createDatabase); err != nil && !postgresCode(err, "42P04") {
		return fmt.Errorf("create PostgreSQL database: %w", err)
	}
	for _, operation := range []struct {
		query string
		args  []any
	}{
		{
			query: `SELECT format('ALTER DATABASE %I OWNER TO %I', $1::text, $2::text)`,
			args:  []any{spec.DatabaseName, spec.Username},
		},
		{
			query: `SELECT format('REVOKE CONNECT ON DATABASE %I FROM PUBLIC', $1::text)`,
			args:  []any{spec.DatabaseName},
		},
		{
			query: `SELECT format('GRANT CONNECT ON DATABASE %I TO %I', $1::text, $2::text)`,
			args:  []any{spec.DatabaseName, spec.Username},
		},
	} {
		statement, err := postgresFormat(ctx, p.postgres, operation.query, operation.args...)
		if err != nil {
			return fmt.Errorf("format PostgreSQL database grant: %w", err)
		}
		if _, err := p.postgres.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply PostgreSQL database grant: %w", err)
		}
	}
	return nil
}

func (p *SQLDatabaseProvisioner) deletePostgres(ctx context.Context, ref DatabaseRef) error {
	if _, err := p.postgres.ExecContext(
		ctx,
		`SELECT pg_terminate_backend(pid)
		 FROM pg_stat_activity
		 WHERE (datname = $1 OR usename = $2)
		   AND pid <> pg_backend_pid()`,
		ref.DatabaseName,
		ref.Username,
	); err != nil {
		return fmt.Errorf("terminate PostgreSQL database sessions: %w", err)
	}
	dropDatabase, err := postgresFormat(
		ctx,
		p.postgres,
		`SELECT format('DROP DATABASE IF EXISTS %I', $1::text)`,
		ref.DatabaseName,
	)
	if err != nil {
		return fmt.Errorf("format PostgreSQL database deletion: %w", err)
	}
	if _, err := p.postgres.ExecContext(ctx, dropDatabase); err != nil {
		return fmt.Errorf("delete PostgreSQL database: %w", err)
	}
	dropRole, err := postgresFormat(
		ctx,
		p.postgres,
		`SELECT format('DROP ROLE IF EXISTS %I', $1::text)`,
		ref.Username,
	)
	if err != nil {
		return fmt.Errorf("format PostgreSQL role deletion: %w", err)
	}
	if _, err := p.postgres.ExecContext(ctx, dropRole); err != nil {
		return fmt.Errorf("delete PostgreSQL role: %w", err)
	}
	return nil
}

func (p *SQLDatabaseProvisioner) createMariaDB(
	ctx context.Context,
	spec DatabaseSpec,
	password string,
) error {
	quotedUser, err := mariaDBQuote(ctx, p.mariaDB, spec.Username)
	if err != nil {
		return fmt.Errorf("quote MariaDB username: %w", err)
	}
	quotedPassword, err := mariaDBQuote(ctx, p.mariaDB, password)
	if err != nil {
		return safeDatabaseProvisionerError("quote MariaDB password", err)
	}
	account := quotedUser + "@'%'"
	databaseName := mariaDBIdentifier(spec.DatabaseName)
	statements := []string{
		"CREATE DATABASE IF NOT EXISTS " + databaseName +
			" CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		"CREATE USER IF NOT EXISTS " + account + " IDENTIFIED BY " + quotedPassword,
		"ALTER USER " + account + " IDENTIFIED BY " + quotedPassword,
		"GRANT ALL PRIVILEGES ON " + databaseName + ".* TO " + account,
	}
	for _, statement := range statements {
		if _, err := p.mariaDB.ExecContext(ctx, statement); err != nil {
			return safeDatabaseProvisionerError("apply MariaDB database lifecycle statement", err)
		}
	}
	return nil
}

func (p *SQLDatabaseProvisioner) deleteMariaDB(ctx context.Context, ref DatabaseRef) error {
	quotedUser, err := mariaDBQuote(ctx, p.mariaDB, ref.Username)
	if err != nil {
		return fmt.Errorf("quote MariaDB username: %w", err)
	}
	if _, err := p.mariaDB.ExecContext(
		ctx,
		"DROP DATABASE IF EXISTS "+mariaDBIdentifier(ref.DatabaseName),
	); err != nil {
		return fmt.Errorf("delete MariaDB database: %w", err)
	}
	if _, err := p.mariaDB.ExecContext(
		ctx,
		"DROP USER IF EXISTS "+quotedUser+"@'%'",
	); err != nil {
		return fmt.Errorf("delete MariaDB user: %w", err)
	}
	return nil
}

func postgresFormat(
	ctx context.Context,
	db *sql.DB,
	query string,
	args ...any,
) (string, error) {
	var statement string
	if err := db.QueryRowContext(ctx, query, args...).Scan(&statement); err != nil {
		return "", err
	}
	return statement, nil
}

func postgresCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

func safeDatabaseProvisionerError(action string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return fmt.Errorf("%s (PostgreSQL SQLSTATE %s)", action, pgErr.Code)
	}
	var mariaErr *mysql.MySQLError
	if errors.As(err, &mariaErr) {
		return fmt.Errorf("%s (MariaDB error %d)", action, mariaErr.Number)
	}
	return errors.New(action)
}

func mariaDBQuote(ctx context.Context, db *sql.DB, value string) (string, error) {
	var quoted sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT QUOTE(?)`, value).Scan(&quoted); err != nil {
		return "", err
	}
	if !quoted.Valid {
		return "", errors.New("MariaDB returned a null quoted value")
	}
	return quoted.String, nil
}

func mariaDBIdentifier(value string) string {
	// validateDatabaseSpec/ref accepts only a deterministic lower-hex pattern,
	// so backtick escaping is unnecessary and customer input never reaches DDL.
	return "`" + value + "`"
}

func validateDatabaseSpec(spec DatabaseSpec) error {
	if spec.DatabaseID == [16]byte{} || spec.AccountID == [16]byte{} {
		return errors.New("database and account ids are required")
	}
	if !physicalDatabasePattern.MatchString(spec.DatabaseName) ||
		!physicalUsernamePattern.MatchString(spec.Username) {
		return errors.New("invalid managed database physical identifier")
	}
	if spec.Engine != model.DatabaseEnginePostgres && spec.Engine != model.DatabaseEngineMariaDB {
		return fmt.Errorf("unsupported database engine %q", spec.Engine)
	}
	return nil
}

func validateDatabaseRef(ref DatabaseRef) error {
	return validateDatabaseSpec(DatabaseSpec(ref))
}

func credentialsFor(
	spec DatabaseSpec,
	password string,
	endpoint DatabaseEndpoint,
) DatabaseCredentials {
	return DatabaseCredentials{
		Engine:      spec.Engine,
		Host:        endpoint.Host,
		Port:        endpoint.Port,
		Database:    spec.DatabaseName,
		Username:    spec.Username,
		Password:    password,
		TLSRequired: endpoint.TLSRequired,
	}
}

func configureDatabasePool(db *sql.DB) {
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
}
