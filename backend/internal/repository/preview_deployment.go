package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// PreviewDeploymentRepo manages preview deployment records.
type PreviewDeploymentRepo struct {
	db bun.IDB
}

// NewPreviewDeploymentRepo constructs a preview deployment repo.
func NewPreviewDeploymentRepo(db bun.IDB) *PreviewDeploymentRepo {
	return &PreviewDeploymentRepo{db: db}
}

// WithDB returns a copy using the given DB handle.
func (r *PreviewDeploymentRepo) WithDB(db bun.IDB) *PreviewDeploymentRepo {
	return &PreviewDeploymentRepo{db: db}
}

// ListByService returns active preview deployments for a service.
func (r *PreviewDeploymentRepo) ListByService(ctx context.Context, accountID, serviceID uuid.UUID) ([]model.PreviewDeployment, error) {
	var previews []model.PreviewDeployment
	err := r.db.NewSelect().Model(&previews).
		Where("account_id = ?", accountID).
		Where("service_id = ?", serviceID).
		Where("deleted_at IS NULL").
		Where("status IN (?)", model.PreviewBuilding, model.PreviewDeploying, model.PreviewActive).
		Order("created_at DESC").
		Scan(ctx)
	return previews, err
}

// Create inserts a new preview deployment.
func (r *PreviewDeploymentRepo) Create(ctx context.Context, preview *model.PreviewDeployment) error {
	_, err := r.db.NewInsert().Model(preview).Exec(ctx)
	return err
}
