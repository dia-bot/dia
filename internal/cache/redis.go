// Package cache provides the shared Redis client used for ephemeral state:
// per-user cooldowns, automod counters, realtime guild snapshots and pub/sub
// fan-out to the dashboard. Durable data lives in Postgres (see internal/store).
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrMiss indicates a missing or expired cache entry.
var ErrMiss = errors.New("cache miss")

// Store owns all Redis operations used by the app.
type Store struct {
	rdb *redis.Client
}

// Connect parses a redis:// URL, dials and verifies connectivity.
func Connect(ctx context.Context, url string, log *slog.Logger) (*Store, error) {
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
	return &Store{rdb: rdb}, nil
}

// Close closes the Redis connection.
func (s *Store) Close() error {
	return s.rdb.Close()
}

// SetString stores a string value with a TTL.
func (s *Store) SetString(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.rdb.Set(ctx, key, value, ttl).Err()
}

// TakeString atomically reads and deletes a string value.
func (s *Store) TakeString(ctx context.Context, key string) (string, error) {
	value, err := s.rdb.GetDel(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrMiss
	}
	return value, err
}

// SetJSON stores a JSON value with a TTL.
func (s *Store) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, key, raw, ttl).Err()
}

// GetJSON reads a JSON value into out.
func (s *Store) GetJSON(ctx context.Context, key string, out any) error {
	raw, err := s.rdb.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return ErrMiss
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// Delete removes one or more keys.
func (s *Store) Delete(ctx context.Context, keys ...string) error {
	return s.rdb.Del(ctx, keys...).Err()
}

// Reserve sets a key only if it does not already exist.
func (s *Store) Reserve(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.rdb.SetNX(ctx, key, 1, ttl).Result()
}

// Incr atomically increments a counter and, on the first increment, sets its
// TTL so the counter is a fixed window that auto-expires. It returns the new
// value. Used for rate-based automod (e.g. N messages within W seconds).
func (s *Store) Incr(ctx context.Context, key string, ttl time.Duration) (int64, error) {
	n, err := s.rdb.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	if n == 1 {
		_ = s.rdb.Expire(ctx, key, ttl).Err()
	}
	return n, nil
}

// ReplaceHashes deletes the target hashes and writes the replacement fields in
// one Redis transaction.
func (s *Store) ReplaceHashes(ctx context.Context, deleteKeys []string, hashes map[string]map[string]any) error {
	pipe := s.rdb.TxPipeline()
	if len(deleteKeys) > 0 {
		pipe.Del(ctx, deleteKeys...)
	}
	for key, fields := range hashes {
		if len(fields) > 0 {
			pipe.HSet(ctx, key, fields)
		}
	}
	_, err := pipe.Exec(ctx)
	return err
}

// SetHashField updates a single hash field.
func (s *Store) SetHashField(ctx context.Context, key, field string, value any) error {
	return s.rdb.HSet(ctx, key, field, value).Err()
}

// SetHashFields updates multiple hash fields.
func (s *Store) SetHashFields(ctx context.Context, key string, values map[string]any) error {
	return s.rdb.HSet(ctx, key, values).Err()
}

// DeleteHashField removes a single hash field.
func (s *Store) DeleteHashField(ctx context.Context, key, field string) error {
	return s.rdb.HDel(ctx, key, field).Err()
}

// HashFields returns every field from a Redis hash.
func (s *Store) HashFields(ctx context.Context, key string) (map[string]string, error) {
	return s.rdb.HGetAll(ctx, key).Result()
}
