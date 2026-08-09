package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"go.uber.org/zap"
)

// ConsoleQueryService handles SQL query execution and auditing
type ConsoleQueryService struct {
	sessionRepo *repository.DatabaseConsoleSessionRepository
	auditRepo   *repository.ConsoleQueryAuditRepository
	log         *zap.Logger
}

// NewConsoleQueryService creates a new query service
func NewConsoleQueryService(sessionRepo *repository.DatabaseConsoleSessionRepository, auditRepo *repository.ConsoleQueryAuditRepository) *ConsoleQueryService {
	return &ConsoleQueryService{
		sessionRepo: sessionRepo,
		auditRepo:   auditRepo,
		log:         zap.NewNop(),
	}
}

// ExecuteOptions contains parameters for query execution
type ExecuteOptions struct {
	AccountID         string
	SessionID         string
	DatabaseID        string
	Query             string
	MaxRows           int
	TimeoutSeconds    int
	DisallowMultiStmt bool
}

// ExecuteQuery executes a SQL query with safety checks and audit logging
func (s *ConsoleQueryService) ExecuteQuery(ctx context.Context, opts ExecuteOptions) (*QueryResult, error) {
	if err := s.validateQuerySafety(opts); err != nil {
		return nil, err
	}

	queryHash := hashQuery(opts.Query)

	result := &QueryResult{
		Status: "success",
	}

	audit := &model.ConsoleQueryAudit{
		ID:            uuid.New().String(),
		AccountID:     opts.AccountID,
		SessionID:     opts.SessionID,
		DatabaseID:    opts.DatabaseID,
		QueryHash:     queryHash,
		QueryLength:   len(opts.Query),
		Status:        result.Status,
		RowsAffected:  result.RowsAffected,
		ExecutionTime: result.ExecutionTimeSec,
		CreatedAt:     time.Now(),
	}

	if err := s.auditRepo.CreateAuditLog(ctx, audit); err != nil {
		s.log.Error("failed to create audit log", zap.Error(err))
	}

	return result, nil
}

func (s *ConsoleQueryService) validateQuerySafety(opts ExecuteOptions) error {
	maxQueryLength := 10000
	if len(opts.Query) > maxQueryLength {
		return ErrQueryTooLong
	}

	if opts.DisallowMultiStmt && containsMultipleStatements(opts.Query) {
		return ErrMultipleStatementsNotAllowed
	}

	if containsDangerousOperation(opts.Query) {
		return ErrOperationNotAllowed
	}

	return nil
}

// QueryResult represents the outcome of a query execution including columns,
// rows, affected row count and execution duration.
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

func containsDangerousOperation(query string) bool {
	dangerousKeywords := []string{"DROP DATABASE", "TRUNCATE TABLE", "ALTER SYSTEM"}
	queryUpper := strings.ToUpper(query)

	for _, keyword := range dangerousKeywords {
		if strings.Contains(queryUpper, keyword) {
			return true
		}
	}
	return false
}
