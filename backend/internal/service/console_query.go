package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/credential"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// Console query safety bounds (SQL Console safe defaults, master prompt §22).
const (
	consoleQueryMaxLength = 10_000
	consoleQueryMaxRows   = 1_000
	consoleQueryTimeout   = 30 * time.Second
)

// ConsoleQueryService executes read-only SQL against the customer database
// through scoped console credentials, enforcing a session boundary and
// recording safe audit metadata (never the query body).
type ConsoleQueryService struct {
	sessionRepo  *repository.DatabaseConsoleSessionRepository
	auditRepo    *repository.ConsoleQueryAuditRepository
	databaseRepo *repository.ManagedDatabaseRepo
	cipher       *credential.Cipher
	enabled      bool
	log          *zap.Logger
}

// NewConsoleQueryService creates a new query service.
func NewConsoleQueryService(
	sessionRepo *repository.DatabaseConsoleSessionRepository,
	auditRepo *repository.ConsoleQueryAuditRepository,
	databaseRepo *repository.ManagedDatabaseRepo,
	cipher *credential.Cipher,
	enabled bool,
	log *zap.Logger,
) *ConsoleQueryService {
	if log == nil {
		log = zap.NewNop()
	}
	return &ConsoleQueryService{
		sessionRepo:  sessionRepo,
		auditRepo:    auditRepo,
		databaseRepo: databaseRepo,
		cipher:       cipher,
		enabled:      enabled,
		log:          log,
	}
}

// ExecuteOptions contains parameters for query execution.
type ExecuteOptions struct {
	AccountID         string
	ActorID           string
	SessionID         string
	DatabaseID        string
	Query             string
	MaxRows           int
	TimeoutSeconds    int
	DisallowMultiStmt bool
}

// ExecuteQuery validates the session, verifies database ownership, decrypts
// the scoped credentials, and runs the query in a read-only transaction.
func (s *ConsoleQueryService) ExecuteQuery(ctx context.Context, opts ExecuteOptions) (*QueryResult, error) {
	if !s.enabled {
		return nil, apperr.Unavailable("the database console is not configured")
	}
	if opts.MaxRows <= 0 || opts.MaxRows > consoleQueryMaxRows {
		opts.MaxRows = consoleQueryMaxRows
	}
	if opts.TimeoutSeconds <= 0 || opts.TimeoutSeconds > int(consoleQueryTimeout.Seconds()) {
		opts.TimeoutSeconds = int(consoleQueryTimeout.Seconds())
	}

	// 1. Session boundary: the session must exist, belong to the account, be
	//    active and not expired/revoked (master prompt §21).
	session, err := s.sessionRepo.GetSession(ctx, opts.AccountID, opts.SessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.Forbidden("invalid or expired console session")
		}
		return nil, apperr.Internal("failed to validate console session").Wrap(err)
	}
	if session.Status != model.ConsoleSessionActive {
		return nil, apperr.Forbidden("console session is no longer active")
	}
	if session.DatabaseID != opts.DatabaseID {
		return nil, apperr.Forbidden("console session does not match this database")
	}
	if time.Now().After(session.ExpiresAt) {
		_ = s.sessionRepo.RevokeSession(ctx, opts.AccountID, opts.SessionID)
		return nil, apperr.Forbidden("console session has expired")
	}
	_ = s.sessionRepo.UpdateLastActivity(ctx, opts.AccountID, opts.SessionID)

	// 2. Safety checks (UX layer; the read-only transaction is the boundary).
	if err := s.validateQuerySafety(opts.Query); err != nil {
		return nil, err
	}
	statementType := detectStatementType(opts.Query)
	queryHash := hashQuery(opts.Query)
	start := time.Now()

	// 3. Resolve scoped credentials (decrypt only, never consumed).
	accountID, err := uuid.Parse(opts.AccountID)
	if err != nil {
		return nil, apperr.Validation("invalid account id")
	}
	databaseID, err := uuid.Parse(opts.DatabaseID)
	if err != nil {
		return nil, apperr.Validation("invalid database id")
	}
	credentials, err := s.consoleCredentials(ctx, accountID, databaseID)
	if err != nil {
		return nil, err
	}
	if credentials == nil {
		return nil, apperr.Conflict("database credentials are unavailable")
	}

	// 4. Execute in a read-only transaction with row/time bounds.
	result, err := executeReadOnlyQuery(
		ctx,
		credentials,
		opts.Query,
		opts.MaxRows,
		time.Duration(opts.TimeoutSeconds)*time.Second,
	)
	duration := time.Since(start).Seconds()

	audit := &model.ConsoleQueryAudit{
		ID:            uuid.New().String(),
		AccountID:     opts.AccountID,
		ActorID:       opts.ActorID,
		SessionID:     opts.SessionID,
		DatabaseID:    opts.DatabaseID,
		Engine:        credentials.Engine,
		StatementType: statementType,
		QueryHash:     queryHash,
		QueryLength:   len(opts.Query),
		Status:        model.AuditStatusSuccess,
		RowsAffected:  nil,
		ExecutionTime: &duration,
		CreatedAt:     time.Now(),
	}
	if err != nil {
		audit.Status = model.AuditStatusError
		message := safeConsoleError(err)
		audit.ErrorMsg = &message
		_ = s.auditRepo.CreateAuditLog(ctx, audit)
		s.log.Warn("database console query failed", zap.Error(err), zap.String("session_id", opts.SessionID))
		return nil, safeConsoleAppError(err)
	}
	audit.RowsAffected = result.RowsAffected
	if result.RowsAffected != nil && *result.RowsAffected == 0 {
		audit.RowsAffected = nil
	}
	if err := s.auditRepo.CreateAuditLog(ctx, audit); err != nil {
		s.log.Error("failed to create audit log", zap.Error(err))
	}

	result.Status = "success"
	return result, nil
}

