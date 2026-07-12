// Command seed loads idempotent fixture data into the local dev database so the
// dashboard, worker and API have something to render the moment infra is up —
// no real Discord traffic required.
//
// It is safe to re-run: every write is an upsert or guarded by an existence
// check. It applies the embedded goose migrations first (the same ones the
// services run on boot), so `make seed` works against a brand-new database with
// no service started yet.
//
// What it seeds:
//
//   - Three guilds. The primary one is configurable via SEED_GUILD_ID; the
//     other two give the dashboard's server list and the leaderboards some
//     variety.
//   - Every feature config (welcome, leveling, autorole, moderation, automod,
//     customcommands) for the primary guild, enabled, using each feature's own
//     Default() so the stored JSONB always matches what the dashboard expects.
//   - A populated leveling leaderboard (so /rank, /leaderboard and rank cards
//     render) plus a couple of level→role rewards.
//   - A handful of moderation cases (warn / timeout / ban) for the case log.
//   - One self-assign reaction-role menu and two custom slash commands.
//   - A few dashboard audit-log entries.
//
// Tip: set SEED_GUILD_ID to a guild you actually own so the seeded config shows
// up for you in the dashboard (which only lists guilds the bot is in).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/dia-bot/dia/internal/config"
	"github.com/dia-bot/dia/internal/logging"
	"github.com/dia-bot/dia/internal/store"

	"github.com/dia-bot/dia/internal/features/automations"
	"github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/dia-bot/dia/internal/features/leveling"
	"github.com/dia-bot/dia/internal/features/moderation"
	"github.com/dia-bot/dia/internal/features/roles"
	"github.com/dia-bot/dia/internal/features/tickets"
	"github.com/dia-bot/dia/internal/features/welcome"
)

// defaultPrimaryGuild is used when SEED_GUILD_ID is unset. It is an obviously
// fake snowflake so it never collides with a real guild.
const defaultPrimaryGuild int64 = 1000000000000000001

// Stable fixture snowflakes. Decimal strings/ints to mirror Discord's IDs.
const (
	auroraGuild int64 = 1000000000000000002
	nebulaGuild int64 = 1000000000000000003

	ownerUser int64 = 1500000000000000000 // notional guild owner / acting moderator

	// Notional channels/roles referenced by the seeded configs. They won't
	// resolve to real Discord objects in dev, but the dashboard renders the
	// stored IDs regardless, which is what we want for UI work.
	welcomeChannel int64 = 1000000000000000010
	logChannel     int64 = 1000000000000000011
	roleMember     int64 = 1000000000000000020
	roleLevel5     int64 = 1000000000000000021
	roleLevel10    int64 = 1000000000000000022
	roleNotify     int64 = 1000000000000000023
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logging.New(cfg.LogLevel, cfg.Env)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	st, err := store.Open(ctx, cfg.Postgres, log)
	if err != nil {
		fatal(log, "postgres", err)
	}
	defer st.Close()

	// Self-sufficient: apply migrations so `make seed` works on a fresh db.
	if err := st.Migrate(ctx, log); err != nil {
		fatal(log, "migrate", err)
	}

	if err := run(ctx, st); err != nil {
		fatal(log, "seed", err)
	}
}

