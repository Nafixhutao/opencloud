package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/provisioner"
	"github.com/nazxf/opencloud/backend/internal/repository"
)

const maxObjectKeyLength = 1024

// StorageObjectService manages object storage operations.
type StorageObjectService struct {
	log        *zap.Logger
	bucketRepo *repository.StorageBucketRepo
	objectRepo *repository.StorageObjectRepo
	provider   provisioner.ObjectStorageProvider
}

// NewStorageObjectService creates a new StorageObjectService.
func NewStorageObjectService(
	log *zap.Logger,
	bucketRepo *repository.StorageBucketRepo,
	objectRepo *repository.StorageObjectRepo,
	provider provisioner.ObjectStorageProvider,
) *StorageObjectService {
	return &StorageObjectService{log: log, bucketRepo: bucketRepo, objectRepo: objectRepo, provider: provider}
}

// bucketHandle is the deep module for tenant-owned bucket resolution. It owns
// the account→physical-name translation and the sql.ErrNoRows→NotFound mapping
// once, so the seven public methods stop re-deriving it as boilerplate.
type bucketHandle struct {
	bucket *model.StorageBucket
}

// resolveBucket loads a tenant-owned bucket and builds the provider ObjectRef.
// The tenancy check, not-found translation, and physical-name mapping live here
// — one tested unit instead of seven copies.
func (s *StorageObjectService) resolveBucket(ctx context.Context, accountID, bucketID uuid.UUID) (*bucketHandle, error) {
	bucket, err := s.bucketRepo.GetByAccount(ctx, accountID, bucketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("bucket not found")
		}
		return nil, apperr.Internal("failed to load bucket").Wrap(err)
	}
	return &bucketHandle{
		bucket: bucket,
	}, nil
}

// PutObject stores an object in the bucket and records its metadata.
func (s *StorageObjectService) PutObject(ctx context.Context, accountID, bucketID uuid.UUID, key string, body io.Reader, size int64, contentType string) (*model.StorageObject, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, err
	}

	h, err := s.resolveBucket(ctx, accountID, bucketID)
	if err != nil {
		return nil, err
	}
	bucket := h.bucket
	if bucket.Status != model.BucketActive {
		return nil, apperr.Conflict("bucket is not active")
	}

	if size > bucket.MaxObjectSizeBytes {
		return nil, apperr.Validation("object exceeds maximum size",
			apperr.FieldIssue{Field: "size", Issue: fmt.Sprintf("max %d bytes", bucket.MaxObjectSizeBytes)})
	}
	if contentType == "" {
		contentType = detectContentType(key)
	}

	// An existing object under the same key is replaced, so only the size
	// delta (and no extra object count) is charged against the quota.
	existing, existingErr := s.objectRepo.GetByBucketAndKey(ctx, accountID, bucketID, key)
	if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
		return nil, apperr.Internal("failed to load object metadata").Wrap(existingErr)
	}
	isOverwrite := existingErr == nil
	byteDelta, objectDelta := size, 1
	if isOverwrite {
		byteDelta, objectDelta = size-existing.Size, 0
	}

	// Reserve quota first with one conditional UPDATE: concurrent uploads
	// cannot both pass a read-then-check. Compensation releases the
	// reservation when the provider or metadata write fails.
	reserved := byteDelta > 0 || objectDelta > 0
	if reserved {
		ok, reserveErr := s.bucketRepo.ReserveUsage(ctx, bucketID, byteDelta, objectDelta)
		if reserveErr != nil {
			return nil, apperr.Internal("failed to reserve bucket quota").Wrap(reserveErr)
		}
		if !ok {
			return nil, apperr.Conflict("bucket storage quota exceeded").
				WithDetails(apperr.FieldIssue{Field: "size", Issue: fmt.Sprintf("limit %d bytes, used %d bytes", bucket.StorageLimitBytes, bucket.BytesUsed)})
		}
	}
	release := func() {
		if relErr := s.bucketRepo.DecrementUsageBy(ctx, bucketID, byteDelta, objectDelta); relErr != nil {
			s.log.Warn("failed to release reserved bucket quota", zap.Error(relErr))
		}
	}

	info, err := s.provider.PutObject(ctx, provisioner.PutObjectSpec{
		Bucket:      bucket.PhysicalName,
		Key:         key,
		Body:        body,
		Size:        size,
		ContentType: contentType,
	})
	if err != nil {
		if reserved {
			release()
		}
		return nil, wrapProviderErr(err)
	}

	obj := &model.StorageObject{
		AccountID:   accountID,
		ProjectID:   bucket.ProjectID,
		BucketID:    bucketID,
		ObjectKey:   key,
		Size:        info.Size,
		ContentType: info.ContentType,
		ETag:        info.ETag,
	}
	if err := s.objectRepo.Upsert(ctx, obj); err != nil {
		if reserved {
			release()
		}
		return nil, apperr.Internal("failed to store object metadata").Wrap(err)
	}
	return obj, nil
}

