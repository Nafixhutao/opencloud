package repository

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/database"
	"github.com/nazxf/opencloud/backend/internal/model"
)

func openSiteRepoTestDB(t *testing.T) *bun.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration tests")
	}
	db, err := database.Connect(context.Background(), dsn)
	if err != nil {
		t.Skipf("database unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func seedSiteFixtures(ctx context.Context, t *testing.T, db *bun.DB) (accountA, accountB uuid.UUID) {
	t.Helper()
	accountA = uuid.New()
	accountB = uuid.New()

	for _, acc := range []uuid.UUID{accountA, accountB} {
		account := &model.Account{
			ID:     acc,
			Name:   "test account",
			Status: model.AccountActive,
		}
		_, err := db.NewInsert().Model(account).Exec(ctx)
		require.NoError(t, err)
	}
	// Domains and sites cascade from accounts, and hostname claims cascade
	// from both; removing the accounts leaves later integration packages
	// (service tests reconcile globally) with a clean slate.
	t.Cleanup(func() {
		for _, acc := range []uuid.UUID{accountA, accountB} {
			_, _ = db.NewDelete().Model((*model.Account)(nil)).Where("id = ?", acc).Exec(ctx)
		}
	})
	return
}

// seedNodeFixture inserts the node every site row must reference. Placement
// assigns node/port/resource values in production; these repo-level tests
// bypass it, so fixtures must satisfy the table's check constraints directly.
func seedNodeFixture(ctx context.Context, t *testing.T, db *bun.DB) uuid.UUID {
	t.Helper()
	node := &model.Node{
		ID:            uuid.New(),
		Hostname:      "node-" + uuid.NewString()[:12],
		Backend:       "fake",
		Status:        "online",
		CapacitySites: 100,
	}
	_, err := db.NewInsert().Model(node).Exec(ctx)
	require.NoError(t, err)
	// Sites place on the least-loaded node, so leftover capacity-holding
	// nodes skew placement in later integration packages.
	t.Cleanup(func() {
		// Cleanups run LIFO, so this executes before the account cleanup:
		// remove this node's sites first or the FK silently refuses the
		// node delete and the residue skews later placement tests.
		_, _ = db.NewDelete().Model((*model.Site)(nil)).Where("node_id = ?", node.ID).Exec(ctx)
		_, _ = db.NewDelete().Model((*model.Node)(nil)).Where("id = ?", node.ID).Exec(ctx)
	})
	return node.ID
}

func TestSiteRepo_AccountScoping(t *testing.T) {
	db := openSiteRepoTestDB(t)
	siteRepo := NewSiteRepo(db)
	ctx := context.Background()
	accountA, accountB := seedSiteFixtures(ctx, t, db)
	nodeID := seedNodeFixture(ctx, t, db)

	t.Run("ListByAccount_ScopedCorrectly", func(t *testing.T) {
		siteA1 := &model.Site{
			ID:           uuid.New(),
			AccountID:    accountA,
			InternalPort: 8081,
			MemoryBytes:  268435456,
			NanoCPUs:     500000000,
			NodeID:       nodeID,
			Domain:       "site-" + uuid.NewString() + ".test",
			Image:        "opencloud/site-static:phase2",
			Status:       model.SiteActive,
			CreatedAt:    time.Now().UTC(),
		}
		err := siteRepo.Create(ctx, siteA1)
		require.NoError(t, err)

		siteB1 := &model.Site{
			ID:           uuid.New(),
			AccountID:    accountB,
			InternalPort: 8082,
			MemoryBytes:  268435456,
			NanoCPUs:     500000000,
			NodeID:       nodeID,
			Domain:       "site-" + uuid.NewString() + ".test",
			Image:        "opencloud/site-static:phase2",
			Status:       model.SiteActive,
			CreatedAt:    time.Now().UTC(),
		}
		err = siteRepo.Create(ctx, siteB1)
		require.NoError(t, err)

		sitesA, total, err := siteRepo.ListByAccount(ctx, accountA, 25, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, sitesA, 1)
		assert.Equal(t, siteA1.ID, sitesA[0].ID)

		sitesB, total, err := siteRepo.ListByAccount(ctx, accountB, 25, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, sitesB, 1)
		assert.Equal(t, siteB1.ID, sitesB[0].ID)
	})

	t.Run("GetByAccount_NotFoundWhenWrongAccount", func(t *testing.T) {
		site := &model.Site{
			ID:           uuid.New(),
			AccountID:    accountA,
			InternalPort: 8083,
			MemoryBytes:  268435456,
			NanoCPUs:     500000000,
			NodeID:       nodeID,
			Domain:       "site-" + uuid.NewString() + ".test",
			Image:        "opencloud/site-static:phase2",
			Status:       model.SiteActive,
			CreatedAt:    time.Now().UTC(),
		}
		err := siteRepo.Create(ctx, site)
		require.NoError(t, err)

		_, err = siteRepo.GetByAccount(ctx, accountB, site.ID)
		assert.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("SetStatus_TenantScoped", func(t *testing.T) {
		site := &model.Site{
			ID:           uuid.New(),
			AccountID:    accountA,
			InternalPort: 8084,
			MemoryBytes:  268435456,
			NanoCPUs:     500000000,
			NodeID:       nodeID,
			Domain:       "site-" + uuid.NewString() + ".test",
			Image:        "opencloud/site-static:phase2",
			Status:       model.SiteActive,
			CreatedAt:    time.Now().UTC(),
		}
		err := siteRepo.Create(ctx, site)
		require.NoError(t, err)

		err = siteRepo.SetStatus(ctx, accountB, site.ID, model.SiteDeleted)
		assert.ErrorIs(t, err, sql.ErrNoRows)

		current, err := siteRepo.GetByAccount(ctx, accountA, site.ID)
		require.NoError(t, err)
		assert.Equal(t, model.SiteActive, current.Status)
	})
}

func TestDomainRepo_AccountScoping(t *testing.T) {
	db := openSiteRepoTestDB(t)
	domainRepo := NewDomainRepo(db)
	ctx := context.Background()
	accountA, accountB := seedSiteFixtures(ctx, t, db)
	nodeID := seedNodeFixture(ctx, t, db)

	siteA := &model.Site{
		ID:           uuid.New(),
		AccountID:    accountA,
		InternalPort: 8085,
		MemoryBytes:  268435456,
		NanoCPUs:     500000000,
		NodeID:       nodeID,
		Domain:       "site-" + uuid.NewString() + ".test",
		Image:        "opencloud/site-static:phase2",
		Status:       model.SiteActive,
		CreatedAt:    time.Now().UTC(),
	}
	_, err := domainRepo.db.NewInsert().Model(siteA).Exec(ctx)
	require.NoError(t, err)

	siteB := &model.Site{
		ID:           uuid.New(),
		AccountID:    accountB,
		InternalPort: 8086,
		MemoryBytes:  268435456,
		NanoCPUs:     500000000,
		NodeID:       nodeID,
		Domain:       "site-" + uuid.NewString() + ".test",
		Image:        "opencloud/site-static:phase2",
		Status:       model.SiteActive,
		CreatedAt:    time.Now().UTC(),
	}
	_, err = domainRepo.db.NewInsert().Model(siteB).Exec(ctx)
	require.NoError(t, err)

	t.Run("GetByAccount_TenantIsolation", func(t *testing.T) {
		verifiedAt := time.Now().UTC()
		domain := &model.Domain{
			ID:                      uuid.New(),
			AccountID:               accountA,
			SiteID:                  siteA.ID,
			VerificationTokenDigest: make([]byte, 32),
			CertStatus:              "none",
			DNSProvider:             model.DNSProviderManual,
			DNSRecordIDs:            []byte("[]"),
			VerificationExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
			VerifiedAt:              &verifiedAt,
			VerificationConsumedAt:  &verifiedAt,
			Hostname:                "test.example.com",
			Status:                  model.DomainActive,
			CreatedAt:               time.Now().UTC(),
		}
		err = domainRepo.Create(ctx, domain)
		require.NoError(t, err)

		_, err = domainRepo.GetByAccount(ctx, accountB, domain.ID)
		assert.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("ListBySite_CrossTenantIsolation", func(t *testing.T) {
		verifiedAt := time.Now().UTC()
		domainA := &model.Domain{
			ID:                      uuid.New(),
			AccountID:               accountA,
			SiteID:                  siteA.ID,
			VerificationTokenDigest: make([]byte, 32),
			CertStatus:              "none",
			DNSProvider:             model.DNSProviderManual,
			DNSRecordIDs:            []byte("[]"),
			VerificationExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
			VerifiedAt:              &verifiedAt,
			VerificationConsumedAt:  &verifiedAt,
			Hostname:                "same-host.com",
			Status:                  model.DomainActive,
			CreatedAt:               time.Now().UTC(),
		}
		err = domainRepo.Create(ctx, domainA)
		require.NoError(t, err)

		domainB := &model.Domain{
			ID:                      uuid.New(),
			AccountID:               accountB,
			SiteID:                  siteB.ID,
			VerificationTokenDigest: make([]byte, 32),
			CertStatus:              "none",
			DNSProvider:             model.DNSProviderManual,
			DNSRecordIDs:            []byte("[]"),
			VerificationExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
			VerifiedAt:              &verifiedAt,
			VerificationConsumedAt:  &verifiedAt,
			Hostname:                "other-host.com",
			Status:                  model.DomainActive,
			CreatedAt:               time.Now().UTC(),
		}
		err = domainRepo.Create(ctx, domainB)
		require.NoError(t, err)

		// Earlier subtests share accountA, so assert membership and tenant
		// exclusion instead of absolute counts.
		domainsA, _, err := domainRepo.ListBySite(ctx, accountA, siteA.ID, 25, 0)
		require.NoError(t, err)
		hostnamesA := make([]string, len(domainsA))
		for i, d := range domainsA {
			hostnamesA[i] = d.Hostname
		}
		assert.Contains(t, hostnamesA, domainA.Hostname)
		assert.NotContains(t, hostnamesA, domainB.Hostname)

		domainsB, _, err := domainRepo.ListBySite(ctx, accountB, siteB.ID, 25, 0)
		require.NoError(t, err)
		hostnamesB := make([]string, len(domainsB))
		for i, d := range domainsB {
			hostnamesB[i] = d.Hostname
		}
		assert.Contains(t, hostnamesB, domainB.Hostname)
		assert.NotContains(t, hostnamesB, domainA.Hostname)
	})

	t.Run("AccountHostnameInUse_IsolatedPerTenant", func(t *testing.T) {
		verifiedAt := time.Now().UTC()
		domain := &model.Domain{
			ID:                      uuid.New(),
			AccountID:               accountA,
			SiteID:                  siteA.ID,
			VerificationTokenDigest: make([]byte, 32),
			CertStatus:              "none",
			DNSProvider:             model.DNSProviderManual,
			DNSRecordIDs:            []byte("[]"),
			VerificationExpiresAt:   time.Now().UTC().Add(24 * time.Hour),
			VerifiedAt:              &verifiedAt,
			VerificationConsumedAt:  &verifiedAt,
			Hostname:                "exclusive.example.com",
			Status:                  model.DomainActive,
			CreatedAt:               time.Now().UTC(),
		}
		err = domainRepo.Create(ctx, domain)
		require.NoError(t, err)

		inUseA, err := domainRepo.AccountHostnameInUse(ctx, accountA, "exclusive.example.com")
		require.NoError(t, err)
		assert.True(t, inUseA)

		inUseB, err := domainRepo.AccountHostnameInUse(ctx, accountB, "exclusive.example.com")
		require.NoError(t, err)
		assert.False(t, inUseB)
	})
}
