// Package runtime is the worker-side glue for the automations feature: it
// subscribes to the gateway events the trigger catalogue needs, matches enabled
// automations (trigger transition + filters), builds an event-scoped run scope,
// and walks the step program on the shared customcommands exec engine. Durable
// steps (wait / wait_for) persist to automation_runs and resume via the
// component/modal intercepts and a scheduler worker — mirroring
// customcommands/runtime, but keyed on server events instead of slash
// invocations.
package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/customcommands/exec"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/dia-bot/dia/internal/features/schedmessages"
	"github.com/dia-bot/dia/internal/features/socialnotifications"
	"github.com/dia-bot/dia/internal/features/statschannels"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
)

// Component custom_id namespace for automation-sent messages, so clicks route
// back to this plugin's resume handler (distinct from custom commands' "ccmd:").
const (
	routePrefix = "auto:"
	noopPrefix  = "auto:noop:"
)

// Plugin is the automations feature.
type Plugin struct {
	deps plugin.Deps
	eng  *exec.Engine
}

// New returns the plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         automations.FeatureKey,
		Name:        "Automations",
		Description: "Run step flows when things happen on your server: joins, leaves, messages, reactions, role changes, voice and more.",
		Category:    plugin.CategoryUtility,
	}
}

// Init subscribes to the trigger events, wires the component/modal intercepts
// for wait_for resume, and starts the durable-wait scheduler.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.deps = d
	p.eng = exec.New(exec.Deps{
		Log:       d.Log,
		Discord:   &exec.DiscordAdapter{C: d.Discord},
		Store:     &exec.StoreAdapter{S: d.Store},
		Imaging:   &exec.ImagingAdapter{R: d.Imaging},
		HTTP:      &exec.HTTPAdapter{Client: &http.Client{Timeout: 10 * time.Second}},
		Giveaways: giveaway.NewManager(d),
	})
	p.eng.SetRouting(routePrefix, noopPrefix)
	// Event runs have no interaction keeping them "live", so cap every wait_for
	// / modal listening window at one minute.
	p.eng.SetMaxWaitFor(time.Minute)

	for _, et := range automations.SubscribedEvents() {
		et := et
		reg.OnEvent(et, func(ctx context.Context, env *event.Envelope) error {
			return p.handleEvent(ctx, et, env)
		})
	}
	reg.Component(routePrefix, p.handleResumeComponent)
	reg.Modal(routePrefix, p.handleResumeModal)
	reg.Worker("automations-scheduler", p.runScheduler)
	return nil
}

// ── Event dispatch ──────────────────────────────────────────────────────────

func (p *Plugin) handleEvent(ctx context.Context, et event.Type, env *event.Envelope) error {
	gid, ok := event.ParseID(env.GuildID)
	if !ok {
		return nil
	}
	// Each automation carries its own enabled flag (the indexed query below
	// filters on it), so that per-automation toggle is the control — there is
	// no separate master switch to forget to flip after creating one.
	autos, err := p.deps.Store.Automations.ListEnabledByEvent(ctx, gid, string(et))
	if err != nil {
		return err
	}

	// Message / reaction events can also RESUME runs parked on a wait_for
	// (e.g. "wait for the member's reply"). Look those up alongside triggers.
	waitKind := resumeWaitKind(et)
	var waiting []store.AutomationRun
	if waitKind != "" {
		waiting, _ = p.deps.Store.AutomationRuns.FindWaitingByKind(ctx, gid, waitKind)
	}

	// The giveaway feature's two built-in automations carry editable follow-up
	// tails (stored on its config) that run alongside any user-authored automations
	// on their respective events: "Draw giveaway winners" on giveaway_ended and "On
	// giveaway entry" on giveaway_entered. Load whichever matches (only touches the
	// store on those two events) so an otherwise-empty dispatch still runs it.
	drawTail, entryTail := p.giveawayTails(ctx, et, gid)
	// The social and stats features' built-ins carry the same kind of editable
	// tail, run on SOCIAL_UPDATE / MEMBER_MILESTONE respectively.
	socialTail := p.socialTail(ctx, et, gid)
	milestoneTail := p.statsTail(ctx, et, gid)
	schedTail := p.schedTail(ctx, et, gid)

	if len(autos) == 0 && len(waiting) == 0 && len(drawTail) == 0 && len(entryTail) == 0 && len(socialTail) == 0 && len(milestoneTail) == 0 && len(schedTail) == 0 {
		return nil
	}

	ec, ok := p.prepare(ctx, et, env)
	if !ok {
		return nil
	}

	for _, a := range autos {
		cfg := automations.DecodeTriggerConfig(a.TriggerConfig)
		if !p.matches(ctx, a, cfg, ec) {
			continue
		}
		if !p.passCooldown(ctx, a.ID, cfg, ec) {
			continue
		}
		if err := p.run(ctx, a, ec); err != nil {
			p.deps.Log.Warn("automation run error", "automation", a.ID, "trigger", a.TriggerType, "err", err)
		}
	}

	if len(drawTail) > 0 {
		p.runBuiltinTail(ctx, "giveaway.ended", "giveaway_ended", drawTail, ec)
	}
	if len(entryTail) > 0 {
		p.runBuiltinTail(ctx, "giveaway.entry", "giveaway_entry", entryTail, ec)
	}
	if len(socialTail) > 0 {
		p.runBuiltinTail(ctx, "social.update", "social_update", socialTail, ec)
	}
	if len(milestoneTail) > 0 {
		p.runBuiltinTail(ctx, "stats.milestone", "member_milestone", milestoneTail, ec)
	}
	if len(schedTail) > 0 {
		p.runBuiltinTail(ctx, "scheduler.sent", "scheduled_message", schedTail, ec)
	}

	if len(waiting) > 0 {
		p.resumeWaits(ctx, waitKind, waiting, ec)
	}
	return nil
}

// resumeWaitKind maps an event to the wait_for trigger it can satisfy ("" = none).
func resumeWaitKind(et event.Type) string {
	switch et {
	case event.TypeMessageCreate:
		return "message"
	case event.TypeReactionAdd:
		return "reaction"
	}
	return ""
}

