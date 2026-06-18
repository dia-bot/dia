package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// InfractionRepo manages the automod heat ledger (automod_infractions). Points
// accumulate per user and decay past their expires_at; the active sum drives the
// escalation ladder.
type InfractionRepo struct{ pool *pgxpool.Pool }

// Add inserts an infraction and returns the populated record.
func (r *InfractionRepo) Add(ctx context.Context, in AutomodInfraction) (AutomodInfraction, error) {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO automod_infractions
			(guild_id, user_id, rule_id, rule_name, trigger_type, points, reason, channel_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at`,
		in.GuildID, in.UserID, in.RuleID, in.RuleName, in.TriggerType, in.Points,
		in.Reason, in.ChannelID, in.ExpiresAt).
		Scan(&in.ID, &in.CreatedAt)
	if err != nil {
		return AutomodInfraction{}, fmt.Errorf("add infraction: %w", err)
	}
	return in, nil
}

// ActivePoints sums a user's still-active points (expires_at NULL or in the
// future relative to now). This is the value compared against escalation tiers.
func (r *InfractionRepo) ActivePoints(ctx context.Context, guildID, userID int64, now time.Time) (int, error) {
	var total int
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(points), 0) FROM automod_infractions
		WHERE guild_id = $1 AND user_id = $2 AND (expires_at IS NULL OR expires_at > $3)`,
		guildID, userID, now).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("active points: %w", err)
	}
	return total, nil
}

// ListByUser returns a user's infraction history, newest first.
func (r *InfractionRepo) ListByUser(ctx context.Context, guildID, userID int64, limit int) ([]AutomodInfraction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, rule_id, rule_name, trigger_type, points, reason, channel_id, created_at, expires_at
		FROM automod_infractions
		WHERE guild_id = $1 AND user_id = $2
		ORDER BY created_at DESC LIMIT $3`, guildID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInfractions(rows, guildID)
}

// ListRecent returns the guild's most recent infractions across all users.
func (r *InfractionRepo) ListRecent(ctx context.Context, guildID int64, limit int) ([]AutomodInfraction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, rule_id, rule_name, trigger_type, points, reason, channel_id, created_at, expires_at
		FROM automod_infractions
		WHERE guild_id = $1
		ORDER BY created_at DESC LIMIT $2`, guildID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInfractions(rows, guildID)
}

// TopOffenders aggregates active points per user since the given time, ordered
// by total points (the automod leaderboard).
func (r *InfractionRepo) TopOffenders(ctx context.Context, guildID int64, since time.Time, now time.Time, limit int) ([]Offender, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, COALESCE(SUM(points), 0) AS pts, COUNT(*) AS hits, MAX(created_at) AS last_at
		FROM automod_infractions
		WHERE guild_id = $1 AND created_at >= $2 AND (expires_at IS NULL OR expires_at > $3)
		GROUP BY user_id
		ORDER BY pts DESC, hits DESC
		LIMIT $4`, guildID, since, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Offender
	for rows.Next() {
		var o Offender
		if err := rows.Scan(&o.UserID, &o.TotalPoints, &o.Hits, &o.LastAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// CountSince returns the number of automod hits in a guild since a time (stats).
func (r *InfractionRepo) CountSince(ctx context.Context, guildID int64, since time.Time) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM automod_infractions WHERE guild_id = $1 AND created_at >= $2`,
		guildID, since).Scan(&n)
	return n, err
}

func scanInfractions(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}, guildID int64) ([]AutomodInfraction, error) {
	var out []AutomodInfraction
	for rows.Next() {
		in := AutomodInfraction{GuildID: guildID}
		if err := rows.Scan(&in.ID, &in.UserID, &in.RuleID, &in.RuleName, &in.TriggerType,
			&in.Points, &in.Reason, &in.ChannelID, &in.CreatedAt, &in.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}
