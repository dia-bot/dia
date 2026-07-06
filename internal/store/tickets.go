package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TicketRepo manages the ticketing tables: panels (with their JSONB categories),
// live tickets, the lifecycle event log, staff notes and participants. Every
// mutation is guild-scoped so one guild can never act on another's rows.
type TicketRepo struct{ pool *pgxpool.Pool }

// ── Panels ───────────────────────────────────────────────────

// ListPanels returns a guild's panels in display order.
func (r *TicketRepo) ListPanels(ctx context.Context, guildID int64) ([]TicketPanel, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, channel_id, message_id, name, style, config, enabled, position, created_at, updated_at
		FROM ticket_panels WHERE guild_id = $1 ORDER BY position, created_at`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TicketPanel
	for rows.Next() {
		p := TicketPanel{GuildID: guildID}
		if err := rows.Scan(&p.ID, &p.ChannelID, &p.MessageID, &p.Name, &p.Style,
			&p.Config, &p.Enabled, &p.Position, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetPanel returns one panel scoped to a guild, or ErrNotFound.
func (r *TicketRepo) GetPanel(ctx context.Context, guildID int64, id string) (TicketPanel, error) {
	p := TicketPanel{GuildID: guildID}
	err := r.pool.QueryRow(ctx, `
		SELECT id, channel_id, message_id, name, style, config, enabled, position, created_at, updated_at
		FROM ticket_panels WHERE id = $1 AND guild_id = $2`, id, guildID).
		Scan(&p.ID, &p.ChannelID, &p.MessageID, &p.Name, &p.Style, &p.Config, &p.Enabled,
			&p.Position, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	return p, err
}

// UpsertPanel inserts (when p.ID=="") or updates a panel's authored fields
// (name/style/config/enabled/position). It never writes channel_id/message_id —
// those are owned by SetPanelMessage (publish), so an editor save can't clear a
// posted panel's location.
func (r *TicketRepo) UpsertPanel(ctx context.Context, p TicketPanel) (TicketPanel, error) {
	if len(p.Config) == 0 {
		p.Config = json.RawMessage("{}")
	}
	if p.Style == "" {
		p.Style = "buttons"
	}
	if p.ID == "" {
		err := r.pool.QueryRow(ctx, `
			INSERT INTO ticket_panels (guild_id, name, style, config, enabled, position)
			VALUES ($1, $2, $3, $4, $5, COALESCE((SELECT max(position) + 1 FROM ticket_panels WHERE guild_id = $1), 0))
			RETURNING id, channel_id, message_id, position, created_at, updated_at`,
			p.GuildID, p.Name, p.Style, []byte(p.Config), p.Enabled).
			Scan(&p.ID, &p.ChannelID, &p.MessageID, &p.Position, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return TicketPanel{}, fmt.Errorf("insert ticket panel: %w", err)
		}
		return p, nil
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE ticket_panels
		SET name = $3, style = $4, config = $5, enabled = $6, updated_at = now()
		WHERE id = $1 AND guild_id = $2
		RETURNING channel_id, message_id, position, created_at, updated_at`,
		p.ID, p.GuildID, p.Name, p.Style, []byte(p.Config), p.Enabled).
		Scan(&p.ChannelID, &p.MessageID, &p.Position, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, ErrNotFound
	}
	if err != nil {
		return TicketPanel{}, fmt.Errorf("update ticket panel: %w", err)
	}
	return p, nil
}

// SetPanelMessage records where a panel was posted (channel + message).
func (r *TicketRepo) SetPanelMessage(ctx context.Context, guildID int64, id string, channelID, messageID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE ticket_panels SET channel_id = $3, message_id = $4, updated_at = now() WHERE id = $1 AND guild_id = $2`,
		id, guildID, channelID, messageID)
	return err
}

// DeletePanel removes a panel scoped to a guild (its tickets keep working; their
// panel_id is set NULL by the FK).
func (r *TicketRepo) DeletePanel(ctx context.Context, guildID int64, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM ticket_panels WHERE id = $1 AND guild_id = $2`, id, guildID)
	return err
}

