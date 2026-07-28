package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// Node is registered hosting capacity. Provider metadata is operator-only and
// never returned by customer routes.
type Node struct {
	bun.BaseModel `bun:"table:nodes,alias:n"`

	ID               uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	Hostname         string          `bun:"hostname,notnull" json:"hostname"`
	Backend          string          `bun:"backend,notnull" json:"backend"`
	Status           string          `bun:"status,notnull" json:"status"`
	CapacitySites    int             `bun:"capacity_sites,notnull" json:"capacity_sites"`
	UsedSites        int             `bun:"used_sites,notnull" json:"used_sites"`
	ProviderMetadata json.RawMessage `bun:"provider_metadata,type:jsonb,notnull" json:"-"`
	CreatedAt        time.Time       `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt        time.Time       `bun:"updated_at,notnull,default:now()" json:"updated_at"`
}

// Node status values control whether placement may reserve new capacity.
const (
	NodeOnline   = "online"
	NodeDraining = "draining"
	NodeOffline  = "offline"
)

// Site is a tenant-owned website scheduled onto one hosting node.
type Site struct {
	bun.BaseModel `bun:"table:sites,alias:s"`

	ID                 uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID          uuid.UUID  `bun:"account_id,notnull,type:uuid" json:"account_id"`
	NodeID             uuid.UUID  `bun:"node_id,notnull,type:uuid" json:"node_id"`
	Domain             string     `bun:"domain,notnull" json:"domain"`
	Image              string     `bun:"image,notnull" json:"image"`
	InternalPort       int        `bun:"internal_port,notnull" json:"internal_port"`
	MemoryBytes        int64      `bun:"memory_bytes,notnull" json:"memory_bytes"`
	NanoCPUs           int64      `bun:"nano_cpus,notnull" json:"nano_cpus"`
	Status             string     `bun:"status,notnull" json:"status"`
	IdempotencyKey     *string    `bun:"idempotency_key" json:"-"`
	LastError          *string    `bun:"last_error" json:"last_error,omitempty"`
	CreatedAt          time.Time  `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt          time.Time  `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt          *time.Time `bun:"deleted_at" json:"deleted_at,omitempty"`
	CapacityReleasedAt *time.Time `bun:"capacity_released_at" json:"-"`
}

// Site lifecycle status values separate durable intent from worker completion.
const (
	SiteProvisioning = "provisioning"
	SiteActive       = "active"
	SiteSuspending   = "suspending"
	SiteSuspended    = "suspended"
	SiteResuming     = "resuming"
	SiteDeleting     = "deleting"
	SiteDeleted      = "deleted"
	SiteFailed       = "failed"
)

// Job is durable work claimed by workers through PostgreSQL SKIP LOCKED.
type Job struct {
	bun.BaseModel `bun:"table:jobs,alias:j"`

	ID          uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID   *uuid.UUID      `bun:"account_id,type:uuid" json:"account_id,omitempty"`
	Kind        string          `bun:"kind,notnull" json:"kind"`
	Status      string          `bun:"status,notnull" json:"status"`
	Attempts    int             `bun:"attempts,notnull" json:"attempts"`
	MaxAttempts int             `bun:"max_attempts,notnull" json:"max_attempts"`
	RunAt       time.Time       `bun:"run_at,notnull" json:"run_at"`
	LockedAt    *time.Time      `bun:"locked_at" json:"locked_at,omitempty"`
	LockedBy    *string         `bun:"locked_by" json:"locked_by,omitempty"`
	Payload     json.RawMessage `bun:"payload,type:jsonb,notnull" json:"payload"`
	LastError   *string         `bun:"last_error" json:"last_error,omitempty"`
	CreatedAt   time.Time       `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt   time.Time       `bun:"updated_at,notnull,default:now()" json:"updated_at"`
}

// Job kind and status values define the durable provisioning queue protocol.
const (
	JobProvisionSite     = "provision_site"
	JobDeleteSite        = "delete_site"
	JobSuspendSite       = "suspend_site"
	JobResumeSite        = "resume_site"
	JobCleanupSite       = "cleanup_site"
	JobReconcileSite     = "reconcile_site"
	JobProvisionDatabase = "provision_database"
	JobDeleteDatabase    = "delete_database"
	JobCleanupDatabase   = "cleanup_database"

	JobQueued    = "queued"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
)
