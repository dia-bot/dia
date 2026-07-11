package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Giveaways ───────────────────────────────────────────────────────────────

// GiveawayRepo manages the giveaways + giveaway_entries tables. Entries are one
// weighted row per member; the sweeper (ListDue → ClaimEnd → SetWinners) ends
// running giveaways at their deadline, which is how timers survive restarts.
type GiveawayRepo struct{ pool *pgxpool.Pool }

// ErrGiveawayNotFound is returned when a giveaway id/message doesn't resolve.
var ErrGiveawayNotFound = errors.New("giveaway not found")

const giveawayCols = `id, guild_id, channel_id, message_id, name, prize, description, winner_count,
	host_id, status, spec, requirements, image_url, color, winner_ids, starts_at, ends_at, ended_at,
	created_by, created_at, updated_at`

func scanGiveaway(row pgx.Row, g *Giveaway) error {
	return row.Scan(&g.ID, &g.GuildID, &g.ChannelID, &g.MessageID, &g.Name, &g.Prize, &g.Description,
		&g.WinnerCount, &g.HostID, &g.Status, &g.Spec, &g.Requirements, &g.ImageURL, &g.Color, &g.WinnerIDs,
		&g.StartsAt, &g.EndsAt, &g.EndedAt, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt)
}

// Create inserts a new giveaway and returns it with its assigned id + timestamps.
func (r *GiveawayRepo) Create(ctx context.Context, g Giveaway) (Giveaway, error) {
	if len(g.Spec) == 0 {
		g.Spec = json.RawMessage("{}")
	}
	if len(g.Requirements) == 0 {
		g.Requirements = json.RawMessage("{}")
	}
	if g.Status == "" {
		g.Status = "running"
	}
	if g.WinnerCount <= 0 {
		g.WinnerCount = 1
	}
	if g.WinnerIDs == nil {
		g.WinnerIDs = []int64{}
	}
	var out Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx, `
		INSERT INTO giveaways
			(guild_id, channel_id, message_id, name, prize, description, winner_count, host_id,
			 status, spec, requirements, image_url, color, winner_ids, starts_at, ends_at, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING `+giveawayCols,
		g.GuildID, g.ChannelID, g.MessageID, g.Name, g.Prize, g.Description, g.WinnerCount, g.HostID,
		g.Status, g.Spec, g.Requirements, g.ImageURL, g.Color, g.WinnerIDs, g.StartsAt, g.EndsAt, g.CreatedBy), &out)
	if err != nil {
		return Giveaway{}, fmt.Errorf("create giveaway: %w", err)
	}
	return out, nil
}

// GiveawayPatch is the set of editable fields for Update. Only the fields whose
// pointer is non-nil are written, so the dashboard can patch a subset (e.g. just
// the message spec, or just the end time) without clobbering the rest.
type GiveawayPatch struct {
	Name         *string
	Prize        *string
	Description  *string
	WinnerCount  *int
	ChannelID    *int64
	ImageURL     *string
	Color        *string
	Spec         json.RawMessage
	Requirements json.RawMessage
	StartsAt     *time.Time
	EndsAt       *time.Time
}

// Update edits an editable giveaway (draft, scheduled or running) in place and
// returns the updated row. Ended/cancelled giveaways are immutable, so ok=false
// means the giveaway wasn't in an editable state (or doesn't exist).
func (r *GiveawayRepo) Update(ctx context.Context, guildID int64, id string, p GiveawayPatch) (Giveaway, bool, error) {
	sets := []string{"updated_at = now()"}
	args := []any{guildID, id}
	add := func(col string, val any) {
		args = append(args, val)
		sets = append(sets, fmt.Sprintf("%s = $%d", col, len(args)))
	}
	if p.Name != nil {
		add("name", *p.Name)
	}
	if p.Prize != nil {
		add("prize", *p.Prize)
	}
	if p.Description != nil {
		add("description", *p.Description)
	}
	if p.WinnerCount != nil {
		add("winner_count", *p.WinnerCount)
	}
	if p.ChannelID != nil {
		// A running giveaway's message is already posted in its channel; pin it so
		// a channel edit can't orphan the live message (drafts/scheduled are free
		// to move, since nothing is posted yet).
		args = append(args, *p.ChannelID)
		sets = append(sets, fmt.Sprintf("channel_id = CASE WHEN status = 'running' THEN channel_id ELSE $%d END", len(args)))
	}
	if p.ImageURL != nil {
		add("image_url", *p.ImageURL)
	}
	if p.Color != nil {
		add("color", *p.Color)
	}
	if len(p.Spec) > 0 {
		add("spec", p.Spec)
	}
	if len(p.Requirements) > 0 {
		add("requirements", p.Requirements)
	}
	if p.StartsAt != nil {
		add("starts_at", *p.StartsAt)
	}
	if p.EndsAt != nil {
		add("ends_at", *p.EndsAt)
	}
	var g Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx,
		`UPDATE giveaways SET `+strings.Join(sets, ", ")+`
		 WHERE guild_id = $1 AND id = $2 AND status IN ('draft','scheduled','running')
		 RETURNING `+giveawayCols, args...), &g)
	if errors.Is(err, pgx.ErrNoRows) {
		if _, gErr := r.Get(ctx, guildID, id); errors.Is(gErr, ErrGiveawayNotFound) {
			return Giveaway{}, false, ErrGiveawayNotFound
		}
		return Giveaway{}, false, nil
	}
	if err != nil {
		return Giveaway{}, false, fmt.Errorf("update giveaway: %w", err)
	}
	return g, true, nil
}

