package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	ProjectActive   = "active"
	ProjectArchived = "archived"
	ProjectDeleted  = "deleted"

	ServiceActive   = "active"
	ServiceDisabled = "disabled"
	ServiceDeleted  = "deleted"

	ServiceTypeWeb    = "web"
	ServiceTypeWorker = "worker"
	ServiceTypeCron   = "cron"
	ServiceTypeStatic = "static"

	DeploymentQueued    = "queued"
	DeploymentCloning   = "cloning"
	DeploymentDetecting = "detecting"
	DeploymentPlanning  = "planning"
	DeploymentBuilding  = "building"
	DeploymentPushing   = "pushing"
	DeploymentScanning  = "scanning"
	DeploymentDeploying = "deploying"
	DeploymentReady     = "ready"
	DeploymentFailed    = "failed"
	DeploymentCancelled = "cancelled"
)

// Project is the tenant-owned home for related application services and their
// future deployment resources. Existing sites stay outside this model until an
// explicit compatibility migration/import is introduced.
type Project struct {
	bun.BaseModel `bun:"table:projects,alias:p"`

	ID             uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID      uuid.UUID  `bun:"account_id,notnull,type:uuid" json:"account_id"`
	Name           string     `bun:"name,notnull" json:"name"`
	Status         string     `bun:"status,notnull" json:"status"`
	IdempotencyKey *string    `bun:"idempotency_key" json:"-"`
	CreatedAt      time.Time  `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt      time.Time  `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt      *time.Time `bun:"deleted_at" json:"deleted_at,omitempty"`
}

// Service is one independently deployable workload in a project. Runtime and
// source detection intentionally remain a future build-provider concern.
type Service struct {
	bun.BaseModel `bun:"table:services,alias:srv"`

	ID             uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID      uuid.UUID  `bun:"account_id,notnull,type:uuid" json:"account_id"`
	ProjectID      uuid.UUID  `bun:"project_id,notnull,type:uuid" json:"project_id"`
	Name           string     `bun:"name,notnull" json:"name"`
	ServiceType    string     `bun:"service_type,notnull" json:"service_type"`
	SourceRoot     string     `bun:"source_root,notnull" json:"source_root"`
	Status         string     `bun:"status,notnull" json:"status"`
	IdempotencyKey *string    `bun:"idempotency_key" json:"-"`
	CreatedAt      time.Time  `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt      time.Time  `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt      *time.Time `bun:"deleted_at" json:"deleted_at,omitempty"`
}

// Deployment is an immutable OCI revision. Build and runtime workers will
// create and advance rows in a later slice; this slice only exposes safe reads.
type Deployment struct {
	bun.BaseModel `bun:"table:deployments,alias:dpl"`

	ID             uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID      uuid.UUID  `bun:"account_id,notnull,type:uuid" json:"account_id"`
	ProjectID      uuid.UUID  `bun:"project_id,notnull,type:uuid" json:"project_id"`
	ServiceID      uuid.UUID  `bun:"service_id,notnull,type:uuid" json:"service_id"`
	Revision       int        `bun:"revision,notnull" json:"revision"`
	ImageReference string     `bun:"image_reference,notnull" json:"image_reference"`
	ImageDigest    string     `bun:"image_digest,notnull" json:"image_digest"`
	ImageSizeBytes *int64     `bun:"image_size_bytes" json:"image_size_bytes,omitempty"`
	BuildProvider  string     `bun:"build_provider,notnull" json:"build_provider"`
	SourceRevision *string    `bun:"source_revision" json:"source_revision,omitempty"`
	Status         string     `bun:"status,notnull" json:"status"`
	IsActive       bool       `bun:"is_active,notnull" json:"is_active"`
	LastError      *string    `bun:"last_error" json:"last_error,omitempty"`
	StartedAt      *time.Time `bun:"started_at" json:"started_at,omitempty"`
	ReadyAt        *time.Time `bun:"ready_at" json:"ready_at,omitempty"`
	CompletedAt    *time.Time `bun:"completed_at" json:"completed_at,omitempty"`
	CreatedAt      time.Time  `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt      time.Time  `bun:"updated_at,notnull,default:now()" json:"updated_at"`
}

// DeploymentEvent is a safe customer-facing activity record. The database
// rejects updates and deletes to preserve event history.
type DeploymentEvent struct {
	bun.BaseModel `bun:"table:deployment_events,alias:de"`

	ID           uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID    uuid.UUID       `bun:"account_id,notnull,type:uuid" json:"account_id"`
	ProjectID    uuid.UUID       `bun:"project_id,notnull,type:uuid" json:"project_id"`
	ServiceID    uuid.UUID       `bun:"service_id,notnull,type:uuid" json:"service_id"`
	DeploymentID uuid.UUID       `bun:"deployment_id,notnull,type:uuid" json:"deployment_id"`
	EventType    string          `bun:"event_type,notnull" json:"event_type"`
	Message      string          `bun:"message,notnull" json:"message"`
	Metadata     json.RawMessage `bun:"metadata,type:jsonb,notnull" json:"metadata"`
	CreatedAt    time.Time       `bun:"created_at,notnull,default:now()" json:"created_at"`
}
