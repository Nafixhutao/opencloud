// Package model holds Bun domain structs for the control plane.
package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// StorageBucket is a tenant-owned object storage bucket (Phase 4M).
// Buckets are created asynchronously via jobs; physical_name is generated server-side.
type StorageBucket struct {
	bun.BaseModel `bun:"table:storage_buckets,alias:b"`

	ID                 uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID          uuid.UUID       `bun:"account_id,notnull,type:uuid" json:"account_id"`
	ProjectID          uuid.UUID       `bun:"project_id,notnull,type:uuid" json:"project_id"`
	Name               string          `bun:"name,notnull" json:"name"`
	PhysicalName       string          `bun:"physical_name,notnull" json:"physical_name"`
	Visibility         string          `bun:"visibility,notnull" json:"visibility"`
	Status             string          `bun:"status,notnull" json:"status"`
	StorageLimitBytes  int64           `bun:"storage_limit_bytes,notnull" json:"storage_limit_bytes"`
	BytesUsed          int64           `bun:"bytes_used,notnull" json:"bytes_used"`
	ObjectCount        int64           `bun:"object_count,notnull" json:"object_count"`
	MaxObjectSizeBytes int64           `bun:"max_object_size_bytes,notnull" json:"max_object_size_bytes"`
	AllowedMimeTypes   json.RawMessage `bun:"allowed_mime_types,type:jsonb,notnull,default:'[]'::jsonb" json:"allowed_mime_types"`
	LastError          *string         `bun:"last_error" json:"last_error,omitempty"`
	LastReconciledAt   *time.Time      `bun:"last_reconciled_at" json:"last_reconciled_at,omitempty"`
	IdempotencyKey     *string         `bun:"idempotency_key" json:"-"`
	CreatedAt          time.Time       `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt          time.Time       `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt          *time.Time      `bun:"deleted_at" json:"deleted_at,omitempty"`
}

// Storage bucket status values separate durable intent from worker completion.
const (
	BucketCreating = "creating"
	BucketActive   = "active"
	BucketDeleting = "deleting"
	BucketDeleted  = "deleted"
	BucketFailed   = "failed"
)

// Storage bucket visibility values.
const (
	BucketVisibilityPublic  = "public"
	BucketVisibilityPrivate = "private"
)

// StorageBucketPayload defines job payloads for storage operations.
type StorageBucketPayload interface {
	Kind() string
}

// ProvisionStorageBucketPayload is the unmarshaled job payload.
type ProvisionStorageBucketPayload struct {
	BucketID uuid.UUID `json:"bucket_id"`
}

// Kind returns the job type for ProvisionStorageBucketPayload.
func (p ProvisionStorageBucketPayload) Kind() string {
	return JobProvisionStorageBucket
}

// DeleteStorageBucketPayload is the unmarshaled job payload.
type DeleteStorageBucketPayload struct {
	BucketID uuid.UUID `json:"bucket_id"`
}

// Kind returns the job type for DeleteStorageBucketPayload.
func (p DeleteStorageBucketPayload) Kind() string {
	return JobDeleteStorageBucket
}

// ReconcileStorageBucketPayload is the unmarshaled job payload.
type ReconcileStorageBucketPayload struct {
	BucketID uuid.UUID `json:"bucket_id"`
}

// Kind returns the job type for ReconcileStorageBucketPayload.
func (p ReconcileStorageBucketPayload) Kind() string {
	return JobReconcileStorageBucket
}
