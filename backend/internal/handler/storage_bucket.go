package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/nazxf/opencloud/backend/internal/apperr"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/model"
	service "github.com/nazxf/opencloud/backend/internal/service"
)

// StorageBucketHandler serves tenant-scoped storage bucket endpoints.
type StorageBucketHandler struct {
	svc *service.StorageBucketService
}

// NewStorageBucketHandler constructs a handler for tenant-scoped storage resources.
func NewStorageBucketHandler(svc *service.StorageBucketService) *StorageBucketHandler {
	return &StorageBucketHandler{svc: svc}
}

type bucketResponse struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	PhysicalName       string   `json:"physical_name"`
	Visibility         string   `json:"visibility"`
	Status             string   `json:"status"`
	StorageLimitBytes  int64    `json:"storage_limit_bytes"`
	MaxObjectSizeBytes int64    `json:"max_object_size_bytes"`
	ObjectCount        int64    `json:"object_count"`
	AllowedMimeTypes   []string `json:"allowed_mime_types,omitempty"`
	CreatedAt          string   `json:"created_at"`
}

func newBucketResponse(bucket *model.StorageBucket) bucketResponse {
	var mimeTypes []string
	if len(bucket.AllowedMimeTypes) > 0 {
		if err := json.Unmarshal(bucket.AllowedMimeTypes, &mimeTypes); err != nil {
			mimeTypes = nil
		}
	}
	return bucketResponse{
		ID:                 bucket.ID.String(),
		Name:               bucket.Name,
		PhysicalName:       bucket.PhysicalName,
		Visibility:         bucket.Visibility,
		Status:             bucket.Status,
		StorageLimitBytes:  bucket.StorageLimitBytes,
		MaxObjectSizeBytes: bucket.MaxObjectSizeBytes,
		ObjectCount:        bucket.ObjectCount,
		AllowedMimeTypes:   mimeTypes,
		CreatedAt:          bucket.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}
}

// ListBuckets returns the current account's paginated buckets for a project.
func (h *StorageBucketHandler) ListBuckets(c *gin.Context) {
	projectID, ok := storageProjectIDParam(c)
	if !ok {
		return
	}
	page, perPage := queryPagination(c)
	buckets, total, err := h.svc.ListBuckets(c.Request.Context(), middleware.AccountID(c), projectID, page, perPage)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": bucketResponses(buckets), "meta": gin.H{"page": page, "per_page": perPage, "total": total}})
}

// CreateBucket creates a new storage bucket asynchronously. Returns 202 Accepted.
func (h *StorageBucketHandler) CreateBucket(c *gin.Context) {
	projectID, ok := storageProjectIDParam(c)
	if !ok {
		return
	}
	var req service.CreateBucketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	bucket, err := h.svc.CreateBucket(c.Request.Context(), middleware.UserID(c), middleware.AccountID(c), projectID, c.GetHeader("Idempotency-Key"), req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newBucketResponse(bucket)})
}

// GetBucket returns one account-owned bucket detail.
func (h *StorageBucketHandler) GetBucket(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	bucket, err := h.svc.GetBucket(c.Request.Context(), middleware.AccountID(c), bucketID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newBucketResponse(bucket)})
}

// DeleteBucket initiates async deletion of an empty bucket. Returns 202 Accepted.
// Fast-path rejects non-empty buckets (object_count > 0).
func (h *StorageBucketHandler) DeleteBucket(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	bucket, err := h.svc.DeleteBucket(c.Request.Context(), middleware.AccountID(c), bucketID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"data": newBucketResponse(bucket)})
}

// PatchBucket updates mutable bucket settings.
func (h *StorageBucketHandler) PatchBucket(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	var req service.PatchBucketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, apperr.Validation("invalid request body"))
		return
	}
	bucket, err := h.svc.PatchBucket(c.Request.Context(), middleware.AccountID(c), bucketID, req)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": newBucketResponse(bucket)})
}

func storageProjectIDParam(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("projectID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid project id"))
		return uuid.Nil, false
	}
	return id, true
}

func projectBucketParams(c *gin.Context) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := storageProjectIDParam(c)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	bucketID, err := uuid.Parse(c.Param("bucketID"))
	if err != nil {
		respondError(c, apperr.Validation("invalid bucket id"))
		return uuid.Nil, uuid.Nil, false
	}
	return projectID, bucketID, true
}

func bucketResponses(buckets []model.StorageBucket) []bucketResponse {
	out := make([]bucketResponse, len(buckets))
	for i := range buckets {
		out[i] = newBucketResponse(&buckets[i])
	}
	return out
}
