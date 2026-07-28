package provisioner

import (
	"context"
	"database/sql"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/model"
)

func TestSQLDatabaseProvisionerPostgresAndMariaDBLifecycle(t *testing.T) {
	if os.Getenv("DATABASE_PROVISIONER_INTEGRATION") != "1" {
		t.Skip("DATABASE_PROVISIONER_INTEGRATION is not enabled")
	}
	postgresAdminURL := os.Getenv("CUSTOMER_POSTGRES_ADMIN_URL")
	mariaDBAdminDSN := os.Getenv("CUSTOMER_MARIADB_ADMIN_DSN")
	if postgresAdminURL == "" || mariaDBAdminDSN == "" {
		t.Fatal("customer database integration admin targets are required")
	}
	postgresHost := envOr("CUSTOMER_POSTGRES_HOST", "127.0.0.1")
	mariaDBHost := envOr("CUSTOMER_MARIADB_HOST", "127.0.0.1")
	postgresPort := envPort(t, "CUSTOMER_POSTGRES_PORT", 5432)
	mariaDBPort := envPort(t, "CUSTOMER_MARIADB_PORT", 3306)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p, err := NewSQLDatabaseProvisioner(
		ctx,
		postgresAdminURL,
		DatabaseEndpoint{Host: postgresHost, Port: postgresPort},
		mariaDBAdminDSN,
		DatabaseEndpoint{Host: mariaDBHost, Port: mariaDBPort},
	)
	require.NoError(t, err)
	defer p.Close()

	for _, engine := range []string{model.DatabaseEnginePostgres, model.DatabaseEngineMariaDB} {
		t.Run(engine, func(t *testing.T) {
			id := uuid.New()
			compact := strings.ReplaceAll(id.String(), "-", "")
			spec := DatabaseSpec{
				DatabaseID:   id,
				AccountID:    uuid.New(),
				Engine:       engine,
				DatabaseName: "ocdb_" + compact,
				Username:     "ocu_" + compact,
			}
			ref := DatabaseRef(spec)
			t.Cleanup(func() {
				_ = p.DeleteDatabase(context.Background(), ref)
			})

			first, err := p.CreateDatabase(ctx, spec)
			require.NoError(t, err)
			require.NotEmpty(t, first.Password)
			assertScopedCredentialCannotMutateSystemDatabase(ctx, t, first)
			firstDB := openCustomerDatabase(t, first)
			_, err = firstDB.ExecContext(ctx, `
				CREATE TABLE opencloud_retry_sentinel (
					id INTEGER PRIMARY KEY,
					value VARCHAR(32) NOT NULL
				)`)
			require.NoError(t, err)
			_, err = firstDB.ExecContext(
				ctx,
				`INSERT INTO opencloud_retry_sentinel (id, value) VALUES (1, 'preserved')`,
			)
			require.NoError(t, err)
			require.NoError(t, firstDB.Close())

			second, err := p.CreateDatabase(ctx, spec)
			require.NoError(t, err)
			require.NotEqual(t, first.Password, second.Password, "retry rotates the scoped password")
			oldDB := openCustomerDatabase(t, first)
			require.Error(t, oldDB.PingContext(ctx), "rotated password must revoke the prior credential")
			require.NoError(t, oldDB.Close())

			secondDB := openCustomerDatabase(t, second)
			var value string
			require.NoError(
				t,
				secondDB.QueryRowContext(
					ctx,
					`SELECT value FROM opencloud_retry_sentinel WHERE id = 1`,
				).Scan(&value),
			)
			require.Equal(t, "preserved", value, "idempotent retry must not replace the database")
			require.NoError(t, secondDB.Close())

			require.NoError(t, p.DeleteDatabase(ctx, ref))
			require.NoError(t, p.DeleteDatabase(ctx, ref), "delete must be idempotent")
			assertPhysicalDatabaseAbsent(ctx, t, p, spec)
			deletedDB := openCustomerDatabase(t, second)
			require.Error(t, deletedDB.PingContext(ctx))
			require.NoError(t, deletedDB.Close())
		})
	}
}

