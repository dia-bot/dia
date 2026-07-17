package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CustomBot is a customer's own Discord application run on our infrastructure
// for one guild. TokenEnc / SecretEnc hold AES-GCM ciphertext (nonce||sealed);
// the store never sees plaintext credentials.
type CustomBot struct {
	GuildID        int64
	ApplicationID  int64
	BotUserID      int64
	Username       string
	Avatar         string
	TokenEnc       []byte
	SecretEnc      []byte
	Intents        int64
	PresenceStatus string
	ActivityType   int
	ActivityText   string
	ActivityURL    string
	Enabled        bool
	State          string
	LastError      string
	CommandsSynced bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CustomBotRepo manages the custom_bots table.
type CustomBotRepo struct{ pool *pgxpool.Pool }

const customBotCols = `guild_id, application_id, bot_user_id, username, avatar,
	token_enc, secret_enc, intents, presence_status, activity_type, activity_text,
	activity_url, enabled, state, last_error, commands_synced, created_at, updated_at`

func scanCustomBot(row pgx.Row) (CustomBot, error) {
	var b CustomBot
	err := row.Scan(&b.GuildID, &b.ApplicationID, &b.BotUserID, &b.Username, &b.Avatar,
		&b.TokenEnc, &b.SecretEnc, &b.Intents, &b.PresenceStatus, &b.ActivityType, &b.ActivityText,
		&b.ActivityURL, &b.Enabled, &b.State, &b.LastError, &b.CommandsSynced, &b.CreatedAt, &b.UpdatedAt)
	return b, err
}

// Get returns the custom bot for a guild, or (false) when none is configured.
func (r *CustomBotRepo) Get(ctx context.Context, guildID int64) (CustomBot, bool, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+customBotCols+` FROM custom_bots WHERE guild_id=$1`, guildID)
	b, err := scanCustomBot(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomBot{}, false, nil
	}
	return b, err == nil, err
}

// GetByApp returns any enabled row for an application id (rows for the same app
// share a token), or (false) when none is enabled. Used by the REST client
// registry to build a client for a bot identified only by its app id.
func (r *CustomBotRepo) GetByApp(ctx context.Context, appID int64) (CustomBot, bool, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+customBotCols+` FROM custom_bots
		WHERE application_id=$1 AND enabled ORDER BY updated_at DESC LIMIT 1`, appID)
	b, err := scanCustomBot(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomBot{}, false, nil
	}
	return b, err == nil, err
}

// Upsert creates or replaces the identity + credentials for a guild's custom
// bot, preserving runtime fields (state, last_error, commands_synced) and the
// enabled flag on update. Presence fields are set here too (the dashboard edits
// them together with the identity).
func (r *CustomBotRepo) Upsert(ctx context.Context, b CustomBot) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO custom_bots (guild_id, application_id, bot_user_id, username, avatar,
			token_enc, secret_enc, intents, presence_status, activity_type, activity_text, activity_url, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, now())
		ON CONFLICT (guild_id) DO UPDATE SET
			application_id=EXCLUDED.application_id,
			bot_user_id=EXCLUDED.bot_user_id,
			username=EXCLUDED.username,
			avatar=EXCLUDED.avatar,
			token_enc=EXCLUDED.token_enc,
			secret_enc=COALESCE(EXCLUDED.secret_enc, custom_bots.secret_enc),
			intents=EXCLUDED.intents,
			presence_status=EXCLUDED.presence_status,
			activity_type=EXCLUDED.activity_type,
			activity_text=EXCLUDED.activity_text,
			activity_url=EXCLUDED.activity_url,
			updated_at=now()`,
		b.GuildID, b.ApplicationID, b.BotUserID, b.Username, b.Avatar,
		b.TokenEnc, b.SecretEnc, b.Intents, b.PresenceStatus, b.ActivityType, b.ActivityText, b.ActivityURL)
	return err
}

// SetPresence updates only the presence/activity fields.
func (r *CustomBotRepo) SetPresence(ctx context.Context, guildID int64, status string, actType int, text, url string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE custom_bots SET presence_status=$2, activity_type=$3, activity_text=$4, activity_url=$5, updated_at=now()
		WHERE guild_id=$1`, guildID, status, actType, text, url)
	return err
}

// SetEnabled flips the desired running state.
func (r *CustomBotRepo) SetEnabled(ctx context.Context, guildID int64, enabled bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE custom_bots SET enabled=$2, updated_at=now() WHERE guild_id=$1`, guildID, enabled)
	return err
}

// SetState records the connection state / error reported by the gateway.
func (r *CustomBotRepo) SetState(ctx context.Context, guildID int64, state, lastErr string) error {
	_, err := r.pool.Exec(ctx, `UPDATE custom_bots SET state=$2, last_error=$3, updated_at=now() WHERE guild_id=$1`,
		guildID, state, lastErr)
	return err
}

// SetStateByApp records state for every guild backed by an application (the
// gateway reports per-connection, keyed by application id).
func (r *CustomBotRepo) SetStateByApp(ctx context.Context, appID int64, state, lastErr string) error {
	_, err := r.pool.Exec(ctx, `UPDATE custom_bots SET state=$2, last_error=$3, updated_at=now() WHERE application_id=$1`,
		appID, state, lastErr)
	return err
}

// SetCommandsSynced marks whether commands are registered under the app.
func (r *CustomBotRepo) SetCommandsSynced(ctx context.Context, guildID int64, synced bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE custom_bots SET commands_synced=$2, updated_at=now() WHERE guild_id=$1`, guildID, synced)
	return err
}

// SetCommandsSyncedByApp marks command-sync state for every guild backed by an
// application (commands are registered once per application id).
func (r *CustomBotRepo) SetCommandsSyncedByApp(ctx context.Context, appID int64, synced bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE custom_bots SET commands_synced=$2, updated_at=now() WHERE application_id=$1`, appID, synced)
	return err
}

// Delete removes a guild's custom bot entirely.
func (r *CustomBotRepo) Delete(ctx context.Context, guildID int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM custom_bots WHERE guild_id=$1`, guildID)
	return err
}

// ListEnabled returns every enabled custom bot (the desired running set), used
// by the control loop and by boot-time reconciliation.
func (r *CustomBotRepo) ListEnabled(ctx context.Context) ([]CustomBot, error) {
	return r.collect(ctx, `SELECT `+customBotCols+` FROM custom_bots WHERE enabled ORDER BY application_id`)
}

// GuildsForApp returns the guild ids that have a given application enabled.
func (r *CustomBotRepo) GuildsForApp(ctx context.Context, appID int64) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `SELECT guild_id FROM custom_bots WHERE application_id=$1 AND enabled`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var g int64
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *CustomBotRepo) collect(ctx context.Context, query string, args ...any) ([]CustomBot, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CustomBot
	for rows.Next() {
		b, err := scanCustomBot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
