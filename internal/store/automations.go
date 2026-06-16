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

// ── Automations (server-event step programs) ────────────────────────────────

// AutomationRepo manages the automations + automation_versions tables.
type AutomationRepo struct{ pool *pgxpool.Pool }

const automationCols = `id, guild_id, name, description, enabled, status, version,
	trigger_type, event_type, trigger_config, definition, created_by, created_at, updated_at`

func scanAutomation(row pgx.Row, a *Automation) error {
	return row.Scan(&a.ID, &a.GuildID, &a.Name, &a.Description, &a.Enabled, &a.Status, &a.Version,
		&a.TriggerType, &a.EventType, &a.TriggerConfig, &a.Definition, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
}

// Upsert inserts (when a.ID=="") or updates an automation. The assigned id is
// returned on insert.
func (r *AutomationRepo) Upsert(ctx context.Context, a Automation) (Automation, error) {
	if len(a.Definition) == 0 {
		a.Definition = json.RawMessage("{}")
	}
	if len(a.TriggerConfig) == 0 {
		a.TriggerConfig = json.RawMessage("{}")
	}
	if a.Status == "" {
		a.Status = "draft"
	}
	if a.Version <= 0 {
		a.Version = 1
	}
	if a.ID == "" {
		err := r.pool.QueryRow(ctx, `
			INSERT INTO automations (guild_id, name, description, enabled, status, version,
				trigger_type, event_type, trigger_config, definition, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			RETURNING id, created_at, updated_at`,
			a.GuildID, a.Name, a.Description, a.Enabled, a.Status, a.Version,
			a.TriggerType, a.EventType, []byte(a.TriggerConfig), []byte(a.Definition), a.CreatedBy).
			Scan(&a.ID, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return Automation{}, fmt.Errorf("insert automation: %w", err)
		}
		return a, nil
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE automations SET
			name = $3, description = $4, enabled = $5, status = $6, version = $7,
			trigger_type = $8, event_type = $9, trigger_config = $10, definition = $11, updated_at = now()
		WHERE id = $1 AND guild_id = $2
		RETURNING created_at, updated_at`,
		a.ID, a.GuildID, a.Name, a.Description, a.Enabled, a.Status, a.Version,
		a.TriggerType, a.EventType, []byte(a.TriggerConfig), []byte(a.Definition)).
		Scan(&a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	if err != nil {
		return Automation{}, fmt.Errorf("update automation: %w", err)
	}
	return a, nil
}

// Get returns one automation by id, scoped to a guild.
func (r *AutomationRepo) Get(ctx context.Context, guildID int64, id string) (Automation, error) {
	var a Automation
	err := scanAutomation(r.pool.QueryRow(ctx,
		`SELECT `+automationCols+` FROM automations WHERE id = $1 AND guild_id = $2`, id, guildID), &a)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, err
}

// Delete removes an automation scoped to a guild.
func (r *AutomationRepo) Delete(ctx context.Context, guildID int64, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM automations WHERE id = $1 AND guild_id = $2`, id, guildID)
	return err
}

// List returns all automations for a guild, ordered by name.
func (r *AutomationRepo) List(ctx context.Context, guildID int64) ([]Automation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+automationCols+` FROM automations WHERE guild_id = $1 ORDER BY name`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Automation
	for rows.Next() {
		var a Automation
		if err := scanAutomation(rows, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListEnabledByEvent returns the enabled automations for a guild that derive
// from a given gateway event — the per-event dispatch query (indexed).
func (r *AutomationRepo) ListEnabledByEvent(ctx context.Context, guildID int64, eventType string) ([]Automation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+automationCols+` FROM automations
		 WHERE guild_id = $1 AND event_type = $2 AND enabled ORDER BY name`, guildID, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Automation
	for rows.Next() {
		var a Automation
		if err := scanAutomation(rows, &a); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// PublishVersion writes an immutable snapshot of an automation's definition +
// trigger. The caller bumps automations.version + status='published'.
func (r *AutomationRepo) PublishVersion(ctx context.Context, v AutomationVersion) error {
	if len(v.Definition) == 0 {
		v.Definition = json.RawMessage("{}")
	}
	if len(v.TriggerConfig) == 0 {
		v.TriggerConfig = json.RawMessage("{}")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO automation_versions (automation_id, version, definition, trigger_type, trigger_config, published_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (automation_id, version) DO UPDATE SET
			definition = EXCLUDED.definition,
			trigger_type = EXCLUDED.trigger_type,
			trigger_config = EXCLUDED.trigger_config,
			published_by = EXCLUDED.published_by`,
		v.AutomationID, v.Version, []byte(v.Definition), v.TriggerType, []byte(v.TriggerConfig), v.PublishedBy)
	return err
}

// ── Automation runs (durable execution state) ───────────────────────────────

// AutomationRunRepo manages automation_runs + automation_run_logs. It mirrors
// CommandRunRepo so the same exec engine / scheduler patterns apply.
type AutomationRunRepo struct{ pool *pgxpool.Pool }

const automationRunCols = `id, automation_id, automation_version, guild_id, invoker_id, channel_id,
	trigger_kind, interaction_id, interaction_token, interaction_expires,
	scope, cursor, status, resume_at, awaiting_custom_id, awaiting_user_id,
	awaiting_kind, definition_snapshot, started_at, completed_at, error`

func scanAutomationRun(row pgx.Row, run *AutomationRun) error {
	return row.Scan(&run.ID, &run.AutomationID, &run.AutomationVersion, &run.GuildID, &run.InvokerID, &run.ChannelID,
		&run.TriggerKind, &run.InteractionID, &run.InteractionToken, &run.InteractionExpires,
		&run.Scope, &run.Cursor, &run.Status, &run.ResumeAt, &run.AwaitingCustomID, &run.AwaitingUserID,
		&run.AwaitingKind, &run.DefinitionSnapshot, &run.StartedAt, &run.CompletedAt, &run.Error)
}

// Insert creates a new run row. Caller assigns the id.
func (r *AutomationRunRepo) Insert(ctx context.Context, run AutomationRun) error {
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
		INSERT INTO automation_runs (
			id, automation_id, automation_version, guild_id, invoker_id, channel_id,
			trigger_kind, interaction_id, interaction_token, interaction_expires,
			scope, cursor, status, resume_at, awaiting_custom_id, awaiting_user_id,
			awaiting_kind, definition_snapshot
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		run.ID, run.AutomationID, run.AutomationVersion, run.GuildID, run.InvokerID, run.ChannelID,
		run.TriggerKind, run.InteractionID, run.InteractionToken, run.InteractionExpires,
		[]byte(run.Scope), []byte(run.Cursor), run.Status, run.ResumeAt,
		run.AwaitingCustomID, run.AwaitingUserID, run.AwaitingKind, []byte(run.DefinitionSnapshot))
	if err != nil {
		return fmt.Errorf("insert automation run: %w", err)
	}
	return nil
}

// UpdateState persists scope+cursor+status updates for an in-flight run.
func (r *AutomationRunRepo) UpdateState(ctx context.Context, id string, scope, cursor json.RawMessage,
	status string, resumeAt *time.Time, awaitingCustomID string, awaitingUserID int64, awaitingKind string,
) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE automation_runs SET
			scope = $2, cursor = $3, status = $4, resume_at = $5,
			awaiting_custom_id = $6, awaiting_user_id = $7, awaiting_kind = $8
		WHERE id = $1`,
		id, []byte(scope), []byte(cursor), status, resumeAt,
		awaitingCustomID, awaitingUserID, awaitingKind)
	return err
}

// MarkComplete sets the terminal state of a run.
func (r *AutomationRunRepo) MarkComplete(ctx context.Context, id, status, errMsg string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE automation_runs SET status = $2, error = $3, completed_at = now(),
			resume_at = NULL, awaiting_custom_id = ''
		WHERE id = $1`, id, status, errMsg)
	return err
}

// Get reads one run by id.
func (r *AutomationRunRepo) Get(ctx context.Context, id string) (AutomationRun, error) {
	var run AutomationRun
	err := scanAutomationRun(r.pool.QueryRow(ctx,
		`SELECT `+automationRunCols+` FROM automation_runs WHERE id = $1`, id), &run)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, ErrNotFound
	}
	return run, err
}

// FindWaitingForComponent matches a waiting run by its routed custom_id prefix.
func (r *AutomationRunRepo) FindWaitingForComponent(ctx context.Context, customID string) (AutomationRun, error) {
	var run AutomationRun
	err := scanAutomationRun(r.pool.QueryRow(ctx, `
		SELECT `+automationRunCols+` FROM automation_runs
		WHERE status = 'waiting' AND awaiting_kind IN ('component','modal')
		  AND $1 LIKE awaiting_custom_id || '%'
		ORDER BY started_at DESC LIMIT 1`, customID), &run)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, ErrNotFound
	}
	return run, err
}

// FindWaitingByKind returns the runs in a guild parked on a given wait kind
// ("message" / "reaction"). The worker resumes the matching ones when the
// corresponding event arrives. Backed by the partial wait-kind index.
func (r *AutomationRunRepo) FindWaitingByKind(ctx context.Context, guildID int64, kind string) ([]AutomationRun, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+automationRunCols+` FROM automation_runs
		WHERE guild_id = $1 AND status = 'waiting' AND awaiting_kind = $2
		ORDER BY started_at LIMIT 100`, guildID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutomationRun
	for rows.Next() {
		var run AutomationRun
		if err := scanAutomationRun(rows, &run); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// DueWaits returns runs with resume_at <= now() that are waiting on a timer.
func (r *AutomationRunRepo) DueWaits(ctx context.Context, limit int) ([]AutomationRun, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+automationRunCols+` FROM automation_runs
		WHERE status = 'waiting' AND resume_at IS NOT NULL AND resume_at <= now()
		ORDER BY resume_at LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutomationRun
	for rows.Next() {
		var run AutomationRun
		if err := scanAutomationRun(rows, &run); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// ClaimResume atomically transitions a waiting run into 'running'.
func (r *AutomationRunRepo) ClaimResume(ctx context.Context, id string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE automation_runs SET status = 'running', resume_at = NULL
		WHERE id = $1 AND status = 'waiting'`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListByGuild returns recent runs for a guild (Runs tab), optionally filtered
// to one automation.
func (r *AutomationRunRepo) ListByGuild(ctx context.Context, guildID int64, automationID string, limit int) ([]AutomationRun, error) {
	q := `SELECT ` + automationRunCols + ` FROM automation_runs WHERE guild_id = $1`
	args := []any{guildID}
	if automationID != "" {
		q += ` AND automation_id = $2`
		args = append(args, automationID)
	}
	q += ` ORDER BY started_at DESC LIMIT ` + itoa(limit)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutomationRun
	for rows.Next() {
		var run AutomationRun
		if err := scanAutomationRun(rows, &run); err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, rows.Err()
}

// GuildRunStats aggregates per-automation usage in one query (no N+1), bounded
// to 30 days so the guild range scan stays small.
func (r *AutomationRunRepo) GuildRunStats(ctx context.Context, guildID int64) (map[string]CommandRunStats, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT automation_id,
			count(*) FILTER (WHERE started_at > now() - interval '24 hours'),
			max(started_at)
		FROM automation_runs
		WHERE guild_id = $1 AND started_at > now() - interval '30 days'
		GROUP BY automation_id`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]CommandRunStats{}
	for rows.Next() {
		var id string
		var st CommandRunStats
		if err := rows.Scan(&id, &st.Runs24h, &st.LastRunAt); err != nil {
			return nil, err
		}
		out[id] = st
	}
	return out, rows.Err()
}

// AppendLog inserts one structured step-execution log row.
func (r *AutomationRunRepo) AppendLog(ctx context.Context, l AutomationRunLog) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO automation_run_logs (run_id, step_id, step_kind, cursor_path, duration_ms, status, input, output, error)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		l.RunID, l.StepID, l.StepKind, l.CursorPath, l.DurationMs, l.Status,
		nilOrBytes(l.Input), nilOrBytes(l.Output), l.Error)
	return err
}

// ListLogs returns the step timeline of a run.
func (r *AutomationRunRepo) ListLogs(ctx context.Context, runID string) ([]AutomationRunLog, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, run_id, step_id, step_kind, cursor_path, started_at, duration_ms, status, input, output, error
		FROM automation_run_logs WHERE run_id = $1 ORDER BY started_at, id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutomationRunLog
	for rows.Next() {
		var l AutomationRunLog
		if err := rows.Scan(&l.ID, &l.RunID, &l.StepID, &l.StepKind, &l.CursorPath, &l.StartedAt,
			&l.DurationMs, &l.Status, &l.Input, &l.Output, &l.Error); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}