func assertScopedCredentialCannotMutateSystemDatabase(
	ctx context.Context,
	t *testing.T,
	credentials DatabaseCredentials,
) {
	t.Helper()
	other := credentials
	switch credentials.Engine {
	case model.DatabaseEnginePostgres:
		other.Database = "postgres"
	case model.DatabaseEngineMariaDB:
		other.Database = "mysql"
	}
	db := openCustomerDatabase(t, other)
	defer func() { require.NoError(t, db.Close()) }()
	if err := db.PingContext(ctx); err != nil {
		return
	}
	tableName := "opencloud_scope_probe_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err := db.ExecContext(ctx, "CREATE TABLE "+tableName+" (id INTEGER)")
	if err == nil {
		_, _ = db.ExecContext(ctx, "DROP TABLE "+tableName)
	}
	require.Error(t, err, "scoped customer login must not mutate a system database")
}

func TestValidateDatabaseSpecRejectsNonDeterministicIdentifiers(t *testing.T) {
	err := validateDatabaseSpec(DatabaseSpec{
		DatabaseID:   uuid.New(),
		AccountID:    uuid.New(),
		Engine:       model.DatabaseEnginePostgres,
		DatabaseName: "customer_input",
		Username:     "root",
	})
	require.ErrorContains(t, err, "physical identifier")
}

func TestSafeDatabaseProvisionerErrorDropsDriverMessage(t *testing.T) {
	err := safeDatabaseProvisionerError(
		"apply MariaDB lifecycle",
		&mysql.MySQLError{Number: 1064, Message: "syntax near one-time-password"},
	)
	require.ErrorContains(t, err, "MariaDB error 1064")
	require.NotContains(t, err.Error(), "one-time-password")
}

func openCustomerDatabase(t *testing.T, credentials DatabaseCredentials) *sql.DB {
	t.Helper()
	switch credentials.Engine {
	case model.DatabaseEnginePostgres:
		dsn := (&url.URL{
			Scheme: "postgres",
			User:   url.UserPassword(credentials.Username, credentials.Password),
			Host:   net.JoinHostPort(credentials.Host, strconv.Itoa(credentials.Port)),
			Path:   credentials.Database,
			RawQuery: url.Values{
				"sslmode": []string{"disable"},
			}.Encode(),
		}).String()
		cfg, err := pgx.ParseConfig(dsn)
		require.NoError(t, err)
		return stdlib.OpenDB(*cfg)
	case model.DatabaseEngineMariaDB:
		cfg := mysql.NewConfig()
		cfg.User = credentials.Username
		cfg.Passwd = credentials.Password
		cfg.Net = "tcp"
		cfg.Addr = net.JoinHostPort(credentials.Host, strconv.Itoa(credentials.Port))
		cfg.DBName = credentials.Database
		cfg.ParseTime = true
		return mustOpenSQL(t, "mysql", cfg.FormatDSN())
	default:
		t.Fatalf("unsupported engine %q", credentials.Engine)
		return nil
	}
}

func mustOpenSQL(t *testing.T, driver, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open(driver, dsn)
	require.NoError(t, err)
	return db
}

func assertPhysicalDatabaseAbsent(
	ctx context.Context,
	t *testing.T,
	p *SQLDatabaseProvisioner,
	spec DatabaseSpec,
) {
	t.Helper()
	switch spec.Engine {
	case model.DatabaseEnginePostgres:
		var databaseExists, roleExists bool
		require.NoError(
			t,
			p.postgres.QueryRowContext(
				ctx,
				`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`,
				spec.DatabaseName,
			).Scan(&databaseExists),
		)
		require.NoError(
			t,
			p.postgres.QueryRowContext(
				ctx,
				`SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`,
				spec.Username,
			).Scan(&roleExists),
		)
		require.False(t, databaseExists)
		require.False(t, roleExists)
	case model.DatabaseEngineMariaDB:
		var databaseCount, userCount int
		require.NoError(
			t,
			p.mariaDB.QueryRowContext(
				ctx,
				`SELECT count(*) FROM information_schema.SCHEMATA WHERE SCHEMA_NAME = ?`,
				spec.DatabaseName,
			).Scan(&databaseCount),
		)
		require.NoError(
			t,
			p.mariaDB.QueryRowContext(
				ctx,
				`SELECT count(*) FROM mysql.user WHERE User = ? AND Host = '%'`,
				spec.Username,
			).Scan(&userCount),
		)
		require.Zero(t, databaseCount)
		require.Zero(t, userCount)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envPort(t *testing.T, name string, fallback int) int {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	port, err := strconv.Atoi(value)
	require.NoError(t, err)
	return port
}
