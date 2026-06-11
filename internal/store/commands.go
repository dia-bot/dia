package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Command runs (durable execution state for wait/wait_for/scheduled) ───────

// CommandRunRepo manages command_runs and command_run_logs.
type CommandRunRepo struct{ pool *pgxpool.Pool }

// Insert creates a new run row. Caller assigns the ULID id.
func (r *CommandRunRepo) Insert(ctx context.Context, run CommandRun) error {
	if len(run.Scope) == 0 {
		run.Scope = json.RawMessage("{}")
	}
	if len(run.Cursor) == 0 {
		run.Cursor = json.RawMessage("[]")
	}
	if len(run.DefinitionSnapshot) == 0 {
		run.DefinitionSnapshot = json.RawMessage("{}")
	}
	if run.Status == "" {
		run.Status = "running"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO command_runs (
			id, command_id, command_version, guild_id, invoker_id, channel_id,
			trigger_kind, interaction_id, interaction_token, interaction_expires,
			scope, cursor, status, resume_at, awaiting_custom_id, awaiting_user_id,
			awaiting_kind, definition_snapshot
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		run.ID, run.CommandID, run.CommandVersion, run.GuildID, run.InvokerID, run.ChannelID,
		run.TriggerKind, run.InteractionID, run.InteractionToken, run.InteractionExpires,
		[]byte(run.Scope), []byte(run.Cursor), run.Status, run.ResumeAt,
		run.AwaitingCustomID, run.AwaitingUserID, run.AwaitingKind, []byte(run.DefinitionSnapshot))
	if err != nil {
		return fmt.Errorf("insert run: %w", err)
	}
	return nil
}

// UpdateState persists scope+cursor+status updates for an in-flight run.
func (r *CommandRunRepo) UpdateState(ctx context.Context, id string, scope, cursor json.RawMessage,
	status string, resumeAt *time.Time, awaitingCustomID string, awaitingUserID int64, awaitingKind string,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE command_runs SET
			scope = $2, cursor = $3, status = $4, resume_at = $5,
			awaiting_custom_id = $6, awaiting_user_id = $7, awaiting_kind = $8
		WHERE id = $1`,
		id, []byte(scope), []byte(cursor), status, resumeAt,
		awaitingCustomID, awaitingUserID, awaitingKind)
	return err
}

// MarkComplete sets the terminal state of a run.
func (r *CommandRunRepo) MarkComplete(ctx context.Context, id, status, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE command_runs SET status = $2, error = $3, completed_at = now(),
			resume_at = NULL, awaiting_custom_id = ''
		WHERE id = $1`, id, status, errMsg)
	return err
}

// Get reads one run by id.
func (r *CommandRunRepo) Get(ctx context.Context, id string) (CommandRun, error) {
	var run CommandRun
	err := r.pool.QueryRow(ctx, `
		SELECT id, command_id, command_version, guild_id, invoker_id, channel_id,
			trigger_kind, interaction_id, interaction_token, interaction_expires,
			scope, cursor, status, resume_at, awaiting_custom_id, awaiting_user_id,
			awaiting_kind, definition_snapshot, started_at, completed_at, error
		FROM command_runs WHERE id = $1`, id).
		Scan(&run.ID, &run.CommandID, &run.CommandVersion, &run.GuildID, &run.InvokerID, &run.ChannelID,
			&run.TriggerKind, &run.InteractionID, &run.InteractionToken, &run.InteractionExpires,
			&run.Scope, &run.Cursor, &run.Status, &run.ResumeAt, &run.AwaitingCustomID, &run.AwaitingUserID,
			&run.AwaitingKind, &run.DefinitionSnapshot, &run.StartedAt, &run.CompletedAt, &run.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, ErrNotFound
	}
	return run, err
}

// FindWaitingForComponent matches a waiting run by its routed custom_id prefix.
// The dispatcher calls this for every incoming component before falling through
// to feature handlers. Returns ErrNotFound when no run is parked on this id.
func (r *CommandRunRepo) FindWaitingForComponent(ctx context.Context, customID string) (CommandRun, error) {
	var run CommandRun
	err := r.pool.QueryRow(ctx, `
		SELECT id, command_id, command_version, guild_id, invoker_id, channel_id,
			trigger_kind, interaction_id, interaction_token, interaction_expires,
			scope, cursor, status, resume_at, awaiting_custom_id, awaiting_user_id,
			awaiting_kind, definition_snapshot, started_at, completed_at, error
		FROM command_runs
		WHERE status = 'waiting'
		  AND awaiting_kind IN ('component','modal')
		  AND $1 LIKE awaiting_custom_id || '%'
		ORDER BY started_at DESC
		LIMIT 1`, customID).
		Scan(&run.ID, &run.CommandID, &run.CommandVersion, &run.GuildID, &run.InvokerID, &run.ChannelID,
			&run.TriggerKind, &run.InteractionID, &run.InteractionToken, &run.InteractionExpires,
			&run.Scope, &run.Cursor, &run.Status, &run.ResumeAt, &run.AwaitingCustomID, &run.AwaitingUserID,
			&run.AwaitingKind, &run.DefinitionSnapshot, &run.StartedAt, &run.CompletedAt, &run.Error)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, ErrNotFound
	}
	return run, err
}

