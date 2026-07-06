// Package leveling awards members XP for chatting, ranks them on a classic polynomial
// curve, renders rank cards, and grants configurable level-role rewards.
package leveling

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations/runner"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/internal/tmpllookup"
	"github.com/dia-bot/dia/pkg/discordgo"
)

const (
	// componentPrefix namespaces this feature's component clicks. A posted
	// level-up message mints custom_ids "leveling:<suffix>" so a click routes back
	// here (and never collides with custom-command runs). Leveling has a single
	// message surface (no join/DM tabs), so the prefix is flat.
	componentPrefix = "leveling:"

	// automationID is the stable durable-run label (and builtin Key) for the
	// level-up flow: both the post-announce tail and the per-button click actions
	// run under it, so they share the flow's KV scope and Runs filter.
	automationID = "leveling.levelup"
)

// Plugin implements the leveling feature.
type Plugin struct {
	// runner runs the durable flows leveling owns — the per-button click-action
	// programs (Config.Actions) and the post-announce tail (Config.Tail) — on the
	// shared automations machinery: it persists a parked run to automation_runs
	// and emits "auto:" components, so waits, modals and follow-up clicks resume
	// through the automations plugin's handlers + the wait scheduler.
	runner *runner.Runner
}

// New returns the leveling plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Leveling",
		Description: "Award XP for chatting, rank members, and grant level-role rewards.",
		Category:    plugin.CategoryEngagement,
	}
}

// Init wires the message XP handler, the leveling:* component handler that runs
// per-button click actions, and the /rank, /leaderboard and /level-rewards
// commands.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.runner = runner.New(d)

	reg.OnEvent(event.TypeMessageCreate, func(ctx context.Context, env *event.Envelope) error {
		return p.handleMessage(ctx, d, env)
	})

	// Clicks on a posted level-up message route here (custom_id
	// "leveling:<suffix>"); a wired Action runs as a durable flow, an unwired one
	// is acked silently. These buttons are persistent: the custom_id carries no
	// run reference, so they keep working for the life of the message, and each
	// click spins up a fresh run.
	reg.Component(componentPrefix, func(c *interactions.Context) error {
		return p.handleComponent(c, d)
	})

	reg.Command(&interactions.Command{
		Def: interactions.Slash("rank",
			"Show your (or another member's) rank card",
			interactions.UserOpt("user", "The member to look up", false),
		),
		Handler: func(c *interactions.Context) error { return handleRank(c, d) },
	})

	reg.Command(&interactions.Command{
		Def:     interactions.Slash("leaderboard", "Show the server XP leaderboard"),
		Handler: func(c *interactions.Context) error { return handleLeaderboard(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.AdminOnly(interactions.Slash("level-rewards",
			"Manage level-role rewards",
			interactions.SubCommand("add", "Grant a role when a member reaches a level",
				interactions.IntOpt("level", "The level to reward", true),
				interactions.RoleOpt("role", "The role to grant", true),
			),
			interactions.SubCommand("remove", "Remove the reward at a level",
				interactions.IntOpt("level", "The level whose reward to remove", true),
			),
			interactions.SubCommand("list", "List configured level-role rewards"),
		)),
		Handler: func(c *interactions.Context) error { return handleRewards(c, d) },
	})
	return nil
}

// ── XP earning ───────────────────────────────────────────────

func (p *Plugin) handleMessage(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	msg, err := plugin.DecodeData[event.Message](env)
	if err != nil {
		return err
	}
	if msg.Author.Bot || msg.GuildID == "" {
		return nil
	}

	gid, ok := event.ParseID(msg.GuildID)
	if !ok {
		return nil
	}
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return err
	}

	// Channel / role exclusions.
	if contains(cfg.NoXPChannels, msg.ChannelID) {
		return nil
	}
	if msg.Member != nil {
		for _, rid := range msg.Member.Roles {
			if contains(cfg.NoXPRoles, rid) {
				return nil
			}
		}
	}

	// Anti-spam cooldown: only earn once per cooldown window.
	if d.Cache != nil && cfg.CooldownSeconds > 0 {
		key := "lvl:cd:" + msg.GuildID + ":" + msg.Author.ID
		set, rerr := d.Cache.Reserve(ctx, key, time.Duration(cfg.CooldownSeconds)*time.Second)
		if rerr == nil && !set {
			return nil // still on cooldown
		}
	}

	uid, ok := event.ParseID(msg.Author.ID)
	if !ok {
		return nil
	}

	var memberRoles []string
	if msg.Member != nil {
		memberRoles = msg.Member.Roles
	}
	delta := xpDelta(cfg, memberRoles)
	updated, err := d.Store.Levels.AddXP(ctx, gid, uid, delta, time.Now())
	if err != nil {
		return err
	}

	prevLevel := updated.Level // the level stored before this message
	newLevel := LevelFromXP(updated.XP)
	if newLevel <= prevLevel {
		return nil
	}

	if err := d.Store.Levels.SetLevel(ctx, gid, uid, newLevel); err != nil {
		d.Log.Warn("leveling: set level failed", "err", err)
	}

	grantRewards(ctx, d, cfg, msg.GuildID, msg.Author.ID, newLevel)

	// The member's leaderboard position, best-effort (0 when the rank query
	// fails). It rides the announce scope, the durable tail/click flows and the
	// published LEVEL_UP event.
	rank, _ := d.Store.Levels.Rank(ctx, gid, uid)

	if cfg.AnnounceLevelUp {
		p.announce(ctx, d, cfg, msg, newLevel, rank, updated.XP)
	}
	// Publish LEVEL_UP so the automations runtime (and any level_up flow) can
	// react. Worker-published like AUTOMOD_ACTION: there is no gateway mapper.
	p.publishLevelUp(ctx, d, msg, newLevel, rank, updated.XP)
	return nil
}

