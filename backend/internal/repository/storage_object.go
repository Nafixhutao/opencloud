package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/model"
)

// StorageObjectRepo manages storage object records in the database.
type StorageObjectRepo struct {
	db bun.IDB
}

// NewStorageObjectRepo creates a new StorageObjectRepo.
func NewStorageObjectRepo(db bun.IDB) *StorageObjectRepo {
	return &StorageObjectRepo{db: db}
}

// WithDB returns a copy of StorageObjectRepo using the given database handle.
func (r *StorageObjectRepo) WithDB(db bun.IDB) *StorageObjectRepo {
	return &StorageObjectRepo{db: db}
}

// Upsert inserts or updates a storage object record.
func (r *StorageObjectRepo) Upsert(ctx context.Context, obj *model.StorageObject) error {
	now := time.Now().UTC()
	if obj.ID == uuid.Nil {
		obj.ID = uuid.New()
	}
	obj.CreatedAt = now
	obj.UpdatedAt = now

	_, err := r.db.NewInsert().Model(obj).
		On("CONFLICT (bucket_id, object_key) WHERE deleted_at IS NULL DO UPDATE").
		Set("size = EXCLUDED.size").
		Set("content_type = EXCLUDED.content_type").
		Set("etag = EXCLUDED.etag").
		Set("updated_at = now()").
		Exec(ctx)
	return err
}

// GetByBucketAndKey retrieves a storage object by bucket and key.
func (r *StorageObjectRepo) GetByBucketAndKey(ctx context.Context, accountID, bucketID uuid.UUID, key string) (*model.StorageObject, error) {
	obj := new(model.StorageObject)
	err := r.db.NewSelect().Model(obj).
		Where("account_id = ?", accountID).
		Where("bucket_id = ?", bucketID).
		Where("object_key = ?", key).
		Where("deleted_at IS NULL").
		Scan(ctx)
	return obj, err
}

// ListByBucket lists storage objects in a bucket, optionally filtered by prefix.
func (r *StorageObjectRepo) ListByBucket(ctx context.Context, accountID, bucketID uuid.UUID, prefix string, limit int) ([]model.StorageObject, error) {
	var objects []model.StorageObject
	q := r.db.NewSelect().Model(&objects).
		Where("account_id = ?", accountID).
		Where("bucket_id = ?", bucketID).
		Where("deleted_at IS NULL").
		Order("object_key ASC").
		Limit(limit + 1)

	if prefix != "" {
		q.Where("object_key LIKE ?", prefix+"%")
	}

	err := q.Scan(ctx)
	return objects, err
}

// SoftDelete marks a storage object as deleted.
func (r *StorageObjectRepo) SoftDelete(ctx context.Context, accountID, bucketID uuid.UUID, key string) (sql.Result, error) {
	return r.db.NewUpdate().Model((*model.StorageObject)(nil)).
		Set("deleted_at = now()").
		Set("updated_at = now()").
		Where("account_id = ?", accountID).
		Where("bucket_id = ?", bucketID).
		Where("object_key = ?", key).
		Where("deleted_at IS NULL").
		Exec(ctx)
}
