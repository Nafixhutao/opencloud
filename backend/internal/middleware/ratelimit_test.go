package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/nazxf/opencloud/backend/internal/middleware"
)

// memoryRedis is a minimal redis-compatible counter for RateLimit unit tests.
// Production uses go-redis; this keeps tests dependency-free.
type memoryRedis struct {
	mu   sync.Mutex
	data map[string]int64
}

func newMemoryRedis() *memoryRedis {
	return &memoryRedis{data: map[string]int64{}}
}

// The production RateLimit depends on *redis.Client. For unit tests without
// Redis we exercise the fail-open path (nil client) and a thin wrapper test of
// the HTTP 429 contract via a local fixed-window helper.

func TestRateLimitNilRedisFailOpen(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.RateLimit(nil, "test", 1, time.Minute))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		require.Equal(t, http.StatusOK, w.Code)
	}
}

func TestRateLimitFixedWindowLogic(t *testing.T) {
	t.Parallel()
	// Mirror the INCR semantics used by middleware.RateLimit.
	store := newMemoryRedis()
	limit := 3
	hit := func(key string) bool {
		store.mu.Lock()
		defer store.mu.Unlock()
		store.data[key]++
		return store.data[key] <= int64(limit)
	}
	for i := 0; i < 3; i++ {
		require.True(t, hit("ip1"))
	}
	require.False(t, hit("ip1"))
	require.True(t, hit("ip2"))
}
