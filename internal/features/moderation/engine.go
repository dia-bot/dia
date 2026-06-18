package moderation

import (
	"context"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// modPermBits are the permissions that mark a member as staff (exempt when
// IgnoreMods is on): manage messages, kick, ban, moderate members, administrator.
const modPermBits = discordgo.PermissionManageMessages |
	discordgo.PermissionKickMembers |
	discordgo.PermissionBanMembers |
	discordgo.PermissionModerateMembers |
	discordgo.PermissionAdministrator

// handleAutomodMessage screens a MESSAGE_CREATE / MESSAGE_UPDATE against the
// guild's automod rules. The first message-trigger rule that fires wins: its
// actions run, escalation is applied, the event is emitted, and evaluation
// stops for this message.
func handleAutomodMessage(ctx context.Context, d plugin.Deps, env *event.Envelope, isEdit bool) error {
	msg, err := plugin.DecodeData[event.Message](env)
	if err != nil {
		return err
	}
	if msg.GuildID == "" {
		return nil
	}

	gid, ok := event.ParseID(msg.GuildID)
	if !ok {
		return nil
	}
	cfg, enabled, err := plugin.LoadConfig[AutomodConfig](ctx, d, gid, AutomodKey)
	if err != nil || !enabled {
		return err
	}
	if cfg.IgnoreBots && msg.Author.Bot {
		return nil
	}

	// Channel exemption is cheap; check before anything else.
	if contains(cfg.ExemptChannels, msg.ChannelID) {
		return nil
	}
	var roles []string
	if msg.Member != nil {
		roles = msg.Member.Roles
	}
	if globallyExempt(ctx, d, cfg, msg.GuildID, roles) {
		return nil
	}

	in := scanInput{
		GuildID:   msg.GuildID,
		UserID:    msg.Author.ID,
		Username:  msg.Author.Username,
		Content:   msg.Content,
		Mentions:  msg.Mentions,
		Everyone:  msg.MentionEveryone,
		RolePings: msg.MentionRoles,
		Attach:    msg.AttachmentCount,
		Cache:     d.Cache,
		Ctx:       ctx,
	}
	if msg.Member != nil {
		in.Nick = msg.Member.Nick
	}

	for _, rule := range cfg.Rules {
		if !rule.Enabled || !IsMessageTrigger(rule.Trigger.Type) {
			continue
		}
		if ruleExempt(rule, msg.ChannelID, roles) {
			continue
		}
		// Rate triggers (spam/duplicates) carry inherent side effects, but the
		// global gate is already past, so evaluating them here is correct.
		reason, hit := detect(in, rule.Trigger)
		if !hit {
			continue
		}
		h := hitContext{
			GuildID:   gid,
			GuildName: guildName(ctx, d, msg.GuildID),
			Rule:      rule,
			Trigger:   rule.Trigger,
			Reason:    reason,
			User:      msg.Author,
			Member:    msg.Member,
			ChannelID: msg.ChannelID,
			MessageID: msg.ID,
			Content:   msg.Content,
			OnMessage: true,
			modCfg:    loadModConfig(ctx, d, gid),
		}
		runHit(ctx, d, cfg, h)
		return nil
	}
	return nil
}

// handleAutomodMember screens member identity triggers. On MEMBER_ADD it runs
// account_age + name; on MEMBER_UPDATE only name. First match wins.
func handleAutomodMember(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	var (
		gidStr string
		member event.Member
	)
	switch env.Type {
	case event.TypeMemberAdd:
		p, err := plugin.DecodeData[event.MemberAdd](env)
		if err != nil {
			return err
		}
		gidStr, member = p.GuildID, p.Member
	case event.TypeMemberUpdate:
		p, err := plugin.DecodeData[event.MemberUpdate](env)
		if err != nil {
			return err
		}
		gidStr, member = p.GuildID, p.Member
	default:
		return nil
	}
	if gidStr == "" {
		return nil
	}
	gid, ok := event.ParseID(gidStr)
	if !ok {
		return nil
	}
	cfg, enabled, err := plugin.LoadConfig[AutomodConfig](ctx, d, gid, AutomodKey)
	if err != nil || !enabled {
		return err
	}
	if cfg.IgnoreBots && member.User.Bot {
		return nil
	}

	// Anti-raid runs on join before (and independent of) the per-rule loop and
	// the exemption gate, so a raid of fresh accounts can't slip through role
	// exemptions. It is a no-op unless cfg.Raid.Enabled.
	if env.Type == event.TypeMemberAdd {
		raidCheck(ctx, d, cfg, gid, gidStr, member, cfg.Raid)
	}

	if globallyExempt(ctx, d, cfg, gidStr, member.Roles) {
		return nil
	}

	in := scanInput{
		GuildID:  gidStr,
		UserID:   member.User.ID,
		Username: member.User.Username,
		Nick:     member.Nick,
	}

	for _, rule := range cfg.Rules {
		if !rule.Enabled || !IsMemberTrigger(rule.Trigger.Type) {
			continue
		}
		// account_age only makes sense on join.
		if rule.Trigger.Type == TriggerAccountAge && env.Type != event.TypeMemberAdd {
			continue
		}
		if ruleExempt(rule, "", member.Roles) {
			continue
		}
		reason, hit := detect(in, rule.Trigger)
		if !hit {
			continue
		}
		h := hitContext{
			GuildID:   gid,
			GuildName: guildName(ctx, d, gidStr),
			Rule:      rule,
			Trigger:   rule.Trigger,
			Reason:    reason,
			User:      member.User,
			Member:    &member,
			OnMessage: false,
			modCfg:    loadModConfig(ctx, d, gid),
		}
		runHit(ctx, d, cfg, h)
		return nil
	}
	return nil
}

// runHit applies the matched rule's actions, then escalation, then emits.
func runHit(ctx context.Context, d plugin.Deps, cfg AutomodConfig, h hitContext) {
	applied, points := applyActions(ctx, d, h)

	escAction, total := "", 0
	if points > 0 {
		escAction, total = escalate(ctx, d, h, cfg.Escalation, points)
	}

	res := emitResult{Applied: applied, Points: points, TotalPoints: total, Escalated: escAction}
	emit(ctx, d, h, cfg, res)
}

// ── Exemption helpers ────────────────────────────────────────

// globallyExempt applies the guild-wide exemptions: exempt roles and (when
// IgnoreMods is on) members whose roles grant moderation permissions. The mod
// check is best-effort via the cached guild snapshot; if no snapshot is
// available it degrades to role-based exemptions only.
func globallyExempt(ctx context.Context, d plugin.Deps, cfg AutomodConfig, guildID string, roles []string) bool {
	if intersects(cfg.ExemptRoles, roles) {
		return true
	}
	if cfg.IgnoreMods && memberIsMod(ctx, d, guildID, roles) {
		return true
	}
	return false
}

// ruleExempt applies the per-rule exemptions (merged with, not replacing, the
// global ones already checked).
func ruleExempt(rule AutomodRule, channelID string, roles []string) bool {
	if channelID != "" && contains(rule.Exempt.Channels, channelID) {
		return true
	}
	return intersects(rule.Exempt.Roles, roles)
}

// memberIsMod reports whether any of the member's roles grants a moderation
// permission, using the cached guild snapshot's role permission bitfields.
func memberIsMod(ctx context.Context, d plugin.Deps, guildID string, roles []string) bool {
	if d.GuildState == nil || len(roles) == 0 {
		return false
	}
	snap, err := d.GuildState.Snapshot(ctx, guildID)
	if err != nil {
		return false
	}
	want := map[string]bool{}
	for _, r := range roles {
		want[r] = true
	}
	for _, role := range snap.Roles {
		if !want[role.ID] {
			continue
		}
		perms, err := strconv.ParseInt(strings.TrimSpace(role.Permissions), 10, 64)
		if err != nil {
			continue
		}
		if perms&modPermBits != 0 {
			return true
		}
	}
	return false
}

// guildName resolves the guild's display name from the cached snapshot (best
// effort; "" when unavailable).
func guildName(ctx context.Context, d plugin.Deps, guildID string) string {
	if d.GuildState == nil {
		return ""
	}
	snap, err := d.GuildState.Snapshot(ctx, guildID)
	if err != nil {
		return ""
	}
	return snap.Meta.Name
}

// loadModConfig loads the moderation feature config (DMOnAction, LogChannel)
// used by automod actions and the log embed.
func loadModConfig(ctx context.Context, d plugin.Deps, gid int64) Config {
	cfg, _, _ := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	return cfg
}

// contains reports whether needle is in haystack.
func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// intersects reports whether a and b share any element.
func intersects(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := set[s]; ok {
			return true
		}
	}
	return false
}