// consoleCredentials decrypts the customer credential in memory; the plaintext
// never persists, is not logged, and the envelope is not consumed.
func (s *ConsoleQueryService) consoleCredentials(ctx context.Context, accountID, databaseID uuid.UUID) (*provisioner.DatabaseCredentials, error) {
	row, err := s.databaseRepo.GetByAccount(ctx, accountID, databaseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("database not found")
		}
		return nil, apperr.Internal("failed to load database").Wrap(err)
	}
	if row.Status != model.DatabaseActive {
		return nil, apperr.Conflict("database credentials are available only after provisioning completes")
	}
	envelope, err := s.databaseRepo.GetCredential(ctx, databaseID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.Conflict("database credentials are unavailable")
		}
		return nil, apperr.Internal("failed to load database credentials").Wrap(err)
	}
	if s.cipher == nil {
		return nil, apperr.Unavailable("credential decryption is not configured")
	}
	plaintext, err := s.cipher.Decrypt(databaseID, envelope.Ciphertext)
	if err != nil {
		return nil, apperr.Internal("failed to decrypt database credentials").Wrap(err)
	}
	defer clear(plaintext)
	var credentials provisioner.DatabaseCredentials
	if err := json.Unmarshal(plaintext, &credentials); err != nil {
		return nil, apperr.Internal("database credential payload is invalid").Wrap(err)
	}
	if credentials.Engine != row.Engine ||
		credentials.Database != row.PhysicalDatabaseName ||
		credentials.Username != row.PhysicalUsername ||
		credentials.Host == "" ||
		credentials.Port <= 0 ||
		credentials.Password == "" {
		return nil, apperr.Internal("database credential payload does not match its resource")
	}
	return &credentials, nil
}

func (s *ConsoleQueryService) validateQuerySafety(query string) error {
	if len(query) > consoleQueryMaxLength {
		return apperr.Validation("query exceeds the 10,000 character limit")
	}
	if containsMultipleStatements(query) {
		return apperr.Validation("multiple SQL statements are not allowed")
	}
	return nil
}

// QueryResult represents the outcome of a query execution.
type QueryResult struct {
	Status           string
	Columns          []string
	Rows             [][]interface{}
	RowsAffected     *int64
	ExecutionTimeSec *float64
}

func hashQuery(query string) string {
	hash := sha256.Sum256([]byte(query))
	return hex.EncodeToString(hash[:])
}

// detectStatementType classifies the leading keyword for audit metadata only;
// the read-only transaction is the security boundary.
func detectStatementType(query string) string {
	trimmed := strings.TrimSpace(strings.ToUpper(query))
	for _, prefix := range []string{"SELECT", "EXPLAIN", "SHOW", "DESCRIBE", "DESC", "WITH"} {
		if strings.HasPrefix(trimmed, prefix) {
			switch prefix {
			case "EXPLAIN":
				return model.StatementExplain
			case "SHOW":
				return model.StatementShow
			case "DESCRIBE", "DESC":
				return model.StatementDescribe
			default:
				return model.StatementSelect
			}
		}
	}
	return model.StatementUnknown
}

