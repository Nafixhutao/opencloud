package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// SiteRepo manages tenant-owned sites. Customer methods always require an
// account ID; explicitly named Worker methods are reserved for async jobs.
type SiteRepo struct {
	db bun.IDB
}

// NewSiteRepo constructs a SiteRepo.
func NewSiteRepo(db bun.IDB) *SiteRepo {
	return &SiteRepo{db: db}
}

// WithDB returns a copy using db.
func (r *SiteRepo) WithDB(db bun.IDB) *SiteRepo {
	return &SiteRepo{db: db}
}

func siteRoutingLockScope(siteID uuid.UUID) string {
	return "domain-routing:" + siteID.String()
}

// LockRoutingTransition serializes a customer lifecycle transition with all
// provider and route work for the same site. It must run inside the lifecycle
// transaction so PostgreSQL releases it automatically on commit or rollback.
func (r *SiteRepo) LockRoutingTransition(ctx context.Context, siteID uuid.UUID) error {
	scope := siteRoutingLockScope(siteID)
	_, err := r.db.NewRaw(
		`SELECT pg_advisory_xact_lock(hashtext(?))`,
		scope,
	).Exec(ctx)
	if err != nil {
		return fmt.Errorf("lock routing transition: %w", err)
	}
	return nil
}

// LockRoutingSession holds the same lock across provider work and the worker's
// final persistence transaction. The caller owns the dedicated connection.
func (r *SiteRepo) LockRoutingSession(ctx context.Context, siteID uuid.UUID) error {
	scope := siteRoutingLockScope(siteID)
	_, err := r.db.NewRaw(
		`SELECT pg_advisory_lock(hashtext(?))`,
		scope,
	).Exec(ctx)
	if err != nil {
		return fmt.Errorf("lock routing session: %w", err)
	}
	return nil
}

// UnlockRoutingSession releases a session lock obtained by LockRoutingSession.
func (r *SiteRepo) UnlockRoutingSession(ctx context.Context, siteID uuid.UUID) (bool, error) {
	scope := siteRoutingLockScope(siteID)
	var unlocked bool
	err := r.db.NewRaw(
		`SELECT pg_advisory_unlock(hashtext(?))`,
		scope,
	).Scan(ctx, &unlocked)
	if err != nil {
		return false, fmt.Errorf("unlock routing session: %w", err)
	}
	return unlocked, nil
}

// LockCreateRequest serializes idempotent retries for one account/key. The
// second transaction re-reads and returns the winner instead of surfacing a
// unique-constraint race.
func (r *SiteRepo) LockCreateRequest(ctx context.Context, accountID uuid.UUID, key string) error {
	if key == "" {
		return nil
	}
	lockScope := accountID.String() + ":" + key
	_, err := r.db.NewRaw(
		`SELECT pg_advisory_xact_lock(hashtext(?))`,
		lockScope,
	).Exec(ctx)
	if err != nil {
		return fmt.Errorf("lock create request: %w", err)
	}
	return nil
}

// Create inserts a site row.
func (r *SiteRepo) Create(ctx context.Context, site *model.Site) error {
	now := time.Now().UTC()
	if site.ID == uuid.Nil {
		site.ID = uuid.New()
	}
	site.CreatedAt = now
	site.UpdatedAt = now
	_, err := r.db.NewInsert().Model(site).Exec(ctx)
	return err
}

// ListByAccount returns only live sites owned by accountID.
func (r *SiteRepo) ListByAccount(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]model.Site, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := r.db.NewSelect().
		Model((*model.Site)(nil)).
		Where("account_id = ?", accountID).
		Where("deleted_at IS NULL")
	total, err := query.Count(ctx)
	if err != nil {
		return nil, 0, err
	}

	var sites []model.Site
	err = r.db.NewSelect().
		Model(&sites).
		Where("account_id = ?", accountID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return sites, total, err
}

// GetByAccount returns a live site only when it belongs to accountID.
func (r *SiteRepo) GetByAccount(ctx context.Context, accountID, siteID uuid.UUID) (*model.Site, error) {
	site := new(model.Site)
	err := r.db.NewSelect().
		Model(site).
		Where("account_id = ?", accountID).
		Where("id = ?", siteID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get site by account: %w", err)
	}
	return site, nil
}

// GetByAccountForUpdate locks a tenant-owned live site.
func (r *SiteRepo) GetByAccountForUpdate(ctx context.Context, accountID, siteID uuid.UUID) (*model.Site, error) {
	site := new(model.Site)
	err := r.db.NewSelect().
		Model(site).
		Where("account_id = ?", accountID).
		Where("id = ?", siteID).
		Where("deleted_at IS NULL").
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get site by account for update: %w", err)
	}
	return site, nil
}

// GetByAccountForUpdateIncludingDeleted locks a tenant-owned site even after
// deletion so repeated DELETE requests can return the original terminal state.
func (r *SiteRepo) GetByAccountForUpdateIncludingDeleted(
	ctx context.Context,
	accountID, siteID uuid.UUID,
) (*model.Site, error) {
	site := new(model.Site)
	err := r.db.NewSelect().
		Model(site).
		Where("account_id = ?", accountID).
		Where("id = ?", siteID).
		For("UPDATE").
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get site by account for update including deleted: %w", err)
	}
	return site, nil
}

// GetByIdempotencyKey returns a prior create result for a safe retry.
func (r *SiteRepo) GetByIdempotencyKey(ctx context.Context, accountID uuid.UUID, key string) (*model.Site, error) {
	site := new(model.Site)
	err := r.db.NewSelect().
		Model(site).
		Where("account_id = ?", accountID).
		Where("idempotency_key = ?", key).
		Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get site by idempotency key: %w", err)
	}
	return site, nil
}