// ── Tickets ──────────────────────────────────────────────────

const ticketCols = `id, guild_id, number, COALESCE(panel_id, '') AS panel_id, category_id, category_label,
	channel_id, is_thread, opener_id, opener_username, opener_global_name, subject, status, claimed_by,
	form_answers, auto_close_minutes, auto_warn_minutes, opened_at, claimed_at, first_response_at,
	last_activity_at, close_warned_at, closed_at, closed_by, close_reason, rating,
	feedback, transcript_url, transcript_messages`

func scanTicket(row pgx.Row, t *Ticket) error {
	return row.Scan(&t.ID, &t.GuildID, &t.Number, &t.PanelID, &t.CategoryID, &t.CategoryLabel,
		&t.ChannelID, &t.IsThread, &t.OpenerID, &t.OpenerUsername, &t.OpenerGlobalName, &t.Subject,
		&t.Status, &t.ClaimedBy, &t.FormAnswers, &t.AutoCloseMinutes, &t.AutoWarnMinutes, &t.OpenedAt,
		&t.ClaimedAt, &t.FirstResponseAt, &t.LastActivityAt, &t.CloseWarnedAt, &t.ClosedAt, &t.ClosedBy,
		&t.CloseReason, &t.Rating, &t.Feedback, &t.TranscriptURL, &t.TranscriptMessages)
}

// ErrOpenLimit / ErrCategoryLimit are returned by CreateTicketChecked when the
// opener already holds the maximum number of open tickets.
var (
	ErrOpenLimit     = errors.New("open ticket limit reached")
	ErrCategoryLimit = errors.New("category ticket limit reached")
)

// CreateTicketChecked opens a ticket while atomically enforcing the per-opener
// limits and allocating the next per-guild number. It takes a per-(guild,opener)
// transaction advisory lock so two concurrent opens by the same member serialize
// (closing the check-then-act gap in the caller's precheck), re-counts inside the
// lock, and retries the INSERT on a unique-violation from the per-guild `number`
// racing with a DIFFERENT opener. maxTotal / maxCategory of 0 disable that check.
func (r *TicketRepo) CreateTicketChecked(ctx context.Context, t Ticket, maxTotal, maxCategory int) (Ticket, error) {
	if len(t.FormAnswers) == 0 {
		t.FormAnswers = json.RawMessage("{}")
	}
	if t.Status == "" {
		t.Status = "open"
	}
	const maxAttempts = 5
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		out, retry, err := r.createTicketTx(ctx, t, maxTotal, maxCategory)
		if err == nil {
			return out, nil
		}
		lastErr = err
		if !retry {
			return Ticket{}, err
		}
	}
	return Ticket{}, fmt.Errorf("create ticket: %w", lastErr)
}

