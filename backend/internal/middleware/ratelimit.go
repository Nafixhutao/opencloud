package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RateLimit is a Redis fixed-window limiter for sensitive routes.
// keyPrefix groups counters; limit is the allowed hits per window.
func RateLimit(rdb *redis.Client, keyPrefix string, limit int, window time.Duration) gin.HandlerFunc {
	if limit <= 0 {
		limit = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	return func(c *gin.Context) {
		if rdb == nil {
			c.Next()
			return
		}
		key := fmt.Sprintf("ratelimit:%s:%s", keyPrefix, rateLimitIdentity(c))
		ctx := c.Request.Context()

		allowed, retryAfter, err := consume(ctx, rdb, key, limit, window)
		if err != nil {
			// Fail open on Redis errors for availability, but log via header for ops.
			c.Header("X-RateLimit-Error", "1")
			c.Next()
			return
		}
		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		if !allowed {
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
			}
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": gin.H{"code": "RATE_LIMITED", "message": "too many requests"},
			})
			return
		}
		c.Next()
	}
}

func rateLimitIdentity(c *gin.Context) string {
	if accountID := AccountID(c); accountID != uuid.Nil {
		return "account:" + accountID.String()
	}
	if userID := UserID(c); userID != "" {
		return "user:" + userID
	}
	return "ip:" + c.ClientIP()
}

func consume(ctx context.Context, rdb *redis.Client, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	// Fixed-window counter: INCR + EXPIRE on first hit. Good enough for auth abuse.
	n, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, 0, err
	}
	if n == 1 {
		if err := rdb.Expire(ctx, key, window).Err(); err != nil {
			return false, 0, err
		}
	}
	if n > int64(limit) {
		ttl, err := rdb.TTL(ctx, key).Result()
		if err != nil {
			ttl = window
		}
		return false, ttl, nil
	}
	return true, 0, nil
}
