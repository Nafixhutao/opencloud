package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

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

// PutObject stores an object in the bucket and records its metadata.
func (s *StorageObjectService) PutObject(ctx context.Context, accountID, bucketID uuid.UUID, key string, body io.Reader, size int64, contentType string) (*model.StorageObject, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, err
	}

	bucket, err := s.bucketRepo.GetByAccount(ctx, accountID, bucketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("bucket not found")
		}
		return nil, apperr.Internal("failed to load bucket").Wrap(err)
	}
	if bucket.Status != model.BucketActive {
		return nil, apperr.Conflict("bucket is not active")
	}

	if size > bucket.MaxObjectSizeBytes {
		return nil, apperr.Validation("object exceeds maximum size",
			apperr.FieldIssue{Field: "size", Issue: fmt.Sprintf("max %d bytes", bucket.MaxObjectSizeBytes)})
	}
	if bucket.BytesUsed+size > bucket.StorageLimitBytes {
		return nil, apperr.Conflict("bucket storage quota exceeded").
			WithDetails(apperr.FieldIssue{Field: "size", Issue: fmt.Sprintf("limit %d bytes, used %d bytes", bucket.StorageLimitBytes, bucket.BytesUsed)})
	}
	if contentType == "" {
		contentType = detectContentType(key)
	}

	info, err := s.provider.PutObject(ctx, provisioner.PutObjectSpec{
		Bucket:      bucket.PhysicalName,
		Key:         key,
		Body:        body,
		Size:        size,
		ContentType: contentType,
	})
	if err != nil {
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
		return nil, apperr.Internal("failed to store object metadata").Wrap(err)
	}
	if _, err := s.bucketRepo.IncrementUsage(ctx, bucketID, info.Size); err != nil {
		s.log.Warn("failed to update bucket usage counter", zap.Error(err))
	}
	return obj, nil
}

// GetObject retrieves an object's content and metadata.
func (s *StorageObjectService) GetObject(ctx context.Context, accountID, bucketID uuid.UUID, key string) (io.ReadCloser, *ObjectDownloadInfo, error) {
	if err := validateObjectKey(key); err != nil {
		return nil, nil, err
	}

	bucket, err := s.bucketRepo.GetByAccount(ctx, accountID, bucketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, apperr.NotFound("bucket not found")
		}
		return nil, nil, apperr.Internal("failed to load bucket").Wrap(err)
	}

	body, info, err := s.provider.GetObject(ctx, provisioner.ObjectRef{
		BucketPhysicalName: bucket.PhysicalName,
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
	bucket, err := s.bucketRepo.GetByAccount(ctx, accountID, bucketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", apperr.NotFound("bucket not found")
		}
		return nil, "", apperr.Internal("failed to load bucket").Wrap(err)
	}

	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	objects, nextToken, err := s.provider.ListObjects(ctx, provisioner.ObjectRef{
		BucketPhysicalName: bucket.PhysicalName,
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

	bucket, err := s.bucketRepo.GetByAccount(ctx, accountID, bucketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return apperr.NotFound("bucket not found")
		}
		return apperr.Internal("failed to load bucket").Wrap(err)
	}

	// Fetch object size before deletion for quota tracking.
	objMeta, _ := s.objectRepo.GetByBucketAndKey(ctx, accountID, bucketID, key)
	objSize := int64(0)
	if objMeta != nil {
		objSize = objMeta.Size
	}

	if err := s.provider.DeleteObject(ctx, provisioner.ObjectRef{
		BucketPhysicalName: bucket.PhysicalName,
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

	bucket, err := s.bucketRepo.GetByAccount(ctx, accountID, bucketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperr.NotFound("bucket not found")
		}
		return nil, apperr.Internal("failed to load bucket").Wrap(err)
	}

	info, err := s.provider.HeadObject(ctx, provisioner.ObjectRef{
		BucketPhysicalName: bucket.PhysicalName,
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

	bucket, err := s.bucketRepo.GetByAccount(ctx, accountID, bucketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", apperr.NotFound("bucket not found")
		}
		return "", apperr.Internal("failed to load bucket").Wrap(err)
	}

	url, err := s.provider.PresignedGetURL(ctx, provisioner.ObjectRef{
		BucketPhysicalName: bucket.PhysicalName,
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

	bucket, err := s.bucketRepo.GetByAccount(ctx, accountID, bucketID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", apperr.NotFound("bucket not found")
		}
		return "", apperr.Internal("failed to load bucket").Wrap(err)
	}

	url, err := s.provider.PresignedPutURL(ctx, provisioner.ObjectRef{
		BucketPhysicalName: bucket.PhysicalName,
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
