package handler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/model"
)

func TestSiteResponseHidesControlPlanePlacementDetails(t *testing.T) {
	now := time.Now().UTC()
	site := &model.Site{
		ID:           uuid.New(),
		AccountID:    uuid.New(),
		NodeID:       uuid.New(),
		Domain:       "safe.example.test",
		Image:        "private.registry/opencloud/site:internal",
		InternalPort: 8080,
		MemoryBytes:  256 * 1024 * 1024,
		NanoCPUs:     500_000_000,
		Status:       model.SiteActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	raw, err := json.Marshal(newSiteResponse(site))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"domain":"safe.example.test"`)
	require.NotContains(t, string(raw), site.AccountID.String())
	require.NotContains(t, string(raw), site.NodeID.String())
	require.NotContains(t, string(raw), "private.registry")
	require.NotContains(t, string(raw), "internal_port")
	require.NotContains(t, string(raw), "memory_bytes")
	require.NotContains(t, string(raw), "nano_cpus")
}

func TestManagedDatabaseResponseHidesTenantAndPhysicalIdentifiers(t *testing.T) {
	now := time.Now().UTC()
	row := &model.ManagedDatabase{
		ID:                   uuid.New(),
		AccountID:            uuid.New(),
		Name:                 "orders",
		Engine:               model.DatabaseEnginePostgres,
		PhysicalDatabaseName: "ocdb_0123456789abcdef0123456789abcdef",
		PhysicalUsername:     "ocu_0123456789abcdef0123456789abcdef",
		Status:               model.DatabaseActive,
		IdempotencyKey:       "private-idempotency-key",
		CredentialAvailable:  true,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	raw, err := json.Marshal(newManagedDatabaseResponse(row))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"name":"orders"`)
	require.Contains(t, string(raw), `"credential_available":true`)
	require.NotContains(t, string(raw), row.AccountID.String())
	require.NotContains(t, string(raw), row.PhysicalDatabaseName)
	require.NotContains(t, string(raw), row.PhysicalUsername)
	require.NotContains(t, string(raw), row.IdempotencyKey)
}