// Activate transitions a draft into a live state (running or scheduled), setting
// its start/end window, and returns the activated row. ok=false means the row
// wasn't a draft anymore (already started/deleted).
func (r *GiveawayRepo) Activate(ctx context.Context, id, status string, startsAt, endsAt time.Time) (Giveaway, bool, error) {
	var g Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx,
		`UPDATE giveaways SET status = $2, starts_at = $3, ends_at = $4, updated_at = now()
		 WHERE id = $1 AND status = 'draft'
		 RETURNING `+giveawayCols, id, status, startsAt, endsAt), &g)
	if errors.Is(err, pgx.ErrNoRows) {
		return Giveaway{}, false, nil
	}
	if err != nil {
		return Giveaway{}, false, fmt.Errorf("activate giveaway: %w", err)
	}
	return g, true, nil
}

// Get returns a giveaway by id within a guild.
func (r *GiveawayRepo) Get(ctx context.Context, guildID int64, id string) (Giveaway, error) {
	var g Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx,
		`SELECT `+giveawayCols+` FROM giveaways WHERE guild_id = $1 AND id = $2`, guildID, id), &g)
	if errors.Is(err, pgx.ErrNoRows) {
		return Giveaway{}, ErrGiveawayNotFound
	}
	if err != nil {
		return Giveaway{}, fmt.Errorf("get giveaway: %w", err)
	}
	return g, nil
}

// GetByID resolves a giveaway by its id alone, without a guild scope. Used for
// component clicks that arrive outside the guild (a button on a DM'd entry
// reply): the custom_id embedding the id was authored by the bot on its own
// message, so the id is trusted; ids are ULIDs, globally unique.
func (r *GiveawayRepo) GetByID(ctx context.Context, id string) (Giveaway, error) {
	var g Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx,
		`SELECT `+giveawayCols+` FROM giveaways WHERE id = $1`, id), &g)
	if errors.Is(err, pgx.ErrNoRows) {
		return Giveaway{}, ErrGiveawayNotFound
	}
	if err != nil {
		return Giveaway{}, fmt.Errorf("get giveaway by id: %w", err)
	}
	return g, nil
}

// GetByMessage resolves the giveaway a posted message belongs to (Enter clicks).
func (r *GiveawayRepo) GetByMessage(ctx context.Context, messageID int64) (Giveaway, error) {
	var g Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx,
		`SELECT `+giveawayCols+` FROM giveaways WHERE message_id = $1`, messageID), &g)
	if errors.Is(err, pgx.ErrNoRows) {
		return Giveaway{}, ErrGiveawayNotFound
	}
	if err != nil {
		return Giveaway{}, fmt.Errorf("get giveaway by message: %w", err)
	}
	return g, nil
}