// resumeWaits resumes every parked run whose wait_for matches the incoming
// event: the right kind, the awaited user (if any), the channel scope, and (for
// reactions) the emoji. A run is claimed atomically so the scheduler's timeout
// path and this resume can't both fire.
func (p *Plugin) resumeWaits(ctx context.Context, kind string, waiting []store.AutomationRun, ec *eventContext) {
	actorID, _ := event.ParseID(ec.user.ID)
	payload := waitPayload(kind, ec)
	for _, r := range waiting {
		if r.AwaitingUserID != 0 && r.AwaitingUserID != actorID {
			continue
		}
		var def cc.Definition
		if json.Unmarshal(r.DefinitionSnapshot, &def) != nil {
			continue
		}
		var cursor []cc.CursorFrame
		if len(r.Cursor) > 0 {
			_ = json.Unmarshal(r.Cursor, &cursor)
		}
		branch, idx := exec.BranchAtCursor(def.Steps, cursor)
		if branch == nil || idx < 0 || idx >= len(branch) {
			continue
		}
		st := branch[idx]
		if st.Kind != cc.KindWaitFor || len(st.Spec) == 0 {
			continue
		}
		var ws cc.SpecWaitFor
		if json.Unmarshal(st.Spec, &ws) != nil || ws.Trigger != kind {
			continue
		}
		if !waitChannelMatches(ws, event.FormatID(r.ChannelID), ec.channelID) {
			continue
		}
		if kind == "reaction" && ws.Emoji != "" && !emojiMatches([]string{ws.Emoji}, ec.eventMap) {
			continue
		}
		claimed, err := p.deps.Store.AutomationRuns.ClaimResume(ctx, r.ID)
		if err != nil || !claimed {
			continue
		}
		p.resumeWithEvent(ctx, r, def, cursor, ws.Into, payload)
	}
}

// resumeWithEvent continues a parked run past its wait_for, injecting the event
// payload under the wait's `into` variable (and the legacy "trigger").
func (p *Plugin) resumeWithEvent(ctx context.Context, r store.AutomationRun, def cc.Definition, cursor []cc.CursorFrame, into string, payload map[string]any) {
	scope, err := cc.RestoreScope(p.deps.GuildState, event.FormatID(r.GuildID), r.Scope)
	if err != nil {
		_ = p.deps.Store.AutomationRuns.MarkComplete(ctx, r.ID, "failed", err.Error())
		return
	}
	scope.Set("trigger", payload)
	if into != "" {
		scope.Set(into, payload)
	}
	run := &exec.RunState{
		ID:                 r.ID,
		CommandID:          r.AutomationID,
		CommandVersion:     r.AutomationVersion,
		GuildID:            event.FormatID(r.GuildID),
		InvokerID:          event.FormatID(r.InvokerID),
		ChannelID:          event.FormatID(r.ChannelID),
		TriggerKind:        r.TriggerKind,
		DefinitionSnapshot: r.DefinitionSnapshot,
	}
	run.SetCursor(cursor)
	outcome, pause, runErr := p.eng.Resume(ctx, run, def, scope, cursor)
	if runErr != nil {
		p.deps.Log.Warn("automation wait resume", "run", r.ID, "err", runErr)
	}
	p.persistLogs(ctx, run)
	if pause != nil {
		_ = p.persistResume(ctx, run, scope, pause)
		return
	}
	_ = p.deps.Store.AutomationRuns.MarkComplete(ctx, r.ID, outcome.Status, outcome.Error)
}

// waitChannelMatches applies a wait_for's channel scope to an incoming event.
func waitChannelMatches(ws cc.SpecWaitFor, runChannelID, eventChannelID string) bool {
	switch ws.ChannelMode {
	case "current":
		return runChannelID != "" && eventChannelID == runChannelID
	case "only":
		return len(ws.Channels) == 0 || contains(ws.Channels, eventChannelID)
	case "except":
		return !contains(ws.Channels, eventChannelID)
	default: // "any" / unset
		return true
	}
}

// waitPayload builds the `.Vars.<into>` value handed to a resumed wait.
func waitPayload(kind string, ec *eventContext) map[string]any {
	msg, _ := ec.eventMap["message"].(map[string]any)
	switch kind {
	case "message":
		content, _ := ec.eventMap["content"].(string)
		return map[string]any{
			"kind":       "message",
			"id":         mapStr(msg, "id"),
			"content":    content,
			"channel_id": ec.channelID,
			"user_id":    ec.user.ID,
		}
	case "reaction":
		return map[string]any{
			"kind":       "reaction",
			"emoji":      ec.eventMap["emoji"],
			"emoji_id":   ec.eventMap["emoji_id"],
			"emoji_name": ec.eventMap["emoji_name"],
			"message_id": mapStr(msg, "id"),
			"channel_id": ec.channelID,
			"user_id":    ec.user.ID,
		}
	}
	return map[string]any{"kind": kind}
}

func mapStr(m map[string]any, k string) string {
	if m == nil {
		return ""
	}
	s, _ := m[k].(string)
	return s
}

// eventContext is the decoded, trigger-agnostic view of one gateway event:
// the actor, the channel, the `.Event.*` payload, and any computed transitions
// (role diff, voice join/leave/move) shared by every trigger derived from it.
type eventContext struct {
	guildID   string
	channelID string
	user      event.User
	member    *event.Member
	eventMap  map[string]any

	addedRoles   []string
	removedRoles []string
	voiceKind    string // "join" | "leave" | "move" | ""
}

