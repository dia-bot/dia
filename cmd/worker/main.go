// Command worker is Dia's bot brain: it consumes Discord gateway events from
// NATS JetStream, keeps core guild state in sync, routes interactions and runs
// the feature plugins. It makes no gateway connection itself (the Elixir
// gateway does); it only consumes events and calls Discord REST.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/dia-bot/dia/internal/bot"
	"github.com/dia-bot/dia/internal/botreg"
	"github.com/dia-bot/dia/internal/cache"
	"github.com/dia-bot/dia/internal/config"
	"github.com/dia-bot/dia/internal/custombot"
	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/guildstate"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/logging"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/secret"
	"github.com/dia-bot/dia/internal/store"

	automations "github.com/dia-bot/dia/internal/features/automations/runtime"
	customcommands "github.com/dia-bot/dia/internal/features/customcommands/runtime"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/dia-bot/dia/internal/features/leveling"
	serverlogs "github.com/dia-bot/dia/internal/features/logging"
	"github.com/dia-bot/dia/internal/features/moderation"
	"github.com/dia-bot/dia/internal/features/roles"
	"github.com/dia-bot/dia/internal/features/schedmessages"
	"github.com/dia-bot/dia/internal/features/socialnotifications"
	"github.com/dia-bot/dia/internal/features/statschannels"
	"github.com/dia-bot/dia/internal/features/tickets"
	"github.com/dia-bot/dia/internal/features/verification"
	"github.com/dia-bot/dia/internal/features/welcome"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logging.New(cfg.LogLevel, cfg.Env)
	if err := cfg.RequireBot(); err != nil {
		fatal(log, "invalid configuration", err)
	}

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

	box, err := secret.NewBox(cfg.Discord.CustomBotEncKey)
	if err != nil {
		fatal(log, "custom bot encryption key", err)
	}
	registry := botreg.New(dg, st, box, log)

	deps := plugin.Deps{
		Config:     cfg,
		Log:        log,
		Store:      st,
		Cache:      caches,
		Discord:    dg,
		Imaging:    imaging.New(cfg.Imaging.FontsDir, log),
		Bus:        bus,
		GuildState: guildstate.New(caches),
		Bots:       registry,
	}

	b := bot.New(deps)
	automationsPlugin := automations.New()
	giveawayPlugin := giveaway.New()
	socialPlugin := socialnotifications.New()
	schedPlugin := schedmessages.New()
	if err := b.Register(ctx,
		welcome.New(),
		leveling.New(),
		roles.New(),
		moderation.New(),
		verification.New(),
		serverlogs.New(),
		tickets.New(),
		socialPlugin,
		statschannels.New(),
		schedPlugin,
		customcommands.New(),
		automationsPlugin,
		giveawayPlugin,
	); err != nil {
		fatal(log, "register plugins", err)
	}
	// Composed giveaway and social action buttons (and social per-kind
	// attachments) fire a saved automation. Those plugins can't import the
	// automations runner (it would cycle), so the automations runtime is
	// injected as the bridge once all have initialised.
	giveawayPlugin.SetAutomationRunner(automationsPlugin)
	socialPlugin.SetAutomationRunner(automationsPlugin)
	schedPlugin.SetAutomationRunner(automationsPlugin)

	// DEV_GUILD_ID registers commands to one guild (instant) for development;
	// empty registers globally (~1h propagation).
	if err := b.SyncCommands(ctx, os.Getenv("DEV_GUILD_ID")); err != nil {
		log.Error("command sync failed (continuing)", "err", err)
	}

	// Custom-bot control service: reconciles the gateway's running set of
	// customer bots with the database and registers their commands on connect.
	cbManager := custombot.NewManager(st, box, bus, registry, log)
	cbService := custombot.NewService(cbManager, st, registry, bus, b.CommandDefs, log)
	go cbService.Run(ctx)

	stopBot, err := b.Start(ctx)
	if err != nil {
		fatal(log, "start", err)
	}
	log.Info("dia worker running")

	<-ctx.Done()
	log.Info("shutting down worker")
	stopBot()
}

func fatal(log *slog.Logger, msg string, err error) {
	log.Error(msg, "err", err)
	os.Exit(1)
}