// publishLevelUp wraps an event.LevelUp payload in an Envelope and publishes it
// on the LEVEL_UP subject for the guild, so the automations runtime can trigger
// the level_up flow. Best-effort; failures are logged.
func (p *Plugin) publishLevelUp(ctx context.Context, d plugin.Deps, msg event.Message, level, rank int, xp int64) {
	if d.Bus == nil {
		return
	}
	payload := event.LevelUp{
		GuildID:   msg.GuildID,
		User:      msg.Author,
		Member:    msg.Member,
		ChannelID: msg.ChannelID,
		Level:     level,
		NewLevel:  level,
		XP:        xp,
		Rank:      rank,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		d.Log.Warn("leveling: marshal level-up payload failed", "err", err)
		return
	}
	envBytes, err := json.Marshal(event.Envelope{
		Type:    event.TypeLevelUp,
		GuildID: msg.GuildID,
		TS:      time.Now().UnixMilli(),
		Data:    data,
	})
	if err != nil {
		d.Log.Warn("leveling: marshal level-up envelope failed", "err", err)
		return
	}
	subject := event.Subject(event.TypeLevelUp, msg.GuildID)
	if err := d.Bus.Publish(ctx, subject, envBytes, ""); err != nil {
		d.Log.Warn("leveling: publish level-up failed", "subject", subject, "err", err)
	}
}

// xpDelta picks a random XP amount in [XPMin, XPMax], scaled by the global
// Multiplier and by the highest RoleBoost whose role the member holds. Boosts
// never stack with each other (the highest matching multiplier wins) and a
// non-positive boost is ignored; with no matching boost the factor is 1.0.
func xpDelta(cfg Config, roles []string) int64 {
	min, max := cfg.XPMin, cfg.XPMax
	if min <= 0 {
		min = 1
	}
	if max < min {
		max = min
	}
	base := min
	if max > min {
		base = min + rand.IntN(max-min+1)
	}
	mult := cfg.Multiplier
	if mult <= 0 {
		mult = 1
	}
	boost := 0.0
	for _, b := range cfg.RoleBoosts {
		if b.Multiplier > boost && contains(roles, b.RoleID) {
			boost = b.Multiplier
		}
	}
	if boost <= 0 {
		boost = 1
	}
	delta := int64(float64(base) * mult * boost)
	if delta < 1 {
		delta = 1
	}
	return delta
}