// prepare decodes the envelope once into an eventContext. ok=false drops the
// event (malformed, or a no-op transition like a voice mute toggle).
func (p *Plugin) prepare(ctx context.Context, et event.Type, env *event.Envelope) (*eventContext, bool) {
	ec := &eventContext{guildID: env.GuildID, eventMap: map[string]any{}}
	switch et {
	case event.TypeMemberAdd:
		m, err := plugin.DecodeData[event.MemberAdd](env)
		if err != nil {
			return nil, false
		}
		ec.user = m.Member.User
		ec.member = &m.Member
		ec.eventMap = map[string]any{"member_count": m.MemberCount, "pending": m.Member.Pending}

	case event.TypeMemberRemove:
		m, err := plugin.DecodeData[event.MemberRemove](env)
		if err != nil {
			return nil, false
		}
		ec.user = m.User
		ec.eventMap = map[string]any{"member_count": m.MemberCount}

	case event.TypeMemberUpdate:
		m, err := plugin.DecodeData[event.MemberUpdate](env)
		if err != nil {
			return nil, false
		}
		ec.user = m.Member.User
		ec.member = &m.Member
		ec.addedRoles, ec.removedRoles = p.roleDiff(ctx, env.GuildID, m)
		ec.eventMap = map[string]any{
			"roles":         m.Member.Roles,
			"added_roles":   ec.addedRoles,
			"removed_roles": ec.removedRoles,
			"nick":          m.Member.Nick,
			"premium_since": m.Member.PremiumSince,
			"boosting":      m.Member.PremiumSince != "",
		}

	case event.TypeBanAdd, event.TypeBanRemove:
		b, err := plugin.DecodeData[event.BanEvent](env)
		if err != nil {
			return nil, false
		}
		ec.user = b.User

	case event.TypeAutomodAction:
		a, err := plugin.DecodeData[event.AutomodAction](env)
		if err != nil {
			return nil, false
		}
		ec.user = a.User
		ec.member = a.Member
		ec.channelID = a.ChannelID
		ec.eventMap = map[string]any{
			"rule_id":      a.RuleID,
			"rule_name":    a.RuleName,
			"trigger_type": a.TriggerType,
			"reason":       a.Reason,
			"points":       a.Points,
			"total_points": a.TotalPoints,
			"escalated":    a.Escalated,
			"content":      a.Content,
			"message_id":   a.MessageID,
			"channel_id":   a.ChannelID,
			"actions":      a.Actions,
		}

	case event.TypeLevelUp:
		l, err := plugin.DecodeData[event.LevelUp](env)
		if err != nil {
			return nil, false
		}
		ec.user = l.User
		ec.member = l.Member
		ec.channelID = l.ChannelID
		ec.eventMap = map[string]any{
			"level":      l.Level,
			"new_level":  l.NewLevel,
			"xp":         l.XP,
			"rank":       l.Rank,
			"channel_id": l.ChannelID,
		}

	case event.TypeReactionRolePick:
		r, err := plugin.DecodeData[event.ReactionRolePick](env)
		if err != nil {
			return nil, false
		}
		ec.user = r.Member.User
		ec.member = &r.Member
		ec.channelID = r.ChannelID
		ec.eventMap = map[string]any{
			"menu_id":    r.MenuID,
			"menu_title": r.MenuTitle,
			"mode":       r.Mode,
			"values":     r.Values,
			"added":      r.Added,
			"removed":    r.Removed,
		}

	case event.TypeGiveawayEnded:
		g, err := plugin.DecodeData[event.GiveawayEnded](env)
		if err != nil {
			return nil, false
		}
		ec.user = g.User // the first winner (zero value when nobody won)
		ec.member = g.Member
		ec.channelID = g.ChannelID
		ec.eventMap = map[string]any{
			"giveaway_id":  g.GiveawayID,
			"prize":        g.Prize,
			"host_id":      g.HostID,
			"winner_count": g.WinnerCount,
			"winner_ids":   g.WinnerIDs,
			"entry_count":  g.EntryCount,
			"rerolled":     g.Rerolled,
			"message_id":   g.MessageID,
			"channel_id":   g.ChannelID,
		}

	case event.TypeGiveawayEntered:
		g, err := plugin.DecodeData[event.GiveawayEntered](env)
		if err != nil {
			return nil, false
		}
		ec.user = g.User // the member who clicked Enter
		ec.member = g.Member
		ec.channelID = g.ChannelID
		ec.eventMap = map[string]any{
			"giveaway_id": g.GiveawayID,
			"prize":       g.Prize,
			"host_id":     g.HostID,
			"outcome":     g.Outcome,
			"entries":     g.Entries,
			"reason":      g.Reason,
			"entry_count": g.EntryCount,
			"message_id":  g.MessageID,
			"channel_id":  g.ChannelID,
		}

	case event.TypeScheduledMessageSent:
		m, err := plugin.DecodeData[event.ScheduledMessageSent](env)
		if err != nil {
			return nil, false
		}
		ec.channelID = m.ChannelID
		ec.eventMap = map[string]any{
			"schedule":   m.ScheduleID,
			"name":       m.Name,
			"channel_id": m.ChannelID,
			"message_id": m.MessageID,
		}

	case event.TypeMemberMilestone:
		m, err := plugin.DecodeData[event.MemberMilestone](env)
		if err != nil {
			return nil, false
		}
		ec.eventMap = map[string]any{
			"count":     m.Count,
			"step":      m.Step,
			"milestone": m.Reached,
		}

	case event.TypeSocialUpdate:
		s, err := plugin.DecodeData[event.SocialUpdate](env)
		if err != nil {
			return nil, false
		}
		ec.eventMap = map[string]any{
			"provider":     s.Provider,
			"kind":         s.Kind,
			"account":      s.AccountName,
			"account_id":   s.AccountID,
			"account_url":  s.AccountURL,
			"item_id":      s.ItemID,
			"title":        s.Title,
			"url":          s.URL,
			"description":  s.Description,
			"category":     s.Category,
			"started_at":   s.StartedAt,
			"subscription": s.SubscriptionID,
		}

	case event.TypeMessageCreate, event.TypeMessageUpdate:
		m, err := decodeMessage(et, env)
		if err != nil {
			return nil, false
		}
		ec.user = m.Author
		ec.member = m.Member
		ec.channelID = m.ChannelID
		ec.eventMap = map[string]any{
			"content": m.Content,
			"message": map[string]any{
				"id":               m.ID,
				"content":          m.Content,
				"channel_id":       m.ChannelID,
				"attachment_count": m.AttachmentCount,
				"mention_everyone": m.MentionEveryone,
			},
		}

	case event.TypeMessageDelete:
		m, err := plugin.DecodeData[event.MessageDelete](env)
		if err != nil {
			return nil, false
		}
		ec.channelID = m.ChannelID
		ec.eventMap = map[string]any{
			"content": "",
			"message": map[string]any{"id": m.ID, "channel_id": m.ChannelID},
		}

	case event.TypeReactionAdd, event.TypeReactionRemove:
		r, err := plugin.DecodeData[event.Reaction](env)
		if err != nil {
			return nil, false
		}
		ec.user = event.User{ID: r.UserID}
		if r.Member != nil {
			ec.user = r.Member.User
			ec.member = r.Member
		}
		ec.channelID = r.ChannelID
		ec.eventMap = map[string]any{
			"emoji":      reactionGlyph(r.Emoji),
			"emoji_id":   r.Emoji.ID,
			"emoji_name": r.Emoji.Name,
			"message":    map[string]any{"id": r.MessageID, "channel_id": r.ChannelID},
		}

	case event.TypeVoiceStateUpdate:
		vs, err := plugin.DecodeData[event.VoiceState](env)
		if err != nil {
			return nil, false
		}
		ec.user = event.User{ID: vs.UserID}
		if vs.Member != nil {
			ec.user = vs.Member.User
			ec.member = vs.Member
		}
		prev := p.voicePrev(ctx, env.GuildID, vs.UserID)
		p.voiceStore(ctx, env.GuildID, vs.UserID, vs.ChannelID)
		ec.voiceKind = voiceTransition(prev, vs.ChannelID)
		if ec.voiceKind == "" {
			return nil, false // mute/deafen/video toggle — no channel transition
		}
		if vs.ChannelID != "" {
			ec.channelID = vs.ChannelID
		} else {
			ec.channelID = prev
		}
		ec.eventMap = map[string]any{
			"channel_id":     vs.ChannelID,
			"old_channel_id": prev,
			"self_mute":      vs.SelfMute,
			"self_deaf":      vs.SelfDeaf,
			"self_video":     vs.SelfVideo,
			"self_stream":    vs.Stream,
		}

	case event.TypeVerificationPassed:
		v, err := plugin.DecodeData[event.VerificationPassed](env)
		if err != nil {
			return nil, false
		}
		ec.user = v.User
		ec.member = v.Member
		ec.channelID = v.ChannelID
		ec.eventMap = map[string]any{"mode": v.Mode, "channel_id": v.ChannelID}

	case event.TypeVerificationFailed:
		v, err := plugin.DecodeData[event.VerificationFailed](env)
		if err != nil {
			return nil, false
		}
		ec.user = v.User
		ec.member = v.Member
		ec.eventMap = map[string]any{"reason": v.Reason, "kicked": v.Kicked}

	case event.TypeRaidAlert:
		r, err := plugin.DecodeData[event.RaidAlert](env)
		if err != nil {
			return nil, false
		}
		ec.eventMap = map[string]any{
			"active":    r.Active,
			"joins":     r.Joins,
			"threshold": r.Threshold,
			"window":    r.Window,
			"action":    r.Action,
		}

	case event.TypeModerationAction:
		a, err := plugin.DecodeData[event.ModerationAction](env)
		if err != nil {
			return nil, false
		}
		ec.user = a.User
		modName := a.Moderator.GlobalName
		if modName == "" {
			modName = a.Moderator.Username
		}
		ec.eventMap = map[string]any{
			"action":           a.Action,
			"reason":           a.Reason,
			"moderator_id":     a.Moderator.ID,
			"moderator_name":   modName,
			"case_number":      a.CaseNumber,
			"duration_seconds": a.DurationSeconds,
		}

	case event.TypeTicketOpened, event.TypeTicketClaimed, event.TypeTicketClosed,
		event.TypeTicketCloseRequested, event.TypeTicketReopened, event.TypeTicketRated:
		tk, err := plugin.DecodeData[event.TicketEvent](env)
		if err != nil {
			return nil, false
		}
		ec.user = tk.User
		ec.member = tk.Member
		ec.channelID = tk.ChannelID
		ec.eventMap = map[string]any{
			"ticket_id":      tk.TicketID,
			"number":         tk.Number,
			"panel_id":       tk.PanelID,
			"category_id":    tk.CategoryID,
			"category_label": tk.CategoryLabel,
			"subject":        tk.Subject,
			"channel_id":     tk.ChannelID,
			"actor_id":       tk.ActorID,
			"claimed_by":     tk.ClaimedBy,
			"closed_by":      tk.ClosedBy,
			"reason":         tk.Reason,
			"rating":         tk.Rating,
		}

	case event.TypeChannelCreate, event.TypeChannelDelete, event.TypeThreadCreate:
		ce, err := plugin.DecodeData[event.ChannelEvent](env)
		if err != nil {
			return nil, false
		}
		ec.channelID = ce.ID
		ec.eventMap = map[string]any{
			"channel": map[string]any{
				"id":        ce.ID,
				"name":      ce.Name,
				"type":      ce.Type,
				"parent_id": ce.ParentID,
				"topic":     ce.Topic,
			},
		}

	default:
		return nil, false
	}
	return ec, true
}

