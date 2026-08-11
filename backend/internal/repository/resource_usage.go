package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// ResourceUsageRepo manages resource metering records.
type ResourceUsageRepo struct {
	db bun.IDB
}

// NewResourceUsageRepo constructs a resource usage repo.
func NewResourceUsageRepo(db bun.IDB) *ResourceUsageRepo {
	return &ResourceUsageRepo{db: db}
}

// WithDB returns a copy using the given DB handle.
func (r *ResourceUsageRepo) WithDB(db bun.IDB) *ResourceUsageRepo {
	return &ResourceUsageRepo{db: db}
}

// RecordSnapshot inserts a new usage snapshot.
func (r *ResourceUsageRepo) RecordSnapshot(ctx context.Context, usage *model.ResourceUsage) error {
	usage.RecordedAt = time.Now().UTC()
	_, err := r.db.NewInsert().Model(usage).Exec(ctx)
	return err
}

// LatestByAccount returns the most recent snapshot for an account.
func (r *ResourceUsageRepo) LatestByAccount(ctx context.Context, accountID uuid.UUID) (*model.ResourceUsage, error) {
	usage := new(model.ResourceUsage)
	err := r.db.NewSelect().Model(usage).
		Where("account_id = ?", accountID).
		Order("recorded_at DESC").
		Limit(1).
		Scan(ctx)
	return usage, err
}
