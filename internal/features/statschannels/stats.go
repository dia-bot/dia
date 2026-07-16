// Package statschannels keeps templated "server stats" channels current
// (locked voice channels whose names render live values like the member count)
// and publishes MEMBER_MILESTONE events when the member count crosses a
// configured step, so automations can celebrate via the member_milestone
// trigger.
package statschannels

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// minRenameGap keeps channel renames inside Discord's rate limit (2 renames
// per channel per 10 minutes) with headroom for the periodic sweep.
const minRenameGap = 6 * time.Minute

// sweepEvery is the periodic refresh cadence for all enabled guilds.
const sweepEvery = 10 * time.Minute

// Plugin implements the stats channels feature.
type Plugin struct {
	tmpl *templating.Engine

	mu         sync.Mutex
	lastRename map[string]time.Time // channelID → last rename we issued
	lastName   map[string]string    // channelID → last name we set
}

// New returns the stats channels plugin.
func New() *Plugin {
	return &Plugin{
		lastRename: map[string]time.Time{},
		lastName:   map[string]string{},
	}
}

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Server Stats",
		Description: "Live stats channels (member count and more) with templated names, plus member-count milestone events for automations.",
		Category:    plugin.CategoryUtility,
	}
}

// Init wires member-change reactions and the periodic sweep.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.tmpl = templating.New()
	reg.OnEvent(event.TypeMemberAdd, func(ctx context.Context, env *event.Envelope) error {
		return p.onMemberChange(ctx, d, env, true)
	})
	reg.OnEvent(event.TypeMemberRemove, func(ctx context.Context, env *event.Envelope) error {
		return p.onMemberChange(ctx, d, env, false)
	})
	reg.Worker("stats-updater", func(ctx context.Context) { p.sweepLoop(ctx, d) })
	return nil
}

// onMemberChange refreshes the guild's counters promptly on join/leave and
// checks the milestone step on joins.
func (p *Plugin) onMemberChange(ctx context.Context, d plugin.Deps, env *event.Envelope, joined bool) error {
	gid, ok := event.ParseID(env.GuildID)
	if !ok {
		return nil
	}
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return err
	}

	if joined && cfg.MilestoneStep > 0 {
		var m event.MemberAdd
		if json.Unmarshal(env.Data, &m) == nil && m.MemberCount > 0 {
			step := cfg.MilestoneStep
			if (m.MemberCount / step) > ((m.MemberCount - 1) / step) {
				PublishMilestone(ctx, d.Bus, d.Log, event.MemberMilestone{
					GuildID: env.GuildID,
					Count:   m.MemberCount,
					Step:    step,
					Reached: (m.MemberCount / step) * step,
				})
			}
		}
	}

	p.updateGuild(ctx, d, env.GuildID, cfg)
	return nil
}

// sweepLoop refreshes every enabled guild's counters on a slow cadence so
// values that change without a member event (channel or role counts) stay
// current, and so a restart repaints promptly.
func (p *Plugin) sweepLoop(ctx context.Context, d plugin.Deps) {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()
	p.sweep(ctx, d)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sweep(ctx, d)
		}
	}
}

func (p *Plugin) sweep(ctx context.Context, d plugin.Deps) {
	guilds, err := d.Store.Features.ListGuildsEnabled(ctx, FeatureKey)
	if err != nil {
		d.Log.Warn("stats: list enabled guilds", "err", err)
		return
	}
	for _, gid := range guilds {
		cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
		if err != nil || !enabled {
			continue
		}
		p.updateGuild(ctx, d, event.FormatID(gid), cfg)
	}
}

// updateGuild renders every enabled counter against the live snapshot and
// renames channels whose rendered name changed (respecting the rename gap).
func (p *Plugin) updateGuild(ctx context.Context, d plugin.Deps, guildID string, cfg Config) {
	if len(cfg.Counters) == 0 {
		return
	}
	snap, err := d.GuildState.Snapshot(ctx, guildID)
	if err != nil {
		return
	}
	data := map[string]any{
		"Members":  snap.Meta.MemberCount,
		"Channels": len(snap.Channels),
		"Roles":    len(snap.Roles),
		"Guild":    map[string]any{"Name": snap.Meta.Name},
	}
	if cfg.MilestoneStep > 0 {
		data["Milestone"] = (snap.Meta.MemberCount / cfg.MilestoneStep) * cfg.MilestoneStep
	} else {
		data["Milestone"] = 0
	}

	for _, c := range cfg.Counters {
		if !c.Enabled || c.ChannelID == "" || strings.TrimSpace(c.Template) == "" {
			continue
		}
		name, err := p.tmpl.RenderCard(ctx, c.Template, data)
		if err != nil {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if r := []rune(name); len(r) > 100 {
			name = string(r[:100])
		}
		if !p.claimRename(c.ChannelID, name) {
			continue
		}
		if _, err := d.Discord.EditChannel(c.ChannelID, &discordgo.ChannelEdit{Name: name}, "stats counter update"); err != nil {
			d.Log.Warn("stats: rename failed", "guild", guildID, "channel", c.ChannelID, "err", err)
			p.releaseRename(c.ChannelID)
		}
	}
}

// claimRename reports whether a rename to name should be issued now: the name
// must differ from the last one we set and the channel must be outside its
// rename gap. Claiming records the attempt so concurrent updates don't race.
func (p *Plugin) claimRename(channelID, name string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.lastName[channelID] == name {
		return false
	}
	if time.Since(p.lastRename[channelID]) < minRenameGap {
		return false
	}
	p.lastRename[channelID] = time.Now()
	p.lastName[channelID] = name
	return true
}

// releaseRename forgets a failed rename so the next pass retries it.
func (p *Plugin) releaseRename(channelID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.lastName, channelID)
}

// PublishMilestone emits one MemberMilestone envelope on the event bus, keyed
// so a burst of joins around the boundary publishes each milestone once.
func PublishMilestone(ctx context.Context, bus eventbus.Bus, log *slog.Logger, m event.MemberMilestone) {
	if bus == nil {
		return
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	envBytes, err := json.Marshal(event.Envelope{
		Type:    event.TypeMemberMilestone,
		GuildID: m.GuildID,
		TS:      time.Now().UnixMilli(),
		Data:    data,
	})
	if err != nil {
		return
	}
	subject := event.Subject(event.TypeMemberMilestone, m.GuildID)
	dedup := fmt.Sprintf("milestone:%s:%d", m.GuildID, m.Reached)
	if err := bus.Publish(ctx, subject, envBytes, dedup); err != nil {
		log.Warn("stats: publish milestone failed", "subject", subject, "err", err)
	}
}
