// Package bot is the worker runtime: it consumes gateway events from the bus,
// maintains core guild state, routes interactions, and hosts feature plugins.
package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/guildstate"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Bot is the worker runtime.
type Bot struct {
	deps    plugin.Deps
	router  *interactions.Router
	gstate  *guildstate.Store
	events  map[event.Type][]plugin.EventHandler
	workers []plugin.NamedWorker
	log     *slog.Logger
}

// New creates a Bot from injected dependencies.
func New(deps plugin.Deps) *Bot {
	return &Bot{
		deps:   deps,
		router: interactions.NewRouter(deps.Discord, deps.Log),
		gstate: guildstate.New(deps.Cache),
		events: map[event.Type][]plugin.EventHandler{},
		log:    deps.Log,
	}
}

// Register initializes plugins and wires their declared capabilities.
func (b *Bot) Register(ctx context.Context, plugins ...plugin.Plugin) error {
	for _, p := range plugins {
		reg := plugin.NewRegistrar()
		if err := p.Init(ctx, b.deps, reg); err != nil {
			return fmt.Errorf("init plugin %q: %w", p.Info().Key, err)
		}
		for _, c := range reg.Commands {
			b.router.AddCommand(c)
		}
		for _, cr := range reg.Components {
			b.router.OnComponent(cr.Prefix, cr.Handler)
		}
		for _, mr := range reg.Modals {
			b.router.OnModal(mr.Prefix, mr.Handler)
		}
		for t, hs := range reg.Events {
			b.events[t] = append(b.events[t], hs...)
		}
		b.workers = append(b.workers, reg.Workers...)
		if reg.Fallback != nil {
			b.router.SetCommandFallback(reg.Fallback)
		}
		b.log.Info("registered plugin",
			"key", p.Info().Key, "commands", len(reg.Commands), "events", len(reg.Events))
	}
	return nil
}

// SyncCommands publishes the registered command set to Discord. A non-empty
// devGuildID registers to that guild (instant) instead of globally (~1h).
func (b *Bot) SyncCommands(ctx context.Context, devGuildID string) error {
	defs := b.router.CommandDefs()
	if devGuildID != "" {
		if _, err := b.deps.Discord.BulkOverwriteGuildCommands(devGuildID, defs); err != nil {
			return fmt.Errorf("register guild commands: %w", err)
		}
		b.log.Info("registered guild commands", "guild", devGuildID, "count", len(defs))
		return nil
	}
	if _, err := b.deps.Discord.BulkOverwriteGlobalCommands(defs); err != nil {
		return fmt.Errorf("register global commands: %w", err)
	}
	b.log.Info("registered global commands", "count", len(defs))
	return nil
}

// CommandDefs returns the global slash-command set (for registering the same
// commands under a customer's custom-bot application).
func (b *Bot) CommandDefs() []*discordgo.ApplicationCommand {
	return b.router.CommandDefs()
}

// Start launches background workers and begins consuming events. It returns a
// stop function that drains them.
func (b *Bot) Start(ctx context.Context) (func(), error) {
	var wg sync.WaitGroup
	for _, w := range b.workers {
		wg.Add(1)
		go func(nw plugin.NamedWorker) {
			defer wg.Done()
			b.log.Info("starting worker", "name", nw.Name)
			nw.Run(ctx)
		}(w)
	}

	sub, err := b.deps.Bus.Consume(ctx, eventbus.ConsumerSpec{
		Durable:        "dia-worker",
		FilterSubjects: []string{event.SubjectPrefix + ".>"},
		AckWait:        30 * time.Second,
		MaxDeliver:     5,
		MaxAckPending:  256,
	}, b.handle)
	if err != nil {
		return nil, err
	}

	stop := func() {
		sub.Stop()
		wg.Wait()
	}
	return stop, nil
}

// handle decodes an envelope and dispatches it. It always acks (returns nil):
// errors are logged rather than triggering redelivery, to avoid duplicate
// side-effects from at-least-once retries.
func (b *Bot) handle(ctx context.Context, msg eventbus.Msg) error {
	var env event.Envelope
	if err := json.Unmarshal(msg.Data(), &env); err != nil {
		b.log.Warn("dropping malformed event", "err", err)
		return nil
	}

	// Resolve which bot produced this event and inject its REST client into the
	// context, so every downstream send/action for a custom-bot guild acts as
	// that customer's bot (whose token is the only one with access there). No
	// DB hit: the app id rides on the envelope.
	if b.deps.Bots != nil {
		ctx = discord.WithClient(ctx, b.deps.Bots.ForApp(ctx, env.AppID))
	}

	// Core state tracking (guilds, channels, roles, member counts).
	b.handleCore(ctx, &env)

	// Interactions → router.
	if env.Type == event.TypeInteractionCreate {
		var i event.Interaction
		if err := json.Unmarshal(env.Data, &i); err != nil {
			b.log.Warn("malformed interaction", "err", err)
			return nil
		}
		b.router.Dispatch(ctx, &i)
		return nil
	}

	// Plugin event handlers.
	for _, h := range b.events[env.Type] {
		if err := h(ctx, &env); err != nil {
			b.log.Warn("plugin event handler error", "type", env.Type, "guild", env.GuildID, "err", err)
		}
	}
	return nil
}
