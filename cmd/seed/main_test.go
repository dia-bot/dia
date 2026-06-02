package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dia-bot/dia/internal/config"
	"github.com/dia-bot/dia/internal/logging"
	"github.com/dia-bot/dia/internal/store"
)

// TestSeedIdempotent runs the seeder twice against a throwaway Postgres and
// asserts that the second run changes no row counts. It is skipped unless
// SEED_TEST_DB points at a database (so `go test ./cmd/...` stays green in CI
// without infra). Run it locally with:
//
//	make infra
//	SEED_TEST_DB="postgres://dia:dia@localhost:5432/dia?sslmode=disable" go test ./cmd/seed/
func TestSeedIdempotent(t *testing.T) {
	dsn := os.Getenv("SEED_TEST_DB")
	if dsn == "" {
		t.Skip("set SEED_TEST_DB to a Postgres DSN to run the seed integration test")
	}
	// Deterministic primary guild so the test never depends on the caller's env.
	t.Setenv("SEED_GUILD_ID", "1000000000000000099")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	log := logging.New("error", "development")
	st, err := store.Open(ctx, config.PostgresConfig{URL: dsn, MaxConns: 4}, log)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx, log); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if err := run(ctx, st); err != nil {
		t.Fatalf("first run: %v", err)
	}
	first := snapshotCounts(ctx, t, st)

	if err := run(ctx, st); err != nil {
		t.Fatalf("second run: %v", err)
	}
	second := snapshotCounts(ctx, t, st)

	for table, n := range first {
		if second[table] != n {
			t.Errorf("table %q not idempotent: first=%d second=%d", table, n, second[table])
		}
	}

	if first["level_users"] < len(levelFixtures) {
		t.Errorf("expected at least %d level_users, got %d", len(levelFixtures), first["level_users"])
	}
}

func snapshotCounts(ctx context.Context, t *testing.T, st *store.Store) map[string]int {
	t.Helper()
	tables := []string{
		"guilds", "guild_feature_configs", "level_users", "level_rewards",
		"mod_cases", "reaction_role_menus", "custom_commands", "dashboard_audit_log",
	}
	out := make(map[string]int, len(tables))
	for _, tbl := range tables {
		var n int
		if err := st.Pool.QueryRow(ctx, "SELECT count(*) FROM "+tbl).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		out[tbl] = n
	}
	return out
}
