// Package schedmessages posts composed messages on a schedule — once, every N
// minutes, daily or weekly — and publishes SCHEDULED_MESSAGE_SENT after each
// post so automations can chain off the "scheduled_message" trigger. The
// timer is durable: next_run_at lives in Postgres, so a restart resumes
// cleanly and posts anything that came due while the worker was down.
package schedmessages

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// componentPrefix namespaces composed action-button clicks on scheduled
// messages (sched:act:<scheduleID>:<suffix>).
const componentPrefix = "sched:"

// AutomationRunner runs a saved automation on demand — the cycle-safe bridge
// for composed action buttons. The worker injects the automations runtime.
type AutomationRunner interface {
	RunAutomation(ctx context.Context, guildID, automationID string, user event.User, member *event.Member, channelID string, eventMap map[string]any) error
}

// Plugin implements the scheduled messages feature.
type Plugin struct {
	tmpl       *templating.Engine
	deps       plugin.Deps
	autoRunner AutomationRunner
}

// New returns the scheduled messages plugin.
func New() *Plugin { return &Plugin{} }

// SetAutomationRunner injects the automations bridge. Called by the worker
// after plugin registration.
func (p *Plugin) SetAutomationRunner(r AutomationRunner) { p.autoRunner = r }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Scheduled Messages",
		Description: "Post composed messages on a schedule: announcements, reminders, recurring events.",
		Category:    plugin.CategoryUtility,
	}
}

// Init wires the due sweeper and the action-button handler.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.tmpl = templating.New()
	p.deps = d
	reg.Component(componentPrefix, func(c *interactions.Context) error { return p.handleComponent(c) })
	reg.Worker("schedule-sweeper", func(ctx context.Context) { p.sweepLoop(ctx, d) })
	return nil
}

func (p *Plugin) sweepLoop(ctx context.Context, d plugin.Deps) {
	ticker := time.NewTicker(30 * time.Second)
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
	due, err := d.Store.Schedules.ListDue(ctx, time.Now(), 25)
	if err != nil {
		d.Log.Warn("scheduler: list due", "err", err)
		return
	}
	for _, s := range due {
		p.runOne(ctx, d, s)
	}
}

// runOne posts one due schedule, advances its timer, and publishes the event.
// The timer advances even when the send fails, so a broken channel can't wedge
// the sweep into a retry loop.
func (p *Plugin) runOne(ctx context.Context, d plugin.Deps, s store.ScheduledMessage) {
	now := time.Now()
	def := DecodeSchedule(s.Schedule)
	next, more := def.NextRun(now)
	var nextAt *time.Time
	if more {
		nextAt = &next
	}
	if err := d.Store.Schedules.SetRun(ctx, s.ID, now, nextAt, more); err != nil {
		d.Log.Warn("scheduler: advance failed", "schedule", s.ID, "err", err)
		return
	}

	_, enabled, err := plugin.LoadConfig[Config](ctx, d, s.GuildID, FeatureKey)
	if err != nil || !enabled {
		return
	}

	send, err := Build(ctx, p.tmpl, s, ScopeData(ctx, d, s))
	if err != nil {
		d.Log.Warn("scheduler: build failed", "guild", s.GuildID, "schedule", s.ID, "err", err)
		return
	}
	m, err := d.Discord.SendMessage(event.FormatID(s.ChannelID), send)
	if err != nil {
		d.Log.Warn("scheduler: send failed", "guild", s.GuildID, "schedule", s.ID, "err", err)
		return
	}
	msgID := ""
	if m != nil {
		msgID = m.ID
	}
	Publish(ctx, d.Bus, d.Log, event.ScheduledMessageSent{
		GuildID:    event.FormatID(s.GuildID),
		ScheduleID: s.ID,
		Name:       s.Name,
		ChannelID:  event.FormatID(s.ChannelID),
		MessageID:  msgID,
	})
}

