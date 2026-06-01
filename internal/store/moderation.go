package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ModRepo manages the moderation case log.
type ModRepo struct{ pool *pgxpool.Pool }

// CreateCase inserts a new moderation case, assigning the next per-guild case
// number, and returns the populated record.
func (r *ModRepo) CreateCase(ctx context.Context, c ModCase) (ModCase, error) {
	err := r.pool.QueryRow(ctx, `
		INSERT INTO mod_cases
			(guild_id, case_number, user_id, moderator_id, action, reason, duration_seconds, expires_at, active)
		VALUES (
			$1,
			(SELECT COALESCE(MAX(case_number), 0) + 1 FROM mod_cases WHERE guild_id = $1),
			$2, $3, $4, $5, $6, $7, $8
		)
		RETURNING id, case_number, created_at`,
		c.GuildID, c.UserID, c.ModeratorID, c.Action, c.Reason, c.DurationSeconds, c.ExpiresAt, c.Active).
		Scan(&c.ID, &c.CaseNumber, &c.CreatedAt)
	if err != nil {
		return ModCase{}, fmt.Errorf("create mod case: %w", err)
	}
	return c, nil
}

// ListCases returns cases for a guild (optionally filtered by user), newest first.
func (r *ModRepo) ListCases(ctx context.Context, guildID int64, userID *int64, limit, offset int) ([]ModCase, error) {
	q := `SELECT id, case_number, user_id, moderator_id, action, reason, duration_seconds, created_at, expires_at, active
	      FROM mod_cases WHERE guild_id = $1`
	args := []any{guildID}
	if userID != nil {
		q += ` AND user_id = $2`
		args = append(args, *userID)
	}
	q += fmt.Sprintf(` ORDER BY case_number DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ModCase
	for rows.Next() {
		c := ModCase{GuildID: guildID}
		if err := rows.Scan(&c.ID, &c.CaseNumber, &c.UserID, &c.ModeratorID, &c.Action,
			&c.Reason, &c.DurationSeconds, &c.CreatedAt, &c.ExpiresAt, &c.Active); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCase returns a single case by per-guild number, or ErrNotFound.
func (r *ModRepo) GetCase(ctx context.Context, guildID int64, caseNumber int) (ModCase, error) {
	c := ModCase{GuildID: guildID}
	err := r.pool.QueryRow(ctx, `
		SELECT id, case_number, user_id, moderator_id, action, reason, duration_seconds, created_at, expires_at, active
		FROM mod_cases WHERE guild_id = $1 AND case_number = $2`, guildID, caseNumber).
		Scan(&c.ID, &c.CaseNumber, &c.UserID, &c.ModeratorID, &c.Action,
			&c.Reason, &c.DurationSeconds, &c.CreatedAt, &c.ExpiresAt, &c.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// ListExpired returns active cases whose expiry has passed (for the scheduler
// to reverse: untimeout, unban).
func (r *ModRepo) ListExpired(ctx context.Context, now time.Time, limit int) ([]ModCase, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, case_number, user_id, moderator_id, action, reason, duration_seconds, created_at, expires_at, active
		FROM mod_cases
		WHERE active AND expires_at IS NOT NULL AND expires_at <= $1
		ORDER BY expires_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ModCase
	for rows.Next() {
		var c ModCase
		if err := rows.Scan(&c.ID, &c.CaseNumber, &c.UserID, &c.ModeratorID, &c.Action,
			&c.Reason, &c.DurationSeconds, &c.CreatedAt, &c.ExpiresAt, &c.Active); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Deactivate marks a case resolved (e.g. after an automatic unban).
func (r *ModRepo) Deactivate(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE mod_cases SET active = false WHERE id = $1`, id)
	return err
}
