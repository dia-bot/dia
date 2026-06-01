package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LevelRepo manages level_users and level_rewards.
type LevelRepo struct{ pool *pgxpool.Pool }

// AddXP atomically increments a member's XP and message count, returning the
// updated state.
func (r *LevelRepo) AddXP(ctx context.Context, guildID, userID, delta int64, now time.Time) (LevelUser, error) {
	lu := LevelUser{GuildID: guildID, UserID: userID}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO level_users (guild_id, user_id, xp, level, messages, last_message_at)
		VALUES ($1, $2, $3, 0, 1, $4)
		ON CONFLICT (guild_id, user_id) DO UPDATE SET
			xp = level_users.xp + $3,
			messages = level_users.messages + 1,
			last_message_at = $4
		RETURNING xp, level, messages, last_message_at`,
		guildID, userID, delta, now).
		Scan(&lu.XP, &lu.Level, &lu.Messages, &lu.LastMessageAt)
	if err != nil {
		return lu, fmt.Errorf("add xp: %w", err)
	}
	return lu, nil
}

// SetLevel updates the computed level for a member.
func (r *LevelRepo) SetLevel(ctx context.Context, guildID, userID int64, level int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE level_users SET level = $3 WHERE guild_id = $1 AND user_id = $2`, guildID, userID, level)
	return err
}

// Get returns a member's leveling state, or ErrNotFound.
func (r *LevelRepo) Get(ctx context.Context, guildID, userID int64) (LevelUser, error) {
	lu := LevelUser{GuildID: guildID, UserID: userID}
	err := r.pool.QueryRow(ctx,
		`SELECT xp, level, messages, last_message_at FROM level_users WHERE guild_id = $1 AND user_id = $2`,
		guildID, userID).Scan(&lu.XP, &lu.Level, &lu.Messages, &lu.LastMessageAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return lu, ErrNotFound
	}
	return lu, err
}

// Rank returns the 1-based rank of a member by XP (1 = highest), or ErrNotFound.
func (r *LevelRepo) Rank(ctx context.Context, guildID, userID int64) (int, error) {
	var rank int
	err := r.pool.QueryRow(ctx, `
		SELECT count(*) + 1 FROM level_users
		WHERE guild_id = $1
		  AND xp > (SELECT xp FROM level_users WHERE guild_id = $1 AND user_id = $2)`,
		guildID, userID).Scan(&rank)
	if err != nil {
		return 0, err
	}
	// Verify the member actually exists (otherwise the subquery was NULL).
	if _, err := r.Get(ctx, guildID, userID); err != nil {
		return 0, err
	}
	return rank, nil
}

// Leaderboard returns members ordered by XP descending.
func (r *LevelRepo) Leaderboard(ctx context.Context, guildID int64, limit, offset int) ([]LevelUser, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, xp, level, messages, last_message_at FROM level_users
		WHERE guild_id = $1 ORDER BY xp DESC LIMIT $2 OFFSET $3`, guildID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LevelUser
	for rows.Next() {
		lu := LevelUser{GuildID: guildID}
		if err := rows.Scan(&lu.UserID, &lu.XP, &lu.Level, &lu.Messages, &lu.LastMessageAt); err != nil {
			return nil, err
		}
		out = append(out, lu)
	}
	return out, rows.Err()
}

// ── Rewards ──────────────────────────────────────────────────

// ListRewards returns all configured level→role rewards for a guild.
func (r *LevelRepo) ListRewards(ctx context.Context, guildID int64) ([]LevelReward, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT level, role_id, remove_previous FROM level_rewards WHERE guild_id = $1 ORDER BY level`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LevelReward
	for rows.Next() {
		lr := LevelReward{GuildID: guildID}
		if err := rows.Scan(&lr.Level, &lr.RoleID, &lr.RemovePrevious); err != nil {
			return nil, err
		}
		out = append(out, lr)
	}
	return out, rows.Err()
}

// SetReward upserts a level reward.
func (r *LevelRepo) SetReward(ctx context.Context, lr LevelReward) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO level_rewards (guild_id, level, role_id, remove_previous)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (guild_id, level) DO UPDATE SET
			role_id = EXCLUDED.role_id, remove_previous = EXCLUDED.remove_previous`,
		lr.GuildID, lr.Level, lr.RoleID, lr.RemovePrevious)
	return err
}

// DeleteReward removes the reward at a level.
func (r *LevelRepo) DeleteReward(ctx context.Context, guildID int64, level int) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM level_rewards WHERE guild_id = $1 AND level = $2`, guildID, level)
	return err
}

// RewardsUpTo returns rewards for levels <= the given level (roles a member
// should hold), ordered by level ascending.
func (r *LevelRepo) RewardsUpTo(ctx context.Context, guildID int64, level int) ([]LevelReward, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT level, role_id, remove_previous FROM level_rewards WHERE guild_id = $1 AND level <= $2 ORDER BY level`,
		guildID, level)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []LevelReward
	for rows.Next() {
		lr := LevelReward{GuildID: guildID}
		if err := rows.Scan(&lr.Level, &lr.RoleID, &lr.RemovePrevious); err != nil {
			return nil, err
		}
		out = append(out, lr)
	}
	return out, rows.Err()
}