// Build renders one schedule's composed message into the outgoing send.
// Exported so the dashboard's "send now" endpoint posts exactly what the
// sweeper would at runtime.
func Build(ctx context.Context, tmpl *templating.Engine, s store.ScheduledMessage, data map[string]any) (*discordgo.MessageSend, error) {
	spec := DecodeSpec(s.Spec)
	if spec.Empty() {
		return nil, fmt.Errorf("schedule %d has an empty message", s.ID)
	}
	content, embeds, rows := renderComposed(ctx, tmpl, spec, data, s.ID, 0xff6363)
	if strings.TrimSpace(content) == "" && len(embeds) == 0 && len(rows) == 0 {
		return nil, fmt.Errorf("schedule %d rendered empty", s.ID)
	}
	return &discordgo.MessageSend{
		Content:         content,
		Embeds:          embeds,
		Components:      rows,
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeEveryone, discordgo.AllowedMentionTypeRoles, discordgo.AllowedMentionTypeUsers}},
	}, nil
}

// ScopeData builds the template scope a scheduled message renders against,
// kept in lockstep with the variable chips in web/src/lib/schedules.ts.
func ScopeData(ctx context.Context, d plugin.Deps, s store.ScheduledMessage) map[string]any {
	data := map[string]any{
		"Name": s.Name,
		"Date": time.Now().UTC().Format("January 2, 2006"),
		"Guild": map[string]any{
			"Name":        "",
			"MemberCount": 0,
		},
	}
	if d.GuildState != nil {
		if snap, err := d.GuildState.Snapshot(ctx, event.FormatID(s.GuildID)); err == nil {
			data["Guild"] = map[string]any{
				"Name":        snap.Meta.Name,
				"MemberCount": snap.Meta.MemberCount,
			}
		}
	}
	return data
}

// handleComponent handles clicks on a scheduled message's composed action
// buttons (sched:act:<scheduleID>:<suffix>): the click fires the saved
// automation the button points at.
func (p *Plugin) handleComponent(c *interactions.Context) error {
	rest := strings.TrimPrefix(c.CustomID(), componentPrefix)
	action, rest, ok := strings.Cut(rest, ":")
	if !ok || action != "act" {
		return c.DeferUpdate()
	}
	idStr, suffix, ok := strings.Cut(rest, ":")
	if !ok || suffix == "" {
		return c.DeferUpdate()
	}
	schedID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return c.DeferUpdate()
	}
	gid, _ := event.ParseID(c.GuildID)
	s, found, err := p.deps.Store.Schedules.Get(c.Ctx, gid, schedID)
	if err != nil || !found {
		return c.RespondEphemeral("This scheduled message is no longer available.")
	}
	autoID := DecodeSpec(s.Spec).ButtonActions[suffix]
	if autoID == "" || p.autoRunner == nil {
		return c.RespondEphemeral("This button isn't set up yet.")
	}
	_ = c.DeferUpdate()
	ev := map[string]any{
		"schedule":   s.ID,
		"name":       s.Name,
		"button":     suffix,
		"channel_id": c.I.ChannelID,
	}
	if err := p.autoRunner.RunAutomation(context.WithoutCancel(c.Ctx), c.GuildID, autoID, c.User, c.I.Member, c.I.ChannelID, ev); err != nil {
		p.deps.Log.Warn("scheduler: action button automation", "schedule", s.ID, "automation", autoID, "err", err)
	}
	return nil
}

// actionCustomID routes a composed (non-link) button back to this feature.
func actionCustomID(schedID int64, suffix string) string {
	return componentPrefix + "act:" + strconv.FormatInt(schedID, 10) + ":" + suffix
}

// Publish emits one ScheduledMessageSent envelope on the event bus.
func Publish(ctx context.Context, bus eventbus.Bus, log *slog.Logger, m event.ScheduledMessageSent) {
	if bus == nil {
		return
	}
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	envBytes, err := json.Marshal(event.Envelope{
		Type:    event.TypeScheduledMessageSent,
		GuildID: m.GuildID,
		TS:      time.Now().UnixMilli(),
		Data:    data,
	})
	if err != nil {
		return
	}
	subject := event.Subject(event.TypeScheduledMessageSent, m.GuildID)
	dedup := fmt.Sprintf("sched:%d:%d", m.ScheduleID, time.Now().Unix()/60)
	if err := bus.Publish(ctx, subject, envBytes, dedup); err != nil {
		log.Warn("scheduler: publish failed", "subject", subject, "err", err)
	}
}
