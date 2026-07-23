package handler_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"

	"github.com/nazxf/opencloud/backend/internal/handler"
	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/model"
	"github.com/nazxf/opencloud/backend/internal/repository"
	"github.com/nazxf/opencloud/backend/internal/service"
)

func openDB(t *testing.T) *bun.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set")
	}
	sqldb := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(dsn)))
	db := bun.NewDB(sqldb, pgdialect.New())
	t.Cleanup(func() { _ = db.Close() })
	var n int
	if err := db.NewRaw(`SELECT count(*) FROM information_schema.tables WHERE table_name='account_memberships'`).Scan(context.Background(), &n); err != nil || n == 0 {
		t.Skip("migrations not applied")
	}
	return db
}

func withIdentity(userID string, accountID uuid.UUID, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", userID)
		c.Set("account_id", accountID)
		c.Set("role", role)
		c.Next()
	}
}

func TestMeAndAdminRBAC(t *testing.T) {
	db := openDB(t)
	svc := service.NewAccountService(db, repository.NewAccountRepo(db), repository.NewAuditRepo(db))
	h := handler.NewAccountHandler(svc)
	gin.SetMode(gin.TestMode)

	// Seed two customers.
	uA := "ha_" + uuid.NewString()
	uB := "hb_" + uuid.NewString()
	meA, err := svc.GetMe(context.Background(), uA, "TenantA")
	require.NoError(t, err)
	meB, err := svc.GetMe(context.Background(), uB, "TenantB")
	require.NoError(t, err)

	// Customer A /me
	r := gin.New()
	r.GET("/me", withIdentity(uA, meA.AccountID, model.RoleCustomer), h.Me)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/me", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), meA.AccountID.String())

	// Token account mismatch → 401
	r2 := gin.New()
	r2.GET("/me", withIdentity(uA, meB.AccountID, model.RoleCustomer), h.Me)
	w2 := httptest.NewRecorder()
	r2.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/me", nil))
	require.Equal(t, http.StatusUnauthorized, w2.Code)

	// Customer cannot list admin users (RequireRole gate)
	r3 := gin.New()
	r3.GET("/admin/users",
		withIdentity(uA, meA.AccountID, model.RoleCustomer),
		middleware.RequireRole(model.RoleAdmin),
		h.ListUsers,
	)
	w3 := httptest.NewRecorder()
	r3.ServeHTTP(w3, httptest.NewRequest(http.MethodGet, "/admin/users", nil))
	require.Equal(t, http.StatusForbidden, w3.Code)

	// Promote A to admin and list users.
	_, err = svc.BootstrapAdmin(context.Background(), uA)
	require.NoError(t, err)
	r4 := gin.New()
	r4.GET("/admin/users",
		withIdentity(uA, meA.AccountID, model.RoleAdmin),
		middleware.RequireRole(model.RoleAdmin),
		h.ListUsers,
	)
	w4 := httptest.NewRecorder()
	r4.ServeHTTP(w4, httptest.NewRequest(http.MethodGet, "/admin/users", nil))
	require.Equal(t, http.StatusOK, w4.Code)
	require.Contains(t, w4.Body.String(), `"data"`)

	// Cross-tenant profile patch rejected at service (handler path)
	r5 := gin.New()
	r5.PATCH("/me", withIdentity(uB, meA.AccountID, model.RoleCustomer), h.UpdateMe)
	body, _ := json.Marshal(map[string]string{"name": "Hijack"})
	req := httptest.NewRequest(http.MethodPatch, "/me", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w5 := httptest.NewRecorder()
	r5.ServeHTTP(w5, req)
	require.Equal(t, http.StatusNotFound, w5.Code)
}
