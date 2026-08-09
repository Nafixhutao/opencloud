package repository

import (
	"context"

	"github.com/nazxf/opencloud/backend/internal/model"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"xorm.io/bun"
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
	ctx, span := tracer.StartSpanFromContext(ctx, "repository.ConsoleQueryAuditRepository.CreateAuditLog")
	defer span.Finish()

	_, err := r.db.NewInsert().Model(audit).Exec(ctx)
	return err
}

// GetAuditLogs retrieves audit logs for an account with pagination
func (r *ConsoleQueryAuditRepository) GetAuditLogs(ctx context.Context, accountID string, limit, offset int) ([]*model.ConsoleQueryAudit, error) {
	ctx, span := tracer.StartSpanFromContext(ctx, "repository.ConsoleQueryAuditRepository.GetAuditLogs")
	defer span.Finish()

	var audits []*model.ConsoleQueryAudit
	err := r.db.NewSelect().
		Model(&audits).
		Where("account_id = ?", accountID).
		OrderDesc("created_at").
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
	ctx, span := tracer.StartSpanFromContext(ctx, "repository.ConsoleQueryAuditRepository.GetAuditBySession")
	defer span.Finish()

	var audits []*model.ConsoleQueryAudit
	err := r.db.NewSelect().
		Model(&audits).
		Where("account_id = ? AND session_id = ?", accountID, sessionID).
		OrderDesc("created_at").
		Limit(limit).
		Scan(ctx)

	if err != nil {
		return nil, err
	}

	return audits, nil
}
