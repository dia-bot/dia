// Package migrations embeds the versioned SQL migrations so they can be applied
// programmatically at startup (no external goose CLI required).
package migrations

import "embed"

// FS holds the *.sql migration files.
//
//go:embed *.sql
var FS embed.FS
