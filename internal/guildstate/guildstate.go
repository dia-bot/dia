// Package guildstate maintains a lightweight, realtime snapshot of each guild's
// channels, roles and meta in Redis. The worker is the single writer (it
// consumes gateway events); the API reads snapshots to populate dashboard
// dropdowns instantly and pushes live deltas over WebSocket. This is why a
// freshly-created channel appears in the dashboard immediately.
package guildstate

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/dia-bot/dia/internal/event"
	"github.com/redis/go-redis/v9"
)

// Store is a Redis-backed guild snapshot store.
type Store struct{ rdb *redis.Client }

// New creates a Store.
func New(rdb *redis.Client) *Store { return &Store{rdb: rdb} }

func metaKey(g string) string     { return "guild:" + g + ":meta" }
func channelsKey(g string) string { return "guild:" + g + ":channels" }
func rolesKey(g string) string    { return "guild:" + g + ":roles" }

// Meta is the cached guild header.
type Meta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	OwnerID     string `json:"owner_id"`
	MemberCount int    `json:"member_count"`
}

// Snapshot is the full cached view of a guild.
type Snapshot struct {
	Meta     Meta            `json:"meta"`
	Channels []event.Channel `json:"channels"`
	Roles    []event.Role    `json:"roles"`
}

// PutGuild replaces the entire snapshot for a guild (from GUILD_CREATE).
func (s *Store) PutGuild(ctx context.Context, g event.Guild) error {
	pipe := s.rdb.TxPipeline()
	pipe.Del(ctx, channelsKey(g.ID), rolesKey(g.ID))
	pipe.HSet(ctx, metaKey(g.ID), map[string]any{
		"id": g.ID, "name": g.Name, "icon": g.Icon,
		"owner_id": g.OwnerID, "member_count": g.MemberCount,
	})
	if len(g.Channels) > 0 {
		m := make(map[string]any, len(g.Channels))
		for _, c := range g.Channels {
			m[c.ID] = mustJSON(c)
		}
		pipe.HSet(ctx, channelsKey(g.ID), m)
	}
	if len(g.Roles) > 0 {
		m := make(map[string]any, len(g.Roles))
		for _, r := range g.Roles {
			m[r.ID] = mustJSON(r)
		}
		pipe.HSet(ctx, rolesKey(g.ID), m)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// RemoveGuild drops a guild's snapshot (bot left).
func (s *Store) RemoveGuild(ctx context.Context, guildID string) error {
	return s.rdb.Del(ctx, metaKey(guildID), channelsKey(guildID), rolesKey(guildID)).Err()
}

// SetMemberCount updates the cached member count.
func (s *Store) SetMemberCount(ctx context.Context, guildID string, n int) error {
	return s.rdb.HSet(ctx, metaKey(guildID), "member_count", n).Err()
}

// UpsertChannel adds/updates a channel.
func (s *Store) UpsertChannel(ctx context.Context, guildID string, c event.Channel) error {
	return s.rdb.HSet(ctx, channelsKey(guildID), c.ID, mustJSON(c)).Err()
}

// DeleteChannel removes a channel.
func (s *Store) DeleteChannel(ctx context.Context, guildID, channelID string) error {
	return s.rdb.HDel(ctx, channelsKey(guildID), channelID).Err()
}

// UpsertRole adds/updates a role.
func (s *Store) UpsertRole(ctx context.Context, guildID string, r event.Role) error {
	return s.rdb.HSet(ctx, rolesKey(guildID), r.ID, mustJSON(r)).Err()
}

// DeleteRole removes a role.
func (s *Store) DeleteRole(ctx context.Context, guildID, roleID string) error {
	return s.rdb.HDel(ctx, rolesKey(guildID), roleID).Err()
}

// Snapshot returns the full cached view of a guild.
func (s *Store) Snapshot(ctx context.Context, guildID string) (Snapshot, error) {
	var snap Snapshot

	metaMap, err := s.rdb.HGetAll(ctx, metaKey(guildID)).Result()
	if err != nil {
		return snap, err
	}
	snap.Meta = Meta{
		ID:          metaMap["id"],
		Name:        metaMap["name"],
		Icon:        metaMap["icon"],
		OwnerID:     metaMap["owner_id"],
		MemberCount: atoi(metaMap["member_count"]),
	}

	chMap, err := s.rdb.HGetAll(ctx, channelsKey(guildID)).Result()
	if err != nil {
		return snap, err
	}
	for _, raw := range chMap {
		var c event.Channel
		if json.Unmarshal([]byte(raw), &c) == nil {
			snap.Channels = append(snap.Channels, c)
		}
	}

	roleMap, err := s.rdb.HGetAll(ctx, rolesKey(guildID)).Result()
	if err != nil {
		return snap, err
	}
	for _, raw := range roleMap {
		var r event.Role
		if json.Unmarshal([]byte(raw), &r) == nil {
			snap.Roles = append(snap.Roles, r)
		}
	}
	return snap, nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// MetaError is returned when a guild has no cached snapshot yet.
var MetaError = fmt.Errorf("guildstate: no snapshot")
