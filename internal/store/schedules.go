package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScheduledMessage is one composed message posted on a schedule.
type ScheduledMessage struct {
	ID        int64
	GuildID   int64
	Name      string
	ChannelID int64
	Spec      json.RawMessage // composed message (schedmessages.MessageSpec)
	Schedule  json.RawMessage // cadence (schedmessages.ScheduleDef)
	Enabled   bool
	NextRunAt *time.Time
	LastRunAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// SchedulesRepo manages scheduled_messages.
type SchedulesRepo struct{ pool *pgxpool.Pool }

const schedCols = `id, guild_id, name, channel_id, spec, schedule, enabled,
	next_run_at, last_run_at, created_at, updated_at`

func scanSched(row pgx.Row) (ScheduledMessage, error) {
	var s ScheduledMessage
	err := row.Scan(&s.ID, &s.GuildID, &s.Name, &s.ChannelID, &s.Spec, &s.Schedule, &s.Enabled,
		&s.NextRunAt, &s.LastRunAt, &s.CreatedAt, &s.UpdatedAt)
	if len(s.Spec) == 0 {
		s.Spec = json.RawMessage("{}")
	}
	if len(s.Schedule) == 0 {
		s.Schedule = json.RawMessage("{}")
	}
	return s, err
}

// ListByGuild returns a guild's schedules, oldest first.
func (r *SchedulesRepo) ListByGuild(ctx context.Context, guildID int64) ([]ScheduledMessage, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+schedCols+` FROM scheduled_messages
		WHERE guild_id = $1 ORDER BY id`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduledMessage
	for rows.Next() {
		s, err := scanSched(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Get returns one schedule scoped to a guild (found=false when absent).
func (r *SchedulesRepo) Get(ctx context.Context, guildID, id int64) (ScheduledMessage, bool, error) {
	s, err := scanSched(r.pool.QueryRow(ctx, `SELECT `+schedCols+` FROM scheduled_messages
		WHERE guild_id = $1 AND id = $2`, guildID, id))
	if err == pgx.ErrNoRows {
		return ScheduledMessage{}, false, nil
	}
	return s, err == nil, err
}

// ListDue returns enabled schedules whose next run has arrived.
func (r *SchedulesRepo) ListDue(ctx context.Context, now time.Time, limit int) ([]ScheduledMessage, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+schedCols+` FROM scheduled_messages
		WHERE enabled AND next_run_at IS NOT NULL AND next_run_at <= $1
		ORDER BY next_run_at LIMIT $2`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ScheduledMessage
	for rows.Next() {
		s, err := scanSched(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Create inserts a schedule and returns it with its id.
func (r *SchedulesRepo) Create(ctx context.Context, s ScheduledMessage) (ScheduledMessage, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO scheduled_messages (guild_id, name, channel_id, spec, schedule, enabled, next_run_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING `+schedCols,
		s.GuildID, s.Name, s.ChannelID, []byte(s.Spec), []byte(s.Schedule), s.Enabled, s.NextRunAt)
	out, err := scanSched(row)
	if err != nil {
		return ScheduledMessage{}, fmt.Errorf("create scheduled message: %w", err)
	}
	return out, nil
}

// Update saves the editable fields of a guild's schedule.
func (r *SchedulesRepo) Update(ctx context.Context, s ScheduledMessage) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE scheduled_messages
		SET name = $3, channel_id = $4, spec = $5, schedule = $6, enabled = $7,
		    next_run_at = $8, updated_at = now()
		WHERE guild_id = $1 AND id = $2`,
		s.GuildID, s.ID, s.Name, s.ChannelID, []byte(s.Spec), []byte(s.Schedule), s.Enabled, s.NextRunAt)
	return err
}

// Delete removes a guild's schedule.
func (r *SchedulesRepo) Delete(ctx context.Context, guildID, id int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM scheduled_messages WHERE guild_id = $1 AND id = $2`, guildID, id)
	return err
}

// SetRun records a completed run: the send time, the next due time (nil for a
// finished one-off) and whether the schedule stays enabled.
func (r *SchedulesRepo) SetRun(ctx context.Context, id int64, last time.Time, next *time.Time, enabled bool) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE scheduled_messages
		SET last_run_at = $2, next_run_at = $3, enabled = $4, updated_at = now()
		WHERE id = $1`, id, last, next, enabled)
	return err
}
