package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// ResourceOverviewRepo reads tenant-scoped dashboard aggregates.
type ResourceOverviewRepo struct {
	db bun.IDB
}

// NewResourceOverviewRepo constructs a ResourceOverviewRepo.
func NewResourceOverviewRepo(db bun.IDB) *ResourceOverviewRepo {
	return &ResourceOverviewRepo{db: db}
}

// GetByAccount returns live site/database totals and active counts in one
// PostgreSQL statement so dashboard metrics do not depend on paginated lists.
func (r *ResourceOverviewRepo) GetByAccount(
	ctx context.Context,
	accountID uuid.UUID,
) (*model.ResourceOverview, error) {
	overview := new(model.ResourceOverview)
	err := r.db.NewRaw(`
		WITH site_counts AS (
			SELECT
				count(*) AS total,
				count(*) FILTER (WHERE status = ?) AS active
			FROM sites
			WHERE account_id = ?
			  AND deleted_at IS NULL
		),
		database_counts AS (
			SELECT
				count(*) AS total,
				count(*) FILTER (WHERE status = ?) AS active
			FROM databases
			WHERE account_id = ?
			  AND deleted_at IS NULL
		)
		SELECT
			site_counts.total AS sites_total,
			site_counts.active AS sites_active,
			database_counts.total AS databases_total,
			database_counts.active AS databases_active
		FROM site_counts
		CROSS JOIN database_counts
	`, model.SiteActive, accountID, model.DatabaseActive, accountID).Scan(ctx, overview)
	if err != nil {
		return nil, err
	}
	return overview, nil
}
