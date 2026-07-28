package provisioner

import (
	"context"

	"github.com/google/uuid"
)

// DatabaseEndpoint is the customer-visible connection endpoint. Admin
// connection strings remain worker-only and never enter this type.
type DatabaseEndpoint struct {
	Host        string
	Port        int
	TLSRequired bool
}

// DatabaseSpec contains only deterministic, non-secret data-plane identifiers.
type DatabaseSpec struct {
	DatabaseID   uuid.UUID
	AccountID    uuid.UUID
	Engine       string
	DatabaseName string
	Username     string
}

// DatabaseRef identifies a managed data-plane database for deletion.
type DatabaseRef struct {
	DatabaseID   uuid.UUID
	AccountID    uuid.UUID
	Engine       string
	DatabaseName string
	Username     string
}

// DatabaseCredentials is returned only in worker memory, encrypted before it
// reaches PostgreSQL, and revealed once through the authenticated API.
type DatabaseCredentials struct {
	Engine      string `json:"engine"`
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Database    string `json:"database"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	TLSRequired bool   `json:"tls_required"`
}

// DatabaseProvisioner is the sole gateway to customer PostgreSQL/MariaDB
// targets. Implementations must converge safely when a job is retried after an
// ambiguous result.
type DatabaseProvisioner interface {
	CreateDatabase(ctx context.Context, spec DatabaseSpec) (DatabaseCredentials, error)
	DeleteDatabase(ctx context.Context, ref DatabaseRef) error
}