// DueWaits returns runs with resume_at <= now() that are waiting on a timer.
// The scheduler worker calls this on a tick to find runs to resume.
func (r *CommandRunRepo) DueWaits(ctx context.Context, limit int) ([]CommandRun, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, command_id, command_version, guild_id, invoker_id, channel_id,
			trigger_kind, interaction_id, interaction_token, interaction_expires,
			scope, cursor, status, resume_at, awaiting_custom_id, awaiting_user_id,
			awaiting_kind, definition_snapshot, started_at, completed_at, error
		FROM command_runs
		WHERE status = 'waiting' AND resume_at IS NOT NULL AND resume_at <= now()
		ORDER BY resume_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CommandRun
	for rows.Next() {
		var run CommandRun
		if err := rows.Scan(&run.ID, &run.CommandID, &run.CommandVersion, &run.GuildID, &run.InvokerID, &run.ChannelID,
			&run.TriggerKind, &run.InteractionID, &run.InteractionToken, &run.InteractionExpires,
			&run.Scope, &run.Cursor, &run.Status, &run.ResumeAt, &run.AwaitingCustomID, &run.AwaitingUserID,
			&run.AwaitingKind, &run.DefinitionSnapshot, &run.StartedAt, &run.CompletedAt, &run.Error); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// ClaimResume atomically transitions a waiting run into 'running' so two
// schedulers can't double-resume the same row. Returns true on success.
func (r *CommandRunRepo) ClaimResume(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE command_runs SET status = 'running', resume_at = NULL
		WHERE id = $1 AND status = 'waiting'`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListByGuild returns recent runs for a guild (Runs tab).
func (r *CommandRunRepo) ListByGuild(ctx context.Context, guildID int64, commandID int64, limit int) ([]CommandRun, error) {
	q := `
		SELECT id, command_id, command_version, guild_id, invoker_id, channel_id,
			trigger_kind, interaction_id, interaction_token, interaction_expires,
			scope, cursor, status, resume_at, awaiting_custom_id, awaiting_user_id,
			awaiting_kind, definition_snapshot, started_at, completed_at, error
		FROM command_runs WHERE guild_id = $1`
	args := []any{guildID}
	if commandID > 0 {
		q += ` AND command_id = $2`
		args = append(args, commandID)
	}
	q += ` ORDER BY started_at DESC LIMIT ` + itoa(limit)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CommandRun
	for rows.Next() {
		var run CommandRun
		if err := rows.Scan(&run.ID, &run.CommandID, &run.CommandVersion, &run.GuildID, &run.InvokerID, &run.ChannelID,
			&run.TriggerKind, &run.InteractionID, &run.InteractionToken, &run.InteractionExpires,
			&run.Scope, &run.Cursor, &run.Status, &run.ResumeAt, &run.AwaitingCustomID, &run.AwaitingUserID,
			&run.AwaitingKind, &run.DefinitionSnapshot, &run.StartedAt, &run.CompletedAt, &run.Error); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// ── Run logs ────────────────────────────────────────────────

// AppendLog inserts one structured step-execution log row.
func (r *CommandRunRepo) AppendLog(ctx context.Context, l CommandRunLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO command_run_logs (run_id, step_id, step_kind, cursor_path, duration_ms, status, input, output, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		l.RunID, l.StepID, l.StepKind, l.CursorPath, l.DurationMs, l.Status,
		nilOrBytes(l.Input), nilOrBytes(l.Output), l.Error)
	return err
}

// ListLogs returns the step timeline of a run.
func (r *CommandRunRepo) ListLogs(ctx context.Context, runID string) ([]CommandRunLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, run_id, step_id, step_kind, cursor_path, started_at, duration_ms, status, input, output, error
		FROM command_run_logs WHERE run_id = $1 ORDER BY started_at, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CommandRunLog
	for rows.Next() {
		var l CommandRunLog
		if err := rows.Scan(&l.ID, &l.RunID, &l.StepID, &l.StepKind, &l.CursorPath, &l.StartedAt,
			&l.DurationMs, &l.Status, &l.Input, &l.Output, &l.Error); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ── Feature KV ──────────────────────────────────────────────

// FeatureKVRepo manages feature_kv.
type FeatureKVRepo struct{ pool *pgxpool.Pool }

// Get reads a value, or ErrNotFound. Expired entries are treated as missing.
func (r *FeatureKVRepo) Get(ctx context.Context, e FeatureKVEntry) (FeatureKVEntry, error) {
	out := e
	err := r.pool.QueryRow(ctx, `
		SELECT value, expires_at, updated_at FROM feature_kv
		WHERE guild_id = $1 AND command_id = $2 AND scope = $3 AND owner_id = $4 AND key = $5
		  AND (expires_at IS NULL OR expires_at > now())`,
		e.GuildID, e.CommandID, e.Scope, e.OwnerID, e.Key).
		Scan(&out.Value, &out.ExpiresAt, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	return out, err
}

// Set upserts a value. ExpiresAt nil means no expiry.
func (r *FeatureKVRepo) Set(ctx context.Context, e FeatureKVEntry) error {
	if len(e.Value) == 0 {
		e.Value = json.RawMessage("null")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO feature_kv (guild_id, command_id, scope, owner_id, key, value, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (guild_id, command_id, scope, owner_id, key) DO UPDATE SET
			value = EXCLUDED.value, expires_at = EXCLUDED.expires_at, updated_at = now()`,
		e.GuildID, e.CommandID, e.Scope, e.OwnerID, e.Key, []byte(e.Value), e.ExpiresAt)
	return err
}

// Delete removes a value.
func (r *FeatureKVRepo) Delete(ctx context.Context, e FeatureKVEntry) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM feature_kv
		WHERE guild_id = $1 AND command_id = $2 AND scope = $3 AND owner_id = $4 AND key = $5`,
		e.GuildID, e.CommandID, e.Scope, e.OwnerID, e.Key)
	return err
}

// ── Command image templates ────────────────────────────────

// CommandImageTemplateRepo manages command_image_templates.
type CommandImageTemplateRepo struct{ pool *pgxpool.Pool }

// Upsert inserts/updates by id (when nonzero) or by (guild, name).
func (r *CommandImageTemplateRepo) Upsert(ctx context.Context, t CommandImageTemplate) (CommandImageTemplate, error) {
	if len(t.Layout) == 0 {
		t.Layout = json.RawMessage("{}")
	}
	if t.ID == 0 {
		err := r.pool.QueryRow(ctx, `
			INSERT INTO command_image_templates (guild_id, name, description, layout)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (guild_id, name) DO UPDATE SET
				description = EXCLUDED.description,
				layout = EXCLUDED.layout,
				updated_at = now()
			RETURNING id, created_at, updated_at`,
			t.GuildID, t.Name, t.Description, []byte(t.Layout)).
			Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return CommandImageTemplate{}, fmt.Errorf("upsert image template: %w", err)
		}
		return t, nil
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE command_image_templates SET name = $3, description = $4, layout = $5, updated_at = now()
		WHERE id = $1 AND guild_id = $2 RETURNING created_at, updated_at`,
		t.ID, t.GuildID, t.Name, t.Description, []byte(t.Layout)).
		Scan(&t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// Get returns one template by id.
func (r *CommandImageTemplateRepo) Get(ctx context.Context, guildID, id int64) (CommandImageTemplate, error) {
	t := CommandImageTemplate{GuildID: guildID}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, layout, created_at, updated_at
		FROM command_image_templates WHERE id = $1 AND guild_id = $2`, id, guildID).
		Scan(&t.ID, &t.Name, &t.Description, &t.Layout, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// List returns every template for a guild.
func (r *CommandImageTemplateRepo) List(ctx context.Context, guildID int64) ([]CommandImageTemplate, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, layout, created_at, updated_at
		FROM command_image_templates WHERE guild_id = $1 ORDER BY name`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CommandImageTemplate
	for rows.Next() {
		t := CommandImageTemplate{GuildID: guildID}
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Layout, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Delete removes one template.
func (r *CommandImageTemplateRepo) Delete(ctx context.Context, guildID, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM command_image_templates WHERE id = $1 AND guild_id = $2`, id, guildID)
	return err
}

// ── helpers ─────────────────────────────────────────────────

func nilOrBytes(r json.RawMessage) []byte {
	if len(r) == 0 {
		return nil
	}
	return []byte(r)
}

// itoa keeps the LIMIT injection sanitised (callers never pass user input).
func itoa(n int) string {
	if n <= 0 {
		return "50"
	}
	if n > 500 {
		n = 500
	}
	// Inline since Sprintf would pull fmt for one call; manual conversion is fine.
	const digits = "0123456789"
	buf := make([]byte, 0, 4)
	if n == 0 {
		return "0"
	}
	for n > 0 {
		buf = append([]byte{digits[n%10]}, buf...)
		n /= 10
	}
	return string(buf)
}
