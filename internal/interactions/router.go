package interactions

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Handler processes an interaction Context.
type Handler func(c *Context) error

// Command is a slash/context-menu command definition plus its handlers.
type Command struct {
	Def          *discordgo.ApplicationCommand
	Handler      Handler
	Autocomplete Handler // optional, invoked for autocomplete interactions
}

// Router registers commands, component and modal handlers and dispatches
// incoming interactions to them.
type Router struct {
	mu         sync.RWMutex
	commands   map[string]*Command
	components []prefixRoute
	modals     []prefixRoute

	client *discord.Client
	log    *slog.Logger
}

type prefixRoute struct {
	prefix  string
	handler Handler
}

// NewRouter creates a router bound to a REST client.
func NewRouter(client *discord.Client, log *slog.Logger) *Router {
	return &Router{commands: map[string]*Command{}, client: client, log: log}
}

// AddCommand registers (or replaces) a command by name.
func (r *Router) AddCommand(cmd *Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands[cmd.Def.Name] = cmd
}

// OnComponent registers a message-component handler matched by custom_id prefix.
// Convention: custom_id is "<feature>:<action>:<args...>"; register "<feature>:".
func (r *Router) OnComponent(prefix string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.components = append(r.components, prefixRoute{prefix, h})
}

// OnModal registers a modal-submit handler matched by custom_id prefix.
func (r *Router) OnModal(prefix string, h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.modals = append(r.modals, prefixRoute{prefix, h})
}

// CommandDefs returns the registered command definitions (for REST registration).
func (r *Router) CommandDefs() []*discordgo.ApplicationCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]*discordgo.ApplicationCommand, 0, len(r.commands))
	for _, c := range r.commands {
		defs = append(defs, c.Def)
	}
	return defs
}

// Dispatch routes an interaction to the appropriate handler.
func (r *Router) Dispatch(ctx context.Context, i *event.Interaction) {
	c := &Context{Ctx: ctx, I: i, Client: r.client, Log: r.log, GuildID: i.GuildID}
	if u, ok := i.Actor(); ok {
		c.User = u
	}

	var err error
	switch i.Type {
	case event.InteractionApplicationCommand:
		err = r.dispatchCommand(c, false)
	case event.InteractionAutocomplete:
		err = r.dispatchCommand(c, true)
	case event.InteractionMessageComponent:
		err = r.dispatchPrefix(c, r.componentHandler(i.Data.CustomID))
	case event.InteractionModalSubmit:
		err = r.dispatchPrefix(c, r.modalHandler(i.Data.CustomID))
	default:
		return
	}

	if err != nil {
		r.log.Warn("interaction handler error",
			"type", i.Type, "command", i.Data.Name, "custom_id", i.Data.CustomID, "err", err)
		if !c.responded && i.Type != event.InteractionAutocomplete {
			_ = c.RespondEphemeral("⚠️ Something went wrong handling that. Please try again.")
		}
	}
}

func (r *Router) dispatchCommand(c *Context, auto bool) error {
	r.mu.RLock()
	cmd := r.commands[c.I.Data.Name]
	r.mu.RUnlock()
	if cmd == nil {
		if auto {
			return nil
		}
		return c.RespondEphemeral("Unknown command.")
	}
	if auto {
		if cmd.Autocomplete == nil {
			return nil
		}
		return cmd.Autocomplete(c)
	}
	return cmd.Handler(c)
}

func (r *Router) dispatchPrefix(c *Context, h Handler) error {
	if h == nil {
		return nil // no matching handler; silently ignore stale components
	}
	return h(c)
}

func (r *Router) componentHandler(customID string) Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rt := range r.components {
		if strings.HasPrefix(customID, rt.prefix) {
			return rt.handler
		}
	}
	return nil
}

func (r *Router) modalHandler(customID string) Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, rt := range r.modals {
		if strings.HasPrefix(customID, rt.prefix) {
			return rt.handler
		}
	}
	return nil
}
