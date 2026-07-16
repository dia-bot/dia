package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a lookup matches no row.
var ErrNotFound = errors.New("store: not found")

// GuildRepo manages the guilds table.
type GuildRepo struct{ pool *pgxpool.Pool }

// Upsert inserts or refreshes a guild (clears left_at — the bot is present).
func (r *GuildRepo) Upsert(ctx context.Context, g Guild) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO guilds (id, name, icon, owner_id, member_count, joined_at, updated_at, left_at)
		VALUES ($1, $2, $3, $4, $5, now(), now(), NULL)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			icon = EXCLUDED.icon,
			owner_id = EXCLUDED.owner_id,
			member_count = EXCLUDED.member_count,
			updated_at = now(),
			left_at = NULL`,
		g.ID, g.Name, g.Icon, g.OwnerID, g.MemberCount)
	if err != nil {
		return fmt.Errorf("upsert guild: %w", err)
	}
	return nil
}

// UpdateMemberCount adjusts the cached member count.
func (r *GuildRepo) UpdateMemberCount(ctx context.Context, id int64, count int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE guilds SET member_count = $2, updated_at = now() WHERE id = $1`, id, count)
	return err
}

// MarkLeft flags that the bot left a guild (kept for history/analytics).
func (r *GuildRepo) MarkLeft(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE guilds SET left_at = now(), updated_at = now() WHERE id = $1`, id)
	return err
}

// Get returns a guild by ID, or ErrNotFound.
func (r *GuildRepo) Get(ctx context.Context, id int64) (Guild, error) {
	var g Guild
	err := r.pool.QueryRow(ctx,
		`SELECT id, name, icon, owner_id, member_count, joined_at, left_at, updated_at
		 FROM guilds WHERE id = $1`, id).
		Scan(&g.ID, &g.Name, &g.Icon, &g.OwnerID, &g.MemberCount, &g.JoinedAt, &g.LeftAt, &g.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Guild{}, ErrNotFound
	}
	return g, err
}

// ListByIDs returns the active (present) guilds among the given IDs. Used by the
// API to intersect a user's Discord guilds with guilds the bot is in.
func (r *GuildRepo) ListByIDs(ctx context.Context, ids []int64) ([]Guild, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, icon, owner_id, member_count, joined_at, left_at, updated_at
		 FROM guilds WHERE id = ANY($1) AND left_at IS NULL`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Guild
	for rows.Next() {
		var g Guild
		if err := rows.Scan(&g.ID, &g.Name, &g.Icon, &g.OwnerID, &g.MemberCount,
			&g.JoinedAt, &g.LeftAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// ── Feature configs ──────────────────────────────────────────

// FeatureConfigRepo manages guild_feature_configs.
type FeatureConfigRepo struct{ pool *pgxpool.Pool }

// Get returns the config for a feature. A missing row yields a disabled config
// with an empty JSON object (never ErrNotFound — features always have defaults).
func (r *FeatureConfigRepo) Get(ctx context.Context, guildID int64, feature string) (FeatureConfig, error) {
	fc := FeatureConfig{GuildID: guildID, Feature: feature, Config: json.RawMessage("{}")}
	err := r.pool.QueryRow(ctx,
		`SELECT enabled, config, updated_at FROM guild_feature_configs
		 WHERE guild_id = $1 AND feature_key = $2`, guildID, feature).
		Scan(&fc.Enabled, &fc.Config, &fc.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return fc, nil
	}
	if err != nil {
		return fc, fmt.Errorf("get feature config: %w", err)
	}
	return fc, nil
}

// Upsert writes a feature config (the row is created if absent).
func (r *FeatureConfigRepo) Upsert(ctx context.Context, guildID int64, feature string, enabled bool, cfg json.RawMessage) error {
	if len(cfg) == 0 {
		cfg = json.RawMessage("{}")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO guild_feature_configs (guild_id, feature_key, enabled, config, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (guild_id, feature_key) DO UPDATE SET
			enabled = EXCLUDED.enabled,
			config = EXCLUDED.config,
			updated_at = now()`,
		guildID, feature, enabled, []byte(cfg))
	if err != nil {
		return fmt.Errorf("upsert feature config: %w", err)
	}
	return nil
}

// ListGuildsEnabled returns every guild id with a feature switched on — the
// sweep set for background workers (stats counters, schedule sweeps).
func (r *FeatureConfigRepo) ListGuildsEnabled(ctx context.Context, feature string) ([]int64, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT guild_id FROM guild_feature_configs WHERE feature_key = $1 AND enabled`, feature)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// GetAll returns every feature config for a guild keyed by feature_key.
func (r *FeatureConfigRepo) GetAll(ctx context.Context, guildID int64) (map[string]FeatureConfig, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT feature_key, enabled, config, updated_at FROM guild_feature_configs WHERE guild_id = $1`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]FeatureConfig)
	for rows.Next() {
		fc := FeatureConfig{GuildID: guildID}
		if err := rows.Scan(&fc.Feature, &fc.Enabled, &fc.Config, &fc.UpdatedAt); err != nil {
			return nil, err
		}
		out[fc.Feature] = fc
	}
	return out, rows.Err()
}
