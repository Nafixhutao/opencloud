package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/handler"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
)

func TestResourceOverviewUsesTenantScopedAggregatesBeyondOnePage(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	requirePhase2ResourceTables(t, db)

	now := time.Now().UTC()
	account := &model.Account{
		ID:        uuid.New(),
		Name:      "Overview " + uuid.NewString(),
		Status:    model.AccountActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	otherAccount := &model.Account{
		ID:        uuid.New(),
		Name:      "Other overview " + uuid.NewString(),
		Status:    model.AccountActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := db.NewInsert().Model(&[]*model.Account{account, otherAccount}).Exec(ctx)
	require.NoError(t, err)

	node, err := repository.NewNodeRepo(db).Create(
		ctx,
		"overview-"+uuid.NewString()+".invalid",
		"fake",
		200,
		nil,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.NewDelete().
			Model((*model.Account)(nil)).
			Where("id = ? OR id = ?", account.ID, otherAccount.ID).
			Exec(ctx)
		_, _ = db.NewDelete().Model((*model.Node)(nil)).Where("id = ?", node.ID).Exec(ctx)
	})

	sites := make([]*model.Site, 0, 128)
	for index := range 126 {
		status := model.SiteProvisioning
		if index < 101 {
			status = model.SiteActive
		}
		sites = append(sites, overviewSite(account.ID, node.ID, status, index))
	}
	deletedSite := overviewSite(account.ID, node.ID, model.SiteDeleted, 126)
	deletedSite.DeletedAt = &now
	sites = append(sites, deletedSite)
	sites = append(sites, overviewSite(otherAccount.ID, node.ID, model.SiteActive, 127))
	_, err = db.NewInsert().Model(&sites).Exec(ctx)
	require.NoError(t, err)

	databases := make([]*model.ManagedDatabase, 0, 45)
	for index := range 43 {
		status := model.DatabaseFailed
		if index < 37 {
			status = model.DatabaseActive
		}
		databases = append(databases, overviewDatabase(account.ID, status, index))
	}
	deletedDatabase := overviewDatabase(account.ID, model.DatabaseDeleted, 43)
	deletedDatabase.DeletedAt = &now
	databases = append(databases, deletedDatabase)
	databases = append(
		databases,
		overviewDatabase(otherAccount.ID, model.DatabaseActive, 44),
	)
	_, err = db.NewInsert().Model(&databases).Exec(ctx)
	require.NoError(t, err)

	svc := service.NewResourceOverviewService(repository.NewResourceOverviewRepo(db))
	h := handler.NewResourceOverviewHandler(svc)
	router := gin.New()
	router.GET("/overview", withIdentity("overview-user", account.ID, model.RoleCustomer), h.Get)

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/overview", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{
		"data": {
			"sites_total": 126,
			"sites_active": 101,
			"databases_total": 43,
			"databases_active": 37
		}
	}`, response.Body.String())
}

func TestListHandlersReturnCanonicalPaginationMetadata(t *testing.T) {
	db := openDB(t)
	requirePhase2ResourceTables(t, db)

	accountService := service.NewAccountService(
		db,
		repository.NewAccountRepo(db),
		repository.NewAuditRepo(db),
	)
	userID := "pagination_" + uuid.NewString()
	me, err := accountService.GetMe(context.Background(), userID, "Pagination User")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = db.NewDelete().
			Model((*model.Account)(nil)).
			Where("id = ?", me.AccountID).
			Exec(context.Background())
	})

	siteService := service.NewSiteService(
		db,
		repository.NewSiteRepo(db),
		repository.NewNodeRepo(db),
		repository.NewJobRepo(db),
		repository.NewAuditRepo(db),
		"fake",
		"opencloud/site-static:test",
	)
	databaseService := service.NewManagedDatabaseService(
		db,
		repository.NewManagedDatabaseRepo(db),
		repository.NewJobRepo(db),
		repository.NewAuditRepo(db),
		false,
		nil,
	)

	router := gin.New()
	identity := withIdentity(userID, me.AccountID, model.RoleAdmin)
	router.GET("/sites", identity, handler.NewSiteHandler(siteService).List)
	router.GET("/databases", identity, handler.NewManagedDatabaseHandler(databaseService).List)
	router.GET("/admin/users", identity, handler.NewAccountHandler(accountService).ListUsers)

	for _, path := range []string{
		"/sites?page=2&per_page=1000",
		"/databases?page=2&per_page=1000",
		"/admin/users?page=2&per_page=1000",
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		require.Equal(t, http.StatusOK, response.Code, path)

		var payload struct {
			Meta struct {
				Page    int `json:"page"`
				PerPage int `json:"per_page"`
			} `json:"meta"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload), path)
		require.Equal(t, 2, payload.Meta.Page, path)
		require.Equal(t, 100, payload.Meta.PerPage, path)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(
			http.MethodGet,
			"/databases?page=999999999999999999999999&per_page=1000",
			nil,
		),
	)
	require.Equal(t, http.StatusOK, response.Code)
	var overflowPayload struct {
		Meta struct {
			Page    int `json:"page"`
			PerPage int `json:"per_page"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &overflowPayload))
	require.Equal(t, 1, overflowPayload.Meta.Page)
	require.Equal(t, 100, overflowPayload.Meta.PerPage)

	response = httptest.NewRecorder()
	router.ServeHTTP(
		response,
		httptest.NewRequest(http.MethodGet, "/sites?page=2&per_page=1", nil),
	)
	require.Equal(t, http.StatusOK, response.Code)
	var onePerPagePayload struct {
		Meta struct {
			Page    int `json:"page"`
			PerPage int `json:"per_page"`
		} `json:"meta"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &onePerPagePayload))
	require.Equal(t, 2, onePerPagePayload.Meta.Page)
	require.Equal(t, 1, onePerPagePayload.Meta.PerPage)
}