// createTicketTx is one attempt of CreateTicketChecked. retry=true signals a
// transient per-guild number collision worth another attempt.
func (r *TicketRepo) createTicketTx(ctx context.Context, t Ticket, maxTotal, maxCategory int) (out Ticket, retry bool, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Ticket{}, false, err
	}
	defer tx.Rollback(ctx)

	// Serialize concurrent opens by the same member so the re-count below is
	// authoritative (released automatically on commit/rollback).
	if _, err = tx.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1::text || ':' || $2::text, 0))`,
		t.GuildID, t.OpenerID); err != nil {
		return Ticket{}, false, err
	}
	if maxTotal > 0 {
		var n int
		if err = tx.QueryRow(ctx,
			`SELECT count(*) FROM tickets WHERE guild_id = $1 AND opener_id = $2 AND status = 'open'`,
			t.GuildID, t.OpenerID).Scan(&n); err != nil {
			return Ticket{}, false, err
		}
		if n >= maxTotal {
			return Ticket{}, false, ErrOpenLimit
		}
	}
	if maxCategory > 0 {
		var n int
		if err = tx.QueryRow(ctx,
			`SELECT count(*) FROM tickets WHERE guild_id = $1 AND opener_id = $2 AND category_id = $3 AND status = 'open'`,
			t.GuildID, t.OpenerID, t.CategoryID).Scan(&n); err != nil {
			return Ticket{}, false, err
		}
		if n >= maxCategory {
			return Ticket{}, false, ErrCategoryLimit
		}
	}

	out = t
	err = tx.QueryRow(ctx, `
		INSERT INTO tickets (guild_id, number, panel_id, category_id, category_label, channel_id,
			is_thread, opener_id, opener_username, opener_global_name, subject, status, form_answers,
			auto_close_minutes, auto_warn_minutes)
		VALUES ($1, (SELECT COALESCE(MAX(number), 0) + 1 FROM tickets WHERE guild_id = $1),
			NULLIF($2, ''), $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, number, opened_at, last_activity_at`,
		t.GuildID, t.PanelID, t.CategoryID, t.CategoryLabel, t.ChannelID, t.IsThread, t.OpenerID,
		t.OpenerUsername, t.OpenerGlobalName, t.Subject, t.Status, []byte(t.FormAnswers),
		t.AutoCloseMinutes, t.AutoWarnMinutes).
		Scan(&out.ID, &out.Number, &out.OpenedAt, &out.LastActivityAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Ticket{}, true, err // number raced with another opener; retry
		}
		return Ticket{}, false, fmt.Errorf("insert ticket: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Ticket{}, true, err
		}
		return Ticket{}, false, err
	}
	return out, false, nil
}

// SetTicketChannel records the Discord channel (or thread) a ticket was opened
// in, once it has been created (tickets are inserted with channel_id 0 so the
// per-guild number is available for the channel name before creation).
func (r *TicketRepo) SetTicketChannel(ctx context.Context, id string, channelID int64, isThread bool) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tickets SET channel_id = $2, is_thread = $3 WHERE id = $1`, id, channelID, isThread)
	return err
}

// GetTicket returns one ticket scoped to a guild, or ErrNotFound.
func (r *TicketRepo) GetTicket(ctx context.Context, guildID int64, id string) (Ticket, error) {
	var t Ticket
	err := scanTicket(r.pool.QueryRow(ctx,
		`SELECT `+ticketCols+` FROM tickets WHERE id = $1 AND guild_id = $2`, id, guildID), &t)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// GetTicketByChannel resolves the live (non-deleted) ticket a channel belongs
// to, or ErrNotFound. Backed by the partial unique index on channel_id.
func (r *TicketRepo) GetTicketByChannel(ctx context.Context, guildID, channelID int64) (Ticket, error) {
	var t Ticket
	err := scanTicket(r.pool.QueryRow(ctx,
		`SELECT `+ticketCols+` FROM tickets WHERE channel_id = $1 AND guild_id = $2 AND status <> 'deleted'`,
		channelID, guildID), &t)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, ErrNotFound
	}
	return t, err
}

// ListTickets returns a guild's tickets (optionally filtered by status),
// newest first.
func (r *TicketRepo) ListTickets(ctx context.Context, guildID int64, status string, limit int) ([]Ticket, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT ` + ticketCols + ` FROM tickets WHERE guild_id = $1 AND status <> 'deleted'`
	args := []any{guildID}
	if status != "" {
		q += ` AND status = $2`
		args = append(args, status)
	}
	q += fmt.Sprintf(` ORDER BY opened_at DESC LIMIT %d`, limit)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ticket
	for rows.Next() {
		var t Ticket
		if err := scanTicket(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// CountOpenByOpener counts a member's currently-open tickets (optionally within
// one category). categoryID "" counts across all categories.
func (r *TicketRepo) CountOpenByOpener(ctx context.Context, guildID, openerID int64, categoryID string) (int, error) {
	var n int
	q := `SELECT count(*) FROM tickets WHERE guild_id = $1 AND opener_id = $2 AND status = 'open'`
	args := []any{guildID, openerID}
	if categoryID != "" {
		q += ` AND category_id = $3`
		args = append(args, categoryID)
	}
	err := r.pool.QueryRow(ctx, q, args...).Scan(&n)
	return n, err
}

// SetClaim sets (or clears, claimedBy=0) a ticket's claimant.
func (r *TicketRepo) SetClaim(ctx context.Context, guildID int64, id string, claimedBy int64) error {
	if claimedBy == 0 {
		_, err := r.pool.Exec(ctx,
			`UPDATE tickets SET claimed_by = 0, claimed_at = NULL WHERE id = $1 AND guild_id = $2`, id, guildID)
		return err
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE tickets SET claimed_by = $3, claimed_at = now() WHERE id = $1 AND guild_id = $2`,
		id, guildID, claimedBy)
	return err
}

