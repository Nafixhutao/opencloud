package repository

import (
	"context"

	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/uptrace/bun"
)

// ConsoleQueryAuditRepository handles audit log operations for SQL queries
type ConsoleQueryAuditRepository struct {
	db *bun.DB
}

// NewConsoleQueryAuditRepository creates a new audit repository
func NewConsoleQueryAuditRepository(db *bun.DB) *ConsoleQueryAuditRepository {
	return &ConsoleQueryAuditRepository{db: db}
}

// CreateAuditLog creates an audit record for executed query
func (r *ConsoleQueryAuditRepository) CreateAuditLog(ctx context.Context, audit *model.ConsoleQueryAudit) error {

	_, err := r.db.NewInsert().Model(audit).Exec(ctx)
	return err
}

// GetAuditLogs retrieves audit logs for an account with pagination
func (r *ConsoleQueryAuditRepository) GetAuditLogs(ctx context.Context, accountID string, limit, offset int) ([]*model.ConsoleQueryAudit, error) {

	var audits []*model.ConsoleQueryAudit
	err := r.db.NewSelect().
		Model(&audits).
		Where("account_id = ?", accountID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return audits, nil
}

// GetAuditBySession retrieves audit logs for a specific session
func (r *ConsoleQueryAuditRepository) GetAuditBySession(ctx context.Context, accountID string, sessionID string, limit int) ([]*model.ConsoleQueryAudit, error) {

	var audits []*model.ConsoleQueryAudit
	err := r.db.NewSelect().
		Model(&audits).
		Where("account_id = ? AND session_id = ?", accountID, sessionID).
		Order("created_at DESC").
		Limit(limit).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return audits, nil
}