// GetObject retrieves an object's content and metadata.
func (s *StorageObjectService) GetObject(ctx context.Context, accountID, bucketID uuid.UUID, key string) (io.ReadCloser, *ObjectDownloadInfo, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, nil, err
	}

	h, err := s.resolveBucket(ctx, accountID, bucketID)
	if err != nil {
		return nil, nil, err
	}

	body, info, err := s.provider.GetObject(ctx, provisioner.ObjectRef{
		BucketPhysicalName: h.bucket.PhysicalName,
		Key:                key,
	})
	if err != nil {
		return nil, nil, wrapProviderErr(err)
	}

	return body, &ObjectDownloadInfo{
		Key:         info.Key,
		Size:        info.Size,
		ContentType: info.ContentType,
		ETag:        info.ETag,
	}, nil
}

// ListObjects lists objects in a bucket with optional prefix and pagination.
func (s *StorageObjectService) ListObjects(ctx context.Context, accountID, bucketID uuid.UUID, prefix, continuationToken string, limit int32) ([]ObjectItem, string, error) {
	h, err := s.resolveBucket(ctx, accountID, bucketID)
	if err != nil {
		return nil, "", err
	}

	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	objects, nextToken, err := s.provider.ListObjects(ctx, provisioner.ObjectRef{
		BucketPhysicalName: h.bucket.PhysicalName,
	}, provisioner.ListObjectsOptions{
		Prefix:            prefix,
		MaxKeys:           limit,
		ContinuationToken: continuationToken,
	})
	if err != nil {
		return nil, "", wrapProviderErr(err)
	}

	result := make([]ObjectItem, len(objects))
	for i, o := range objects {
		result[i] = ObjectItem{
			Key:          o.Key,
			Size:         o.Size,
			ContentType:  o.ContentType,
			ETag:         o.ETag,
			LastModified: o.LastModified.UTC().Format(time.RFC3339),
		}
	}
	return result, nextToken, nil
}

// DeleteObject removes an object from the bucket and soft-deletes its metadata.
func (s *StorageObjectService) DeleteObject(ctx context.Context, accountID, bucketID uuid.UUID, key string) error {
	if err := validateObjectKey(key); err != nil {
		return err
	}

	h, err := s.resolveBucket(ctx, accountID, bucketID)
	if err != nil {
		return err
	}

	// Fetch object size before deletion for quota tracking. A missing
	// metadata row (e.g. an object uploaded through a presigned URL) simply
	// means there is no usage to release; other errors fail the delete.
	objMeta, metaErr := s.objectRepo.GetByBucketAndKey(ctx, accountID, bucketID, key)
	if metaErr != nil && !errors.Is(metaErr, sql.ErrNoRows) {
		return apperr.Internal("failed to load object metadata").Wrap(metaErr)
	}
	objSize := int64(0)
	if metaErr == nil && objMeta != nil {
		objSize = objMeta.Size
	}

	if err := s.provider.DeleteObject(ctx, provisioner.ObjectRef{
		BucketPhysicalName: h.bucket.PhysicalName,
		Key:                key,
	}); err != nil {
		return wrapProviderErr(err)
	}

	result, err := s.objectRepo.SoftDelete(ctx, accountID, bucketID, key)
	if err != nil {
		return apperr.Internal("failed to delete object metadata").Wrap(err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		s.log.Warn("object deleted from provider but no metadata row found",
			zap.String("key", key), zap.Stringer("bucket_id", bucketID))
	} else {
		if _, decErr := s.bucketRepo.DecrementUsage(ctx, bucketID, objSize); decErr != nil {
			s.log.Warn("failed to decrement bucket usage counter", zap.Error(decErr))
		}
	}
	return nil
}

// HeadObject returns metadata for an object without its body.
func (s *StorageObjectService) HeadObject(ctx context.Context, accountID, bucketID uuid.UUID, key string) (*ObjectStatInfo, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, err
	}

	h, err := s.resolveBucket(ctx, accountID, bucketID)
	if err != nil {
		return nil, err
	}

	info, err := s.provider.HeadObject(ctx, provisioner.ObjectRef{
		BucketPhysicalName: h.bucket.PhysicalName,
		Key:                key,
	})
	if err != nil {
		return nil, wrapProviderErr(err)
	}

	return &ObjectStatInfo{
		Key:          info.Key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		ETag:         info.ETag,
		LastModified: info.LastModified.UTC().Format(time.RFC3339),
	}, nil
}