// grantRewards adds the roles the member should now hold and, when not stacking,
// removes superseded reward roles.
func grantRewards(ctx context.Context, d plugin.Deps, cfg Config, guildID, userID string, level int) {
	rewards, err := d.Store.Levels.RewardsUpTo(ctx, mustID(guildID), level)
	if err != nil || len(rewards) == 0 {
		return
	}
	// rewards is ordered by level ascending; the last entry is the highest
	// reward the member qualifies for.
	for i, rw := range rewards {
		roleID := event.FormatID(rw.RoleID)
		isHighest := i == len(rewards)-1
		// When not stacking, only the highest-level reward role should be held;
		// also honour a per-reward RemovePrevious flag.
		if (!cfg.StackRewards || rw.RemovePrevious) && !isHighest {
			_ = d.Discord.RemoveRole(guildID, userID, roleID, "leveling: reward superseded")
			continue
		}
		if err := d.Discord.AddRole(guildID, userID, roleID, "leveling: reached level "+strconv.Itoa(level)); err != nil {
			d.Log.Warn("leveling: add reward role failed", "role", roleID, "err", err)
		}
	}
}

// announce posts the level-up message to the configured destination, then runs
// the durable post-announce tail. The rich LevelUp message (content + embeds +
// components) renders when configured; older configs fall back to the legacy
// single-string LevelUpMessage. Both render as templates against the leveling
// scope. The primary send stays synchronous (so DM stays content-only and the
// AnnounceChannel selection is unchanged); only the tail + button click actions
// run durably through the runner.
func (p *Plugin) announce(ctx context.Context, d plugin.Deps, cfg Config, msg event.Message, level, rank int, xp int64) {
	name, _ := guildInfo(ctx, d, msg.GuildID)
	v := levelVars{
		user:    msg.Author,
		guildID: msg.GuildID,
		server:  name,
		level:   level,
		lookup:  tmpllookup.New(ctx, d.GuildState, msg.GuildID),
	}

	var send *discordgo.MessageSend
	if hasLevelUp(cfg.LevelUp) {
		send = buildLevelUp(cfg.LevelUp, v)
	} else if text := v.render(cfg.LevelUpMessage); text != "" {
		send = &discordgo.MessageSend{Content: text}
	}

	// channelID is where the announcement lands: "" = the message's channel, a
	// channel id = that channel, "dm" = the member's DM. It also anchors the
	// durable tail's scope.
	channelID := msg.ChannelID
	if cfg.AnnounceChannel != "" && cfg.AnnounceChannel != "dm" {
		channelID = cfg.AnnounceChannel
	}

	if send != nil {
		switch cfg.AnnounceChannel {
		case "dm":
			// A DM carries plain content only; an embed-only message can't be DMed,
			// and Discord rejects components on a plain DM content push (there is no
			// DM component route for leveling), so components are intentionally not
			// attached here.
			if send.Content != "" {
				_ = d.Discord.SendDM(msg.Author.ID, send.Content)
			}
		default:
			// Attach the message's interactive components (buttons / selects) on the
			// channel send only; their clicks route back to handleComponent.
			if len(cfg.LevelUp.Components) > 0 {
				send.Components = buildComponents(cfg.LevelUp.Components, v, componentPrefix)
			}
			_, _ = d.Discord.SendMessage(channelID, send)
		}
	}

	// Run the post-announce durable tail (add roles, post elsewhere, wait, …).
	p.runTail(ctx, d, cfg, msg, name, level, rank, xp, channelID)
}

// runTail runs the post-announce flow (cfg.Tail) as a durable automation run
// once the level-up message has been posted. The scope mirrors an automations
// level_up run — same .User / .Member / .Guild / .Event vars — so a tail
// authored on the canvas behaves exactly like a hand-built automation. Nothing
// runs (and nothing persists) when the tail is empty.
func (p *Plugin) runTail(ctx context.Context, d plugin.Deps, cfg Config, msg event.Message, guildName string, level, rank int, xp int64, channelID string) {
	if len(cfg.Tail) == 0 || p.runner == nil {
		return
	}
	guildCtx := cc.ContextGuild{ID: msg.GuildID, Name: guildName}
	if id, ok := event.ParseID(msg.GuildID); ok {
		if g, err := d.Store.Guilds.Get(ctx, id); err == nil {
			guildCtx.MemberCount = g.MemberCount
		}
	}
	ctxVars := cc.BuildContext(msg.GuildID, channelID, msg.Author, msg.Member, guildCtx, time.Now().UnixMilli())
	scope := cc.NewScope(d.GuildState, msg.GuildID, ctxVars, nil, nil)
	scope.SetEvent(levelEventMap(level, level, rank, xp, channelID))
	p.runner.Start(ctx, runner.Meta{
		AutomationID: automationID,
		Version:      1,
		GuildID:      msg.GuildID,
		InvokerID:    msg.Author.ID,
		ActorID:      msg.Author.ID,
		ChannelID:    channelID,
		TriggerKind:  "level_up",
	}, cc.Definition{Steps: cfg.Tail}, scope)
}

