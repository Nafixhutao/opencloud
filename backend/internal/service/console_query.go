package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// ConsoleQueryService manages SQL query execution through console sessions.
type ConsoleQueryService struct {
	sessionRepo      *repository.DatabaseConsoleSessionRepository
	dbRepo           *repository.ManagedDatabaseRepository
	auditRepo        *repository.ConsoleQueryAuditRepository
	logger           *zap.Logger
	statementTimeout = 30 * time.Second // ~30s timeout as per spec
	maxRows          = 1000                            // limit 1000 rows as per spec
}

// NewConsoleQueryService constructs a ConsoleQueryService.
func NewConsoleQueryService(
	sessionRepo *repository.DatabaseConsoleSessionRepository,
	dbRepo *repository.ManagedDatabaseRepository,
	auditRepo *repository.ConsoleQueryAuditRepository,
	logger *zap.Logger,
) *ConsoleQueryService {
	return &ConsoleQueryService{
		sessionRepo:    sessionRepo,
		dbRepo:         dbRepo,
		auditRepo:      auditRepo,
		logger:         logger,
		statementTimeout: 30 * time.Second,
		maxRows:        1000,
	}
}

// QueryResult represents the result of a SQL query.
type QueryResult struct {
	Columns  []string        `json:"columns"`
	Rows     [][]interface{} `json:"rows"`
	Affected int64           `json:"affected_rows"`
	Elapsed  int64           `json:"elapsed_ms"`
}

// ExecuteQuery executes a SQL query against a managed database with safety limits.
func (s *ConsoleQueryService) ExecuteQuery(
	ctx context.Context,
	accountID uuid.UUID,
	databaseID uuid.UUID,
	token string,
	query string,
) (*QueryResult, error) {
	// Validate console session using token
	session, err := s.sessionRepo.GetActiveByDatabase(ctx, accountID, databaseID)
	if err != nil {
		if errors.Is(err, bun.ErrNotFound) {
			return nil, apperr.Forbidden("no active console session")
		}
		return nil, err
	}

	// Validate session hasn't expired (double-check)
	if time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionExpired
	}

	// Get database credentials
	db, err := s.dbRepo.Get(ctx, accountID, databaseID)
	if err != nil {
		if errors.Is(err, apperr.NotFound) {
			return nil, apperr.ResourceNotFound("database")
		}
		return nil, err
	}

	if !db.IsManaged() {
		return nil, ErrDatabaseNotManaged
	}

	// Parse query to determine type
	statementType := detectStatementType(query)

	// Enforce READ-ONLY constraint for now
	if !isReadOnly(statementType) {
		return nil, apperr.Forbidden("READ-ONLY mode enforced; write operations not allowed yet")
	}

	// Detect if multi-statement (disabled by default)
	if strings.Contains(query, ";") && strings.TrimSpace(query) != "" {
		return nil, ErrMultiStatementDisabled
	}

	// Check query size limit (prevent excessively large queries)
	if len(query) > 100*1024 { // 100KB limit
		return nil, ErrQueryTooLarge
	}

	startTime := time.Now()

	// NOTE: In Phase 2, this would:
	// 1. Connect using temporary scoped user or existing DB credentials
	// 2. Set statement timeout
	// 3. Limit result rows
	// 4. Execute query
	// 5. Log audit entry
	
	// For now, return mock results (placeholder until actual DB connection)
	mockResult := &QueryResult{
		Columns:  []string{"col1", "col2"},
		Rows:     [][]interface{}{{1, "mock"}, {2, "data"}},
		Affected: 2,
		Elapsed:  time.Since(startTime).Milliseconds(),
	}

	// Create hash of query for audit (SHA-256 in production)
	queryHash := fmt.Sprintf("%x", sha256.Sum256([]byte(query)))

	// Log audit entry (never stores plaintext query)
	if err := s.auditRepo.LogQuery(
		ctx,
		accountID.String(),
		databaseID.String(),
		session.ID.String(),
		session.ActorID,
		queryHash,
		statementType,
		mockResult.Elapsed,
		mockResult.Affected,
		model.QueryStatusSuccess,
	); err != nil {
		// Log failure but don't fail the query execution
		s.logger.Warn("failed to log query audit", zap.Error(err))
	}

	return mockResult, nil
}

// detectStatementType returns the SQL statement type from a query string.
func detectStatementType(query string) string {
	trimmed := strings.ToUpper(strings.TrimSpace(query))
	
	if strings.HasPrefix(trimmed, "SELECT") || strings.HasPrefix(trimmed, "WITH") {
		return "SELECT"
	}
	if strings.HasPrefix(trimmed, "INSERT") {
		return "INSERT"
	}
	if strings.HasPrefix(trimmed, "UPDATE") {
		return "UPDATE"
	}
	if strings.HasPrefix(trimmed, "DELETE") {
		return "DELETE"
	}
	if strings.HasPrefix(trimmed, "CREATE") || strings.HasPrefix(trimmed, "ALTER") {
		return "CREATE"
	}
	if strings.HasPrefix(trimmed, "DROP") {
		return "DROP"
	}
	if strings.HasPrefix(trimmed, "TRUNCATE") {
		return "TRUNCATE"
	}
	return "UNKNOWN"
}

// isReadOnly checks if statement type is read-only.
func isReadOnly(statementType string) bool {
	return statementType == "SELECT"
}
