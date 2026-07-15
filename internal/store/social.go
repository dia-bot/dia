package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SocialSubscription is one followed social account for a guild.
type SocialSubscription struct {
	ID           int64
	GuildID      int64
	Provider     string // twitch|youtube|kick|bluesky|rss
	AccountID    string // canonical upstream id (or feed URL for rss)
	AccountName  string
	AccountURL   string
	ChannelID    int64 // Discord channel announcements post to
	PingRoleID   int64 // 0 = no ping
	Template     string
	Embed        bool
	Enabled      bool
	Live         bool
	HookStatus   string
	LastError    string
	ETag         string
	LastModified string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// SocialRepo manages social_subscriptions and their seen-item dedupe ledger.
type SocialRepo struct{ pool *pgxpool.Pool }

const socialCols = `id, guild_id, provider, account_id, account_name, account_url,
	channel_id, ping_role_id, template, embed, enabled, live, hook_status,
	last_error, etag, last_modified, created_at, updated_at`

func scanSocial(row pgx.Row) (SocialSubscription, error) {
	var s SocialSubscription
	err := row.Scan(&s.ID, &s.GuildID, &s.Provider, &s.AccountID, &s.AccountName, &s.AccountURL,
		&s.ChannelID, &s.PingRoleID, &s.Template, &s.Embed, &s.Enabled, &s.Live, &s.HookStatus,
		&s.LastError, &s.ETag, &s.LastModified, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *SocialRepo) collect(ctx context.Context, query string, args ...any) ([]SocialSubscription, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SocialSubscription
	for rows.Next() {
		s, err := scanSocial(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// ListByGuild returns a guild's subscriptions, newest last.
func (r *SocialRepo) ListByGuild(ctx context.Context, guildID int64) ([]SocialSubscription, error) {
	return r.collect(ctx, `SELECT `+socialCols+` FROM social_subscriptions
		WHERE guild_id = $1 ORDER BY id`, guildID)
}

// Get returns one subscription scoped to a guild (found=false when absent).
func (r *SocialRepo) Get(ctx context.Context, guildID, id int64) (SocialSubscription, bool, error) {
	s, err := scanSocial(r.pool.QueryRow(ctx, `SELECT `+socialCols+` FROM social_subscriptions
		WHERE guild_id = $1 AND id = $2`, guildID, id))
	if err == pgx.ErrNoRows {
		return SocialSubscription{}, false, nil
	}
	return s, err == nil, err
}

// GetByID returns one subscription by id alone (worker announce path).
func (r *SocialRepo) GetByID(ctx context.Context, id int64) (SocialSubscription, bool, error) {
	s, err := scanSocial(r.pool.QueryRow(ctx, `SELECT `+socialCols+` FROM social_subscriptions
		WHERE id = $1`, id))
	if err == pgx.ErrNoRows {
		return SocialSubscription{}, false, nil
	}
	return s, err == nil, err
}

// ListEnabledByAccount returns every enabled subscription following one
// upstream account — the webhook fan-out (one event per matching guild).
func (r *SocialRepo) ListEnabledByAccount(ctx context.Context, provider, accountID string) ([]SocialSubscription, error) {
	return r.collect(ctx, `SELECT `+socialCols+` FROM social_subscriptions
		WHERE provider = $1 AND account_id = $2 AND enabled ORDER BY id`, provider, accountID)
}

// ListEnabledByProvider returns every enabled subscription for a provider
// (the poll set for rss/bluesky, the reconcile set for push providers).
func (r *SocialRepo) ListEnabledByProvider(ctx context.Context, provider string) ([]SocialSubscription, error) {
	return r.collect(ctx, `SELECT `+socialCols+` FROM social_subscriptions
		WHERE provider = $1 AND enabled ORDER BY id`, provider)
}

// CountByGuild returns how many subscriptions a guild has (plan limits).
func (r *SocialRepo) CountByGuild(ctx context.Context, guildID int64) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM social_subscriptions WHERE guild_id = $1`, guildID).Scan(&n)
	return n, err
}

// CountByAccount returns how many subscriptions (any guild) follow an upstream
// account — 0 after a delete means the upstream webhook can be torn down.
func (r *SocialRepo) CountByAccount(ctx context.Context, provider, accountID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM social_subscriptions WHERE provider = $1 AND account_id = $2`,
		provider, accountID).Scan(&n)
	return n, err
}

// Create inserts a subscription and returns it with its id.
func (r *SocialRepo) Create(ctx context.Context, s SocialSubscription) (SocialSubscription, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO social_subscriptions
			(guild_id, provider, account_id, account_name, account_url,
			 channel_id, ping_role_id, template, embed, enabled, hook_status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+socialCols,
		s.GuildID, s.Provider, s.AccountID, s.AccountName, s.AccountURL,
		s.ChannelID, s.PingRoleID, s.Template, s.Embed, s.Enabled, s.HookStatus)
	out, err := scanSocial(row)
	if err != nil {
		return SocialSubscription{}, fmt.Errorf("create social subscription: %w", err)
	}
	return out, nil
}

// Update saves the user-editable fields of a guild's subscription.
func (r *SocialRepo) Update(ctx context.Context, s SocialSubscription) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE social_subscriptions
		SET channel_id = $3, ping_role_id = $4, template = $5, embed = $6,
		    enabled = $7, updated_at = now()
		WHERE guild_id = $1 AND id = $2`,
		s.GuildID, s.ID, s.ChannelID, s.PingRoleID, s.Template, s.Embed, s.Enabled)
	return err
}

// Delete removes a guild's subscription.
func (r *SocialRepo) Delete(ctx context.Context, guildID, id int64) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM social_subscriptions WHERE guild_id = $1 AND id = $2`, guildID, id)
	return err
}

// ClaimLive flips a subscription to the given live state, reporting whether
// this call performed the transition. Duplicate webhook deliveries (or an
// online notification racing a reconnect) claim false and skip announcing.
func (r *SocialRepo) ClaimLive(ctx context.Context, id int64, live bool) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE social_subscriptions SET live = $2, updated_at = now()
		WHERE id = $1 AND live = NOT $2`, id, live)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// SetHookStatus records the upstream webhook state for every subscription of
// one account ("active", "pending", "error" + message).
func (r *SocialRepo) SetHookStatus(ctx context.Context, provider, accountID, status, lastError string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE social_subscriptions SET hook_status = $3, last_error = $4, updated_at = now()
		WHERE provider = $1 AND account_id = $2`, provider, accountID, status, lastError)
	return err
}

// SetPollState stores RSS conditional-GET validators after a poll.
func (r *SocialRepo) SetPollState(ctx context.Context, id int64, etag, lastModified string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE social_subscriptions SET etag = $2, last_modified = $3, updated_at = now()
		WHERE id = $1`, id, etag, lastModified)
	return err
}

// MarkSeen records an upstream item id for a subscription, reporting whether
// it was newly seen (true = announce it, false = duplicate).
func (r *SocialRepo) MarkSeen(ctx context.Context, subscriptionID int64, itemID string) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		INSERT INTO social_seen_items (subscription_id, item_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, subscriptionID, itemID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// PruneSeen drops seen-item rows older than the retention window (feeds only
// surface recent items, so an old id can't come back as "new").
func (r *SocialRepo) PruneSeen(ctx context.Context, olderThan time.Duration) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM social_seen_items WHERE seen_at < now() - $1::interval`,
		fmt.Sprintf("%d seconds", int(olderThan.Seconds())))
	return err
}
