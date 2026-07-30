package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/nazxf/opencloud/backend/internal/repository"
)

const rateLimitWindowCaddy = time.Minute

// CaddyPermissionHandler serves the Caddy on-demand TLS permission endpoint.
// It is intentionally unauthenticated — Caddy calls it as a webhook — but is
// rate-limited and only returns success for verified, active domains.
type CaddyPermissionHandler struct {
	domains *repository.DomainRepo
	rdb     *redis.Client
}

// NewCaddyPermissionHandler constructs a handler with a Redis-backed rate
// limiter keyed by hostname to prevent abuse.
func NewCaddyPermissionHandler(domains *repository.DomainRepo, rdb *redis.Client) *CaddyPermissionHandler {
	return &CaddyPermissionHandler{domains: domains, rdb: rdb}
}

// Check handles GET /api/v1/caddy/permission?hostname=example.com.
// Returns 200 OK only for a verified, active domain. Any other case returns
// 403 Forbidden without revealing whether the domain exists in another state.
func (h *CaddyPermissionHandler) Check(c *gin.Context) {
	hostname := c.Query("hostname")
	if hostname == "" {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// Simple rate limit: 10 checks per hostname per minute to prevent
	// enumeration and abuse of the permission endpoint.
	if h.rdb != nil {
		key := "caddy_permission:" + hostname
		count, err := h.rdb.Incr(c.Request.Context(), key).Result()
		if err == nil {
			if count == 1 {
				_ = h.rdb.Expire(c.Request.Context(), key, rateLimitWindowCaddy).Err()
			}
			if count > 10 {
				c.AbortWithStatus(http.StatusTooManyRequests)
				return
			}
		}
	}

	domain, err := h.domains.GetVerifiedActiveByHostname(c.Request.Context(), hostname)
	if err != nil || domain == nil {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	c.Status(http.StatusOK)
}