// decodeMessage decodes MESSAGE_CREATE / MESSAGE_UPDATE into event.Message
// (MessageUpdate embeds Message, so the same shape decodes both).
func decodeMessage(et event.Type, env *event.Envelope) (event.Message, error) {
	var m event.Message
	err := json.Unmarshal(env.Data, &m)
	return m, err
}

// ── Trigger matching ────────────────────────────────────────────────────────

func (p *Plugin) matches(ctx context.Context, a store.Automation, cfg automations.TriggerConfig, ec *eventContext) bool {
	// Transition gating (multiple triggers share one gateway event).
	switch a.TriggerType {
	case "role_added":
		if !roleChanged(ec.addedRoles, cfg.Role) {
			return false
		}
	case "role_removed":
		if !roleChanged(ec.removedRoles, cfg.Role) {
			return false
		}
	case "voice_join":
		if ec.voiceKind != "join" {
			return false
		}
	case "voice_leave":
		if ec.voiceKind != "leave" {
			return false
		}
	case "voice_move":
		if ec.voiceKind != "move" {
			return false
		}
	}

	// Generic filters (only those the editor set; absent slices are no-ops).
	if cfg.IgnoreBots && ec.user.Bot {
		return false
	}
	if len(cfg.Channels) > 0 && !contains(cfg.Channels, ec.channelID) {
		return false
	}
	if len(cfg.IgnoreChannels) > 0 && contains(cfg.IgnoreChannels, ec.channelID) {
		return false
	}
	if len(cfg.Roles) > 0 && !memberHasAny(ec.member, cfg.Roles) {
		return false
	}
	if len(cfg.IgnoreRoles) > 0 && memberHasAny(ec.member, cfg.IgnoreRoles) {
		return false
	}
	if len(cfg.Emojis) > 0 && !emojiMatches(cfg.Emojis, ec.eventMap) {
		return false
	}
	if len(cfg.Keywords) > 0 && !keywordMatches(cfg, contentOf(ec.eventMap)) {
		return false
	}
	// Social scoping: restrict to specific followed accounts and update kinds.
	if len(cfg.Subscriptions) > 0 && !contains(cfg.Subscriptions, socialSubOf(ec.eventMap)) {
		return false
	}
	if len(cfg.Kinds) > 0 {
		kind, _ := ec.eventMap["kind"].(string)
		if !contains(cfg.Kinds, kind) {
			return false
		}
	}
	// Scheduling scoping: restrict to specific schedules.
	if len(cfg.Schedules) > 0 && !contains(cfg.Schedules, int64Of(ec.eventMap, "schedule")) {
		return false
	}
	return true
}

