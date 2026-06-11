package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ── Reaction role menus ──────────────────────────────────────

// ReactionRoleRepo manages reaction_role_menus.
type ReactionRoleRepo struct{ pool *pgxpool.Pool }

// Create inserts a menu and returns it with the assigned ID.
func (r *ReactionRoleRepo) Create(ctx context.Context, m ReactionRoleMenu) (ReactionRoleMenu, error) {
	if len(m.Options) == 0 {
		m.Options = json.RawMessage("[]")
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO reaction_role_menus (guild_id, channel_id, message_id, title, mode, options)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at`,
		m.GuildID, m.ChannelID, m.MessageID, m.Title, m.Mode, []byte(m.Options)).
		Scan(&m.ID, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return ReactionRoleMenu{}, fmt.Errorf("create reaction role menu: %w", err)
	}
	return m, nil
}

// Update saves title/mode/options for a menu scoped to a guild.
func (r *ReactionRoleRepo) Update(ctx context.Context, m ReactionRoleMenu) error {
	if len(m.Options) == 0 {
		m.Options = json.RawMessage("[]")
	}
	_, err := r.pool.Exec(ctx, `
		UPDATE reaction_role_menus
		SET title = $3, mode = $4, options = $5, updated_at = now()
		WHERE id = $1 AND guild_id = $2`,
		m.ID, m.GuildID, m.Title, m.Mode, []byte(m.Options))
	return err
}

// SetMessage records where a menu was posted (channel + message).
func (r *ReactionRoleRepo) SetMessage(ctx context.Context, id, channelID, messageID int64) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE reaction_role_menus SET channel_id = $2, message_id = $3, updated_at = now() WHERE id = $1`,
		id, channelID, messageID)
	return err
}

