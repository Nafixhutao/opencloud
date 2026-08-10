package handler

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/model"
	service "github.com/nazxf/opencloud/backend/internal/service"
)

type StorageObjectHandler struct {
	svc *service.StorageObjectService
}

func NewStorageObjectHandler(svc *service.StorageObjectService) *StorageObjectHandler {
	return &StorageObjectHandler{svc: svc}
}

func (h *StorageObjectHandler) PutObject(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	key := c.Query("key")
	accountID := middleware.AccountID(c)

	contentType := c.GetHeader("Content-Type")
	if ct := c.GetHeader("X-Object-Content-Type"); ct != "" {
		contentType = ct
	}
	contentLength := c.Request.ContentLength

	obj, err := h.svc.PutObject(c.Request.Context(), accountID, bucketID, key, c.Request.Body, contentLength, contentType)
	if err != nil {
		respondError(c, err)
		return
	}

	c.JSON(http.StatusOK, toObjectResponse(obj))
}

func (h *StorageObjectHandler) GetObject(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	key := c.Query("key")
	accountID := middleware.AccountID(c)

	body, info, err := h.svc.GetObject(c.Request.Context(), accountID, bucketID, key)
	if err != nil {
		respondError(c, err)
		return
	}
	defer body.Close()

	if info.ContentType != "" {
		c.Header("Content-Type", info.ContentType)
	}
	c.Header("ETag", info.ETag)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileBaseName(key)))
	if info.Size > 0 {
		c.Header("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, body)
}

func (h *StorageObjectHandler) ListObjects(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	accountID := middleware.AccountID(c)
	prefix := c.Query("prefix")
	continuationToken := c.Query("continuation_token")
	limit := int32(100)
	if l, err := strconv.Atoi(c.Query("limit")); err == nil && l > 0 && l <= 1000 {
		limit = int32(l)
	}

	objects, nextToken, err := h.svc.ListObjects(c.Request.Context(), accountID, bucketID, prefix, continuationToken, limit)
	if err != nil {
		respondError(c, err)
		return
	}

	resp := gin.H{"data": objects}
	if nextToken != "" {
		resp["next_continuation_token"] = nextToken
	}
	c.JSON(http.StatusOK, resp)
}

func (h *StorageObjectHandler) DeleteObject(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	key := c.Query("key")
	accountID := middleware.AccountID(c)

	if err := h.svc.DeleteObject(c.Request.Context(), accountID, bucketID, key); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (h *StorageObjectHandler) HeadObject(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	key := c.Query("key")
	accountID := middleware.AccountID(c)

	info, err := h.svc.HeadObject(c.Request.Context(), accountID, bucketID, key)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": info})
}

func (h *StorageObjectHandler) PresignedGetURL(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	key := c.Query("key")
	accountID := middleware.AccountID(c)

	expiry := 15 * time.Minute
	if e, err := time.ParseDuration(c.Query("expiry")); err == nil && e > 0 && e <= 7*24*time.Hour {
		expiry = e
	}

	url, err := h.svc.PresignedGetURL(c.Request.Context(), accountID, bucketID, key, expiry)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"url": url, "expires_in_seconds": int64(expiry.Seconds())}})
}

func (h *StorageObjectHandler) PresignedPutURL(c *gin.Context) {
	_, bucketID, ok := projectBucketParams(c)
	if !ok {
		return
	}
	key := c.Query("key")
	accountID := middleware.AccountID(c)

	expiry := 15 * time.Minute
	if e, err := time.ParseDuration(c.Query("expiry")); err == nil && e > 0 && e <= 24*time.Hour {
		expiry = e
	}

	url, err := h.svc.PresignedPutURL(c.Request.Context(), accountID, bucketID, key, expiry)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"url": url, "expires_in_seconds": int64(expiry.Seconds())}})
}

type objectUploadResponse struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
	ETag        string `json:"etag"`
	CreatedAt   string `json:"created_at"`
}

func toObjectResponse(obj *model.StorageObject) gin.H {
	return gin.H{"data": objectUploadResponse{
		ID:          obj.ID.String(),
		Key:         obj.ObjectKey,
		Size:        obj.Size,
		ContentType: obj.ContentType,
		ETag:        obj.ETag,
		CreatedAt:   obj.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
	}}
}

func fileBaseName(key string) string {
	for i := len(key) - 1; i >= 0; i-- {
		if key[i] == '/' {
			return key[i+1:]
		}
	}
	return key
}
