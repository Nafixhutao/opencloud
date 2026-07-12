// Package middleware holds cross-cutting Gin middleware. Order at wiring time:
// request-id → logger → recovery (auth, cors, ratelimit land in later phases —
// BACKEND.md §5).
package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/nazxf/opencloud/backend/internal/metrics"
)

// requestIDKey is the context/header key carrying the correlation id.
const requestIDHeader = "X-Request-ID"

const contextRequestID = "request_id"

const maxRequestIDLength = 128

// RequestID assigns each request a correlation id (honoring an inbound
// X-Request-ID) and echoes it back on the response. Every log line downstream
// carries it (BACKEND.md §11).
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if !validRequestID(id) {
			id = uuid.NewString()
		}
		c.Set(contextRequestID, id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

// validRequestID accepts common UUID, trace-id and proxy request-id formats
// while preventing arbitrary or oversized user-controlled values reaching logs.
func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for i := 0; i < len(id); i++ {
		b := id[i]
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
			(b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' || b == ':' {
			continue
		}
		return false
	}
	return true
}

// RequestIDOf returns the correlation id set by RequestID, or "" if unset.
func RequestIDOf(c *gin.Context) string {
	if v, ok := c.Get(contextRequestID); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// Recovery turns panics into 500s without killing the process, logging the
// cause with the request id (ARCHITECTURE.md §10).
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, err any) {
		log.Error("panic recovered",
			zap.Any("error", err),
			zap.String("request_id", RequestIDOf(c)),
			zap.String("path", c.FullPath()),
		)
		c.AbortWithStatusJSON(500, gin.H{
			"error": gin.H{"code": "internal", "message": "internal server error"},
		})
	})
}

// Logger emits one structured line per request and records Prometheus metrics.
// It uses the matched route (c.FullPath) as the label to keep cardinality low.
func Logger(log *zap.Logger, m *metrics.Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		dur := time.Since(start)
		status := c.Writer.Status()

		m.ObserveHTTP(route, c.Request.Method, status, dur)

		log.Info("request",
			zap.String("request_id", RequestIDOf(c)),
			zap.String("method", c.Request.Method),
			zap.String("route", route),
			zap.Int("status", status),
			zap.Duration("duration", dur),
		)
	}
}
