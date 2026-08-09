package repository

import (
	"context"
	"time"

	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// ConsoleQueryAuditRepository manages query audit logging.
type ConsoleQueryAuditRepository struct {
	db bun.IDB
}

// NewConsoleQueryAuditRepository constructs a ConsoleQueryAuditRepository.
func NewConsoleQueryAuditRepository(db bun.IDB) *ConsoleQueryAuditRepository {
	return &ConsoleQueryAuditRepository{db: db}
}

// LogQuery logs a query execution with hashed content only (never stores plaintext).
func (r *ConsoleQueryAuditRepository) LogQuery(
	ctx context.Context,
	accountID string,
	databaseID string,
	sessionID string,
	actorID string,
	queryHash string,
	statementType string,
	durationMs int64,
	affectedRows int64,
	status string,
) error {
	log := &model.ConsoleQueryAudit{
		AccountID:     accountID,
		DatabaseID:    databaseID,
		SessionID:     sessionID,
		ActorID:       actorID,
		QueryHash:     queryHash,
		StatementType: statementType,
		DurationMs:    durationMs,
		AffectedRows:  affectedRows,
		Status:        status,
		CreatedAt:     time.Now().UTC(),
	}

	_, err := r.db.NewInsert().Model(log).Exec(ctx)
	return err
}

// ListQueryHistory returns query audit records for a database (account-scoped).
func (r *ConsoleQueryAuditRepository) ListQueryHistory(
	ctx context.Context,
	accountID string,
	databaseID string,
	limit int,
) ([]model.ConsoleQueryAudit, error) {
	var queries []model.ConsoleQueryAudit
	err := r.db.NewSelect().Model(&queries).
		Where("account_id = ?", accountID).
		Where("database_id = ?", databaseID).
		OrderById("DESC").
		Limit(limit).
		Scan(ctx)
	if err != nil {
		return nil, err
	}
	return queries, nil
}
