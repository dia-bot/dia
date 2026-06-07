package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GuildUpload is one stored asset (a Card Studio image or a premium custom font).
type GuildUpload struct {
	ID        int64
	GuildID   int64
	Kind      string // "image" | "font"
	Family    string // font family (kind == "font")
	ObjectKey string // storage key, for deletion
	URL       string
	Bytes     int64
	CreatedAt time.Time
}

// GuildUploadRepo manages guild_uploads (asset pointers + sizes).
type GuildUploadRepo struct{ pool *pgxpool.Pool }

// List returns a guild's assets, newest first.
func (r *GuildUploadRepo) List(ctx context.Context, guildID int64) ([]GuildUpload, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, kind, family, object_key, url, bytes, created_at
		FROM guild_uploads WHERE guild_id = $1 ORDER BY created_at DESC`, guildID)
	if err != nil {
		return nil, fmt.Errorf("list uploads: %w", err)
	}
	defer rows.Close()
	var out []GuildUpload
	for rows.Next() {
		u := GuildUpload{GuildID: guildID}
		if err := rows.Scan(&u.ID, &u.Kind, &u.Family, &u.ObjectKey, &u.URL, &u.Bytes, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// FontMap returns family → URL for a guild's custom fonts (render-time lookup).
func (r *GuildUploadRepo) FontMap(ctx context.Context, guildID int64) (map[string]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT family, url FROM guild_uploads WHERE guild_id = $1 AND kind = 'font'`, guildID)
	if err != nil {
		return nil, fmt.Errorf("font map: %w", err)
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var fam, url string
		if err := rows.Scan(&fam, &url); err != nil {
			return nil, err
		}
		m[fam] = url
	}
	return m, rows.Err()
}

// Usage returns the total bytes a guild is using.
func (r *GuildUploadRepo) Usage(ctx context.Context, guildID int64) (int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(bytes), 0) FROM guild_uploads WHERE guild_id = $1`, guildID).Scan(&total)
	if err != nil {
		return 0, fmt.Errorf("usage: %w", err)
	}
	return total, nil
}

// InsertImage records an uploaded image and returns its row id.
func (r *GuildUploadRepo) InsertImage(ctx context.Context, guildID int64, key, url string, bytes int64) (int64, error) {
	var id int64
	err := r.pool.QueryRow(ctx, `
		INSERT INTO guild_uploads (guild_id, kind, object_key, url, bytes)
		VALUES ($1, 'image', $2, $3, $4) RETURNING id`, guildID, key, url, bytes).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert image: %w", err)
	}
	return id, nil
}

// UpsertFont replaces a guild's font for a family (re-uploading a family swaps
// the file). Returns the previous object key, if any, so the caller can delete it.
func (r *GuildUploadRepo) UpsertFont(ctx context.Context, u GuildUpload) (oldKey string, err error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	err = tx.QueryRow(ctx,
		`SELECT object_key FROM guild_uploads WHERE guild_id = $1 AND kind = 'font' AND family = $2`,
		u.GuildID, u.Family).Scan(&oldKey)
	if err != nil && err != pgx.ErrNoRows {
		return "", err
	}
	if _, err = tx.Exec(ctx,
		`DELETE FROM guild_uploads WHERE guild_id = $1 AND kind = 'font' AND family = $2`,
		u.GuildID, u.Family); err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO guild_uploads (guild_id, kind, family, object_key, url, bytes)
		VALUES ($1, 'font', $2, $3, $4, $5)`,
		u.GuildID, u.Family, u.ObjectKey, u.URL, u.Bytes); err != nil {
		return "", err
	}
	return oldKey, tx.Commit(ctx)
}

// Get fetches one asset scoped to a guild.
func (r *GuildUploadRepo) Get(ctx context.Context, guildID, id int64) (GuildUpload, bool, error) {
	u := GuildUpload{GuildID: guildID}
	err := r.pool.QueryRow(ctx, `
		SELECT id, kind, family, object_key, url, bytes, created_at
		FROM guild_uploads WHERE guild_id = $1 AND id = $2`, guildID, id).
		Scan(&u.ID, &u.Kind, &u.Family, &u.ObjectKey, &u.URL, &u.Bytes, &u.CreatedAt)
	if err == pgx.ErrNoRows {
		return GuildUpload{}, false, nil
	}
	if err != nil {
		return GuildUpload{}, false, err
	}
	return u, true, nil
}

// Delete removes an asset row scoped to a guild.
func (r *GuildUploadRepo) Delete(ctx context.Context, guildID, id int64) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM guild_uploads WHERE guild_id = $1 AND id = $2`, guildID, id)
	return err
}

// DeleteFont removes a font by family and returns its object key for cleanup.
func (r *GuildUploadRepo) DeleteFont(ctx context.Context, guildID int64, family string) (objectKey string, err error) {
	err = r.pool.QueryRow(ctx, `
		DELETE FROM guild_uploads WHERE guild_id = $1 AND kind = 'font' AND family = $2
		RETURNING object_key`, guildID, family).Scan(&objectKey)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return objectKey, err
}
