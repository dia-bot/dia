package moderation

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	runner "github.com/dia-bot/dia/internal/features/automations/runner"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
)

// escalate records the points awarded by a hit and, if the user crosses one or
// more tier thresholds, applies the highest newly-crossed tier exactly once. It
// returns the escalation action applied ("" if none) and the user's active
// point total after this hit.
//
// Thresholds fire as the user *crosses* them: a tier T fires when
// totalBefore < T.Points <= totalAfter, so each rung triggers once on the way
// up and re-crossing requires the points to decay below it and climb again.
func escalate(ctx context.Context, d plugin.Deps, h hitContext, esc Escalation, pointsThisHit int) (string, int) {
	if pointsThisHit <= 0 {
		return "", 0
	}

	uid, _ := event.ParseID(h.User.ID)
	now := time.Now()

	decay := esc.DecayHours
	if decay <= 0 {
		decay = 24
	}
	expires := now.Add(time.Duration(decay) * time.Hour)

	var chID *int64
	if h.ChannelID != "" {
		if c, ok := event.ParseID(h.ChannelID); ok {
			chID = &c
		}
	}

	if _, err := d.Store.Infractions.Add(ctx, store.AutomodInfraction{
		GuildID:     h.GuildID,
		UserID:      uid,
		RuleID:      h.Rule.ID,
		RuleName:    h.Rule.Name,
		TriggerType: h.Trigger.Type,
		Points:      pointsThisHit,
		Reason:      h.Reason,
		ChannelID:   chID,
		ExpiresAt:   &expires,
	}); err != nil {
		d.Log.Warn("automod: add infraction failed", "err", err)
		return "", 0
	}

	totalAfter, err := d.Store.Infractions.ActivePoints(ctx, h.GuildID, uid, now)
	if err != nil {
		d.Log.Warn("automod: active points failed", "err", err)
		return "", pointsThisHit
	}
	if !esc.Enabled || len(esc.Tiers) == 0 {
		return "", totalAfter
	}
	totalBefore := totalAfter - pointsThisHit

	tiers := append([]EscalationTier(nil), esc.Tiers...)
	sort.SliceStable(tiers, func(i, j int) bool { return tiers[i].Points < tiers[j].Points })

	// Highest tier whose threshold sits in (totalBefore, totalAfter].
	var crossed *EscalationTier
	for i := range tiers {
		t := tiers[i]
		if t.Points > totalBefore && t.Points <= totalAfter {
			crossed = &tiers[i]
		}
	}
	if crossed == nil {
		return "", totalAfter
	}

	applyEscalationTier(ctx, d, h, *crossed, totalAfter, pointsThisHit)
	return crossed.Action, totalAfter
}

// applyEscalationTier performs the heavier cross-rule action and records its
// case (moderator_id 0 = automod escalation). total/pointsThisHit are the user's
// active total and this hit's points, exposed to a run_automation tier's flow.
func applyEscalationTier(ctx context.Context, d plugin.Deps, h hitContext, tier EscalationTier, total, pointsThisHit int) {
	guildID := event.FormatID(h.GuildID)
	reason := "[Automod] Escalation threshold reached (" + h.Rule.Name + ")"
	switch tier.Action {
	case "run_automation":
		runEscalationAutomation(ctx, d, h, tier, total, pointsThisHit)
	case "timeout":
		secs := tier.Duration
		if secs <= 0 {
			secs = 600
		}
		until := time.Now().Add(time.Duration(secs) * time.Second)
		if err := d.Discord.Timeout(guildID, h.User.ID, &until, reason); err != nil {
			d.Log.Warn("automod: escalation timeout failed", "user", h.User.ID, "err", err)
			return
		}
		recordCase(ctx, d, h, "timeout", reason, secs, &until)
	case "kick":
		if err := d.Discord.Kick(guildID, h.User.ID, reason); err != nil {
			d.Log.Warn("automod: escalation kick failed", "user", h.User.ID, "err", err)
			return
		}
		recordCase(ctx, d, h, "kick", reason, 0, nil)
	case "ban":
		if err := d.Discord.Ban(guildID, h.User.ID, reason, 0); err != nil {
			d.Log.Warn("automod: escalation ban failed", "user", h.User.ID, "err", err)
			return
		}
		recordCase(ctx, d, h, "ban", reason, 0, nil)
	}
}

// runEscalationAutomation launches the tier's chosen automation flow as a durable
// run when a member crosses a run_automation tier. The scope mirrors the
// automod_action automation trigger (same .User / .Member / .Event vars) so a flow
// authored for "Automod action taken" behaves identically when wired to a tier.
// The flow itself owns whatever it does (DM, log, role, even a punishment step),
// so no moderation case is recorded here.
func runEscalationAutomation(ctx context.Context, d plugin.Deps, h hitContext, tier EscalationTier, total, pointsThisHit int) {
	runAutomationByID(ctx, d, h, tier.Automation, "automod_escalation", map[string]any{
		"rule_id":      h.Rule.ID,
		"rule_name":    h.Rule.Name,
		"trigger_type": h.Trigger.Type,
		"reason":       h.Reason,
		"points":       pointsThisHit,
		"total_points": total,
		"tier_points":  tier.Points,
		"escalated":    tier.Action,
		"content":      truncate(h.Content, 300),
		"message_id":   h.MessageID,
		"channel_id":   h.ChannelID,
	})
}

// runAutomationByID launches a saved automation as a durable run, building the
// scope (.User / .Member / .Guild / .Channel + the given .Event map) exactly as
// the automations runtime does so a flow behaves identically whether it's wired
// to a trigger or launched here. Shared by escalation tiers and the rule
// "run_automation" action. Returns true if a run was started.
func runAutomationByID(ctx context.Context, d plugin.Deps, h hitContext, automationID, triggerKind string, eventMap map[string]any) bool {
	id := strings.TrimSpace(automationID)
	if id == "" {
		return false
	}
	auto, err := d.Store.Automations.Get(ctx, h.GuildID, id)
	if err != nil {
		d.Log.Warn("automod: automation lookup failed", "automation", id, "err", err)
		return false
	}
	if !auto.Enabled {
		return false
	}
	var def cc.Definition
	if err := json.Unmarshal(auto.Definition, &def); err != nil {
		d.Log.Warn("automod: automation decode failed", "automation", id, "err", err)
		return false
	}

	guildID := event.FormatID(h.GuildID)
	guildCtx := cc.ContextGuild{ID: guildID, Name: h.GuildName}
	ctxVars := cc.BuildContext(guildID, h.ChannelID, h.User, h.Member, guildCtx, time.Now().UnixMilli())
	scope := cc.NewScope(d.GuildState, guildID, ctxVars, nil, automationVarDefaults(&def))
	scope.SetEvent(eventMap)
	runner.New(d).Start(ctx, runner.Meta{
		AutomationID: auto.ID,
		Version:      auto.Version,
		GuildID:      guildID,
		InvokerID:    h.User.ID,
		ActorID:      h.User.ID,
		ChannelID:    h.ChannelID,
		TriggerKind:  triggerKind,
	}, def, scope)
	return true
}

// automationVarDefaults seeds the flow's declared-variable defaults into the run,
// mirroring the automations runtime (runtime.defaultVars) so a flow launched from
// a tier behaves exactly as it would under a real "Automod action taken" trigger.
func automationVarDefaults(def *cc.Definition) map[string]any {
	out := map[string]any{}
	for _, v := range def.Variables {
		if len(v.Default) == 0 {
			continue
		}
		var val any
		_ = json.Unmarshal(v.Default, &val)
		out[v.Name] = val
	}
	return out
}