// int64Of reads an int64-ish event value as its decimal string.
func int64Of(m map[string]any, key string) string {
	switch v := m[key].(type) {
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case string:
		return v
	}
	return ""
}

// socialSubOf reads the social subscription id out of a social_update event
// scope as the decimal string the trigger config stores.
func socialSubOf(m map[string]any) string { return int64Of(m, "subscription") }

// passCooldown enforces an optional per-scope rate limit via the cache (SET NX).
func (p *Plugin) passCooldown(ctx context.Context, autoID string, cfg automations.TriggerConfig, ec *eventContext) bool {
	if cfg.Cooldown == nil || cfg.Cooldown.Seconds <= 0 || p.deps.Cache == nil {
		return true
	}
	owner := ec.guildID
	switch cfg.Cooldown.Scope {
	case "user":
		owner = "u" + ec.user.ID
	case "channel":
		owner = "c" + ec.channelID
	}
	key := "auto:cd:" + autoID + ":" + owner
	ok, err := p.deps.Cache.Reserve(ctx, key, time.Duration(cfg.Cooldown.Seconds)*time.Second)
	if err != nil {
		return true // fail open: a cache hiccup shouldn't silently mute automations
	}
	return ok
}

// ── Run execution ───────────────────────────────────────────────────────────

func (p *Plugin) run(ctx context.Context, a store.Automation, ec *eventContext) error {
	var def cc.Definition
	if err := json.Unmarshal(a.Definition, &def); err != nil {
		return fmt.Errorf("decode definition: %w", err)
	}

	guildCtx := p.guildContext(ctx, ec.guildID)
	ctxVars := cc.BuildContext(ec.guildID, ec.channelID, ec.user, ec.member, guildCtx, time.Now().UnixMilli())
	scope := cc.NewScope(p.deps.GuildState, ec.guildID, ctxVars, nil, defaultVars(&def))
	scope.SetEvent(ec.eventMap)

	uid, _ := event.ParseID(ec.user.ID)
	run := &exec.RunState{
		ID:                 newULID(),
		CommandID:          a.ID, // KV scope + automation_runs.automation_id
		CommandVersion:     a.Version,
		GuildID:            ec.guildID,
		InvokerID:          ec.user.ID,
		ActorID:            ec.user.ID,
		ChannelID:          ec.channelID,
		TriggerKind:        a.TriggerType,
		DefinitionSnapshot: a.Definition,
	}
	_ = uid

	outcome, pause, runErr := p.eng.Run(ctx, run, def, scope)
	if runErr != nil {
		p.deps.Log.Warn("automation engine error", "automation", a.ID, "err", runErr)
	}
	p.persistInitial(ctx, run, scope, pause, outcome)
	return nil
}

// giveawayTails returns the giveaway feature's built-in follow-up flows for the
// event being dispatched: the post-draw tail (Config.Tail) on giveaway_ended and
// the on-entry tail (Config.EntryTail) on giveaway_entered. Both are nil when the
// event is something else, the feature is off, or no tail is wired. It only reads
// the store on those two events, so every other event stays a cheap no-op.
func (p *Plugin) giveawayTails(ctx context.Context, et event.Type, gid int64) (draw, entry []cc.Step) {
	if et != event.TypeGiveawayEnded && et != event.TypeGiveawayEntered {
		return nil, nil
	}
	fc, err := p.deps.Store.Features.Get(ctx, gid, giveaway.FeatureKey)
	if err != nil || !fc.Enabled || len(fc.Config) == 0 {
		return nil, nil
	}
	var cfg giveaway.Config
	if json.Unmarshal(fc.Config, &cfg) != nil {
		return nil, nil
	}
	if et == event.TypeGiveawayEntered {
		return nil, cfg.EntryTail
	}
	return cfg.Tail, nil
}

// socialTail returns the social feature's built-in follow-up flow on
// SOCIAL_UPDATE (nil when the event is something else, the feature is off, or
// no tail is wired). Mirrors giveawayTails: it only reads the store on that
// one event, so every other event stays a cheap no-op.
func (p *Plugin) socialTail(ctx context.Context, et event.Type, gid int64) []cc.Step {
	if et != event.TypeSocialUpdate {
		return nil
	}
	fc, err := p.deps.Store.Features.Get(ctx, gid, socialnotifications.FeatureKey)
	if err != nil || !fc.Enabled || len(fc.Config) == 0 {
		return nil
	}
	var cfg socialnotifications.Config
	if json.Unmarshal(fc.Config, &cfg) != nil {
		return nil
	}
	return cfg.Tail
}

// statsTail returns the stats feature's built-in follow-up flow on
// MEMBER_MILESTONE (nil when the event is something else, the feature is off,
// or no tail is wired). Mirrors socialTail.
func (p *Plugin) statsTail(ctx context.Context, et event.Type, gid int64) []cc.Step {
	if et != event.TypeMemberMilestone {
		return nil
	}
	fc, err := p.deps.Store.Features.Get(ctx, gid, statschannels.FeatureKey)
	if err != nil || !fc.Enabled || len(fc.Config) == 0 {
		return nil
	}
	var cfg statschannels.Config
	if json.Unmarshal(fc.Config, &cfg) != nil {
		return nil
	}
	return cfg.Tail
}

