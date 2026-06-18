package moderation

import (
	"context"
	"sort"
	"time"

	"github.com/dia-bot/dia/internal/event"
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

	applyEscalationTier(ctx, d, h, *crossed)
	return crossed.Action, totalAfter
}

// applyEscalationTier performs the heavier cross-rule action and records its
// case (moderator_id 0 = automod escalation).
func applyEscalationTier(ctx context.Context, d plugin.Deps, h hitContext, tier EscalationTier) {
	guildID := event.FormatID(h.GuildID)
	reason := "[Automod] Escalation threshold reached (" + h.Rule.Name + ")"
	switch tier.Action {
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
