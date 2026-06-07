// Command api is Dia's dashboard backend: Discord OAuth2 login, per-guild
// configuration CRUD, welcome/rank image previews and a realtime WebSocket.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dia-bot/dia/internal/api"
	"github.com/dia-bot/dia/internal/cache"
	"github.com/dia-bot/dia/internal/config"
	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/logging"
	"github.com/dia-bot/dia/internal/storage"
	"github.com/dia-bot/dia/internal/store"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "apply database migrations and exit")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logging.New(cfg.LogLevel, cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.Open(ctx, cfg.Postgres, log)
	if err != nil {
		fatal(log, "postgres", err)
	}
	defer st.Close()
	if err := st.Migrate(ctx, log); err != nil {
		fatal(log, "migrate", err)
	}
	if *migrateOnly {
		log.Info("migrations complete; exiting (--migrate-only)")
		return
	}

	if err := cfg.RequireAPI(); err != nil {
		fatal(log, "invalid configuration", err)
	}

	caches, err := cache.Connect(ctx, cfg.Redis.URL, log)
	if err != nil {
		fatal(log, "redis", err)
	}
	defer caches.Close()

	dg, err := discord.New(cfg.Discord.Token, cfg.Discord.ClientID, log)
	if err != nil {
		fatal(log, "discord", err)
	}

	bus, err := eventbus.ConnectNATS(ctx, eventbus.NATSConfig{
		URL:      cfg.NATS.URL,
		Stream:   cfg.NATS.Stream,
		Subjects: []string{event.SubjectPrefix + ".>"},
	}, log)
	if err != nil {
		fatal(log, "nats", err)
	}
	defer bus.Close()

	// Object storage for uploads is optional; log + continue without it so the
	// dashboard still runs (the upload endpoint then returns 503).
	var blob *storage.Store
	if cfg.Storage.Enabled() {
		blob, err = storage.New(storage.Config{
			Endpoint:       cfg.Storage.Endpoint,
			Region:         cfg.Storage.Region,
			Bucket:         cfg.Storage.Bucket,
			AccessKey:      cfg.Storage.AccessKey,
			SecretKey:      cfg.Storage.SecretKey,
			PublicBaseURL:  cfg.Storage.PublicBaseURL,
			ForcePathStyle: cfg.Storage.ForcePathStyle,
			ACL:            cfg.Storage.ACL,
		})
		if err != nil {
			log.Error("object storage disabled (bad config)", "err", err)
			blob = nil
		} else {
			log.Info("object storage enabled", "bucket", cfg.Storage.Bucket)
		}
	} else {
		log.Info("object storage not configured; uploads disabled")
	}

	srv := api.New(api.Deps{
		Config:  cfg,
		Log:     log,
		Store:   st,
		Cache:   caches,
		Discord: dg,
		Imaging: imaging.New(cfg.Imaging.FontsDir, log),
		Bus:     bus,
		Storage: blob,
	})

	if err := srv.StartRealtime(ctx); err != nil {
		log.Error("realtime feed failed to start (continuing)", "err", err)
	}

	httpSrv := &http.Server{
		Addr:              cfg.API.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("dia api listening", "addr", cfg.API.Addr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal(log, "http server", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down api")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}

func fatal(log *slog.Logger, msg string, err error) {
	log.Error(msg, "err", err)
	os.Exit(1)
}
