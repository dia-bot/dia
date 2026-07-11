package giveaway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
)

// Manager exposes the giveaway lifecycle (create, edit, start, draw, reroll,
// cancel, delete) to callers outside the worker: chiefly the dashboard API and
// the custom-command "Start giveaway" step. It reuses the exact same
// post/draw/announce path the sweeper and the Enter button use, so a dashboard
// action behaves identically to the live worker.
//
// Every method runs on a detached context (context.WithoutCancel) so a client
// aborting the HTTP request mid-flight can't strand a half-posted or half-drawn
// giveaway; once started, an action runs to completion.
type Manager struct{ p *Plugin }

// NewManager builds a Manager over the given shared deps.
func NewManager(d plugin.Deps) *Manager { return &Manager{p: &Plugin{deps: d}} }

// Errors surfaced to API callers for a clean 4xx.
var (
	ErrNoPrize        = errors.New("a prize is required")
	ErrNoChannel      = errors.New("a channel is required")
	ErrNotRunning     = errors.New("giveaway is not running")
	ErrNotEnded       = errors.New("giveaway has not ended")
	ErrNotCancellable = errors.New("giveaway can't be cancelled")
	ErrNotEditable    = errors.New("giveaway can't be edited")
	ErrNotStartable   = errors.New("only a draft can be started")
	ErrNotDeletable   = errors.New("only a draft can be deleted")
	ErrBadDuration    = errors.New("invalid duration")
)

// CreateInput is a fully-resolved giveaway to persist (and post, when running).
// The dashboard builds it from the editor; the step builds it from a preset.
type CreateInput struct {
	Name         string
	Prize        string
	Description  string
	ChannelID    int64
	WinnerCount  int
	HostID       int64
	CreatedBy    int64
	ImageURL     string
	Color        string
	Spec         Spec
	Requirements RequirementConfig
	Status       string    // draft | scheduled | running (default running)
	StartsAt     time.Time // scheduled start; zero => now
	EndsAt       time.Time // zero => StartsAt + 24h
}

// Get returns one giveaway.
func (m *Manager) Get(ctx context.Context, guildID int64, id string) (store.Giveaway, error) {
	return m.p.deps.Store.Giveaways.Get(ctx, guildID, id)
}

// List returns a guild's giveaways (status "" = all, "active" = scheduled+running).
func (m *Manager) List(ctx context.Context, guildID int64, status string) ([]store.Giveaway, error) {
	return m.p.deps.Store.Giveaways.ListByGuild(ctx, guildID, status, 100)
}

// Create persists a new giveaway and, when its status is "running", posts the
// live message immediately. A failed post on a fresh create rolls the row back
// so there's never an invisible running giveaway.
func (m *Manager) Create(ctx context.Context, guildID int64, in CreateInput) (store.Giveaway, error) {
	ctx = context.WithoutCancel(ctx)
	if strings.TrimSpace(in.Prize) == "" {
		return store.Giveaway{}, ErrNoPrize
	}
	status := normalizeStatus(in.Status)
	if in.ChannelID == 0 && status != "draft" {
		return store.Giveaway{}, ErrNoChannel
	}
	now := time.Now()
	startsAt := in.StartsAt
	if startsAt.IsZero() {
		startsAt = now
	}
	endsAt := in.EndsAt
	if endsAt.IsZero() {
		endsAt = startsAt.Add(24 * time.Hour)
	}
	specJSON, _ := json.Marshal(in.Spec)
	reqJSON, _ := json.Marshal(in.Requirements)
	g := store.Giveaway{
		GuildID:      guildID,
		ChannelID:    in.ChannelID,
		Name:         in.Name,
		Prize:        in.Prize,
		Description:  in.Description,
		WinnerCount:  clampWinners(in.WinnerCount, 1),
		HostID:       in.HostID,
		Status:       status,
		Spec:         specJSON,
		Requirements: reqJSON,
		ImageURL:     in.ImageURL,
		Color:        in.Color,
		StartsAt:     startsAt,
		EndsAt:       endsAt,
		CreatedBy:    in.CreatedBy,
	}
	created, err := m.p.deps.Store.Giveaways.Create(ctx, g)
	if err != nil {
		return store.Giveaway{}, err
	}
	if status == "running" {
		posted, perr := m.postAndRecord(ctx, in.Spec, created)
		if perr != nil {
			_ = m.p.deps.Store.Giveaways.Delete(ctx, guildID, created.ID)
			return store.Giveaway{}, perr
		}
		return posted, nil
	}
	return created, nil
}

