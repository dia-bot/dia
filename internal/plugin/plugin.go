// Package plugin is Dia's feature SDK. Every feature (welcome, leveling, roles,
// moderation, custom commands, …) implements Plugin and declares exactly what it
// provides during Init via an explicit Registrar — slash commands, component and
// modal handlers, gateway event subscriptions and background workers.
//
// Capabilities are declared explicitly (not discovered by type-assertion), so a
// missing hook is a visible no-op rather than a silent mistake, and the set of
// things a feature touches is obvious from its Init.
package plugin

import (
	"context"
	"log/slog"

	"github.com/dia-bot/dia/internal/cache"
	"github.com/dia-bot/dia/internal/config"
	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/guildstate"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/store"
)

// Category groups features for the dashboard sidebar.
type Category string

const (
	CategoryEngagement Category = "engagement"
	CategoryModeration Category = "moderation"
	CategoryUtility    Category = "utility"
)

// Info is a feature's identity. Key must be unique and match its feature_key in
// guild_feature_configs.
type Info struct {
	Key         string
	Name        string
	Description string
	Category    Category
}

// Deps are the shared services injected into every plugin. There are no global
// singletons — the worker constructs these once and passes them in.
type Deps struct {
	Config  *config.Config
	Log     *slog.Logger
	Store   *store.Store
	Cache   *cache.Store
	Discord *discord.Client
	Imaging *imaging.Renderer
	Bus     eventbus.Bus
	// GuildState exposes the cached per-guild roles/channels snapshot for
	// read-only template lookups (getRole/getChannel).
	GuildState *guildstate.Store
}

// EventHandler reacts to a decoded gateway event envelope.
type EventHandler func(ctx context.Context, env *event.Envelope) error

// NamedWorker is a long-running background goroutine owned by a plugin.
type NamedWorker struct {
	Name string
	Run  func(ctx context.Context)
}

// ComponentRoute / ModalRoute bind a custom_id prefix to a handler.
type ComponentRoute struct {
	Prefix  string
	Handler interactions.Handler
}
type ModalRoute struct {
	Prefix  string
	Handler interactions.Handler
}

// Registrar collects a plugin's declared capabilities during Init. The worker
// reads it afterwards to wire the interaction router, event dispatch and workers.
type Registrar struct {
	Commands   []*interactions.Command
	Components []ComponentRoute
	Modals     []ModalRoute
	Events     map[event.Type][]EventHandler
	Workers    []NamedWorker
	Fallback   interactions.Handler // optional dynamic command fallback (custom commands)
}

// NewRegistrar returns an empty Registrar.
func NewRegistrar() *Registrar {
	return &Registrar{Events: map[event.Type][]EventHandler{}}
}

// Command registers a slash/context-menu command.
func (r *Registrar) Command(c *interactions.Command) { r.Commands = append(r.Commands, c) }

// Component registers a message-component handler by custom_id prefix.
func (r *Registrar) Component(prefix string, h interactions.Handler) {
	r.Components = append(r.Components, ComponentRoute{Prefix: prefix, Handler: h})
}

// Modal registers a modal-submit handler by custom_id prefix.
func (r *Registrar) Modal(prefix string, h interactions.Handler) {
	r.Modals = append(r.Modals, ModalRoute{Prefix: prefix, Handler: h})
}

// OnEvent subscribes to a gateway event type.
func (r *Registrar) OnEvent(t event.Type, h EventHandler) {
	r.Events[t] = append(r.Events[t], h)
}

// Worker registers a background worker.
func (r *Registrar) Worker(name string, run func(ctx context.Context)) {
	r.Workers = append(r.Workers, NamedWorker{Name: name, Run: run})
}

// CommandFallback registers a handler for application commands that have no
// statically-registered handler (dynamic per-guild custom commands).
func (r *Registrar) CommandFallback(h interactions.Handler) { r.Fallback = h }

// Plugin is the minimal feature interface.
type Plugin interface {
	Info() Info
	Init(ctx context.Context, d Deps, reg *Registrar) error
}
