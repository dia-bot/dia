package custombot

import (
	"context"
	"encoding/json"
	"log/slog"
	"strconv"
	"time"

	"github.com/dia-bot/dia/internal/botreg"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// reconcileInterval re-asserts the desired running set periodically, so a
// control message the gateway missed (core NATS is at-most-once) self-heals.
const reconcileInterval = 60 * time.Second

// CommandDefs supplies the global slash-command set to register under a custom
// bot's application (the same commands the platform bot exposes).
type CommandDefs func() []*discordgo.ApplicationCommand

// Service runs in the worker. It listens on the gateway status channel, updates
// each bot's state in the database, registers commands under a custom bot's
// application when it first becomes ready, and reconciles the desired set on an
// interval. Construct with NewService and run Run in a goroutine.
type Service struct {
	mgr     *Manager
	store   *store.Store
	bots    *botreg.Registry
	bus     eventbus.Bus
	cmdDefs CommandDefs
	log     *slog.Logger
}

// NewService builds the worker-side control service.
func NewService(mgr *Manager, st *store.Store, bots *botreg.Registry, bus eventbus.Bus, cmdDefs CommandDefs, log *slog.Logger) *Service {
	return &Service{mgr: mgr, store: st, bots: bots, bus: bus, cmdDefs: cmdDefs, log: log}
}

// Run subscribes to gateway status and reconciles until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	if !s.mgr.Enabled() {
		s.log.Info("custombot: disabled (no encryption key); not running control service")
		return
	}

	sub, err := s.bus.SubscribeCore(event.SubjectGatewayStatus, func(data []byte) {
		s.onStatus(ctx, data)
	})
	if err != nil {
		s.log.Error("custombot: subscribe gateway status failed", "err", err)
		return
	}
	defer sub.Stop()

	// Push the current desired set immediately (covers a worker that starts
	// after the gateway is already up and past its hello).
	if err := s.mgr.ReplayAll(ctx); err != nil {
		s.log.Warn("custombot: initial replay failed", "err", err)
	}

	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.mgr.ReplayAll(ctx); err != nil {
				s.log.Warn("custombot: reconcile replay failed", "err", err)
			}
		}
	}
}

func (s *Service) onStatus(ctx context.Context, data []byte) {
	var st event.GatewayStatus
	if err := json.Unmarshal(data, &st); err != nil {
		s.log.Warn("custombot: bad gateway status", "err", err)
		return
	}
	switch st.Event {
	case event.GatewayEventReady:
		// Gateway (re)started: replay the whole desired set so it reconnects
		// every enabled custom bot.
		if err := s.mgr.ReplayAll(ctx); err != nil {
			s.log.Warn("custombot: replay on gateway hello failed", "err", err)
		}
	case event.GatewayEventBotState:
		s.onBotState(ctx, st)
	}
}

func (s *Service) onBotState(ctx context.Context, st event.GatewayStatus) {
	appID, err := strconv.ParseInt(st.AppID, 10, 64)
	if err != nil {
		return
	}
	if err := s.store.CustomBots.SetStateByApp(ctx, appID, st.State, st.Error); err != nil {
		s.log.Warn("custombot: set state failed", "app_id", st.AppID, "err", err)
	}
	if st.State == event.BotStateReady {
		s.syncCommands(ctx, appID, st.AppID)
	}
}

// syncCommands registers the global command set under a custom bot's application
// the first time it becomes ready, using that bot's own token.
func (s *Service) syncCommands(ctx context.Context, appID int64, appIDStr string) {
	row, ok, err := s.store.CustomBots.GetByApp(ctx, appID)
	if err != nil || !ok || row.CommandsSynced {
		return
	}
	defs := s.cmdDefs()
	client := s.bots.ForApp(ctx, appIDStr)
	if _, err := client.BulkOverwriteGlobalCommands(defs); err != nil {
		s.log.Warn("custombot: command sync failed", "app_id", appIDStr, "err", err)
		return
	}
	if err := s.store.CustomBots.SetCommandsSyncedByApp(ctx, appID, true); err != nil {
		s.log.Warn("custombot: mark commands synced failed", "app_id", appIDStr, "err", err)
		return
	}
	s.log.Info("custombot: registered commands", "app_id", appIDStr, "count", len(defs))
}