func run(ctx context.Context, st *store.Store) error {
	primary := primaryGuildID()

	guilds := []store.Guild{
		{ID: primary, Name: "Dia Dev Server", OwnerID: ownerUser, MemberCount: 420},
		{ID: auroraGuild, Name: "Aurora Lounge", OwnerID: ownerUser, MemberCount: 88},
		{ID: nebulaGuild, Name: "Nebula HQ", OwnerID: ownerUser, MemberCount: 1337},
	}
	for _, g := range guilds {
		if err := st.Guilds.Upsert(ctx, g); err != nil {
			return fmt.Errorf("guild %d: %w", g.ID, err)
		}
	}

	if err := seedPrimary(ctx, st, primary); err != nil {
		return fmt.Errorf("primary guild: %w", err)
	}

	// Lighter fixtures on the other guilds so the server list and any
	// cross-guild views aren't empty.
	for _, g := range []int64{auroraGuild, nebulaGuild} {
		if err := seedLightFeatures(ctx, st, g); err != nil {
			return fmt.Errorf("light features %d: %w", g, err)
		}
		if err := seedLevels(ctx, st, g, levelFixtures[:5]); err != nil {
			return fmt.Errorf("light levels %d: %w", g, err)
		}
	}

	fmt.Println("seed complete")
	fmt.Printf("  primary guild : %d (Dia Dev Server)\n", primary)
	fmt.Printf("  extra guilds  : %d, %d\n", auroraGuild, nebulaGuild)
	fmt.Println("  features      : welcome, leveling, autorole, moderation, automod, customcommands, automations, giveaway (enabled)")
	fmt.Printf("  leaderboard   : %d members; rewards at level 5 and 10\n", len(levelFixtures))
	fmt.Println("  moderation    : warn + timeout + ban cases")
	fmt.Println("  automod       : starter ruleset + blocked-words rule; escalation ladder; demo infractions")
	fmt.Println("  extras        : 1 reaction-role menu, 2 custom commands, audit entries")
	fmt.Println()
	fmt.Println("Re-run any time (idempotent). Point it at your own guild with:")
	fmt.Println("  make seed SEED_GUILD_ID=<your guild id>")
	return nil
}

// seedPrimary loads the full fixture set onto the primary dev guild.
func seedPrimary(ctx context.Context, st *store.Store, guildID int64) error {
	if err := seedFeatures(ctx, st, guildID); err != nil {
		return err
	}
	if err := seedLevels(ctx, st, guildID, levelFixtures); err != nil {
		return err
	}
	if err := seedRewards(ctx, st, guildID); err != nil {
		return err
	}
	if err := seedModCases(ctx, st, guildID); err != nil {
		return err
	}
	if err := seedInfractions(ctx, st, guildID); err != nil {
		return err
	}
	if err := seedReactionMenu(ctx, st, guildID); err != nil {
		return err
	}
	if err := seedTicketPanel(ctx, st, guildID); err != nil {
		return err
	}
	if err := seedCustomCommands(ctx, st, guildID); err != nil {
		return err
	}
	if err := seedAutomations(ctx, st, guildID); err != nil {
		return err
	}
	return seedAuditLog(ctx, st, guildID)
}

// ── Feature configs ──────────────────────────────────────────────────────────

// seedFeatures enables and configures every feature for a guild using each
// feature's own Default(), so the stored JSONB can never drift from the shapes
// the dashboard and worker read.
func seedFeatures(ctx context.Context, st *store.Store, guildID int64) error {
	wel := welcome.Default()
	wel.Welcome.ChannelID = sid(welcomeChannel)
	wel.Goodbye.Enabled = true
	wel.Goodbye.ChannelID = sid(welcomeChannel)

	lvl := leveling.Default()
	lvl.AnnounceLevelUp = true

	autorole := roles.Default()
	autorole.Roles = []string{sid(roleMember)}

	mod := moderation.Default()
	mod.LogChannel = sid(logChannel)

	// Start from the rule-based starter ruleset and add one example "blocked
	// words" rule so the dashboard editor has a content rule to show.
	automod := moderation.DefaultAutomod()
	automod.AlertChannel = sid(logChannel)
	automod.Rules = append(automod.Rules, moderation.AutomodRule{
		ID:      "rule_blocked_words",
		Name:    "Blocked words",
		Enabled: true,
		Trigger: moderation.RuleTrigger{
			Type:      moderation.TriggerWords,
			Words:     []string{"spamword", "blocked-phrase"},
			MatchMode: "substring",
		},
		Actions: []moderation.RuleAction{
			{Type: moderation.ActionDelete},
			{Type: moderation.ActionWarn, Reason: "Blocked word"},
			{Type: moderation.ActionAddPoints, Points: 1},
		},
	})

	cc := customcommands.Default()

	auto := automations.Default()

	tk := tickets.Default()
	tk.StaffRoleIDs = []string{sid(roleMember)}
	tk.LogChannel = sid(logChannel)
	tk.TranscriptChannel = sid(logChannel)

	giv := giveaway.Default()
	// Pre-fill the built-in preset's default channel so seeded giveaways post
	// somewhere sensible out of the box.
	if len(giv.Presets) > 0 {
		giv.Presets[0].DefaultChannelID = sid(welcomeChannel)
	}

	configs := []struct {
		key string
		val any
	}{
		{welcome.FeatureKey, wel},
		{leveling.FeatureKey, lvl},
		{roles.FeatureKey, autorole},
		{moderation.FeatureKey, mod},
		{moderation.AutomodKey, automod},
		{customcommands.FeatureKey, cc},
		{automations.FeatureKey, auto},
		{tickets.FeatureKey, tk},
		{giveaway.FeatureKey, giv},
	}
	for _, c := range configs {
		raw, err := json.Marshal(c.val)
		if err != nil {
			return fmt.Errorf("marshal %s config: %w", c.key, err)
		}
		if err := st.Features.Upsert(ctx, guildID, c.key, true, raw); err != nil {
			return fmt.Errorf("upsert %s config: %w", c.key, err)
		}
	}
	return nil
}