// Delete removes a menu scoped to a guild.
func (r *ReactionRoleRepo) Delete(ctx context.Context, guildID, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM reaction_role_menus WHERE id = $1 AND guild_id = $2`, id, guildID)
	return err
}

// Get returns a menu by ID, or ErrNotFound.
func (r *ReactionRoleRepo) Get(ctx context.Context, id int64) (ReactionRoleMenu, error) {
	var m ReactionRoleMenu
	err := r.pool.QueryRow(ctx, `
		SELECT id, guild_id, channel_id, message_id, title, mode, options, created_at, updated_at
		FROM reaction_role_menus WHERE id = $1`, id).
		Scan(&m.ID, &m.GuildID, &m.ChannelID, &m.MessageID, &m.Title, &m.Mode, &m.Options, &m.CreatedAt, &m.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

// List returns all menus for a guild.
func (r *ReactionRoleRepo) List(ctx context.Context, guildID int64) ([]ReactionRoleMenu, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, channel_id, message_id, title, mode, options, created_at, updated_at
		FROM reaction_role_menus WHERE guild_id = $1 ORDER BY id`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ReactionRoleMenu
	for rows.Next() {
		m := ReactionRoleMenu{GuildID: guildID}
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.MessageID, &m.Title, &m.Mode,
			&m.Options, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ── Custom commands (v2: programmable Step[] tree) ───────────

// CustomCommandRepo manages custom_commands.
type CustomCommandRepo struct{ pool *pgxpool.Pool }

// Upsert inserts or updates a command. When c.ID==0 a new row is created and
// returned with the assigned id; otherwise the row is updated by (guild, id).
// Name uniqueness within a guild is enforced by the index.
func (r *CustomCommandRepo) Upsert(ctx context.Context, c CustomCommand) (CustomCommand, error) {
	if len(c.Definition) == 0 {
		c.Definition = json.RawMessage("{}")
	}
	if c.Status == "" {
		c.Status = "draft"
	}
	if c.Version <= 0 {
		c.Version = 1
	}
	if c.ID == 0 {
		err := r.pool.QueryRow(ctx, `
			INSERT INTO custom_commands (guild_id, name, description, enabled, status, version, requires_defer, definition, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, created_at, updated_at`,
			c.GuildID, c.Name, c.Description, c.Enabled, c.Status, c.Version, c.RequiresDefer, []byte(c.Definition), c.CreatedBy).
			Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
		if err != nil {
			return CustomCommand{}, fmt.Errorf("insert custom command: %w", err)
		}
		return c, nil
	}
	err := r.pool.QueryRow(ctx, `
		UPDATE custom_commands SET
			name = $3, description = $4, enabled = $5, status = $6, version = $7,
			requires_defer = $8, definition = $9, updated_at = now()
		WHERE id = $1 AND guild_id = $2
		RETURNING created_at, updated_at`,
		c.ID, c.GuildID, c.Name, c.Description, c.Enabled, c.Status, c.Version,
		c.RequiresDefer, []byte(c.Definition)).
		Scan(&c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return CustomCommand{}, fmt.Errorf("update custom command: %w", err)
	}
	return c, nil
}

// Get returns one command by id.
func (r *CustomCommandRepo) Get(ctx context.Context, guildID, id int64) (CustomCommand, error) {
	c := CustomCommand{GuildID: guildID}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, enabled, status, version, requires_defer, definition, created_by, created_at, updated_at
		FROM custom_commands WHERE id = $1 AND guild_id = $2`, id, guildID).
		Scan(&c.ID, &c.Name, &c.Description, &c.Enabled, &c.Status, &c.Version, &c.RequiresDefer,
			&c.Definition, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Delete removes a command scoped to a guild.
func (r *CustomCommandRepo) Delete(ctx context.Context, guildID, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM custom_commands WHERE id = $1 AND guild_id = $2`, id, guildID)
	return err
}

// List returns all custom commands for a guild, ordered by name.
func (r *CustomCommandRepo) List(ctx context.Context, guildID int64) ([]CustomCommand, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, description, enabled, status, version, requires_defer, definition, created_by, created_at, updated_at
		FROM custom_commands WHERE guild_id = $1 ORDER BY name`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CustomCommand
	for rows.Next() {
		c := CustomCommand{GuildID: guildID}
		if err := rows.Scan(&c.ID, &c.Name, &c.Description, &c.Enabled, &c.Status, &c.Version,
			&c.RequiresDefer, &c.Definition, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetByName returns a command by (guild, name), or ErrNotFound. Used by the
// interaction-time dispatcher.
func (r *CustomCommandRepo) GetByName(ctx context.Context, guildID int64, name string) (CustomCommand, error) {
	c := CustomCommand{GuildID: guildID}
	err := r.pool.QueryRow(ctx, `
		SELECT id, name, description, enabled, status, version, requires_defer, definition, created_by, created_at, updated_at
		FROM custom_commands WHERE guild_id = $1 AND name = $2`, guildID, name).
		Scan(&c.ID, &c.Name, &c.Description, &c.Enabled, &c.Status, &c.Version, &c.RequiresDefer,
			&c.Definition, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// PublishVersion writes an immutable snapshot of a command's Definition. The
// caller bumps custom_commands.version + status='published' in the same txn.
func (r *CustomCommandRepo) PublishVersion(ctx context.Context, v CustomCommandVersion) error {
	if len(v.Definition) == 0 {
		v.Definition = json.RawMessage("{}")
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO custom_command_versions (command_id, version, definition, published_by)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (command_id, version) DO UPDATE SET
			definition = EXCLUDED.definition,
			published_by = EXCLUDED.published_by`,
		v.CommandID, v.Version, []byte(v.Definition), v.PublishedBy)
	return err
}

// GetVersion returns a specific published snapshot.
func (r *CustomCommandRepo) GetVersion(ctx context.Context, commandID int64, version int) (CustomCommandVersion, error) {
	var v CustomCommandVersion
	err := r.pool.QueryRow(ctx, `
		SELECT command_id, version, definition, published_by, published_at
		FROM custom_command_versions WHERE command_id = $1 AND version = $2`,
		commandID, version).
		Scan(&v.CommandID, &v.Version, &v.Definition, &v.PublishedBy, &v.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

// ── Audit log ────────────────────────────────────────────────

// AuditRepo manages dashboard_audit_log.
type AuditRepo struct{ pool *pgxpool.Pool }

// Add records a dashboard action.
func (r *AuditRepo) Add(ctx context.Context, e AuditEntry) error {
	if len(e.Detail) == 0 {
		e.Detail = json.RawMessage("{}")
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO dashboard_audit_log (guild_id, user_id, action, detail) VALUES ($1, $2, $3, $4)`,
		e.GuildID, e.UserID, e.Action, []byte(e.Detail))
	return err
}

// List returns recent audit entries for a guild.
func (r *AuditRepo) List(ctx context.Context, guildID int64, limit int) ([]AuditEntry, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, user_id, action, detail, created_at
		FROM dashboard_audit_log WHERE guild_id = $1 ORDER BY created_at DESC LIMIT $2`, guildID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuditEntry
	for rows.Next() {
		e := AuditEntry{GuildID: guildID}
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.Detail, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
