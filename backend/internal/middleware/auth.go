package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Identity context keys populated by Auth from a validated JWT.
const (
	contextAccountID = "account_id"
	contextUserID    = "user_id"
	contextRole      = "role"
)

// signing methods we accept. better-auth's jwt plugin signs asymmetrically
// (ADR 0006); we never accept HMAC/none, which would let a client forge tokens.
var acceptedMethods = []string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512", "EdDSA"}

// Claims are the JWT claims better-auth issues (ADR 0006): the registered set
// (sub/iss/aud/exp) plus the custom tenant claims OpenCloud authorizes on.
type Claims struct {
	AccountID string `json:"account_id"`
	Role      string `json:"role"`
	jwt.RegisteredClaims
}

// NewJWKS builds a JWK Set–backed jwt.Keyfunc that fetches better-auth's public
// keys from jwksURL and refreshes them in the background until ctx is cancelled
// (ADR 0006). The result is handed to Auth; wiring lives with the server.
func NewJWKS(ctx context.Context, jwksURL string) (jwt.Keyfunc, error) {
	k, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("build jwks keyfunc: %w", err)
	}
	return k.Keyfunc, nil
}

// Auth authenticates a request by validating its bearer JWT against the
// JWKS-backed keyFunc and the expected iss/aud, then puts the caller's identity
// (user, account, role) on the context. Go is a resource server: it verifies
// tokens and issues none (ADR 0006). On any failure it aborts with 401 and the
// standard error envelope, without revealing which check failed.
//
// issuer/audience are validated only when non-empty, so a dev config without
// them still authenticates on signature + expiry.
func Auth(keyFunc jwt.Keyfunc, issuer, audience string) gin.HandlerFunc {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods(acceptedMethods),
		jwt.WithExpirationRequired(),
	}
	if issuer != "" {
		opts = append(opts, jwt.WithIssuer(issuer))
	}
	if audience != "" {
		opts = append(opts, jwt.WithAudience(audience))
	}

	return func(c *gin.Context) {
		raw, ok := bearerToken(c)
		if !ok {
			abortUnauthorized(c, "missing or malformed Authorization header")
			return
		}

		var claims Claims
		token, err := jwt.ParseWithClaims(raw, &claims, keyFunc, opts...)
		if err != nil || !token.Valid {
			abortUnauthorized(c, "invalid or expired token")
			return
		}

		// account_id is our tenant-scoping key (SECURITY §4) and must be a UUID
		// referencing public.accounts; sub is better-auth's opaque user id.
		if claims.Subject == "" {
			abortUnauthorized(c, "token missing subject")
			return
		}
		acctID, err := uuid.Parse(claims.AccountID)
		if err != nil {
			abortUnauthorized(c, "token missing or invalid account_id")
			return
		}

		c.Set(contextUserID, claims.Subject)
		c.Set(contextAccountID, acctID)
		c.Set(contextRole, claims.Role)
		c.Next()
	}
}

// bearerToken extracts the token from an "Authorization: Bearer <token>" header.
func bearerToken(c *gin.Context) (string, bool) {
	const prefix = "Bearer "
	h := c.GetHeader("Authorization")
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(h[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

func abortUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{"code": "unauthorized", "message": msg},
	})
}

// AccountID returns the tenant account from a validated token, or uuid.Nil if
// the request did not pass Auth. Handlers pass this into services (BACKEND §5).
func AccountID(c *gin.Context) uuid.UUID {
	if v, ok := c.Get(contextAccountID); ok {
		if id, ok := v.(uuid.UUID); ok {
			return id
		}
	}
	return uuid.Nil
}

// UserID returns better-auth's user id (JWT sub), or "" if unset.
func UserID(c *gin.Context) string {
	if v, ok := c.Get(contextUserID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// Role returns the caller's role claim, or "" if unset.
func Role(c *gin.Context) string {
	if v, ok := c.Get(contextRole); ok {
		if r, ok := v.(string); ok {
			return r
		}
	}
	return ""
}
