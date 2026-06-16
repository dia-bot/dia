// Package store is Dia's data layer: a single pgx connection pool, programmatic
// goose migrations, and typed repositories for each feature. There are no
// global singletons — a *Store is constructed once and injected into the
// services that need it.
package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/dia-bot/dia/internal/config"
	"github.com/dia-bot/dia/migrations"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Store owns the pool and exposes the per-domain repositories.
type Store struct {
	Pool *pgxpool.Pool

	Guilds         *GuildRepo
	Features       *FeatureConfigRepo
	Levels         *LevelRepo
	Moderation     *ModRepo
	ReactionRoles  *ReactionRoleRepo
	CustomCommands *CustomCommandRepo
	CommandGroups  *CommandGroupRepo
	CommandRuns    *CommandRunRepo
	Automations    *AutomationRepo
	AutomationRuns *AutomationRunRepo
	FeatureKV      *FeatureKVRepo
	ImageTemplates *CommandImageTemplateRepo
	Audit          *AuditRepo
	Uploads        *GuildUploadRepo
	Subscriptions  *SubscriptionRepo
}

// Open creates the pool, verifies connectivity and wires the repositories.
func Open(ctx context.Context, cfg config.PostgresConfig, log *slog.Logger) (*Store, error) {
	pcfg, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if cfg.MaxConns > 0 {
		pcfg.MaxConns = cfg.MaxConns
	}
	pcfg.MaxConnLifetime = time.Hour
	pcfg.HealthCheckPeriod = time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, pcfg)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	s := &Store{Pool: pool}
	s.Guilds = &GuildRepo{pool: pool}
	s.Features = &FeatureConfigRepo{pool: pool}
	s.Levels = &LevelRepo{pool: pool}
	s.Moderation = &ModRepo{pool: pool}
	s.ReactionRoles = &ReactionRoleRepo{pool: pool}
	s.CustomCommands = &CustomCommandRepo{pool: pool}
	s.CommandGroups = &CommandGroupRepo{pool: pool}
	s.CommandRuns = &CommandRunRepo{pool: pool}
	s.Automations = &AutomationRepo{pool: pool}
	s.AutomationRuns = &AutomationRunRepo{pool: pool}
	s.FeatureKV = &FeatureKVRepo{pool: pool}
	s.ImageTemplates = &CommandImageTemplateRepo{pool: pool}
	s.Audit = &AuditRepo{pool: pool}
	s.Uploads = &GuildUploadRepo{pool: pool}
	s.Subscriptions = &SubscriptionRepo{pool: pool}

	log.Info("connected to postgres", "max_conns", pcfg.MaxConns)
	return s, nil
}

// Migrate applies all pending migrations using goose. It opens a transient
// database/sql handle from the pool (goose requires one) and takes a Postgres
// session-level advisory lock so only one instance migrates at a time — the
// worker and api both call this on startup, and without the lock concurrent
// CREATE TABLE statements race in the pg_type catalog.
func (s *Store) Migrate(ctx context.Context, log *slog.Logger) error {
	db := stdlib.OpenDBFromPool(s.Pool)
	defer db.Close()

	// Pin a single connection for the advisory lock — pg_advisory_lock is
	// session-scoped, so the unlock has to land on the same connection that
	// took it.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration conn: %w", err)
	}
	defer conn.Close()

	// Arbitrary but stable key (high+low halves of bigint("DIA-MIGR")).
	const migrationLockKey int64 = 0x4449414d49475200
	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", migrationLockKey); err != nil {
		return fmt.Errorf("take migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	goose.SetBaseFS(migrations.FS)
	goose.SetLogger(gooseLogger{log})

	if err := goose.UpContext(ctx, db, "."); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	log.Info("migrations applied")
	return nil
}

// Close releases the pool.
func (s *Store) Close() {
	if s.Pool != nil {
		s.Pool.Close()
	}
}

type gooseLogger struct{ log *slog.Logger }

func (g gooseLogger) Printf(format string, v ...any) { g.log.Info(fmt.Sprintf(format, v...)) }
func (g gooseLogger) Fatalf(format string, v ...any) { g.log.Error(fmt.Sprintf(format, v...)) }