// SetMessageID records the posted message id for a freshly created giveaway.
func (r *GiveawayRepo) SetMessageID(ctx context.Context, id string, messageID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE giveaways SET message_id = $2, updated_at = now() WHERE id = $1`, id, messageID)
	if err != nil {
		return fmt.Errorf("set giveaway message: %w", err)
	}
	return nil
}

// ListByGuild returns a guild's giveaways, newest first. status "" returns all;
// "active" is sugar for scheduled+running.
func (r *GiveawayRepo) ListByGuild(ctx context.Context, guildID int64, status string, limit int) ([]Giveaway, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	q := `SELECT ` + giveawayCols + ` FROM giveaways WHERE guild_id = $1`
	args := []any{guildID}
	switch status {
	case "", "all":
		// no filter
	case "active":
		q += ` AND status IN ('scheduled','running')`
	default:
		q += ` AND status = $2`
		args = append(args, status)
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT %d`, limit)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list giveaways: %w", err)
	}
	defer rows.Close()
	return scanGiveaways(rows)
}

// ListDue returns running giveaways whose end time has passed (the sweeper's
// work list).
func (r *GiveawayRepo) ListDue(ctx context.Context, now time.Time, limit int) ([]Giveaway, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+giveawayCols+` FROM giveaways
		 WHERE status = 'running' AND ends_at <= $1
		 ORDER BY ends_at ASC LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list due giveaways: %w", err)
	}
	defer rows.Close()
	return scanGiveaways(rows)
}

// ListScheduledDue returns scheduled giveaways whose start time has arrived.
func (r *GiveawayRepo) ListScheduledDue(ctx context.Context, now time.Time, limit int) ([]Giveaway, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+giveawayCols+` FROM giveaways
		 WHERE status = 'scheduled' AND starts_at <= $1
		 ORDER BY starts_at ASC LIMIT $2`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list scheduled giveaways: %w", err)
	}
	defer rows.Close()
	return scanGiveaways(rows)
}

// ClaimEnd atomically transitions a running giveaway to 'ended' AND records the
// drawn winners in one UPDATE, returning the claimed row. ok=false means it
// wasn't running (already ended/cancelled or claimed by a concurrent sweep) —
// the caller must not draw/announce it. Winners are drawn BEFORE this call and
// persisted here in the same statement, so a crash between the claim and the
// announcement still leaves the correct winners recorded (only the best-effort
// message edit / announcement is lost), and a restart can never end + announce a
// giveaway twice.
func (r *GiveawayRepo) ClaimEnd(ctx context.Context, id string, winnerIDs []int64) (Giveaway, bool, error) {
	if winnerIDs == nil {
		winnerIDs = []int64{}
	}
	var g Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx,
		`UPDATE giveaways SET status = 'ended', ended_at = now(), winner_ids = $2, updated_at = now()
		 WHERE id = $1 AND status = 'running'
		 RETURNING `+giveawayCols, id, winnerIDs), &g)
	if errors.Is(err, pgx.ErrNoRows) {
		return Giveaway{}, false, nil
	}
	if err != nil {
		return Giveaway{}, false, fmt.Errorf("claim giveaway end: %w", err)
	}
	return g, true, nil
}

// ClaimScheduled atomically transitions a scheduled giveaway to 'running',
// returning the claimed row. ok=false means it wasn't scheduled anymore. The
// claim happens BEFORE the message is posted, so a slow/failed post can never
// re-post the giveaway (the next sweep no longer sees it as scheduled).
func (r *GiveawayRepo) ClaimScheduled(ctx context.Context, id string) (Giveaway, bool, error) {
	var g Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx,
		`UPDATE giveaways SET status = 'running', updated_at = now()
		 WHERE id = $1 AND status = 'scheduled'
		 RETURNING `+giveawayCols, id), &g)
	if errors.Is(err, pgx.ErrNoRows) {
		return Giveaway{}, false, nil
	}
	if err != nil {
		return Giveaway{}, false, fmt.Errorf("claim scheduled giveaway: %w", err)
	}
	return g, true, nil
}

// Cancel marks a scheduled/running giveaway cancelled (no draw). ok=false means
// it was already ended/cancelled.
func (r *GiveawayRepo) Cancel(ctx context.Context, guildID int64, id string) (Giveaway, bool, error) {
	var g Giveaway
	err := scanGiveaway(r.pool.QueryRow(ctx,
		`UPDATE giveaways SET status = 'cancelled', ended_at = now(), updated_at = now()
		 WHERE guild_id = $1 AND id = $2 AND status IN ('scheduled','running')
		 RETURNING `+giveawayCols, guildID, id), &g)
	if errors.Is(err, pgx.ErrNoRows) {
		// Distinguish "no such giveaway" from "not cancellable".
		if _, gErr := r.Get(ctx, guildID, id); errors.Is(gErr, ErrGiveawayNotFound) {
			return Giveaway{}, false, ErrGiveawayNotFound
		}
		return Giveaway{}, false, nil
	}
	if err != nil {
		return Giveaway{}, false, fmt.Errorf("cancel giveaway: %w", err)
	}
	return g, true, nil
}

// SetWinners records the drawn winner ids (also used by reroll).
func (r *GiveawayRepo) SetWinners(ctx context.Context, id string, winnerIDs []int64) error {
	if winnerIDs == nil {
		winnerIDs = []int64{}
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE giveaways SET winner_ids = $2, updated_at = now() WHERE id = $1`, id, winnerIDs)
	if err != nil {
		return fmt.Errorf("set giveaway winners: %w", err)
	}
	return nil
}