// schedTail returns the scheduler feature's built-in follow-up flow on
// SCHEDULED_MESSAGE_SENT. Mirrors socialTail.
func (p *Plugin) schedTail(ctx context.Context, et event.Type, gid int64) []cc.Step {
	if et != event.TypeScheduledMessageSent {
		return nil
	}
	fc, err := p.deps.Store.Features.Get(ctx, gid, schedmessages.FeatureKey)
	if err != nil || !fc.Enabled || len(fc.Config) == 0 {
		return nil
	}
	var cfg schedmessages.Config
	if json.Unmarshal(fc.Config, &cfg) != nil {
		return nil
	}
	return cfg.Tail
}

// runBuiltinTail runs a managed feature's built-in follow-up flow as a durable
// automation run, reusing the same execution + persistence path as a
// user-authored automation. autoID mirrors the built-in's key (e.g.
// "giveaway.ended") so its runs share that flow's KV scope and show under the
// built-in in the Runs tab. The tail sees exactly the trigger's .Event scope
// (set on ec in prepare), so it behaves like a hand-built automation.
func (p *Plugin) runBuiltinTail(ctx context.Context, autoID, triggerType string, tail []cc.Step, ec *eventContext) {
	raw, err := json.Marshal(cc.Definition{Steps: tail})
	if err != nil {
		return
	}
	synth := store.Automation{
		ID:          autoID,
		Version:     1,
		TriggerType: triggerType,
		Definition:  raw,
	}
	if err := p.run(ctx, synth, ec); err != nil {
		p.deps.Log.Warn("builtin tail run error", "automation", autoID, "err", err)
	}
}

// RunAutomation runs a saved, enabled automation on demand — the cross-feature
// bridge (e.g. a giveaway action-button click) that satisfies
// giveaway.AutomationRunner. The caller supplies the actor, channel and .Event
// scope; a disabled automation is a silent no-op and a missing one is an error.
func (p *Plugin) RunAutomation(ctx context.Context, guildID, automationID string, user event.User, member *event.Member, channelID string, eventMap map[string]any) error {
	gid, ok := event.ParseID(guildID)
	if !ok {
		return fmt.Errorf("run automation: bad guild id %q", guildID)
	}
	a, err := p.deps.Store.Automations.Get(ctx, gid, automationID)
	if err != nil {
		return err
	}
	if !a.Enabled {
		return nil
	}
	if eventMap == nil {
		eventMap = map[string]any{}
	}
	ec := &eventContext{
		guildID:   guildID,
		channelID: channelID,
		user:      user,
		member:    member,
		eventMap:  eventMap,
	}
	return p.run(ctx, a, ec)
}

// persistInitial records the first execution of an automation: it inserts the
// run row (so the Runs tab and log rows have a parent), appends step logs, and
// either parks it (pause) or stamps the terminal outcome.
func (p *Plugin) persistInitial(ctx context.Context, run *exec.RunState, scope *cc.Scope, pause *exec.PauseError, outcome exec.Outcome) {
	gid, _ := event.ParseID(run.GuildID)
	uid, _ := event.ParseID(run.InvokerID)
	chID, _ := event.ParseID(run.ChannelID)
	scopeJSON, _ := scope.Marshal()

	row := store.AutomationRun{
		ID:                 run.ID,
		AutomationID:       run.CommandID,
		AutomationVersion:  run.CommandVersion,
		GuildID:            gid,
		InvokerID:          uid,
		ChannelID:          chID,
		TriggerKind:        run.TriggerKind,
		Scope:              scopeJSON,
		DefinitionSnapshot: run.DefinitionSnapshot,
		Status:             "running",
	}
	if pause != nil {
		cursorJSON, _ := json.Marshal(pause.Cursor)
		awUID, _ := event.ParseID(pause.AwaitingUserID)
		row.Cursor = cursorJSON
		row.Status = "waiting"
		row.ResumeAt = pause.ResumeAt
		row.AwaitingCustomID = pause.AwaitingCustomID
		row.AwaitingUserID = awUID
		row.AwaitingKind = pause.AwaitingKind
	} else {
		row.Status = outcome.Status
	}
	if err := p.deps.Store.AutomationRuns.Insert(ctx, row); err != nil {
		p.deps.Log.Debug("automation run insert", "err", err)
		return
	}
	p.persistLogs(ctx, run)
	if pause == nil {
		_ = p.deps.Store.AutomationRuns.MarkComplete(ctx, run.ID, outcome.Status, outcome.Error)
	}
}

func (p *Plugin) persistLogs(ctx context.Context, run *exec.RunState) {
	for _, l := range run.Logs() {
		if err := p.deps.Store.AutomationRuns.AppendLog(ctx, store.AutomationRunLog{
			RunID:      run.ID,
			StepID:     l.StepID,
			StepKind:   l.StepKind,
			CursorPath: l.CursorPath,
			DurationMs: l.DurationMs,
			Status:     l.Status,
			Input:      l.Input,
			Output:     l.Output,
			Error:      l.Error,
		}); err != nil {
			p.deps.Log.Debug("automation log write", "err", err)
		}
	}
}

// ── Component / modal resume (wait_for) ─────────────────────────────────────

func (p *Plugin) handleResumeComponent(c *interactions.Context) error {
	return p.resume(c, "component")
}
func (p *Plugin) handleResumeModal(c *interactions.Context) error { return p.resume(c, "modal") }

