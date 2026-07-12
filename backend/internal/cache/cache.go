// Package cache wires the Redis client used for caching, sessions and rate
// limiting (INFRASTRUCTURE.md §1).
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Connect parses a Redis URL and returns a client, pinging once so a bad URL
// fails fast at boot.
func Connect(ctx context.Context, url string) (*redis.Client, error) {
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}

	client := redis.NewClient(opt)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}

// Ping checks Redis reachability with a short deadline. Used by /readyz.
func Ping(ctx context.Context, client *redis.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return client.Ping(ctx).Err()
}