// seedLightFeatures enables just welcome + leveling so secondary guilds aren't
// blank without duplicating the full primary fixture set.
func seedLightFeatures(ctx context.Context, st *store.Store, guildID int64) error {
	wel, err := json.Marshal(welcome.Default())
	if err != nil {
		return err
	}
	if err := st.Features.Upsert(ctx, guildID, welcome.FeatureKey, true, wel); err != nil {
		return err
	}
	lvl, err := json.Marshal(leveling.Default())
	if err != nil {
		return err
	}
	return st.Features.Upsert(ctx, guildID, leveling.FeatureKey, true, lvl)
}

// ── Leveling ─────────────────────────────────────────────────────────────────

// levelFixtures is a descending XP ladder. Levels are computed from XP with the
// real curve so the seeded rows are internally consistent with the app.
var levelFixtures = []struct {
	user int64
	xp   int64
}{
	{2000000000000000001, 7200},
	{2000000000000000002, 5400},
	{2000000000000000003, 4675},
	{2000000000000000004, 3720},
	{2000000000000000005, 2900},
	{2000000000000000006, 2205},
	{2000000000000000007, 1625},
	{2000000000000000008, 1150},
	{2000000000000000009, 770},
	{2000000000000000010, 475},
	{2000000000000000011, 255},
	{2000000000000000012, 100},
}

// seedLevels upserts the leaderboard rows. AddXP is additive (not idempotent),
// so we write deterministic values directly with an upsert instead.
func seedLevels(ctx context.Context, st *store.Store, guildID int64, members []struct {
	user int64
	xp   int64
}) error {
	now := time.Now()
	for _, m := range members {
		lvl := leveling.LevelFromXP(m.xp)
		messages := m.xp / 18 // rough, just so the column isn't zero
		_, err := st.Pool.Exec(ctx, `
			INSERT INTO level_users (guild_id, user_id, xp, level, messages, last_message_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (guild_id, user_id) DO UPDATE SET
				xp = EXCLUDED.xp,
				level = EXCLUDED.level,
				messages = EXCLUDED.messages,
				last_message_at = EXCLUDED.last_message_at`,
			guildID, m.user, m.xp, lvl, messages, now)
		if err != nil {
			return fmt.Errorf("level_users %d: %w", m.user, err)
		}
	}
	return nil
}

func seedRewards(ctx context.Context, st *store.Store, guildID int64) error {
	rewards := []store.LevelReward{
		{GuildID: guildID, Level: 5, RoleID: roleLevel5, RemovePrevious: false},
		{GuildID: guildID, Level: 10, RoleID: roleLevel10, RemovePrevious: true},
	}
	for _, r := range rewards {
		if err := st.Levels.SetReward(ctx, r); err != nil {
			return fmt.Errorf("reward level %d: %w", r.Level, err)
		}
	}
	return nil
}

// ── Moderation ───────────────────────────────────────────────────────────────

