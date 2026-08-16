// Package repository is the only layer that touches PostgreSQL (BACKEND.md §7).
package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// StorageBucketRepo manages tenant-owned storage buckets.
type StorageBucketRepo struct {
	db bun.IDB
}

// NewStorageBucketRepo constructs a StorageBucketRepo.
func NewStorageBucketRepo(db bun.IDB) *StorageBucketRepo {
	return &StorageBucketRepo{db: db}
}

// WithDB returns a copy using db, normally a transaction owned by a service.
func (r *StorageBucketRepo) WithDB(db bun.IDB) *StorageBucketRepo {
	return &StorageBucketRepo{db: db}
}

// Create inserts a new storage bucket.
func (r *StorageBucketRepo) Create(ctx context.Context, bucket *model.StorageBucket) error {
	now := time.Now().UTC()
	if bucket.ID == uuid.Nil {
		bucket.ID = uuid.New()
	}
	bucket.CreatedAt = now
	bucket.UpdatedAt = now
	if bucket.Status == "" {
		bucket.Status = model.BucketCreating
	}
	if bucket.Visibility == "" {
		bucket.Visibility = model.BucketVisibilityPrivate
	}
	if bucket.StorageLimitBytes <= 0 {
		bucket.StorageLimitBytes = 1073741824 // 1GB default
	}
	if bucket.MaxObjectSizeBytes <= 0 {
		bucket.MaxObjectSizeBytes = 104857600 // 100MB default
	}
	_, err := r.db.NewInsert().Model(bucket).Exec(ctx)
	return err
}

// GetByID returns a bucket or sql.ErrNoRows.
func (r *StorageBucketRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.StorageBucket, error) {
	bucket := new(model.StorageBucket)
	err := r.db.NewSelect().Model(bucket).Where("id = ?", id).Scan(ctx)
	if err != nil {
		return nil, err
	}
	return bucket, nil
}

// GetByAccount returns a live bucket only when the tenant owns it.
func (r *StorageBucketRepo) GetByAccount(ctx context.Context, accountID, bucketID uuid.UUID) (*model.StorageBucket, error) {
	bucket := new(model.StorageBucket)
	err := r.db.NewSelect().Model(bucket).
		Where("account_id = ?", accountID).
		Where("id = ?", bucketID).
		Where("deleted_at IS NULL").
		Scan(ctx)
	return bucket, err
}

// GetByAccountForUpdate locks a live bucket while mutations occur.
func (r *StorageBucketRepo) GetByAccountForUpdate(ctx context.Context, accountID, bucketID uuid.UUID) (*model.StorageBucket, error) {
	bucket := new(model.StorageBucket)
	err := r.db.NewSelect().Model(bucket).
		Where("account_id = ?", accountID).
		Where("id = ?", bucketID).
		Where("deleted_at IS NULL").
		For("UPDATE").
		Scan(ctx)
	return bucket, err
}

// GetByIDempotencyKey returns a prior creation result for safe retry.
// Project-scoped idempotency: requires both account_id and project_id.
func (r *StorageBucketRepo) GetByIDempotencyKey(ctx context.Context, accountID, projectID uuid.UUID, idempotencyKey string) (*model.StorageBucket, error) {
	bucket := new(model.StorageBucket)
	err := r.db.NewSelect().Model(bucket).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("idempotency_key = ?", idempotencyKey).
		Scan(ctx)
	return bucket, err
}

// ListByAccountProject returns live buckets for an account/project with pagination.
func (r *StorageBucketRepo) ListByAccountProject(ctx context.Context, accountID, projectID uuid.UUID, limit, offset int) ([]model.StorageBucket, int, error) {
	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	var total int
	result, err := r.db.NewSelect().
		Model((*model.StorageBucket)(nil)).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("deleted_at IS NULL").
		Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	total = result

	var buckets []model.StorageBucket
	err = r.db.NewSelect().Model(&buckets).
		Where("account_id = ?", accountID).
		Where("project_id = ?", projectID).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(ctx)
	return buckets, total, err
}