// PresignedGetURL generates a presigned URL for downloading an object.
func (s *StorageObjectService) PresignedGetURL(ctx context.Context, accountID, bucketID uuid.UUID, key string, expiry time.Duration) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}

	h, err := s.resolveBucket(ctx, accountID, bucketID)
	if err != nil {
		return "", err
	}

	url, err := s.provider.PresignedGetURL(ctx, provisioner.ObjectRef{
		BucketPhysicalName: h.bucket.PhysicalName,
		Key:                key,
	}, expiry)
	if err != nil {
		return "", wrapProviderErr(err)
	}
	return url, nil
}

// PresignedPutURL generates a presigned URL for uploading an object.
func (s *StorageObjectService) PresignedPutURL(ctx context.Context, accountID, bucketID uuid.UUID, key string, expiry time.Duration) (string, error) {
	if err := validateObjectKey(key); err != nil {
		return "", err
	}

	h, err := s.resolveBucket(ctx, accountID, bucketID)
	if err != nil {
		return "", err
	}

	url, err := s.provider.PresignedPutURL(ctx, provisioner.ObjectRef{
		BucketPhysicalName: h.bucket.PhysicalName,
		Key:                key,
	}, expiry)
	if err != nil {
		return "", wrapProviderErr(err)
	}
	return url, nil
}

// ObjectDownloadInfo holds metadata for a downloaded object.
type ObjectDownloadInfo struct {
	Key         string
	Size        int64
	ContentType string
	ETag        string
}

// ObjectItem represents an object in a list response.
type ObjectItem struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type,omitempty"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified"`
}

// ObjectStatInfo holds object metadata for stat/head responses.
type ObjectStatInfo struct {
	Key          string `json:"key"`
	Size         int64  `json:"size"`
	ContentType  string `json:"content_type,omitempty"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"last_modified"`
}

func validateObjectKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return apperr.Validation("object key is required")
	}
	if len(key) > maxObjectKeyLength {
		return apperr.Validation("object key exceeds maximum length", apperr.FieldIssue{Field: "key", Issue: fmt.Sprintf("max %d characters", maxObjectKeyLength)})
	}
	if strings.Contains(key, "..") || strings.HasPrefix(key, "/") || strings.HasPrefix(key, "\\") {
		return apperr.Validation("invalid object key", apperr.FieldIssue{Field: "key", Issue: "key must not contain path traversal"})
	}
	for _, r := range key {
		if r < 32 {
			return apperr.Validation("invalid object key", apperr.FieldIssue{Field: "key", Issue: "key contains control characters"})
		}
	}
	return nil
}

func detectContentType(key string) string {
	ext := strings.ToLower(filepath.Ext(key))
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

func wrapProviderErr(err error) error {
	switch {
	case errors.Is(err, provisioner.ErrBucketNotFound):
		return apperr.NotFound("bucket not found")
	case errors.Is(err, provisioner.ErrObjectNotFound):
		return apperr.NotFound("object not found")
	case errors.Is(err, provisioner.ErrInvalidObjectKey):
		return apperr.Validation("invalid object key")
	case errors.Is(err, provisioner.ErrObjectTooLarge):
		return apperr.Validation("object too large")
	default:
		return apperr.Internal("storage provider error").Wrap(err)
	}
}
