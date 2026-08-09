package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

const (
	// EnvProduction marks production environment variables.
	EnvProduction = "production"
	// EnvPreview marks preview environment variables.
	EnvPreview = "preview"
	// EnvDevelopment marks development environment variables.
	EnvDevelopment = "development"

	// EnvAuditCreated marks variable creation.
	EnvAuditCreated = "created"
	// EnvAuditUpdated marks variable update.
	EnvAuditUpdated = "updated"
	// EnvAuditDeleted marks variable deletion.
	EnvAuditDeleted = "deleted"
	// EnvAuditRevealed marks secret value reveal.
	EnvAuditRevealed = "revealed"
	// EnvAuditRotated marks secret rotation.
	EnvAuditRotated = "rotated"
)

// EnvironmentVariable is a tenant-scoped, service-scoped, environment-scoped
// configuration value. Secrets are encrypted at rest and never logged.
type EnvironmentVariable struct {
	bun.BaseModel `bun:"table:environment_variables,alias:env"`

	ID             uuid.UUID `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID      uuid.UUID `bun:"account_id,notnull,type:uuid" json:"account_id"`
	ProjectID      uuid.UUID `bun:"project_id,notnull,type:uuid" json:"project_id"`
	ServiceID      uuid.UUID `bun:"service_id,notnull,type:uuid" json:"service_id"`
	Key            string    `bun:"key,notnull" json:"key"`
	Value          *string   `bun:"value" json:"value,omitempty"`
	IsSecret       bool      `bun:"is_secret,notnull" json:"is_secret"`
	EncryptedValue []byte    `bun:"encrypted_value" json:"-"`
	Environment    string    `bun:"environment,notnull" json:"environment"`
	CreatedAt      time.Time `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt      time.Time `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	CreatedBy      string    `bun:"created_by,notnull" json:"created_by"`
}

// EnvironmentVariableAudit is an append-only access and rotation trail.
type EnvironmentVariableAudit struct {
	bun.BaseModel `bun:"table:environment_variable_audit,alias:env_audit"`

	ID          uuid.UUID       `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID   uuid.UUID       `bun:"account_id,notnull,type:uuid" json:"account_id"`
	ProjectID   uuid.UUID       `bun:"project_id,notnull,type:uuid" json:"project_id"`
	ServiceID   uuid.UUID       `bun:"service_id,notnull,type:uuid" json:"service_id"`
	VariableID  *uuid.UUID      `bun:"variable_id,type:uuid" json:"variable_id,omitempty"`
	Action      string          `bun:"action,notnull" json:"action"`
	Key         string          `bun:"key,notnull" json:"key"`
	IsSecret    bool            `bun:"is_secret,notnull" json:"is_secret"`
	Environment string          `bun:"environment,notnull" json:"environment"`
	ActorID     string          `bun:"actor_id,notnull" json:"actor_id"`
	Metadata    json.RawMessage `bun:"metadata,type:jsonb,notnull" json:"metadata"`
	CreatedAt   time.Time       `bun:"created_at,notnull,default:now()" json:"created_at"`
}
