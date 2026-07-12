// Package handler holds Gin HTTP handlers, one file per domain. Handlers
// translate HTTP ↔ domain and hold no business logic (BACKEND.md §5).
package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"

	"github.com/nazxf/opencloud/backend/internal/cache"
	"github.com/nazxf/opencloud/backend/internal/database"
)

// errNotConfigured signals a dependency was not wired (e.g. nil in a test).
var errNotConfigured = errors.New("not configured")

// Health serves liveness and readiness probes (INFRASTRUCTURE.md §7).
type Health struct {
	db  *bun.DB
	rdb *redis.Client
}

// NewHealth builds the health handler. rdb/db may be nil in tests that only
// exercise liveness.
func NewHealth(db *bun.DB, rdb *redis.Client) *Health {
	return &Health{db: db, rdb: rdb}
}

// Live handles GET /healthz — the process is up. Always 200.
func (h *Health) Live(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// Ready handles GET /readyz — dependencies reachable. 200 when Postgres and
// Redis both respond, else 503 with which check failed. Used by orchestrators
// and load balancers to gate traffic.
func (h *Health) Ready(c *gin.Context) {
	checks := gin.H{}
	ok := true

	if err := checkDB(c.Request.Context(), h.db); err != nil {
		checks["postgres"] = err.Error()
		ok = false
	} else {
		checks["postgres"] = "ok"
	}

	if err := checkRedis(c.Request.Context(), h.rdb); err != nil {
		checks["redis"] = err.Error()
		ok = false
	} else {
		checks["redis"] = "ok"
	}

	status := http.StatusOK
	word := "ready"
	if !ok {
		status = http.StatusServiceUnavailable
		word = "unready"
	}
	c.JSON(status, gin.H{"status": word, "checks": checks})
}

func checkDB(ctx context.Context, db *bun.DB) error {
	if db == nil {
		return errNotConfigured
	}
	return database.Ping(ctx, db)
}

func checkRedis(ctx context.Context, rdb *redis.Client) error {
	if rdb == nil {
		return errNotConfigured
	}
	return cache.Ping(ctx, rdb)
}