// seedModCases inserts a few cases with explicit, stable case numbers so a
// re-run is a no-op (the table has no natural key beyond guild_id+case_number).
func seedModCases(ctx context.Context, st *store.Store, guildID int64) error {
	now := time.Now()
	timeoutExpiry := now.Add(6 * time.Hour)

	type modCase struct {
		number          int
		user            int64
		action          string
		reason          string
		durationSeconds int
		expiresAt       *time.Time
		active          bool
	}
	cases := []modCase{
		{1, 2000000000000000012, "warn", "Spamming in #general", 0, nil, true},
		{2, 2000000000000000011, "timeout", "Repeated mention spam", 21600, &timeoutExpiry, true},
		{3, 2000000000000000010, "ban", "Posting invite links after warning", 0, nil, true},
	}
	for _, c := range cases {
		_, err := st.Pool.Exec(ctx, `
			INSERT INTO mod_cases
				(guild_id, case_number, user_id, moderator_id, action, reason, duration_seconds, expires_at, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (guild_id, case_number) DO NOTHING`,
			guildID, c.number, c.user, ownerUser, c.action, c.reason, c.durationSeconds, c.expiresAt, c.active)
		if err != nil {
			return fmt.Errorf("mod_case #%d: %w", c.number, err)
		}
	}
	return nil
}

// seedInfractions inserts a few automod heat-ledger rows so the moderation
// dashboard's leaderboard and recent-actions list have demo data. The table
// has only a synthetic id, so we guard the whole batch behind an existence
// check to keep re-runs idempotent.
func seedInfractions(ctx context.Context, st *store.Store, guildID int64) error {
	var existing int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM automod_infractions WHERE guild_id = $1`, guildID).Scan(&existing); err != nil {
		return fmt.Errorf("count infractions: %w", err)
	}
	if existing > 0 {
		return nil // already seeded
	}

	now := time.Now()
	channel := logChannel
	type infraction struct {
		user       int64
		ruleID     string
		ruleName   string
		trigger    string
		points     int
		reason     string
		minutesAgo int
		decayHours int
	}
	fixtures := []infraction{
		{2000000000000000010, "rule_invites", "Block Discord invites", moderation.TriggerInvites, 1, "Invite link", 12, 24},
		{2000000000000000010, "rule_spam", "Message spam", moderation.TriggerSpam, 2, "Sending messages too quickly", 40, 24},
		{2000000000000000010, "rule_blocked_words", "Blocked words", moderation.TriggerWords, 1, "Blocked word", 95, 24},
		{2000000000000000011, "rule_mentions", "Mass mentions", moderation.TriggerMentions, 2, "Too many mentions", 30, 24},
		{2000000000000000011, "rule_everyone", "@everyone / @here pings", moderation.TriggerMassMention, 1, "Mass mention", 180, 24},
		{2000000000000000012, "rule_blocked_words", "Blocked words", moderation.TriggerWords, 1, "Blocked word", 260, 24},
	}
	for _, f := range fixtures {
		createdAt := now.Add(-time.Duration(f.minutesAgo) * time.Minute)
		expiresAt := createdAt.Add(time.Duration(f.decayHours) * time.Hour)
		if _, err := st.Pool.Exec(ctx, `
			INSERT INTO automod_infractions
				(guild_id, user_id, rule_id, rule_name, trigger_type, points, reason, channel_id, created_at, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			guildID, f.user, f.ruleID, f.ruleName, f.trigger, f.points, f.reason, channel, createdAt, expiresAt); err != nil {
			return fmt.Errorf("insert infraction (%s): %w", f.ruleID, err)
		}
	}
	return nil
}

// ── Reaction roles ───────────────────────────────────────────────────────────

