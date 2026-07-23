package middleware_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/middleware"
	"github.com/nazxf/opencloud/backend/internal/model"
)

type membershipLookup struct {
	membership *model.AccountMembership
	err        error
}

func (l membershipLookup) GetMembershipByUserID(context.Context, string) (*model.AccountMembership, error) {
	return l.membership, l.err
}

func TestAuthRejectsMissingRole(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kf := keyfuncFor(&key.PublicKey)

	claims := baseClaims()
	claims.Role = ""
	tok := signRS256(t, key, testKID, claims)

	r := newRouter(kf)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthRejectsInvalidRole(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kf := keyfuncFor(&key.PublicKey)

	claims := baseClaims()
	claims.Role = "superadmin"
	tok := signRS256(t, key, testKID, claims)

	r := newRouter(kf)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireRoleAdmin(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kf := keyfuncFor(&key.PublicKey)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Auth(kf, testIssuer, testAudience))
	r.GET("/admin", middleware.RequireRole("admin"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// customer → 403
	cust := baseClaims()
	cust.Role = "customer"
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+signRS256(t, key, testKID, cust))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)

	// admin → 200
	adm := baseClaims()
	adm.Role = "admin"
	adm.AccountID = uuid.NewString()
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req2.Header.Set("Authorization", "Bearer "+signRS256(t, key, testKID, adm))
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code)
}

func TestAuthAcceptsAdminRole(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kf := keyfuncFor(&key.PublicKey)

	claims := baseClaims()
	claims.Role = "admin"
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(time.Hour))
	tok := signRS256(t, key, testKID, claims)

	r := newRouter(kf)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"role":"admin"`)
}

func TestStaleAdminTokenIsRejectedAfterDemotion(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	claims := baseClaims()
	claims.Role = model.RoleAdmin
	accountID := uuid.MustParse(claims.AccountID)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(
		middleware.Auth(keyfuncFor(&key.PublicKey), testIssuer, testAudience),
		middleware.RequireCurrentMembership(membershipLookup{membership: &model.AccountMembership{
			UserID:    claims.Subject,
			AccountID: accountID,
			Role:      model.RoleCustomer,
			Status:    model.MembershipActive,
		}}),
	)
	r.GET("/admin", middleware.RequireRole(model.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+signRS256(t, key, testKID, claims))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}

func TestStaleTokenIsRejectedAfterSuspension(t *testing.T) {
	t.Parallel()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	claims := baseClaims()
	accountID := uuid.MustParse(claims.AccountID)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(
		middleware.Auth(keyfuncFor(&key.PublicKey), testIssuer, testAudience),
		middleware.RequireCurrentMembership(membershipLookup{membership: &model.AccountMembership{
			UserID:    claims.Subject,
			AccountID: accountID,
			Role:      claims.Role,
			Status:    model.MembershipSuspended,
		}}),
	)
	r.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+signRS256(t, key, testKID, claims))
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code)
}
