package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type StorageObject struct {
	bun.BaseModel `bun:"table:storage_objects,alias:o"`

	ID          uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID   uuid.UUID  `bun:"account_id,notnull,type:uuid" json:"account_id"`
	ProjectID   uuid.UUID  `bun:"project_id,notnull,type:uuid" json:"project_id"`
	BucketID    uuid.UUID  `bun:"bucket_id,notnull,type:uuid" json:"bucket_id"`
	ObjectKey   string     `bun:"object_key,notnull" json:"object_key"`
	Size        int64      `bun:"size,notnull" json:"size"`
	ContentType string     `bun:"content_type,notnull" json:"content_type"`
	ETag        string     `bun:"etag,notnull" json:"etag"`
	CreatedAt   time.Time  `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt   time.Time  `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt   *time.Time `bun:"deleted_at" json:"deleted_at,omitempty"`
}