// Delete removes a giveaway (and its entries, via cascade).
func (r *GiveawayRepo) Delete(ctx context.Context, guildID int64, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM giveaways WHERE guild_id = $1 AND id = $2`, guildID, id)
	if err != nil {
		return fmt.Errorf("delete giveaway: %w", err)
	}
	return nil
}

// ── Entries ─────────────────────────────────────────────────────────────────

// AddEntry records (or updates the weight of) a member's entry. Returns whether
// the entry is new (false = the member had already entered, weight refreshed).
func (r *GiveawayRepo) AddEntry(ctx context.Context, giveawayID string, userID int64, entries int) (bool, error) {
	if entries < 1 {
		entries = 1
	}
	var inserted bool
	err := r.pool.QueryRow(ctx, `
		INSERT INTO giveaway_entries (giveaway_id, user_id, entries)
		VALUES ($1, $2, $3)
		ON CONFLICT (giveaway_id, user_id) DO UPDATE SET entries = EXCLUDED.entries
		RETURNING (xmax = 0) AS inserted`, giveawayID, userID, entries).Scan(&inserted)
	if err != nil {
		return false, fmt.Errorf("add giveaway entry: %w", err)
	}
	return inserted, nil
}

// RemoveEntry deletes a member's entry (leaving the giveaway). Returns whether a
// row was removed.
func (r *GiveawayRepo) RemoveEntry(ctx context.Context, giveawayID string, userID int64) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM giveaway_entries WHERE giveaway_id = $1 AND user_id = $2`, giveawayID, userID)
	if err != nil {
		return false, fmt.Errorf("remove giveaway entry: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// HasEntry reports whether a member has entered.
func (r *GiveawayRepo) HasEntry(ctx context.Context, giveawayID string, userID int64) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM giveaway_entries WHERE giveaway_id = $1 AND user_id = $2)`,
		giveawayID, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("has giveaway entry: %w", err)
	}
	return exists, nil
}

// EntryCount returns the number of distinct entrants (not the ticket sum).
func (r *GiveawayRepo) EntryCount(ctx context.Context, giveawayID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM giveaway_entries WHERE giveaway_id = $1`, giveawayID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count giveaway entries: %w", err)
	}
	return n, nil
}

// EntryCounts returns the distinct-entrant count for each of the given giveaway
// ids (missing ids are absent from the map). Used by the dashboard list.
func (r *GiveawayRepo) EntryCounts(ctx context.Context, ids []string) (map[string]int, error) {
	out := make(map[string]int, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT giveaway_id, COUNT(*) FROM giveaway_entries WHERE giveaway_id = ANY($1) GROUP BY giveaway_id`, ids)
	if err != nil {
		return nil, fmt.Errorf("giveaway entry counts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ListEntries returns every entry for the draw (user id + weight).
func (r *GiveawayRepo) ListEntries(ctx context.Context, giveawayID string) ([]GiveawayEntry, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT giveaway_id, user_id, entries, entered_at FROM giveaway_entries WHERE giveaway_id = $1`,
		giveawayID)
	if err != nil {
		return nil, fmt.Errorf("list giveaway entries: %w", err)
	}
	defer rows.Close()
	var out []GiveawayEntry
	for rows.Next() {
		var e GiveawayEntry
		if err := rows.Scan(&e.GiveawayID, &e.UserID, &e.Entries, &e.EnteredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func scanGiveaways(rows pgx.Rows) ([]Giveaway, error) {
	var out []Giveaway
	for rows.Next() {
		var g Giveaway
		if err := scanGiveaway(rows, &g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