// ── level-up message components + click actions ──────────────

// handleComponent runs the click-action wired to a button / select on a posted
// level-up message. The custom_id is "leveling:<suffix>" (guild from the
// interaction). An unwired component (or any decode / lookup miss) is acked
// silently. A wired program runs as a fresh durable flow: it can open a modal,
// wait for the member's reply, branch and send follow-ups, all resuming through
// the automations plugin's "auto:" handlers + the wait scheduler. Mirrors
// welcome.handleComponent (single surface: no DM / tab routing).
func (p *Plugin) handleComponent(c *interactions.Context, d plugin.Deps) error {
	suffix := strings.TrimPrefix(c.CustomID(), componentPrefix)
	if suffix == "" || c.GuildID == "" {
		return c.DeferUpdate()
	}
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return c.DeferUpdate()
	}
	steps := actionSteps(cfg.Actions, suffix)
	if len(steps) == 0 || p.runner == nil {
		return c.DeferUpdate() // decorative / unwired
	}

	// A program that opens a modal must answer the click with the form itself (a
	// modal is the interaction's first response, so no up-front ack). Every other
	// program is acked silently, claiming the 3s window, and its steps post fresh
	// output to the channel.
	scope := p.clickScope(c, d, suffix)
	modalFirst := stepsOpenModalFirst(steps)
	if modalFirst {
		scope.MarkDeferred(false)
		scope.MarkReplied(false)
	} else {
		_ = c.DeferUpdate()
		scope.MarkDeferred(true)
		scope.MarkReplied(true)
	}

	res := p.runner.Start(c.Ctx, runner.Meta{
		AutomationID:     automationID,
		Version:          1,
		GuildID:          c.GuildID,
		InvokerID:        c.User.ID,
		ActorID:          c.User.ID,
		ChannelID:        c.I.ChannelID,
		TriggerKind:      "leveling_click",
		InteractionID:    c.I.ID,
		InteractionToken: c.I.Token,
	}, cc.Definition{Steps: steps}, scope)

	// Safety net: if a modal-first program didn't actually open the modal (a
	// failing step, or a since-edited config), the click still needs SOME ack
	// within the deadline or Discord shows "interaction failed".
	if modalFirst && !c.Responded() && (res.Pause == nil || res.Pause.AwaitingKind != "modal") {
		_ = c.DeferUpdate()
	}
	return nil
}

// clickScope builds the run scope for a click action: the clicker is the user,
// and the click's id plus any selected values are exposed under .Vars.click. The
// caller sets the interaction-ack flags (deferred / replied) based on whether the
// program opens a modal first.
func (p *Plugin) clickScope(c *interactions.Context, d plugin.Deps, suffix string) *cc.Scope {
	gid, _ := event.ParseID(c.GuildID)
	guildName := "the server"
	memberCount := 0
	if g, err := d.Store.Guilds.Get(c.Ctx, gid); err == nil {
		if g.Name != "" {
			guildName = g.Name
		}
		memberCount = g.MemberCount
	}
	ctxVars := cc.BuildContext(c.GuildID, c.I.ChannelID, c.User, c.I.Member, cc.ContextGuild{
		ID: c.GuildID, Name: guildName, MemberCount: memberCount,
	}, time.Now().UnixMilli())
	scope := cc.NewScope(d.GuildState, c.GuildID, ctxVars, nil, nil)
	scope.Set("click", map[string]any{"id": suffix, "values": c.ComponentValues()})
	return scope
}

