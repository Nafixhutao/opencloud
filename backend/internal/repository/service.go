package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// ServiceRepo manages tenant-owned services.
type ServiceRepo struct {
	db bun.IDB
}

// NewServiceRepo constructs a ServiceRepo.
func NewServiceRepo(db bun.IDB) *ServiceRepo {
	return &ServiceRepo{db: db}
}

// WithDB returns a copy using the given DB.
func (r *ServiceRepo) WithDB(db bun.IDB) *ServiceRepo {
	return &ServiceRepo{db: db}
}

// GetByAccount returns a live service only for its owning tenant.
func (r *ServiceRepo) GetByAccount(ctx context.Context, accountID, serviceID uuid.UUID) (*model.Service, error) {
	svc := new(model.Service)
	err := r.db.NewSelect().Model(svc).
		Where("account_id = ?", accountID).
		Where("id = ?", serviceID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	return svc, err
}
