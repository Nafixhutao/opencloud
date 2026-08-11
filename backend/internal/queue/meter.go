package queue

import (
	"context"
	"time"

	"github.com/uptrace/bun"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

// ResourceMeter collects account resource usage snapshots.
type ResourceMeter struct {
	log        *zap.Logger
	db         *bun.DB
	usageRepo  *repository.ResourceUsageRepo
	siteRepo   *repository.SiteRepo
	dbRepo     *repository.ManagedDatabaseRepo
	bucketRepo *repository.StorageBucketRepo
}

// NewResourceMeter creates a periodic resource usage collector.
func NewResourceMeter(
	log *zap.Logger,
	db *bun.DB,
	usageRepo *repository.ResourceUsageRepo,
	siteRepo *repository.SiteRepo,
	dbRepo *repository.ManagedDatabaseRepo,
	bucketRepo *repository.StorageBucketRepo,
) *ResourceMeter {
	return &ResourceMeter{
		log: log, db: db,
		usageRepo: usageRepo,
		siteRepo: siteRepo, dbRepo: dbRepo, bucketRepo: bucketRepo,
	}
}

// Run periodically records resource snapshots until ctx is cancelled.
func (m *ResourceMeter) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	m.log.Info("resource meter started, snapshot every hour")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.takeSnapshot(ctx)
		}
	}
}

func (m *ResourceMeter) takeSnapshot(ctx context.Context) {
	m.log.Info("taking resource snapshot")

	var accounts []model.Account
	if err := m.db.NewSelect().Model(&accounts).Scan(ctx); err != nil {
		m.log.Error("resource meter failed to list accounts", zap.Error(err))
		return
	}

	var recorded int
	for _, acct := range accounts {
		usage := &model.ResourceUsage{AccountID: acct.ID}

		sites, _ := m.db.NewSelect().Model((*model.Site)(nil)).
			Where("account_id = ?", acct.ID).
			Where("status IN (?)", model.SiteActive, model.SiteSuspended, model.SiteProvisioning).
			Count(ctx)
		usage.ActiveSites = sites

		dbs, _ := m.db.NewSelect().Model((*model.ManagedDatabase)(nil)).
			Where("account_id = ?", acct.ID).
			Where("status IN (?)", model.DatabaseActive, model.DatabaseProvisioning).
			Count(ctx)
		usage.ActiveDatabases = dbs

		var storageBytes int64
		var storageObjects int64
		_ = m.db.NewSelect().Model((*model.StorageBucket)(nil)).
			ColumnExpr("COALESCE(SUM(bytes_used), 0) AS storage_bytes").
			ColumnExpr("COALESCE(SUM(object_count), 0) AS storage_objects").
			Where("account_id = ?", acct.ID).
			Where("status = ?", model.BucketActive).
			Scan(ctx, &storageBytes, &storageObjects)
		usage.StorageBytes = storageBytes
		usage.StorageObjects = storageObjects

		if err := m.usageRepo.RecordSnapshot(ctx, usage); err != nil {
			m.log.Error("resource meter failed to record", zap.Error(err))
			continue
		}
		recorded++
	}

	m.log.Info("resource snapshot complete", zap.Int("accounts", recorded))
}