// stepsOpenModalFirst reports whether a click program opens a modal as its first
// Discord-facing step (pure data steps may precede it). When it does, the click
// must be answered with the modal rather than pre-deferred, which Discord forbids
// before a modal response.
func stepsOpenModalFirst(steps []cc.Step) bool {
	for _, s := range steps {
		switch s.Kind {
		case cc.KindModalOpen:
			return true
		case cc.KindSetVar, cc.KindIncrVar, cc.KindKVGet, cc.KindKVSet, cc.KindKVDelete,
			cc.KindJSONParse, cc.KindPickRandom, cc.KindMemberFetch:
			continue // pure data steps don't touch the interaction
		default:
			return false
		}
	}
	return false
}

// actionSteps returns the click program wired to a component suffix, or nil.
func actionSteps(actions []ButtonAction, suffix string) []cc.Step {
	for _, a := range actions {
		if a.Suffix == suffix {
			return a.Steps
		}
	}
	return nil
}

// MergeStoredActions returns the incoming leveling config JSON with the
// canvas-owned fields (the per-button click Actions and the post-announce Tail)
// replaced by the stored config's. The composer page owns the message (content,
// embeds, components, rank card); the button click actions and the follow-up
// flow are owned by the automation canvas (saved via /leveling/actions), so a
// composer save must not overwrite them with its (possibly stale, or absent)
// copy. On any decode/encode error the incoming config is returned unchanged.
func MergeStoredActions(incoming, stored []byte) []byte {
	var in, st Config
	if json.Unmarshal(incoming, &in) != nil || json.Unmarshal(stored, &st) != nil {
		return incoming
	}
	in.Actions = st.Actions
	in.Tail = st.Tail
	out, err := json.Marshal(in)
	if err != nil {
		return incoming
	}
	return out
}

// buildComponents renders the configured button / select rows into Discord
// message components. A non-link component routes its click back to this
// feature's handler via custom_id routePrefix+suffix; the handler runs the wired
// Action (if any) or acks silently. Link buttons carry their URL instead.
// Labels, placeholders and option text are templated. Mirrors welcome's copy.
func buildComponents(rows []cc.ComponentRow, v levelVars, routePrefix string) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, 0, len(rows))
	for _, row := range rows {
		comps := make([]discordgo.MessageComponent, 0, len(row.Components))
		for _, c := range row.Components {
			if mc := buildComponent(c, v, routePrefix); mc != nil {
				comps = append(comps, mc)
			}
		}
		if len(comps) > 0 {
			out = append(out, discordgo.ActionsRow{Components: comps})
		}
	}
	return out
}

// buildComponent renders one component, or returns nil to skip it when it would
// make Discord reject the whole message (e.g. a string select with no options).
func buildComponent(c cc.Component, v levelVars, routePrefix string) discordgo.MessageComponent {
	routeID := routePrefix + c.CustomIDSuffix
	switch c.Type {
	case "select_string":
		if len(c.Options) == 0 {
			return nil
		}
		opts := make([]discordgo.SelectMenuOption, 0, len(c.Options))
		for _, o := range c.Options {
			so := discordgo.SelectMenuOption{
				Label:       v.render(o.Label),
				Value:       o.Value,
				Description: v.render(o.Description),
				Default:     o.Default,
			}
			if o.Emoji != "" {
				so.Emoji = componentEmoji(o.Emoji)
			}
			opts = append(opts, so)
		}
		return discordgo.SelectMenu{
			MenuType:    discordgo.StringSelectMenu,
			CustomID:    routeID,
			Placeholder: v.render(c.Placeholder),
			Options:     opts,
			MinValues:   c.MinValues,
			MaxValues:   intOrZero(c.MaxValues),
			Disabled:    c.Disabled,
		}
	case "select_user":
		return discordgo.SelectMenu{MenuType: discordgo.UserSelectMenu, CustomID: routeID, Placeholder: v.render(c.Placeholder), Disabled: c.Disabled}
	case "select_role":
		return discordgo.SelectMenu{MenuType: discordgo.RoleSelectMenu, CustomID: routeID, Placeholder: v.render(c.Placeholder), Disabled: c.Disabled}
	case "select_channel":
		return discordgo.SelectMenu{MenuType: discordgo.ChannelSelectMenu, CustomID: routeID, Placeholder: v.render(c.Placeholder), Disabled: c.Disabled}
	default: // button
		style := buttonStyle(c.Style)
		btn := discordgo.Button{Label: v.render(c.Label), Style: style, Disabled: c.Disabled}
		// Discord rejects a button that carries both a URL and a custom_id: a link
		// button is its URL, anything else routes its click to the handler.
		if style == discordgo.LinkButton {
			btn.URL = v.render(c.URL)
		} else {
			btn.CustomID = routeID
		}
		if c.Emoji != "" {
			btn.Emoji = componentEmoji(c.Emoji)
		}
		return btn
	}
}