// Update edits an editable giveaway (draft, scheduled or running). A running
// giveaway with a posted message is re-rendered so the edit is reflected live.
func (m *Manager) Update(ctx context.Context, guildID int64, id string, patch store.GiveawayPatch) (store.Giveaway, error) {
	ctx = context.WithoutCancel(ctx)
	if patch.Prize != nil && strings.TrimSpace(*patch.Prize) == "" {
		return store.Giveaway{}, ErrNoPrize
	}
	g, ok, err := m.p.deps.Store.Giveaways.Update(ctx, guildID, id, patch)
	if err != nil {
		return store.Giveaway{}, err
	}
	if !ok {
		return store.Giveaway{}, ErrNotEditable
	}
	if g.Status == "running" && g.MessageID != 0 {
		m.p.refreshLiveMessage(ctx, decodeSpec(g.Spec), g)
	}
	return g, nil
}

// Start activates a draft: posts it now (startsIn == 0) or schedules it to post
// later. duration sets the run length; the sweeper posts a scheduled one.
func (m *Manager) Start(ctx context.Context, guildID int64, id string, startsIn, duration time.Duration) (store.Giveaway, error) {
	ctx = context.WithoutCancel(ctx)
	g, err := m.p.deps.Store.Giveaways.Get(ctx, guildID, id)
	if err != nil {
		return store.Giveaway{}, err
	}
	if g.Status != "draft" {
		return store.Giveaway{}, ErrNotStartable
	}
	if strings.TrimSpace(g.Prize) == "" {
		return store.Giveaway{}, ErrNoPrize
	}
	if g.ChannelID == 0 {
		return store.Giveaway{}, ErrNoChannel
	}
	if duration <= 0 {
		duration = 24 * time.Hour
	}
	now := time.Now()
	startsAt, status := now, "running"
	if startsIn > 0 {
		startsAt, status = now.Add(startsIn), "scheduled"
	}
	activated, ok, err := m.p.deps.Store.Giveaways.Activate(ctx, id, status, startsAt, startsAt.Add(duration))
	if err != nil {
		return store.Giveaway{}, err
	}
	if !ok {
		return store.Giveaway{}, ErrNotStartable
	}
	if status == "running" {
		return m.postAndRecord(ctx, decodeSpec(activated.Spec), activated)
	}
	return activated, nil // scheduled — the sweeper posts it at starts_at
}

// Delete removes a draft (running/scheduled use Cancel; ended stays for history).
func (m *Manager) Delete(ctx context.Context, guildID int64, id string) error {
	ctx = context.WithoutCancel(ctx)
	g, err := m.p.deps.Store.Giveaways.Get(ctx, guildID, id)
	if err != nil {
		return err
	}
	if g.Status != "draft" {
		return ErrNotDeletable
	}
	return m.p.deps.Store.Giveaways.Delete(ctx, guildID, id)
}

// End ends a running giveaway now, drawing + announcing winners.
func (m *Manager) End(ctx context.Context, guildID int64, id string) (store.Giveaway, error) {
	ctx = context.WithoutCancel(ctx)
	g, err := m.p.deps.Store.Giveaways.Get(ctx, guildID, id)
	if err != nil {
		return store.Giveaway{}, err
	}
	if g.Status != "running" {
		return store.Giveaway{}, ErrNotRunning
	}
	if !m.p.finishGiveaway(ctx, decodeSpec(g.Spec), g) {
		return store.Giveaway{}, ErrNotRunning
	}
	return g, nil
}

