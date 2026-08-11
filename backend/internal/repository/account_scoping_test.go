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

func seedSiteFixtures(t *testing.T, db *bun.DB, ctx context.Context) (accountA, accountB uuid.UUID) {
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
	return
}

func TestSiteRepo_AccountScoping(t *testing.T) {
	db := openSiteRepoTestDB(t)
	siteRepo := NewSiteRepo(db)
	ctx := context.Background()
	accountA, accountB := seedSiteFixtures(t, db, ctx)

	t.Run("ListByAccount_ScopedCorrectly", func(t *testing.T) {
		siteA1 := &model.Site{
			ID:        uuid.New(),
			AccountID: accountA,
			Status:    model.SiteActive,
			CreatedAt: time.Now().UTC(),
		}
		err := siteRepo.Create(ctx, siteA1)
		require.NoError(t, err)

		siteB1 := &model.Site{
			ID:        uuid.New(),
			AccountID: accountB,
			Status:    model.SiteActive,
			CreatedAt: time.Now().UTC(),
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
			ID:        uuid.New(),
			AccountID: accountA,
			Status:    model.SiteActive,
			CreatedAt: time.Now().UTC(),
		}
		err := siteRepo.Create(ctx, site)
		require.NoError(t, err)

		_, err = siteRepo.GetByAccount(ctx, accountB, site.ID)
		assert.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("SetStatus_TenantScoped", func(t *testing.T) {
		site := &model.Site{
			ID:        uuid.New(),
			AccountID: accountA,
			Status:    model.SiteActive,
			CreatedAt: time.Now().UTC(),
		}
		err := siteRepo.Create(ctx, site)
		require.NoError(t, err)

		err = siteRepo.SetStatus(ctx, accountB, site.ID, model.SiteDeleted)
		assert.ErrorIs(t, err, sql.ErrNoRows)

		sites, _, _ := siteRepo.ListByAccount(ctx, accountA, 25, 0)
		assert.Len(t, sites, 1)
		assert.Equal(t, model.SiteActive, sites[0].Status)
	})
}

func TestDomainRepo_AccountScoping(t *testing.T) {
	db := openSiteRepoTestDB(t)
	domainRepo := NewDomainRepo(db)
	ctx := context.Background()
	accountA, accountB := seedSiteFixtures(t, db, ctx)

	siteA := &model.Site{
		ID:        uuid.New(),
		AccountID: accountA,
		Status:    model.SiteActive,
		CreatedAt: time.Now().UTC(),
	}
	_, err := domainRepo.db.NewInsert().Model(siteA).Exec(ctx)
	require.NoError(t, err)

	siteB := &model.Site{
		ID:        uuid.New(),
		AccountID: accountB,
		Status:    model.SiteActive,
		CreatedAt: time.Now().UTC(),
	}
	_, err = domainRepo.db.NewInsert().Model(siteB).Exec(ctx)
	require.NoError(t, err)

	t.Run("GetByAccount_TenantIsolation", func(t *testing.T) {
		domain := &model.Domain{
			ID:        uuid.New(),
			AccountID: accountA,
			SiteID:    siteA.ID,
			Hostname:  "test.example.com",
			Status:    model.DomainActive,
			CreatedAt: time.Now().UTC(),
		}
		err = domainRepo.Create(ctx, domain)
		require.NoError(t, err)

		_, err = domainRepo.GetByAccount(ctx, accountB, domain.ID)
		assert.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("ListBySite_CrossTenantIsolation", func(t *testing.T) {
		domainA := &model.Domain{
			ID:        uuid.New(),
			AccountID: accountA,
			SiteID:    siteA.ID,
			Hostname:  "same-host.com",
			Status:    model.DomainActive,
			CreatedAt: time.Now().UTC(),
		}
		err = domainRepo.Create(ctx, domainA)
		require.NoError(t, err)

		domainB := &model.Domain{
			ID:        uuid.New(),
			AccountID: accountB,
			SiteID:    siteB.ID,
			Hostname:  "same-host.com",
			Status:    model.DomainActive,
			CreatedAt: time.Now().UTC(),
		}
		err = domainRepo.Create(ctx, domainB)
		require.NoError(t, err)

		domainsA, total, err := domainRepo.ListBySite(ctx, accountA, siteA.ID, 25, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, domainsA, 1)
		assert.Equal(t, domainA.Hostname, domainsA[0].Hostname)

		domainsB, total, err := domainRepo.ListBySite(ctx, accountB, siteB.ID, 25, 0)
		require.NoError(t, err)
		assert.Equal(t, 1, total)
		assert.Len(t, domainsB, 1)
		assert.Equal(t, domainB.Hostname, domainsB[0].Hostname)
	})

	t.Run("AccountHostnameInUse_IsolatedPerTenant", func(t *testing.T) {
		domain := &model.Domain{
			ID:        uuid.New(),
			AccountID: accountA,
			SiteID:    siteA.ID,
			Hostname:  "exclusive.example.com",
			Status:    model.DomainActive,
			CreatedAt: time.Now().UTC(),
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