func buttonStyle(s string) discordgo.ButtonStyle {
	switch strings.ToLower(s) {
	case "primary":
		return discordgo.PrimaryButton
	case "success":
		return discordgo.SuccessButton
	case "danger":
		return discordgo.DangerButton
	case "link":
		return discordgo.LinkButton
	}
	return discordgo.SecondaryButton
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// componentEmoji turns the editor's emoji string into Discord's shape: a unicode
// glyph passes through as Name; a custom emoji arrives as "name:id" (also
// tolerated: "a:name:id" for animated, or the full "<a:name:id>" paste) and
// splits into Name + ID + Animated.
func componentEmoji(s string) *discordgo.ComponentEmoji {
	s = strings.Trim(strings.TrimSpace(s), "<>")
	parts := strings.Split(s, ":")
	last := parts[len(parts)-1]
	if len(parts) >= 2 && isSnowflakeID(last) {
		e := &discordgo.ComponentEmoji{Name: parts[len(parts)-2], ID: last}
		if len(parts) >= 3 && parts[0] == "a" {
			e.Animated = true
		}
		return e
	}
	return &discordgo.ComponentEmoji{Name: s}
}

// isSnowflakeID reports whether s looks like a Discord id (long enough that a
// short select value can't be mistaken for one).
func isSnowflakeID(s string) bool {
	if len(s) < 15 || len(s) > 21 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ── /rank ────────────────────────────────────────────────────

func handleRank(c *interactions.Context, d plugin.Deps) error {
	if err := c.Defer(false); err != nil {
		return err
	}

	target := c.User
	if u, ok := c.Options().User("user"); ok {
		target = u
	}

	gid, _ := event.ParseID(c.GuildID)
	uid, _ := event.ParseID(target.ID)

	lu, err := d.Store.Levels.Get(c.Ctx, gid, uid)
	if errors.Is(err, store.ErrNotFound) {
		_, e := c.Followup(&discordgo.WebhookParams{
			Content: "No XP yet for " + displayName(target) + ".",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return e
	}
	if err != nil {
		return err
	}

	rank, err := d.Store.Levels.Rank(c.Ctx, gid, uid)
	if err != nil {
		return err
	}

	level := LevelFromXP(lu.XP)
	into, span := Progress(lu.XP)

	cfg, _, _ := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	card := cfg.RankCard

	var png []byte
	if card.Layout != nil {
		// Studio-designed card: render the declarative layout with rank + guild vars.
		fonts, _ := d.Store.Uploads.FontMap(c.Ctx, gid)
		vars := rankVars(target, level, rank, into, span, lu.XP)
		if g, gerr := d.Store.Guilds.Get(c.Ctx, gid); gerr == nil {
			vars["{server}"] = g.Name
			vars["{server.id}"] = c.GuildID
			vars["{server.icon}"] = discord.GuildIconURL(c.GuildID, g.Icon, 256)
			vars["{count}"] = strconv.Itoa(g.MemberCount)
		}
		// Card formulas can read stored values for THIS member / the guild.
		kv := d.Store.FeatureKV.CardLookup(c.Ctx, gid, uid)
		png, err = d.Imaging.RenderLayout(templating.WithCardKV(c.Ctx, kv), *card.Layout, vars, fonts)
	} else {
		png, err = d.Imaging.RenderRank(c.Ctx, imaging.RankInput{
			Background:   card.Background,
			AccentColor:  card.AccentColor,
			TextColor:    card.TextColor,
			SubTextColor: card.SubTextColor,
			BarColor:     card.BarColor,
			BarBgColor:   card.BarBgColor,
			AvatarURL:    discord.AvatarURL(target.ID, target.Avatar, 256),
			Username:     displayName(target),
			Rank:         rank,
			Level:        level,
			LevelXP:      into,
			NeededXP:     span,
			TotalXP:      lu.XP,
		})
	}
	if err != nil {
		_, e := c.FollowupContent("Failed to render rank card: " + err.Error())
		return e
	}

	_, err = c.Followup(&discordgo.WebhookParams{
		Files: []*discordgo.File{{Name: "rank.png", ContentType: "image/png", Reader: bytes.NewReader(png)}},
	})
	return err
}

// ── /leaderboard ─────────────────────────────────────────────

func handleLeaderboard(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	top, err := d.Store.Levels.Leaderboard(c.Ctx, gid, 10, 0)
	if err != nil {
		return err
	}
	if len(top) == 0 {
		return c.RespondEphemeral("No one has earned any XP yet.")
	}

	name, _ := guildInfo(c.Ctx, d, c.GuildID)
	var b strings.Builder
	for i, lu := range top {
		level := LevelFromXP(lu.XP)
		fmt.Fprintf(&b, "**%d.** <@%s> — Level %d (%s XP)\n",
			i+1, event.FormatID(lu.UserID), level, formatInt(lu.XP))
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🏆 " + name + " Leaderboard",
		Description: b.String(),
		Color:       0xB244FC,
	}
	return c.RespondEmbed(false, embed)
}

// ── /level-rewards ───────────────────────────────────────────

func handleRewards(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	sub := c.Subcommand()
	if len(sub) == 0 {
		return c.RespondEphemeral("Unknown subcommand.")
	}

	switch sub[0] {
	case "add":
		opts := c.Options()
		level := int(opts.Int("level"))
		roleID, _ := event.ParseID(opts.Snowflake("role"))
		if level <= 0 {
			return c.RespondEphemeral("Level must be 1 or higher.")
		}
		if roleID == 0 {
			return c.RespondEphemeral("Please pick a valid role.")
		}
		if err := d.Store.Levels.SetReward(c.Ctx, store.LevelReward{
			GuildID: gid,
			Level:   level,
			RoleID:  roleID,
		}); err != nil {
			return err
		}
		return c.RespondEphemeral(fmt.Sprintf("✅ Members reaching **level %d** will now get <@&%s>.", level, opts.Snowflake("role")))

	case "remove":
		level := int(c.Options().Int("level"))
		if err := d.Store.Levels.DeleteReward(c.Ctx, gid, level); err != nil {
			return err
		}
		return c.RespondEphemeral(fmt.Sprintf("✅ Removed the reward at **level %d**.", level))

	case "list":
		rewards, err := d.Store.Levels.ListRewards(c.Ctx, gid)
		if err != nil {
			return err
		}
		if len(rewards) == 0 {
			return c.RespondEphemeral("No level rewards configured yet. Add one with `/level-rewards add`.")
		}
		var b strings.Builder
		for _, rw := range rewards {
			fmt.Fprintf(&b, "**Level %d** → <@&%s>\n", rw.Level, event.FormatID(rw.RoleID))
		}
		embed := &discordgo.MessageEmbed{
			Title:       "Level Rewards",
			Description: b.String(),
			Color:       0xB244FC,
		}
		return c.RespondEmbed(true, embed)
	}
	return c.RespondEphemeral("Unknown subcommand.")
}

// ── helpers ──────────────────────────────────────────────────

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func mustID(s string) int64 {
	id, _ := event.ParseID(s)
	return id
}

func displayName(u event.User) string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}

func guildInfo(ctx context.Context, d plugin.Deps, guildID string) (name string, count int) {
	if id, ok := event.ParseID(guildID); ok {
		if g, err := d.Store.Guilds.Get(ctx, id); err == nil {
			name, count = g.Name, g.MemberCount
		}
	}
	if name == "" {
		name = "the server"
	}
	return name, count
}

// formatInt adds thousands separators to an int64.
func formatInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