func requirePhase2ResourceTables(t *testing.T, db *bun.DB) {
	t.Helper()
	var count int
	err := db.NewRaw(`
		SELECT count(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('nodes', 'sites', 'databases')`).Scan(context.Background(), &count)
	require.NoError(t, err)
	if count != 3 {
		t.Skip("Phase 2 resource tables missing; run migrations first")
	}
}

func overviewSite(
	accountID uuid.UUID,
	nodeID uuid.UUID,
	status string,
	index int,
) *model.Site {
	return &model.Site{
		ID:             uuid.New(),
		AccountID:      accountID,
		NodeID:         nodeID,
		Domain:         "overview-" + uuid.NewString() + ".example.test",
		Image:          "opencloud/site-static:test",
		InternalPort:   8080,
		MemoryBytes:    256 * 1024 * 1024,
		NanoCPUs:       500_000_000,
		Status:         status,
		IdempotencyKey: strPtrForTest("overview-site-" + uuid.NewString()),
		CreatedAt:      time.Now().UTC().Add(time.Duration(index) * time.Nanosecond),
		UpdatedAt:      time.Now().UTC(),
	}
}

func overviewDatabase(
	accountID uuid.UUID,
	status string,
	index int,
) *model.ManagedDatabase {
	id := uuid.New()
	compactID := strings.ReplaceAll(id.String(), "-", "")
	return &model.ManagedDatabase{
		ID:                   id,
		AccountID:            accountID,
		Name:                 "overview_db_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16],
		Engine:               model.DatabaseEnginePostgres,
		PhysicalDatabaseName: "ocdb_" + compactID,
		PhysicalUsername:     "ocu_" + compactID,
		Status:               status,
		IdempotencyKey:       "overview-database-" + uuid.NewString(),
		CreatedAt:            time.Now().UTC().Add(time.Duration(index) * time.Nanosecond),
		UpdatedAt:            time.Now().UTC(),
	}
}

func strPtrForTest(value string) *string {
	return &value
}
