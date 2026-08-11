package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ResourceUsage is a periodic snapshot of account resource consumption.
type ResourceUsage struct {
	bun.BaseModel `bun:"table:resource_usage,alias:ru"`

	ID        uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID uuid.UUID `bun:"account_id,notnull,type:uuid" json:"account_id"`

	ActiveSites     int   `bun:"active_sites,notnull" json:"active_sites"`
	ActiveDatabases int   `bun:"active_databases,notnull" json:"active_databases"`
	StorageBytes    int64 `bun:"storage_bytes,notnull" json:"storage_bytes"`
	StorageObjects  int64 `bun:"storage_objects,notnull" json:"storage_objects"`

	RecordedAt time.Time  `bun:"recorded_at,notnull,default:now()" json:"recorded_at"`
	CreatedAt  time.Time  `bun:"created_at,notnull,default:now()" json:"created_at"`
	DeletedAt  *time.Time `bun:"deleted_at" json:"deleted_at,omitempty"`
}