// UpdateStatus updates the status and optional last_error of a bucket.
func (r *StorageBucketRepo) UpdateStatus(ctx context.Context, bucketID uuid.UUID, status string, lastError *string) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("status = ?", status).
		Set("last_error = ?", lastError).
		Set("updated_at = now()").
		Where("id = ?", bucketID).
		Exec(ctx)
	return result, err
}

// UpdateStatusWithLastErr updates status and sets a last error message.
func (r *StorageBucketRepo) UpdateStatusWithLastErr(ctx context.Context, bucketID uuid.UUID, status string, lastError *string) (sql.Result, error) {
	return r.UpdateStatus(ctx, bucketID, status, lastError)
}

// MarkDeleted marks a bucket as deleted with current timestamp.
func (r *StorageBucketRepo) MarkDeleted(ctx context.Context, bucketID uuid.UUID) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("status = ?", model.BucketDeleted).
		Set("deleted_at = now()").
		Set("updated_at = now()").
		Where("id = ?", bucketID).
		Where("status = ?", model.BucketDeleting).
		Exec(ctx)
	return result, err
}

// UpdateReconciledAt updates the reconciliation timestamp.
func (r *StorageBucketRepo) UpdateReconciledAt(ctx context.Context, bucketID uuid.UUID) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("last_reconciled_at = now()").
		Set("updated_at = now()").
		Where("id = ?", bucketID).
		Exec(ctx)
	return result, err
}

// FindStaleAndFailingBuckets returns buckets that need reconciliation.
func (r *StorageBucketRepo) FindStaleAndFailingBuckets(ctx context.Context, accountID uuid.UUID) ([]model.StorageBucket, error) {
	var buckets []model.StorageBucket
	err := r.db.NewSelect().Model(&buckets).
		Where("account_id = ?", accountID).
		Where("(status = ? AND deleted_at IS NULL) OR (status = ?)", model.BucketCreating, model.BucketFailed, model.BucketDeleting).
		Order("created_at ASC").
		Limit(100).
		Scan(ctx)
	return buckets, err
}

// UpdateStatusCompleted transitions a bucket to active on successful creation.
// Only updates if the bucket is still in 'creating' state.
func (r *StorageBucketRepo) UpdateStatusCompleted(ctx context.Context, bucketID uuid.UUID, status string, completedAt time.Time, lastError *string) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("status = ?", status).
		Set("last_error = ?", lastError).
		Set("last_reconciled_at = ?", completedAt).
		Set("updated_at = now()").
		Where("id = ?", bucketID).
		Where("status = ?", model.BucketCreating).
		Exec(ctx)
	return result, err
}

// UpdateStatusActiveWithMessage restores a bucket to active with an error message.
func (r *StorageBucketRepo) UpdateStatusActiveWithMessage(ctx context.Context, bucketID uuid.UUID, message string, completedAt time.Time) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("status = ?", model.BucketActive).
		Set("last_error = ?", &message).
		Set("last_reconciled_at = ?", completedAt).
		Set("updated_at = now()").
		Where("id = ?", bucketID).
		Exec(ctx)
	return result, err
}

// FailWithAttempt increments retry count and marks failed.
func (r *StorageBucketRepo) FailWithAttempt(ctx context.Context, bucketID uuid.UUID, message string, updatedAt time.Time) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("status = ?", model.BucketFailed).
		Set("last_error = ?", &message).
		Set("updated_at = ?", updatedAt).
		Where("id = ?", bucketID).
		Exec(ctx)
	return result, err
}

