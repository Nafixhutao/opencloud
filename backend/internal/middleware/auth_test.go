package middleware_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/middleware"
)

const (
	testIssuer   = "http://localhost:3000"
	testAudience = "opencloud"
	testKID      = "key-1"
)

var testAccountID = uuid.MustParse("11111111-1111-4111-8111-111111111111")

// keyfuncFor returns a jwt.Keyfunc that trusts pub only for tokens carrying the
// known kid — mirroring how a JWKS lookup fails on an unknown key id.
func keyfuncFor(pub *rsa.PublicKey) jwt.Keyfunc {
	return func(t *jwt.Token) (any, error) {
		if kid, _ := t.Header["kid"].(string); kid != testKID {
			return nil, jwt.ErrTokenUnverifiable
		}
		return pub, nil
	}
}

func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.Claims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	require.NoError(t, err)
	return s
}

// baseClaims is a valid claim set; each test tweaks a copy.
func baseClaims() middleware.Claims {
	now := time.Now()
	return middleware.Claims{
		AccountID: testAccountID.String(),
		Role:      "customer",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user_abc123",
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{testAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
	}
}

func newRouter(kf jwt.Keyfunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Auth(kf, testIssuer, testAudience))
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"account_id": middleware.AccountID(c).String(),
			"user_id":    middleware.UserID(c),
			"role":       middleware.Role(c),
		})
	})
	return r
}

func TestAuth(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kf := keyfuncFor(&key.PublicKey)

	// header builds the Authorization header for a given case.
	tests := []struct {
		name       string
		header     func() string
		wantStatus int
	}{
		{
			name:       "valid token",
			header:     func() string { return "Bearer " + signRS256(t, key, testKID, baseClaims()) },
			wantStatus: http.StatusOK,
		},
		{
			name: "expired token",
			header: func() string {
				c := baseClaims()
				c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
				return "Bearer " + signRS256(t, key, testKID, c)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing expiry",
			header: func() string {
				c := baseClaims()
				c.ExpiresAt = nil
				return "Bearer " + signRS256(t, key, testKID, c)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "wrong issuer",
			header: func() string {
				c := baseClaims()
				c.Issuer = "https://evil.example"
				return "Bearer " + signRS256(t, key, testKID, c)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "wrong audience",
			header: func() string {
				c := baseClaims()
				c.Audience = jwt.ClaimStrings{"someone-else"}
				return "Bearer " + signRS256(t, key, testKID, c)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "bad signature",
			header:     func() string { return "Bearer " + signRS256(t, otherKey, testKID, baseClaims()) },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "unknown kid",
			header:     func() string { return "Bearer " + signRS256(t, key, "key-unknown", baseClaims()) },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "hmac token rejected",
			header: func() string {
				tok := jwt.NewWithClaims(jwt.SigningMethodHS256, baseClaims())
				tok.Header["kid"] = testKID
				s, err := tok.SignedString([]byte("shared-secret"))
				require.NoError(t, err)
				return "Bearer " + s
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing account_id",
			header: func() string {
				c := baseClaims()
				c.AccountID = ""
				return "Bearer " + signRS256(t, key, testKID, c)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "non-uuid account_id",
			header: func() string {
				c := baseClaims()
				c.AccountID = "not-a-uuid"
				return "Bearer " + signRS256(t, key, testKID, c)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name: "missing subject",
			header: func() string {
				c := baseClaims()
				c.Subject = ""
				return "Bearer " + signRS256(t, key, testKID, c)
			},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no header",
			header:     func() string { return "" },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "malformed header",
			header:     func() string { return "Token abc" },
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if h := tt.header(); h != "" {
				req.Header.Set("Authorization", h)
			}
			rec := httptest.NewRecorder()
			newRouter(kf).ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus == http.StatusOK {
				var body struct {
					AccountID string `json:"account_id"`
					UserID    string `json:"user_id"`
					Role      string `json:"role"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Equal(t, testAccountID.String(), body.AccountID)
				require.Equal(t, "user_abc123", body.UserID)
				require.Equal(t, "customer", body.Role)
			} else {
				require.Contains(t, rec.Body.String(), `"unauthorized"`)
			}
		})
	}
}

// roleRouter mounts Auth then RequireRole(allowed...) so we exercise the real
// chain: a token's role claim flows through Auth onto the context, then gates.
func roleRouter(kf jwt.Keyfunc, allowed ...string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.Auth(kf, testIssuer, testAudience), middleware.RequireRole(allowed...))
	r.GET("/admin", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func TestRequireRole(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	kf := keyfuncFor(&key.PublicKey)

	tokenWithRole := func(role string) string {
		c := baseClaims()
		c.Role = role
		return "Bearer " + signRS256(t, key, testKID, c)
	}

	tests := []struct {
		name       string
		allowed    []string
		role       string
		wantStatus int
	}{
		{name: "admin allowed on admin route", allowed: []string{"admin"}, role: "admin", wantStatus: http.StatusOK},
		{name: "customer forbidden on admin route", allowed: []string{"admin"}, role: "customer", wantStatus: http.StatusForbidden},
		{name: "one of several roles matches", allowed: []string{"admin", "customer"}, role: "customer", wantStatus: http.StatusOK},
		{name: "empty role forbidden", allowed: []string{"admin"}, role: "", wantStatus: http.StatusForbidden},
		{name: "unknown role forbidden", allowed: []string{"admin"}, role: "superuser", wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			req.Header.Set("Authorization", tokenWithRole(tt.role))
			rec := httptest.NewRecorder()
			roleRouter(kf, tt.allowed...).ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			if tt.wantStatus == http.StatusForbidden {
				require.Contains(t, rec.Body.String(), `"forbidden"`)
			}
		})
	}
}

// TestRequireRole_WithoutAuth documents that RequireRole is a no-privilege gate
// on its own: with no Auth ahead of it the role is empty, so it forbids.
func TestRequireRole_WithoutAuth(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequireRole("admin"))
	r.GET("/admin", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusForbidden, rec.Code)
}