// TouchActivity refreshes a ticket's inactivity clock (and clears any pending
// auto-close warning) when a message arrives in its channel.
func (r *TicketRepo) TouchActivity(ctx context.Context, channelID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tickets SET last_activity_at = now(), close_warned_at = NULL
		 WHERE channel_id = $1 AND status = 'open'`, channelID)
	return err
}

// SetFirstResponse stamps first_response_at the first time a staff member
// replies (no-op afterwards).
func (r *TicketRepo) SetFirstResponse(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tickets SET first_response_at = now() WHERE id = $1 AND first_response_at IS NULL`, id)
	return err
}

// CloseTicket closes an open ticket. It is conditional on status='open' so two
// callers (a button, a command, the auto-close sweep across replicas) can't both
// run the close side effects: ok reports whether this call performed the close.
func (r *TicketRepo) CloseTicket(ctx context.Context, guildID int64, id string, closedBy int64, reason string) (bool, error) {
	ct, err := r.pool.Exec(ctx,
		`UPDATE tickets SET status = 'closed', closed_at = now(), closed_by = $3, close_reason = $4
		 WHERE id = $1 AND guild_id = $2 AND status = 'open'`, id, guildID, closedBy, reason)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// ReopenTicket reopens a closed ticket. ok reports whether it was closed.
func (r *TicketRepo) ReopenTicket(ctx context.Context, guildID int64, id string) (bool, error) {
	ct, err := r.pool.Exec(ctx,
		`UPDATE tickets SET status = 'open', closed_at = NULL, closed_by = 0, close_reason = '',
			close_warned_at = NULL, last_activity_at = now()
		 WHERE id = $1 AND guild_id = $2 AND status = 'closed'`, id, guildID)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// MarkDeleted flags a ticket deleted (its channel was removed).
func (r *TicketRepo) MarkDeleted(ctx context.Context, guildID int64, id string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tickets SET status = 'deleted' WHERE id = $1 AND guild_id = $2`, id, guildID)
	return err
}

// SetRating records the opener's post-close rating (1..5) and optional feedback.
func (r *TicketRepo) SetRating(ctx context.Context, guildID int64, id string, rating int, feedback string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tickets SET rating = $3, feedback = $4 WHERE id = $1 AND guild_id = $2`,
		id, guildID, rating, feedback)
	return err
}

// SetTranscript records a generated transcript's location + message count.
func (r *TicketRepo) SetTranscript(ctx context.Context, id, url string, count int) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE tickets SET transcript_url = $2, transcript_messages = $3 WHERE id = $1`, id, url, count)
	return err
}

// MarkWarned atomically records that the inactivity warning was posted. ok is
// true only for the caller that transitioned it (single-flight across replicas).
func (r *TicketRepo) MarkWarned(ctx context.Context, id string) (bool, error) {
	ct, err := r.pool.Exec(ctx,
		`UPDATE tickets SET close_warned_at = now()
		 WHERE id = $1 AND status = 'open' AND close_warned_at IS NULL`, id)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// DueAutoClose returns open tickets whose inactivity window has elapsed. The
// caller decides whether to warn first or close, based on close_warned_at +
// auto_warn_minutes.
func (r *TicketRepo) DueAutoClose(ctx context.Context, limit int) ([]Ticket, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+ticketCols+` FROM tickets
		WHERE status = 'open' AND auto_close_minutes > 0
		  AND last_activity_at < now() - make_interval(mins => auto_close_minutes)
		ORDER BY last_activity_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ticket
	for rows.Next() {
		var t Ticket
		if err := scanTicket(rows, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Stats computes the dashboard analytics aggregate for a guild.
func (r *TicketRepo) Stats(ctx context.Context, guildID int64) (TicketStats, error) {
	var s TicketStats
	err := r.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE status = 'open'),
			count(*) FILTER (WHERE status = 'closed'),
			count(*) FILTER (WHERE status <> 'deleted'),
			count(*) FILTER (WHERE opened_at > now() - interval '7 days' AND status <> 'deleted'),
			count(*) FILTER (WHERE closed_at IS NOT NULL AND closed_at > now() - interval '7 days'),
			count(*) FILTER (WHERE rating > 0),
			COALESCE(avg(rating) FILTER (WHERE rating > 0), 0),
			COALESCE(avg(EXTRACT(EPOCH FROM (first_response_at - opened_at))) FILTER (WHERE first_response_at IS NOT NULL), 0),
			COALESCE(avg(EXTRACT(EPOCH FROM (closed_at - opened_at))) FILTER (WHERE closed_at IS NOT NULL), 0)
		FROM tickets WHERE guild_id = $1`, guildID).
		Scan(&s.Open, &s.Closed, &s.Total, &s.Opened7d, &s.Closed7d, &s.Rated,
			&s.AvgRating, &s.AvgFirstResponseS, &s.AvgResolutionS)
	return s, err
}

// ── Lifecycle events ─────────────────────────────────────────

// AddEvent appends a lifecycle log row.
func (r *TicketRepo) AddEvent(ctx context.Context, e TicketEvent) error {
	if len(e.Data) == 0 {
		e.Data = json.RawMessage("{}")
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO ticket_events (ticket_id, guild_id, kind, actor_id, data) VALUES ($1, $2, $3, $4, $5)`,
		e.TicketID, e.GuildID, e.Kind, e.ActorID, []byte(e.Data))
	return err
}

// ListEvents returns a ticket's lifecycle log, oldest first.
func (r *TicketRepo) ListEvents(ctx context.Context, ticketID string, limit int) ([]TicketEvent, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, guild_id, kind, actor_id, data, created_at
		FROM ticket_events WHERE ticket_id = $1 ORDER BY created_at LIMIT $2`, ticketID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TicketEvent
	for rows.Next() {
		e := TicketEvent{TicketID: ticketID}
		if err := rows.Scan(&e.ID, &e.GuildID, &e.Kind, &e.ActorID, &e.Data, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ── Staff notes ──────────────────────────────────────────────

// AddNote inserts a staff-only note.
func (r *TicketRepo) AddNote(ctx context.Context, n TicketNote) (TicketNote, error) {
	err := r.pool.QueryRow(ctx,
		`INSERT INTO ticket_notes (ticket_id, guild_id, author_id, body) VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`, n.TicketID, n.GuildID, n.AuthorID, n.Body).
		Scan(&n.ID, &n.CreatedAt)
	if err != nil {
		return TicketNote{}, fmt.Errorf("add ticket note: %w", err)
	}
	return n, nil
}

// ListNotes returns a ticket's staff notes, oldest first.
func (r *TicketRepo) ListNotes(ctx context.Context, ticketID string) ([]TicketNote, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, guild_id, author_id, body, created_at FROM ticket_notes
		 WHERE ticket_id = $1 ORDER BY created_at`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TicketNote
	for rows.Next() {
		n := TicketNote{TicketID: ticketID}
		if err := rows.Scan(&n.ID, &n.GuildID, &n.AuthorID, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ── Participants ─────────────────────────────────────────────

// AddParticipant records a member granted access to a ticket (idempotent).
func (r *TicketRepo) AddParticipant(ctx context.Context, ticketID string, userID int64, role string, addedBy int64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ticket_participants (ticket_id, user_id, role, added_by) VALUES ($1, $2, $3, $4)
		ON CONFLICT (ticket_id, user_id) DO UPDATE SET role = EXCLUDED.role`,
		ticketID, userID, role, addedBy)
	return err
}

// RemoveParticipant drops a member's participant record.
func (r *TicketRepo) RemoveParticipant(ctx context.Context, ticketID string, userID int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM ticket_participants WHERE ticket_id = $1 AND user_id = $2`, ticketID, userID)
	return err
}