func seedReactionMenu(ctx context.Context, st *store.Store, guildID int64) error {
	var existing int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM reaction_role_menus WHERE guild_id = $1`, guildID).Scan(&existing); err != nil {
		return fmt.Errorf("count reaction menus: %w", err)
	}
	if existing > 0 {
		return nil // already seeded
	}

	opts := []roles.Option{
		{RoleID: sid(roleNotify), Label: "Announcements", Emoji: "📢", Description: "Ping me for server news"},
		{RoleID: sid(roleMember), Label: "Member", Emoji: "✅", Description: "Self-verify as a member"},
		{RoleID: sid(roleLevel5), Label: "Events", Emoji: "🎉", Description: "Get notified about events"},
	}
	raw, err := json.Marshal(opts)
	if err != nil {
		return fmt.Errorf("marshal menu options: %w", err)
	}
	_, err = st.ReactionRoles.Create(ctx, store.ReactionRoleMenu{
		GuildID:   guildID,
		ChannelID: welcomeChannel,
		Title:     "Pick your roles",
		Mode:      "toggle",
		Options:   raw,
	})
	if err != nil {
		return fmt.Errorf("create reaction menu: %w", err)
	}
	return nil
}

// seedTicketPanel creates one starter "Support" ticket panel (idempotent).
func seedTicketPanel(ctx context.Context, st *store.Store, guildID int64) error {
	var existing int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM ticket_panels WHERE guild_id = $1`, guildID).Scan(&existing); err != nil {
		return fmt.Errorf("count ticket panels: %w", err)
	}
	if existing > 0 {
		return nil // already seeded
	}
	raw, err := json.Marshal(tickets.DefaultPanelConfig())
	if err != nil {
		return fmt.Errorf("marshal ticket panel: %w", err)
	}
	if _, err := st.Tickets.UpsertPanel(ctx, store.TicketPanel{
		GuildID: guildID,
		Name:    "Support",
		Style:   "buttons",
		Enabled: true,
		Config:  raw,
	}); err != nil {
		return fmt.Errorf("create ticket panel: %w", err)
	}
	return nil
}

// ── Custom commands ──────────────────────────────────────────────────────────

func seedCustomCommands(ctx context.Context, st *store.Store, guildID int64) error {
	// One reply step + a simple embed is the v2 equivalent of the legacy
	// content+embed response. Admins extend from there in the dashboard.
	commands := []struct {
		name, desc string
		def        customcommands.Definition
	}{
		{
			name: "rules", desc: "Show the server rules",
			def: customcommands.Definition{Steps: []customcommands.Step{{
				ID: "01_reply", Kind: customcommands.KindReply,
				Spec: mustJSON(customcommands.SpecReply{
					Content: "Be kind, stay on topic, and no spam. Full rules in <#" + sid(welcomeChannel) + ">.",
					Embeds: []customcommands.EmbedSpec{{
						Title:       "📜 Server Rules",
						Description: "1. Be respectful\n2. No spam or self-promo\n3. Keep it on topic",
						Color:       "#F15BB5",
					}},
				}),
			}}},
		},
		{
			name: "links", desc: "Useful links",
			def: customcommands.Definition{Steps: []customcommands.Step{{
				ID: "01_reply", Kind: customcommands.KindReply,
				Spec: mustJSON(customcommands.SpecReply{
					Content:   "Docs, GitHub and more 👇",
					Ephemeral: true,
					Embeds: []customcommands.EmbedSpec{{
						Title:       "🔗 Links",
						Description: "[Website](https://example.com) · [GitHub](https://github.com/dia-bot/dia)",
						Color:       "#9B5DE5",
					}},
				}),
			}}},
		},
	}
	for _, c := range commands {
		raw, err := json.Marshal(c.def)
		if err != nil {
			return fmt.Errorf("marshal command %s: %w", c.name, err)
		}
		if _, err := st.CustomCommands.Upsert(ctx, store.CustomCommand{
			GuildID:     guildID,
			Name:        c.name,
			Description: c.desc,
			Enabled:     true,
			Status:      string(customcommands.StatusPublished),
			Version:     1,
			Definition:  raw,
		}); err != nil {
			return fmt.Errorf("upsert command %s: %w", c.name, err)
		}
	}
	return nil
}