// RestoreDeletingToActiveWithError transitions deleting->active when provider reports BUCKET_NOT_EMPTY.
func (r *StorageBucketRepo) RestoreDeletingToActiveWithError(ctx context.Context, bucketID uuid.UUID, reason string, updatedAt time.Time) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("status = ?", model.BucketActive).
		Set("last_error = ?", &reason).
		Set("updated_at = ?", updatedAt).
		Where("id = ?", bucketID).
		Where("status = ?", model.BucketDeleting).
		Exec(ctx)
	return result, err
}

// SetStatus sets a bucket's status without other metadata changes.
func (r *StorageBucketRepo) SetStatus(ctx context.Context, bucketID uuid.UUID, status string, updatedAt time.Time) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("status = ?", status).
		Set("updated_at = ?", updatedAt).
		Where("id = ?", bucketID).
		Exec(ctx)
	return result, err
}

// UpdateStatusNoError updates status and clears last_error.
func (r *StorageBucketRepo) UpdateStatusNoError(ctx context.Context, bucketID uuid.UUID, status string, updatedAt time.Time) (sql.Result, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("status = ?", status).
		Set("last_error = NULL").
		Set("updated_at = ?", updatedAt).
		Where("id = ?", bucketID).
		Exec(ctx)
	return result, err
}

// IsNameInUse reports whether a name is already in use for this account/project.
func (r *StorageBucketRepo) IsNameInUse(ctx context.Context, accountID, projectID uuid.UUID, name string) (bool, error) {
	var count int
	err := r.db.NewRaw(`SELECT count(*) FROM storage_buckets WHERE account_id = ? AND project_id = ? AND lower(name) = ? AND deleted_at IS NULL`,
		accountID, projectID, name).Scan(ctx, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// CheckNonEmpty checks if the bucket has objects (fast-path only, non-authoritative).
func (r *StorageBucketRepo) CheckNonEmpty(ctx context.Context, bucketID uuid.UUID) (bool, error) {
	var objectCount int64
	err := r.db.NewSelect().Model((*model.StorageBucket)(nil)).
		Column("object_count").
		Where("id = ?", bucketID).
		Scan(ctx, &objectCount)
	if err != nil {
		return false, err
	}
	return objectCount > 0, nil
}

// DecrementUsage removes one object's bytes and count, clamped at zero.
func (r *StorageBucketRepo) DecrementUsage(ctx context.Context, bucketID uuid.UUID, size int64) (sql.Result, error) {
	return r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("bytes_used = GREATEST(0, bytes_used - ?)", size).
		Set("object_count = GREATEST(0, object_count - 1)").
		Set("updated_at = now()").
		Where("id = ?", bucketID).
		Exec(ctx)
}

// ReserveUsage atomically adds bytes and objects to a bucket's usage counters
// only when the byte delta still fits the storage limit. The conditional
// UPDATE makes the check-and-increment one statement, so concurrent uploads
// cannot race past the quota. A negative byte delta (an object overwrite that
// shrank) always satisfies the condition.
func (r *StorageBucketRepo) ReserveUsage(ctx context.Context, bucketID uuid.UUID, byteDelta int64, objectDelta int) (bool, error) {
	result, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("bytes_used = bytes_used + ?", byteDelta).
		Set("object_count = object_count + ?", objectDelta).
		Set("updated_at = now()").
		Where("id = ?", bucketID).
		Where("bytes_used + ? <= storage_limit_bytes", byteDelta).
		Exec(ctx)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// DecrementUsageBy removes an explicit byte/object delta, clamped at zero.
// It is the compensation path when a reserved upload fails.
func (r *StorageBucketRepo) DecrementUsageBy(ctx context.Context, bucketID uuid.UUID, byteDelta int64, objectDelta int) error {
	_, err := r.db.NewUpdate().
		Model((*model.StorageBucket)(nil)).
		Set("bytes_used = GREATEST(0, bytes_used - ?)", byteDelta).
		Set("object_count = GREATEST(0, object_count - ?)", objectDelta).
		Set("updated_at = now()").
		Where("id = ?", bucketID).
		Exec(ctx)
	return err
}
