package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// ResourceOverviewService owns tenant-scoped dashboard aggregate reads.
type ResourceOverviewService struct {
	rows *repository.ResourceOverviewRepo
}

// NewResourceOverviewService constructs a ResourceOverviewService.
func NewResourceOverviewService(rows *repository.ResourceOverviewRepo) *ResourceOverviewService {
	return &ResourceOverviewService{rows: rows}
}

// Get returns live site/database totals and active counts for one account.
func (s *ResourceOverviewService) Get(
	ctx context.Context,
	accountID uuid.UUID,
) (*model.ResourceOverview, error) {
	overview, err := s.rows.GetByAccount(ctx, accountID)
	if err != nil {
		return nil, apperr.Internal("failed to load resource overview").Wrap(err)
	}
	return overview, nil
}