// seedAutomations seeds a couple of sample server-event automations. Names are
// the idempotency key (the table has no unique(name) constraint), so a re-run
// skips ones that already exist instead of duplicating them.
func seedAutomations(ctx context.Context, st *store.Store, guildID int64) error {
	existing, err := st.Automations.List(ctx, guildID)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, a := range existing {
		have[a.Name] = true
	}

	fixtures := []struct {
		name, desc, trigger string
		cfg                 automations.TriggerConfig
		def                 customcommands.Definition
	}{
		{
			name:    "Thank boosters",
			desc:    "Post a thank-you when a member gains the booster role",
			trigger: "role_added",
			cfg:     automations.TriggerConfig{Role: sid(roleMember)},
			def: customcommands.Definition{Steps: []customcommands.Step{{
				ID:   "thank",
				Kind: customcommands.KindSendMessage,
				Spec: mustJSON(customcommands.SpecSendMessage{
					Channel: customcommands.Expr{Src: sid(welcomeChannel)},
					Content: "💖 Thank you {{ .User.Mention }} for boosting **{{ .Guild.Name }}**!",
				}),
			}}},
		},
		{
			name:    "Auto-thread questions",
			desc:    "Open a thread on messages that ask a question",
			trigger: "message_create",
			cfg: automations.TriggerConfig{
				Channels:   []string{sid(welcomeChannel)},
				Keywords:   []string{"?"},
				IgnoreBots: true,
			},
			def: customcommands.Definition{Steps: []customcommands.Step{{
				ID:   "thread",
				Kind: customcommands.KindThreadCreate,
				Spec: mustJSON(customcommands.SpecThreadCreate{
					Channel:        customcommands.Expr{Src: "{{ .Channel.ID }}"},
					Message:        customcommands.Expr{Src: "{{ .Event.message.id }}"},
					Name:           "Discussion",
					AutoArchiveMin: 1440,
				}),
			}}},
		},
	}
	for _, f := range fixtures {
		if have[f.name] {
			continue
		}
		def, err := json.Marshal(f.def)
		if err != nil {
			return fmt.Errorf("marshal automation %s: %w", f.name, err)
		}
		cfg, err := json.Marshal(f.cfg)
		if err != nil {
			return fmt.Errorf("marshal automation cfg %s: %w", f.name, err)
		}
		evType, _ := automations.EventForTrigger(f.trigger)
		if _, err := st.Automations.Upsert(ctx, store.Automation{
			GuildID:       guildID,
			Name:          f.name,
			Description:   f.desc,
			Enabled:       true,
			Status:        string(automations.StatusPublished),
			Version:       1,
			TriggerType:   f.trigger,
			EventType:     string(evType),
			TriggerConfig: cfg,
			Definition:    def,
		}); err != nil {
			return fmt.Errorf("upsert automation %s: %w", f.name, err)
		}
	}
	return nil
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ── Audit log ────────────────────────────────────────────────────────────────

func seedAuditLog(ctx context.Context, st *store.Store, guildID int64) error {
	var existing int
	if err := st.Pool.QueryRow(ctx,
		`SELECT count(*) FROM dashboard_audit_log WHERE guild_id = $1`, guildID).Scan(&existing); err != nil {
		return fmt.Errorf("count audit log: %w", err)
	}
	if existing > 0 {
		return nil
	}

	entries := []struct {
		action string
		detail map[string]any
	}{
		{"feature.enabled", map[string]any{"feature": "welcome"}},
		{"feature.enabled", map[string]any{"feature": "leveling"}},
		{"feature.updated", map[string]any{"feature": "automod", "rules": 5}},
		{"command.created", map[string]any{"name": "rules"}},
	}
	for _, e := range entries {
		raw, err := json.Marshal(e.detail)
		if err != nil {
			return fmt.Errorf("marshal audit detail: %w", err)
		}
		if err := st.Audit.Add(ctx, store.AuditEntry{
			GuildID: guildID,
			UserID:  ownerUser,
			Action:  e.action,
			Detail:  raw,
		}); err != nil {
			return fmt.Errorf("add audit entry: %w", err)
		}
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

// primaryGuildID resolves SEED_GUILD_ID (a decimal snowflake) or falls back to
// the default fixture guild.
func primaryGuildID() int64 {
	if v := os.Getenv("SEED_GUILD_ID"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil && id > 0 {
			return id
		}
	}
	return defaultPrimaryGuild
}

// sid renders a snowflake as the decimal string Discord/the configs use.
func sid(id int64) string { return strconv.FormatInt(id, 10) }

func fatal(log *slog.Logger, msg string, err error) {
	log.Error(msg, "err", err)
	os.Exit(1)
}