// Reroll draws replacement winners for an ended giveaway (no-op if the eligible
// pool is exhausted).
func (m *Manager) Reroll(ctx context.Context, guildID int64, id string, count int) ([]int64, error) {
	ctx = context.WithoutCancel(ctx)
	g, err := m.p.deps.Store.Giveaways.Get(ctx, guildID, id)
	if err != nil {
		return nil, err
	}
	if g.Status != "ended" {
		return nil, ErrNotEnded
	}
	return m.p.rerollGiveaway(ctx, decodeSpec(g.Spec), g, count), nil
}

// Cancel cancels a running/scheduled giveaway with no draw.
func (m *Manager) Cancel(ctx context.Context, guildID int64, id string) (store.Giveaway, error) {
	ctx = context.WithoutCancel(ctx)
	cancelled, ok, err := m.p.deps.Store.Giveaways.Cancel(ctx, guildID, id)
	if err != nil {
		return store.Giveaway{}, err
	}
	if !ok {
		return store.Giveaway{}, ErrNotCancellable
	}
	m.p.markCancelled(ctx, decodeSpec(cancelled.Spec), cancelled)
	return cancelled, nil
}

// StartGiveaway starts a running giveaway from a saved preset, applying the
// caller's overrides (any blank/zero value falls back to the preset default).
// It's the entrypoint for the custom-command "Start giveaway" step (satisfying
// exec.GiveawayStarter with primitive args, so the exec engine needn't depend on
// this package). Returns the new giveaway's id.
func (m *Manager) StartGiveaway(ctx context.Context, guildID int64, preset, prize, channel, duration string, winners int, hostID int64) (string, error) {
	ctx = context.WithoutCancel(ctx)
	cfg := m.loadCfg(ctx, guildID)
	ps := cfg.preset(preset)

	channelID := parseSnowflakeID(channel)
	if channelID == 0 {
		channelID = parseSnowflakeID(ps.DefaultChannelID)
	}
	if channelID == 0 {
		return "", ErrNoChannel
	}
	dur, err := parseGiveawayDuration(firstNonEmpty(duration, ps.DefaultDuration, "24h"))
	if err != nil {
		return "", ErrBadDuration
	}
	if winners <= 0 {
		winners = ps.DefaultWinnerCount
	}
	now := time.Now()
	created, err := m.Create(ctx, guildID, CreateInput{
		Prize:        prize,
		ChannelID:    channelID,
		WinnerCount:  winners,
		HostID:       hostID,
		CreatedBy:    hostID,
		Spec:         ps.Spec,
		Requirements: ps.Requirements,
		Status:       "running",
		StartsAt:     now,
		EndsAt:       now.Add(dur),
	})
	if err != nil {
		return "", err
	}
	return created.ID, nil
}

// postAndRecord posts a running giveaway's live message and records the message
// id, returning the giveaway with MessageID set.
func (m *Manager) postAndRecord(ctx context.Context, spec Spec, g store.Giveaway) (store.Giveaway, error) {
	msg, err := m.p.postGiveaway(ctx, spec, g, 0)
	if err != nil {
		return store.Giveaway{}, err
	}
	mid, _ := event.ParseID(msg.ID)
	if err := m.p.deps.Store.Giveaways.SetMessageID(ctx, g.ID, mid); err != nil {
		m.p.deps.Log.Warn("giveaway: set message id", "giveaway", g.ID, "err", err)
	}
	g.MessageID = mid
	return g, nil
}

func (m *Manager) loadCfg(ctx context.Context, guildID int64) Config {
	cfg, _, _ := plugin.LoadConfig[Config](ctx, m.p.deps, guildID, FeatureKey)
	return cfg
}

// normalizeStatus clamps an incoming status to the states a create may set.
func normalizeStatus(s string) string {
	switch s {
	case "draft", "scheduled", "running":
		return s
	default:
		return "running"
	}
}
