package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// AuditRepo appends audit log rows (never updates/deletes).
type AuditRepo struct {
	db bun.IDB
}

// NewAuditRepo constructs an AuditRepo.
func NewAuditRepo(db bun.IDB) *AuditRepo {
	return &AuditRepo{db: db}
}

// WithDB returns a copy using db.
func (r *AuditRepo) WithDB(db bun.IDB) *AuditRepo {
	return &AuditRepo{db: db}
}

// Entry is the input for a single audit event.
type Entry struct {
	AccountID *uuid.UUID
	ActorID   *string
	Action    string
	Target    *string
	Metadata  map[string]any
}

// Append writes one audit row. Metadata nil becomes {}.
func (r *AuditRepo) Append(ctx context.Context, e Entry) error {
	meta := e.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	row := &model.AuditLog{
		AccountID: e.AccountID,
		ActorID:   e.ActorID,
		Action:    e.Action,
		Target:    e.Target,
		Metadata:  raw,
		CreatedAt: time.Now().UTC(),
	}
	_, err = r.db.NewInsert().Model(row).Exec(ctx)
	return err
}

// ListByAccount returns recent audit rows for an account (admin/debug).
func (r *AuditRepo) ListByAccount(ctx context.Context, accountID uuid.UUID, limit int) ([]model.AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var rows []model.AuditLog
	err := r.db.NewSelect().Model(&rows).
		Where("account_id = ?", accountID).
		Order("created_at DESC").
		Limit(limit).
		Scan(ctx)
	return rows, err
}

// ListByAction returns recent rows matching action (for tests / admin).
func (r *AuditRepo) ListByAction(ctx context.Context, action string, limit int) ([]model.AuditLog, error) {
	if limit <= 0 {
		limit = 50
	}
	var rows []model.AuditLog
	err := r.db.NewSelect().Model(&rows).
		Where("action = ?", action).
		Order("created_at DESC").
		Limit(limit).
		Scan(ctx)
	return rows, err
}
