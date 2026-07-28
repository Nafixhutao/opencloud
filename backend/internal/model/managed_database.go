package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ManagedDatabase is a tenant-owned database on a data-plane PostgreSQL or
// MariaDB target. Physical identifiers are internal and never returned by the
// customer API.
type ManagedDatabase struct {
	bun.BaseModel `bun:"table:databases,alias:d"`

	ID                   uuid.UUID  `bun:"id,pk,type:uuid,default:gen_random_uuid()" json:"id"`
	AccountID            uuid.UUID  `bun:"account_id,notnull,type:uuid" json:"-"`
	Name                 string     `bun:"name,notnull" json:"name"`
	Engine               string     `bun:"engine,notnull" json:"engine"`
	PhysicalDatabaseName string     `bun:"physical_database_name,notnull" json:"-"`
	PhysicalUsername     string     `bun:"physical_username,notnull" json:"-"`
	Status               string     `bun:"status,notnull" json:"status"`
	IdempotencyKey       string     `bun:"idempotency_key,notnull" json:"-"`
	LastError            *string    `bun:"last_error" json:"last_error,omitempty"`
	CredentialAvailable  bool       `bun:"credential_available,scanonly" json:"credential_available"`
	CreatedAt            time.Time  `bun:"created_at,notnull,default:now()" json:"created_at"`
	UpdatedAt            time.Time  `bun:"updated_at,notnull,default:now()" json:"updated_at"`
	DeletedAt            *time.Time `bun:"deleted_at" json:"deleted_at,omitempty"`
}

// DatabaseCredential is the encrypted, one-time credential envelope. The
// ciphertext is never serialized or returned by repositories that list domain
// resources.
type DatabaseCredential struct {
	bun.BaseModel `bun:"table:database_credentials,alias:dc"`

	DatabaseID uuid.UUID `bun:"database_id,pk,type:uuid"`
	Ciphertext []byte    `bun:"ciphertext,notnull"`
	CreatedAt  time.Time `bun:"created_at,notnull,default:now()"`
}

// Managed database lifecycle and engine values.
const (
	DatabaseEnginePostgres = "postgres"
	DatabaseEngineMariaDB  = "mariadb"

	DatabaseProvisioning = "provisioning"
	DatabaseActive       = "active"
	DatabaseDeleting     = "deleting"
	DatabaseDeleted      = "deleted"
	DatabaseFailed       = "failed"
)
