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
	Audit          *AuditRepo
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
	s.Audit = &AuditRepo{pool: pool}

	log.Info("connected to postgres", "max_conns", pcfg.MaxConns)
	return s, nil
}

// Migrate applies all pending migrations using goose. It opens a transient
// database/sql handle from the pool (goose requires one) and takes a Postgres
// advisory lock so only one instance migrates in a multi-replica deploy.
func (s *Store) Migrate(ctx context.Context, log *slog.Logger) error {
	db := stdlib.OpenDBFromPool(s.Pool)
	defer db.Close()

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
