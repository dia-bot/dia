// Package cache provides the shared Redis client used for ephemeral state:
// per-user cooldowns, automod counters, realtime guild snapshots and pub/sub
// fan-out to the dashboard. Durable data lives in Postgres (see internal/store).
package cache

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// Connect parses a redis:// URL, dials and verifies connectivity.
func Connect(ctx context.Context, url string, log *slog.Logger) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rdb := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	log.Info("connected to redis", "addr", opts.Addr)
	return rdb, nil
}