func (p *Plugin) resume(c *interactions.Context, kind string) error {
	cid := c.CustomID()
	if strings.HasPrefix(cid, noopPrefix) {
		return c.DeferUpdate()
	}
	run, err := p.deps.Store.AutomationRuns.FindWaitingForComponent(c.Ctx, cid)
	if errors.Is(err, store.ErrNotFound) {
		return c.RespondEphemeral("This interaction is no longer active.")
	}
	if err != nil {
		return err
	}
	if run.AwaitingUserID != 0 && c.User.ID != "" {
		if uid, _ := event.ParseID(c.User.ID); uid != run.AwaitingUserID {
			return c.RespondEphemeral("Only the original member can use this.")
		}
	}
	claimed, err := p.deps.Store.AutomationRuns.ClaimResume(c.Ctx, run.ID)
	if err != nil || !claimed {
		return c.RespondEphemeral("Already processed.")
	}

	var def cc.Definition
	if err := json.Unmarshal(run.DefinitionSnapshot, &def); err != nil {
		return err
	}
	// A DM interaction carries no guild, so fall back to the run's stored guild
	// (the scope's guild isn't persisted) — otherwise a DM-originated resume
	// loses guildstate-backed lookups. Channel clicks already carry it.
	scopeGuild := c.GuildID
	if scopeGuild == "" {
		scopeGuild = event.FormatID(run.GuildID)
	}
	scope, err := cc.RestoreScope(p.deps.GuildState, scopeGuild, run.Scope)
	if err != nil {
		return err
	}

	payload := map[string]any{"kind": kind, "custom_id": cid, "user_id": c.User.ID}
	if i := strings.LastIndex(cid, ":"); i >= 0 && i < len(cid)-1 {
		payload["id"] = cid[i+1:]
	}
	if kind == "component" {
		payload["values"] = c.ComponentValues()
	}
	if kind == "modal" {
		values := map[string]string{}
		for _, row := range c.I.Data.Components {
			for _, comp := range row.Components {
				values[comp.CustomID] = comp.Value
			}
		}
		payload["fields"] = values
	}
	scope.Set("trigger", payload)

	var cursor []cc.CursorFrame
	if len(run.Cursor) > 0 {
		_ = json.Unmarshal(run.Cursor, &cursor)
	}

	// Land the payload under the awaiting step's `into` name.
	awaitInto, mode := "", cc.ClickResponseReply
	branch, idx := exec.BranchAtCursor(def.Steps, cursor)
	if branch != nil && idx >= 0 && idx < len(branch) && len(branch[idx].Spec) > 0 {
		st := branch[idx]
		switch st.Kind {
		case cc.KindWaitFor:
			var ws cc.SpecWaitFor
			if json.Unmarshal(st.Spec, &ws) == nil {
				awaitInto = ws.Into
				if kind == "component" {
					suffix, _ := payload["id"].(string)
					mode = ws.ResponseFor(suffix)
				}
			}
		case cc.KindModalOpen:
			var ms cc.SpecModalOpen
			if json.Unmarshal(st.Spec, &ms) == nil {
				awaitInto = ms.Into
			}
		}
	}
	if awaitInto != "" {
		scope.Set(awaitInto, payload)
	}

	// Acknowledge the click per the wait_for's response mode.
	switch mode {
	case cc.ClickResponseSilent:
		_ = c.DeferUpdate()
		scope.MarkDeferred(true)
		scope.MarkReplied(true)
	case cc.ClickResponseUpdate:
		_ = c.DeferUpdate()
		scope.MarkDeferred(true)
		scope.MarkReplied(false)
	default:
		_ = c.Defer(false)
		scope.MarkDeferred(true)
		scope.MarkReplied(false)
	}

	resumeRun := &exec.RunState{
		ID:                 run.ID,
		CommandID:          run.AutomationID,
		CommandVersion:     run.AutomationVersion,
		GuildID:            event.FormatID(run.GuildID),
		InvokerID:          event.FormatID(run.InvokerID),
		ActorID:            c.User.ID,
		ChannelID:          event.FormatID(run.ChannelID),
		TriggerKind:        run.TriggerKind,
		InteractionID:      c.I.ID,
		InteractionToken:   c.I.Token,
		DefinitionSnapshot: run.DefinitionSnapshot,
	}
	resumeRun.SetCursor(cursor)

	outcome, pause, runErr := p.eng.Resume(c.Ctx, resumeRun, def, scope, cursor)
	if runErr != nil {
		p.deps.Log.Warn("automation resume error", "run", run.ID, "err", runErr)
	}
	p.persistLogs(c.Ctx, resumeRun)
	if pause != nil {
		_ = p.persistResume(c.Ctx, resumeRun, scope, pause)
		return nil
	}
	_ = p.deps.Store.AutomationRuns.MarkComplete(c.Ctx, run.ID, outcome.Status, outcome.Error)
	return nil
}

// ── Scheduler (resumes due waits) ───────────────────────────────────────────

func (p *Plugin) runScheduler(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.drainDueWaits(ctx)
		}
	}
}

func (p *Plugin) drainDueWaits(ctx context.Context) {
	runs, err := p.deps.Store.AutomationRuns.DueWaits(ctx, 20)
	if err != nil {
		p.deps.Log.Warn("automations: scan due waits", "err", err)
		return
	}
	for _, r := range runs {
		claimed, err := p.deps.Store.AutomationRuns.ClaimResume(ctx, r.ID)
		if err != nil || !claimed {
			continue
		}
		p.resumeWait(ctx, r)
	}
}

func (p *Plugin) resumeWait(ctx context.Context, r store.AutomationRun) {
	var def cc.Definition
	if err := json.Unmarshal(r.DefinitionSnapshot, &def); err != nil {
		_ = p.deps.Store.AutomationRuns.MarkComplete(ctx, r.ID, "failed", err.Error())
		return
	}
	scope, err := cc.RestoreScope(p.deps.GuildState, event.FormatID(r.GuildID), r.Scope)
	if err != nil {
		_ = p.deps.Store.AutomationRuns.MarkComplete(ctx, r.ID, "failed", err.Error())
		return
	}
	var cursor []cc.CursorFrame
	_ = json.Unmarshal(r.Cursor, &cursor)

	run := &exec.RunState{
		ID:                 r.ID,
		CommandID:          r.AutomationID,
		CommandVersion:     r.AutomationVersion,
		GuildID:            event.FormatID(r.GuildID),
		InvokerID:          event.FormatID(r.InvokerID),
		ChannelID:          event.FormatID(r.ChannelID),
		TriggerKind:        r.TriggerKind,
		InteractionID:      r.InteractionID,
		InteractionToken:   r.InteractionToken,
		InteractionExpires: r.InteractionExpires,
		DefinitionSnapshot: r.DefinitionSnapshot,
	}
	run.SetCursor(cursor)

	// An event wait reaching the scheduler means its deadline passed without the
	// event: run the on_timeout branch instead of the continuation.
	timedOut := r.AwaitingKind != ""
	var outcome exec.Outcome
	var pause *exec.PauseError
	if timedOut {
		outcome, pause, _ = p.eng.ResumeTimedOut(ctx, run, def, scope, cursor)
	} else {
		outcome, pause, _ = p.eng.Resume(ctx, run, def, scope, cursor)
	}
	p.persistLogs(ctx, run)
	if pause != nil {
		_ = p.persistResume(ctx, run, scope, pause)
		return
	}
	_ = p.deps.Store.AutomationRuns.MarkComplete(ctx, r.ID, outcome.Status, outcome.Error)
}