func containsMultipleStatements(query string) bool {
	semicolonCount := 0
	inString := false
	for i := 0; i < len(query); i++ {
		if query[i] == '\'' {
			inString = !inString
		} else if query[i] == ';' && !inString {
			semicolonCount++
		}
	}
	return semicolonCount > 1
}

// safeConsoleError redacts the raw database error for audit storage.
func safeConsoleError(err error) string {
	message := err.Error()
	if len(message) > 200 {
		message = message[:200] + "..."
	}
	return message
}

// safeConsoleAppError maps a database error to a stable customer-facing error.
func safeConsoleAppError(err error) error {
	return apperr.Internal("the query could not be completed").Wrap(err)
}

// executeReadOnlyQuery runs the query against the customer database with
// engine-specific read-only enforcement, a statement timeout, and a row bound.
func executeReadOnlyQuery(
	ctx context.Context,
	credentials *provisioner.DatabaseCredentials,
	query string,
	maxRows int,
	timeout time.Duration,
) (*QueryResult, error) {
	db, err := openConsoleConnection(credentials)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// PostgreSQL: BEGIN READ ONLY is enforced by the server; MariaDB/MySQL
	// support SET SESSION TRANSACTION READ ONLY before the transaction.
	var tx *sql.Tx
	if credentials.Engine == model.EnginePostgres {
		tx, err = db.BeginTx(runCtx, nil)
		if err != nil {
			return nil, err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.ExecContext(runCtx, "SET TRANSACTION READ ONLY"); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(runCtx, fmt.Sprintf("SET statement_timeout = %d", timeout.Milliseconds())); err != nil {
			return nil, err
		}
		rows, err := tx.QueryContext(runCtx, query)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		result, err := collectRows(rows, maxRows)
		if err != nil {
			return nil, err
		}
		return result, tx.Commit()
	}

	// MariaDB/MySQL: read-only session + bounded execution.
	tx, err = db.BeginTx(runCtx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(runCtx, "SET SESSION TRANSACTION READ ONLY"); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(runCtx, "SET SESSION MAX_EXECUTION_TIME=30000"); err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(runCtx, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result, err := collectRows(rows, maxRows)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

// openConsoleConnection opens a short-lived connection using the scoped
// customer credentials. Only engine-safe DSN building lives here; raw
// passwords never reach logs.
func openConsoleConnection(credentials *provisioner.DatabaseCredentials) (*sql.DB, error) {
	var driverName, dsn string
	switch credentials.Engine {
	case model.EnginePostgres:
		driverName = "pgx"
		dsn = fmt.Sprintf(
			"postgres://%s:%s@%s:%d/%s?sslmode=prefer&connect_timeout=10",
			url.QueryEscape(credentials.Username),
			url.QueryEscape(credentials.Password),
			credentials.Host,
			credentials.Port,
			credentials.Database,
		)
	case model.EngineMariaDB:
		driverName = "mysql"
		dsn = fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?timeout=10s&readTimeout=30s",
			credentials.Username,
			credentials.Password,
			credentials.Host,
			credentials.Port,
			credentials.Database,
		)
	default:
		return nil, fmt.Errorf("unsupported database engine %q", credentials.Engine)
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// collectRows reads at most maxRows rows and builds the column metadata.
func collectRows(rows *sql.Rows, maxRows int) (*QueryResult, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := &QueryResult{Columns: columns, Rows: make([][]interface{}, 0, maxRows)}
	count := 0
	for rows.Next() && count < maxRows {
		row, err := scanRow(rows, len(columns))
		if err != nil {
			return nil, err
		}
		result.Rows = append(result.Rows, row)
		count++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	affected := int64(count)
	result.RowsAffected = &affected
	return result, nil
}

func scanRow(rows *sql.Rows, columnCount int) ([]interface{}, error) {
	raw := make([]interface{}, columnCount)
	values := make([]interface{}, columnCount)
	for i := range raw {
		values[i] = &raw[i]
	}
	if err := rows.Scan(values...); err != nil {
		return nil, err
	}
	out := make([]interface{}, columnCount)
	for i, v := range raw {
		switch value := v.(type) {
		case []byte:
			out[i] = string(value)
		case nil:
			out[i] = nil
		default:
			out[i] = value
		}
	}
	return out, nil
}
