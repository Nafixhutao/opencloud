package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// PreviewDeployment is a temporary deployment for PR review.
type PreviewDeployment struct {
	bun.BaseModel `bun:"table:preview_deployments,alias:pd"`

	ID        uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID uuid.UUID  `bun:"account_id,notnull,type:uuid" json:"account_id"`
	ProjectID uuid.UUID  `bun:"project_id,notnull,type:uuid" json:"project_id"`
	ServiceID uuid.UUID  `bun:"service_id,notnull,type:uuid" json:"service_id"`
	Branch    string     `bun:"branch,notnull" json:"branch"`
	CommitSHA string     `bun:"commit_sha,notnull" json:"commit_sha"`
	Domain    string     `bun:"domain,notnull" json:"domain"`
	Status    string     `bun:"status,notnull" json:"status"`
	SiteID    uuid.UUID  `bun:"site_id,type:uuid" json:"site_id"`
	CreatedAt time.Time  `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt time.Time  `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt *time.Time `bun:"deleted_at" json:"deleted_at,omitempty"`
}

// Preview deployment status values.
const (
	PreviewBuilding  = "building"
	PreviewDeploying = "deploying"
	PreviewActive    = "active"
	PreviewDestroyed = "destroyed"
	PreviewFailed    = "failed"
)