// persistResume writes the parked state for an already-inserted run row.
func (p *Plugin) persistResume(ctx context.Context, run *exec.RunState, scope *cc.Scope, pause *exec.PauseError) error {
	scopeJSON, _ := scope.Marshal()
	cursorJSON, _ := json.Marshal(pause.Cursor)
	awUID, _ := event.ParseID(pause.AwaitingUserID)
	return p.deps.Store.AutomationRuns.UpdateState(ctx, run.ID, scopeJSON, cursorJSON,
		"waiting", pause.ResumeAt, pause.AwaitingCustomID, awUID, pause.AwaitingKind)
}

// ── Scope / state helpers ───────────────────────────────────────────────────

func (p *Plugin) guildContext(ctx context.Context, guildID string) cc.ContextGuild {
	g := cc.ContextGuild{ID: guildID, Name: "the server"}
	gid, _ := event.ParseID(guildID)
	if row, err := p.deps.Store.Guilds.Get(ctx, gid); err == nil {
		if row.Name != "" {
			g.Name = row.Name
		}
		g.MemberCount = row.MemberCount
	}
	return g
}

func defaultVars(def *cc.Definition) map[string]any {
	out := map[string]any{}
	for _, v := range def.Variables {
		if len(v.Default) == 0 {
			continue
		}
		var any any
		_ = json.Unmarshal(v.Default, &any)
		out[v.Name] = any
	}
	return out
}

// roleDiff returns (added, removed) between a member's previous role snapshot
// (cache, falling back to the event's old_roles) and their new role set, and
// refreshes the snapshot.
func (p *Plugin) roleDiff(ctx context.Context, guildID string, m event.MemberUpdate) (added, removed []string) {
	uid := m.Member.User.ID
	prev := m.OldRoles
	if p.deps.Cache != nil && uid != "" {
		var cached []string
		if err := p.deps.Cache.GetJSON(ctx, rolesKey(guildID, uid), &cached); err == nil && cached != nil {
			prev = cached
		}
		_ = p.deps.Cache.SetJSON(ctx, rolesKey(guildID, uid), m.Member.Roles, 30*24*time.Hour)
	}
	prevSet := toSet(prev)
	newSet := toSet(m.Member.Roles)
	for r := range newSet {
		if !prevSet[r] {
			added = append(added, r)
		}
	}
	for r := range prevSet {
		if !newSet[r] {
			removed = append(removed, r)
		}
	}
	return added, removed
}

func (p *Plugin) voicePrev(ctx context.Context, guildID, userID string) string {
	if p.deps.Cache == nil || userID == "" {
		return ""
	}
	var ch string
	_ = p.deps.Cache.GetJSON(ctx, voiceKey(guildID, userID), &ch)
	return ch
}

func (p *Plugin) voiceStore(ctx context.Context, guildID, userID, channelID string) {
	if p.deps.Cache == nil || userID == "" {
		return
	}
	if channelID == "" {
		_ = p.deps.Cache.Delete(ctx, voiceKey(guildID, userID))
		return
	}
	_ = p.deps.Cache.SetJSON(ctx, voiceKey(guildID, userID), channelID, 24*time.Hour)
}

func rolesKey(guildID, userID string) string { return "auto:roles:" + guildID + ":" + userID }
func voiceKey(guildID, userID string) string { return "auto:voice:" + guildID + ":" + userID }

// ── small pure helpers ──────────────────────────────────────────────────────

func voiceTransition(prev, next string) string {
	switch {
	case prev == "" && next != "":
		return "join"
	case prev != "" && next == "":
		return "leave"
	case prev != "" && next != "" && prev != next:
		return "move"
	default:
		return ""
	}
}

func roleChanged(changed []string, watched string) bool {
	if len(changed) == 0 {
		return false
	}
	if watched == "" {
		return true
	}
	return contains(changed, watched)
}

func memberHasAny(m *event.Member, roles []string) bool {
	if m == nil {
		return false
	}
	set := toSet(m.Roles)
	for _, r := range roles {
		if set[r] {
			return true
		}
	}
	return false
}

func emojiMatches(allow []string, eventMap map[string]any) bool {
	name, _ := eventMap["emoji_name"].(string)
	id, _ := eventMap["emoji_id"].(string)
	glyph, _ := eventMap["emoji"].(string)
	for _, a := range allow {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if a == name || a == id || a == glyph {
			return true
		}
	}
	return false
}

func keywordMatches(cfg automations.TriggerConfig, content string) bool {
	if content == "" {
		return false
	}
	lc := strings.ToLower(content)
	for _, kw := range cfg.Keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw == "" {
			continue
		}
		switch cfg.MatchMode {
		case "equals":
			if lc == kw {
				return true
			}
		case "word":
			if containsWord(lc, kw) {
				return true
			}
		default: // contains
			if strings.Contains(lc, kw) {
				return true
			}
		}
	}
	return false
}

func containsWord(haystack, word string) bool {
	for _, f := range strings.FieldsFunc(haystack, func(r rune) bool {
		return !(r == '_' || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	}) {
		if f == word {
			return true
		}
	}
	return false
}

func reactionGlyph(e event.Emoji) string {
	if e.ID != "" {
		if e.Animated {
			return "<a:" + e.Name + ":" + e.ID + ">"
		}
		return "<:" + e.Name + ":" + e.ID + ">"
	}
	return e.Name
}

func contentOf(eventMap map[string]any) string {
	c, _ := eventMap["content"].(string)
	return c
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func toSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, v := range list {
		out[v] = true
	}
	return out
}

// newULID returns a sortable, opaque run id (mirrors the customcommands helper).
func newULID() string {
	ts := time.Now().UnixNano()
	return fmt.Sprintf("A%013xA%08x", ts/1_000_000, ts&0xFFFFFFFF)
}