// GetForWorker is the deliberate unscoped lookup used only after a durable job
// supplies a server-generated site ID. MUST only be called from:
//   - Job workers (internal/pkg/worker/)
//   - Reconciliation loops in internal/provisioner/
//
// DO NOT call from handlers or services - they must use GetByAccount variants.
//
// Security note: This bypasses tenant scoping by design for worker durability.
// Any accidental caller path from user-facing code will cause data leaks.
func (r *SiteRepo) GetForWorker(ctx context.Context, siteID uuid.UUID) (*model.Site, error) {
	site := new(model.Site)
	err := r.db.NewSelect().Model(site).Where("id = ?", siteID).Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get for worker: %w", err)
	}
	return site, nil
}

// GetForWorkerForUpdate locks a worker-owned site transition.
// Same usage constraints as GetForWorker.
func (r *SiteRepo) GetForWorkerForUpdate(ctx context.Context, siteID uuid.UUID) (*model.Site, error) {
	site := new(model.Site)
	err := r.db.NewSelect().Model(site).Where("id = ?", siteID).For("UPDATE").Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("get for worker for update: %w", err)
	}
	return site, nil
}

// ListReconciliationCandidates is the explicit worker-only scan for resources
// whose desired state should be compared with the data plane.
func (r *SiteRepo) ListReconciliationCandidates(
	ctx context.Context,
	limit int,
	steadyBefore time.Time,
) ([]model.Site, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	base := func(rows *[]model.Site) *bun.SelectQuery {
		return r.db.NewSelect().
			Model(rows).
			Where("deleted_at IS NULL").
			Where(`NOT EXISTS (
				SELECT 1 FROM jobs AS j
				WHERE j.status IN ('queued', 'running')
				  AND j.payload ->> 'site_id' = s.id::text
			)`).
			OrderExpr("COALESCE(last_reconciled_at, created_at) ASC").
			Order("id ASC")
	}
	if limit == 1 {
		var sites []model.Site
		err := base(&sites).
			Where(`(
				status = ? OR
				(status IN (?) AND (last_reconciled_at IS NULL OR last_reconciled_at <= ?))
			)`, model.SiteDeleting, bun.List([]string{
				model.SiteActive, model.SiteSuspended,
			}), steadyBefore).
			Limit(1).
			Scan(ctx)
		return sites, err
	}

	// Reserve half of every normal worker batch for steady-state drift. A large
	// set of repeatedly failing deletes therefore cannot starve active or
	// suspended sites forever.
	deleteLimit := limit / 2
	var deleting []model.Site
	if err := base(&deleting).
		Where("status = ?", model.SiteDeleting).
		Limit(deleteLimit).
		Scan(ctx); err != nil {
		return nil, err
	}
	var steady []model.Site
	if err := base(&steady).
		Where("status IN (?)", bun.List([]string{model.SiteActive, model.SiteSuspended})).
		Where("(last_reconciled_at IS NULL OR last_reconciled_at <= ?)", steadyBefore).
		Limit(limit - len(deleting)).
		Scan(ctx); err != nil {
		return nil, err
	}
	return append(deleting, steady...), nil
}

// MarkReconciled advances the fair steady-state scan cursor. The worker calls
// it in the same transaction as the reconciliation audit and job completion.
func (r *SiteRepo) MarkReconciled(ctx context.Context, siteID uuid.UUID, at time.Time) error {
	result, err := r.db.NewUpdate().Model((*model.Site)(nil)).
		Set("last_reconciled_at = ?", at).
		Where("id = ?", siteID).
		Exec(ctx)
	if err != nil {
		return err
	}
	return requireOneRow(result)
}

// SetStatus updates a site state. accountID keeps customer-triggered transitions
// tenant scoped.
func (r *SiteRepo) SetStatus(ctx context.Context, accountID, siteID uuid.UUID, status string) error {
	result, err := r.db.NewUpdate().
		Model((*model.Site)(nil)).
		Set("status = ?", status).
		Set("last_error = NULL").
		Set("updated_at = now()").
		Where("account_id = ?", accountID).
		Where("id = ?", siteID).
		Where("deleted_at IS NULL").
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetWorkerStatus updates state after a worker operation.
func (r *SiteRepo) SetWorkerStatus(ctx context.Context, siteID uuid.UUID, status string, lastError *string) error {
	result, err := r.db.NewUpdate().
		Model((*model.Site)(nil)).
		Set("status = ?", status).
		Set("last_error = ?", lastError).
		Set("updated_at = now()").
		Where("id = ?", siteID).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MarkDeleted makes a completed delete visible to polling while excluding it
// from normal customer lists.
func (r *SiteRepo) MarkDeleted(ctx context.Context, siteID uuid.UUID) error {
	result, err := r.db.NewUpdate().
		Model((*model.Site)(nil)).
		Set("status = ?", model.SiteDeleted).
		Set("last_error = NULL").
		Set("deleted_at = COALESCE(deleted_at, now())").
		Set("updated_at = now()").
		Where("id = ?", siteID).
		Exec(ctx)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// MarkCapacityReleased sets an exactly-once marker and returns the node whose
// counter must be decremented. A nil UUID means a prior retry already released
// capacity.
func (r *SiteRepo) MarkCapacityReleased(ctx context.Context, siteID uuid.UUID) (uuid.UUID, error) {
	var nodeID uuid.UUID
	err := r.db.NewRaw(`
		UPDATE sites
		SET capacity_released_at = now(),
		    updated_at = now()
		WHERE id = ?
		  AND capacity_released_at IS NULL
		RETURNING node_id`,
		siteID,
	).Scan(ctx, &nodeID)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil
	}
	return nodeID, err
}
