package verification

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Plugin implements the verification gate feature.
type Plugin struct{}

// New returns the verification plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Verification",
		Description: "Gate new members behind a button click or an image captcha before they can access the server.",
		Category:    plugin.CategoryModeration,
	}
}

// custom_id constants. All verification components/modals share the "verify:"
// prefix so a single Component/Modal route handles them.
const (
	idStart  = "verify:start"  // shared, stateless "Verify" button on the prompt
	idEnter  = "verify:enter"  // per-user "Enter code" button (captcha mode)
	idAnswer = "verify:answer" // captcha answer modal

	// vbtnPrefix namespaces clicks on the prompt's custom buttons. Each custom
	// button mints custom_id "vbtn:<custom_id_suffix>"; a click routes here and,
	// when ButtonActions maps the suffix, runs the named automation as a durable
	// run. (The "Verify" button uses idStart and is injected separately.)
	vbtnPrefix = "vbtn:"
)

// Redis keys (all decimal-string ids):
//
//	verify:msg:<gid>            -> message id of the posted prompt (dedupe).
//	verify:code:<gid>:<uid>     -> the expected captcha answer (TTL 10m).
//	verify:salt:<gid>:<uid>     -> per-user attempt counter; bumps the captcha seed.
//	verify:tries:<gid>:<uid>    -> wrong-answer counter (TTL 10m).
//	verify:pending:<gid>        -> HASH uid -> kick-deadline unix seconds.
//	verify:guilds               -> HASH gid -> "1": index of guilds with pending
//	                               members, so the sweep needs no store enumeration.
const (
	keyMsg     = "verify:msg:"
	keyCode    = "verify:code:"
	keySalt    = "verify:salt:"
	keyTries   = "verify:tries:"
	keyPending = "verify:pending:"
	keyGuilds  = "verify:guilds"
)

const (
	codeTTL    = 10 * time.Minute
	maxTries   = 3
	kickTick   = time.Minute
	startEmoji = "✅"
)

// Init wires the join handler, the verify components/modal and (when any guild
// uses KickAfterMinutes) the kick sweep worker.
func (*Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	reg.OnEvent(event.TypeMemberAdd, func(ctx context.Context, env *event.Envelope) error {
		return onJoin(ctx, d, env)
	})
	reg.Component("verify:", func(c *interactions.Context) error {
		return onComponent(c, d)
	})
	// Clicks on the prompt's custom buttons route here ("vbtn:<suffix>"); a
	// mapped button runs its automation as a durable run, an unmapped one is
	// acked silently.
	reg.Component(vbtnPrefix, func(c *interactions.Context) error {
		return onCustomButton(c, d)
	})
	reg.Modal("verify:", func(c *interactions.Context) error {
		return onModal(c, d)
	})
	reg.Worker("verification-kicker", func(ctx context.Context) {
		kickLoop(ctx, d)
	})
	return nil
}

// ── Join ─────────────────────────────────────────────────────

func onJoin(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	ma, err := plugin.DecodeData[event.MemberAdd](env)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(ma.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return err
	}
	u := ma.Member.User
	if u.Bot {
		return nil // bots are admitted/handled by the autorole feature, not gated.
	}

	// OnlySuspicious: a member who passes the behavioural checks (old enough, and
	// has an avatar when RequireAvatar is on) passes instantly (grant the verified
	// role, never restrict them).
	if cfg.OnlySuspicious && !isSuspicious(u, cfg) {
		if cfg.VerifiedRole != "" {
			_ = d.Discord.AddRole(ma.GuildID, u.ID, cfg.VerifiedRole, "verification: trusted joiner")
		}
		return nil
	}

	// Restrict the joiner.
	if cfg.UnverifiedRole != "" {
		_ = d.Discord.AddRole(ma.GuildID, u.ID, cfg.UnverifiedRole, "verification: pending")
	}
	// Track for the kick sweep if auto-kick is on.
	if cfg.KickAfterMinutes > 0 {
		deadline := time.Now().Add(time.Duration(cfg.KickAfterMinutes) * time.Minute).Unix()
		_ = d.Cache.SetHashField(ctx, keyPending+ma.GuildID, u.ID, strconv.FormatInt(deadline, 10))
		_ = d.Cache.SetHashField(ctx, keyGuilds, ma.GuildID, "1")
	}
	// Ensure the shared prompt exists in the verification channel.
	ensurePrompt(ctx, d, ma.GuildID, gid, cfg)
	return nil
}

// isSuspicious reports whether a joiner looks risky per the configured
// behavioural checks: no profile picture (when RequireAvatar is on), or an
// account younger than MinAccountAgeHours.
func isSuspicious(u event.User, cfg Config) bool {
	if cfg.RequireAvatar && u.Avatar == "" {
		return true
	}
	if cfg.MinAccountAgeHours <= 0 {
		return false
	}
	id, _ := strconv.ParseInt(u.ID, 10, 64)
	createdMS := (id >> 22) + 1420070400000
	age := time.Since(time.UnixMilli(createdMS))
	return age < time.Duration(cfg.MinAccountAgeHours)*time.Hour
}

// ensurePrompt posts the persistent verification prompt once per guild and
// remembers its message id in Redis so repeated joins don't spam duplicates.
func ensurePrompt(ctx context.Context, d plugin.Deps, guildID string, gid int64, cfg Config) {
	if cfg.Channel == "" {
		return
	}
	// Already posted? (best-effort dedupe; a miss is fine and we post once.)
	if got, err := getString(ctx, d, keyMsg+guildID); err == nil && got != "" {
		return
	}
	// Guard against two near-simultaneous joins both posting: only the first
	// caller wins the reservation and posts the prompt.
	if ok, _ := d.Cache.Reserve(ctx, keyMsg+guildID+":lock", 30*time.Second); !ok {
		return
	}
	// Build the prompt. Every string is templated; since there is no specific
	// joiner here (the prompt is shared by everyone), {{ .User.* }} renders empty
	// and admins are expected to write guild-level copy.
	name := guildName(ctx, d, gid)
	send := buildPrompt(cfg, guildID, name)
	msg, err := d.Discord.SendMessage(cfg.Channel, send)
	if err != nil || msg == nil {
		return
	}
	// Stored as a JSON string so getString (GetJSON) reads it back cleanly. No
	// TTL: the prompt is persistent and lives with the message.
	_ = d.Cache.SetJSON(ctx, keyMsg+guildID, msg.ID, 0)
}

// ── Components ───────────────────────────────────────────────

func onComponent(c *interactions.Context, d plugin.Deps) error {
	switch c.CustomID() {
	case idStart:
		return onStart(c, d)
	case idEnter:
		return onEnter(c, d)
	default:
		return nil // stale / unknown
	}
}

// onCustomButton handles a click on one of the prompt's custom buttons
// ("vbtn:<suffix>"). It loads the verification config, finds the automation
// mapped to the clicked suffix in ButtonActions, and runs it as a durable run
// (trigger "verification_click"). An unmapped button (or a disabled feature) is
// acked silently. Link buttons never reach here (Discord opens the URL).
func onCustomButton(c *interactions.Context, d plugin.Deps) error {
	suffix := strings.TrimPrefix(c.CustomID(), vbtnPrefix)
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return c.DeferUpdate()
	}
	automationID := ""
	for _, a := range cfg.ButtonActions {
		if a.Suffix == suffix {
			automationID = strings.TrimSpace(a.AutomationID)
			break
		}
	}
	if automationID == "" {
		return c.DeferUpdate() // decorative / unmapped
	}
	// Claim the 3s window; the automation posts its own follow-up output.
	_ = c.DeferUpdate()
	runVerificationAutomation(c.Ctx, d, c.GuildID, interactionUser(c), c.I.Member, runOpts{
		AutomationID:     automationID,
		TriggerKind:      "verification_click",
		ChannelID:        c.I.ChannelID,
		InteractionID:    c.I.ID,
		InteractionToken: c.I.Token,
		Event:            map[string]any{"custom_id": c.CustomID(), "suffix": suffix},
	})
	return nil
}

// onStart handles the shared "Verify" button. Button mode verifies immediately;
// captcha mode generates a code and shows it as an ephemeral image with an
// "Enter code" button (a modal cannot display an image, so the image is posted
// first and the modal opened from the follow-up button).
func onStart(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return c.RespondEphemeral("Verification is not available right now.")
	}
	uid := userIDOf(c)
	if uid == "" {
		return c.RespondEphemeral("Could not read your account; please try again.")
	}

	if cfg.Mode == ModeCaptcha {
		return startCaptcha(c, d, cfg, uid)
	}
	// Button mode: pass immediately.
	if err := pass(c.Ctx, d, c.GuildID, uid, cfg); err != nil {
		return c.RespondEphemeral("Verification hit a snag granting your roles. Please ping a moderator.")
	}
	onVerified(d, c, cfg)
	return c.RespondEphemeral("✅ You're verified. Welcome in!")
}

// startCaptcha generates + stores a code, renders the challenge image and
// replies ephemerally with the image plus an "Enter code" button.
func startCaptcha(c *interactions.Context, d plugin.Deps, cfg Config, uid string) error {
	// Per-attempt salt: bump a counter so a refresh produces a new code.
	salt, _ := d.Cache.Incr(c.Ctx, keySalt+c.GuildID+":"+uid, codeTTL)
	code := generateCode(c.GuildID + ":" + uid + ":" + strconv.FormatInt(salt, 10))
	if err := d.Cache.SetJSON(c.Ctx, keyCode+c.GuildID+":"+uid, code, codeTTL); err != nil {
		return c.RespondEphemeral("Could not start the captcha; please try again.")
	}
	_ = d.Cache.Delete(c.Ctx, keyTries+c.GuildID+":"+uid)

	data := &discordgo.InteractionResponseData{
		Flags: discordgo.MessageFlagsEphemeral,
		Components: []discordgo.MessageComponent{
			discordgo.ActionsRow{Components: []discordgo.MessageComponent{
				discordgo.Button{Style: discordgo.PrimaryButton, Label: "Enter code", CustomID: idEnter},
			}},
		},
	}
	if png, err := renderCaptcha(code); err == nil {
		data.Content = "Read the code in the image, then click **Enter code** and type it."
		data.Files = []*discordgo.File{{Name: "captcha.png", ContentType: "image/png", Reader: bytes.NewReader(png)}}
	} else {
		// Image fallback: show the obfuscated code as text and still require typing.
		data.Content = "Type this code (ignore the spaces) after clicking **Enter code**:\n`" + obfuscate(code) + "`"
	}
	return c.RespondData(data)
}

// onEnter opens the answer modal for the clicking user.
func onEnter(c *interactions.Context, d plugin.Deps) error {
	row := discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.TextInput{
			CustomID:    "code",
			Label:       "Verification code",
			Style:       discordgo.TextInputShort,
			Placeholder: "ABC123",
			Required:    boolPtr(true),
			MinLength:   captchaLen,
			MaxLength:   captchaLen,
		},
	}}
	return c.RespondModal(idAnswer, "Enter your code", []discordgo.MessageComponent{row})
}

// ── Modal ────────────────────────────────────────────────────

func onModal(c *interactions.Context, d plugin.Deps) error {
	if c.CustomID() != idAnswer {
		return nil
	}
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return c.RespondEphemeral("Verification is not available right now.")
	}
	uid := userIDOf(c)
	if uid == "" {
		return c.RespondEphemeral("Could not read your account; please try again.")
	}

	want, err := getString(c.Ctx, d, keyCode+c.GuildID+":"+uid)
	if err != nil || want == "" {
		return c.RespondEphemeral("That code expired. Click **Verify** again for a fresh one.")
	}
	got := strings.ToUpper(strings.TrimSpace(c.ModalValue("code")))
	if got != strings.ToUpper(want) {
		tries, _ := d.Cache.Incr(c.Ctx, keyTries+c.GuildID+":"+uid, codeTTL)
		if tries >= maxTries {
			// Too many wrong answers: drop the code; kick only if auto-kick is on.
			_ = d.Cache.Delete(c.Ctx, keyCode+c.GuildID+":"+uid, keyTries+c.GuildID+":"+uid, keySalt+c.GuildID+":"+uid)
			if cfg.KickAfterMinutes > 0 {
				_ = d.Cache.DeleteHashField(c.Ctx, keyPending+c.GuildID, uid)
				_ = d.Discord.Kick(c.GuildID, uid, "verification: failed captcha")
				publishVerification(c.Ctx, d, event.TypeVerificationFailed, c.GuildID, event.VerificationFailed{
					GuildID: c.GuildID,
					User:    interactionUser(c),
					Member:  c.I.Member,
					Reason:  "failed_captcha",
					Kicked:  true,
				})
				return c.RespondEphemeral("Too many incorrect attempts. You have been removed; you can rejoin and try again.")
			}
			return c.RespondEphemeral("Too many incorrect attempts. Click **Verify** to get a fresh code.")
		}
		left := maxTries - int(tries)
		return c.RespondEphemeral("That code is incorrect. " + strconv.Itoa(left) + " attempt(s) left. Click **Verify** for a new image if you can't read it.")
	}

	if err := pass(c.Ctx, d, c.GuildID, uid, cfg); err != nil {
		return c.RespondEphemeral("Verification hit a snag granting your roles. Please ping a moderator.")
	}
	_ = d.Cache.Delete(c.Ctx, keyCode+c.GuildID+":"+uid, keyTries+c.GuildID+":"+uid, keySalt+c.GuildID+":"+uid)
	onVerified(d, c, cfg)
	return c.RespondEphemeral("✅ You're verified. Welcome in!")
}

// pass removes the unverified role, grants the verified role and clears the
// pending kick entry.
func pass(ctx context.Context, d plugin.Deps, guildID, userID string, cfg Config) error {
	if cfg.UnverifiedRole != "" {
		_ = d.Discord.RemoveRole(guildID, userID, cfg.UnverifiedRole, "verification: passed")
	}
	if cfg.VerifiedRole != "" {
		if err := d.Discord.AddRole(guildID, userID, cfg.VerifiedRole, "verification: passed"); err != nil {
			return err
		}
	}
	_ = d.Cache.DeleteHashField(ctx, keyPending+guildID, userID)
	return nil
}

// ── Kick sweep ───────────────────────────────────────────────

// kickLoop ticks every minute and kicks members past their verification
// deadline who still hold the unverified role. Redis expiry can't call back, so
// we keep a per-guild HASH of {uid -> deadline} (written on join) and sweep it.
// Best-effort: a kick failure (already gone, missing perms) just drops the entry.
func kickLoop(ctx context.Context, d plugin.Deps) {
	t := time.NewTicker(kickTick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweepKicks(ctx, d)
		}
	}
}

func sweepKicks(ctx context.Context, d plugin.Deps) {
	// The set of guilds with pending members is our own Redis index (Cache has no
	// SCAN/set ops, and the store can't enumerate active guilds), populated on
	// each pending join. Empty index => nothing to do.
	guilds, err := d.Cache.HashFields(ctx, keyGuilds)
	if err != nil || len(guilds) == 0 {
		return
	}
	now := time.Now().Unix()
	for guildID := range guilds {
		gid, _ := event.ParseID(guildID)
		pending, err := d.Cache.HashFields(ctx, keyPending+guildID)
		if err != nil || len(pending) == 0 {
			// No pending members left for this guild: drop it from the index.
			_ = d.Cache.DeleteHashField(ctx, keyGuilds, guildID)
			continue
		}
		cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
		if err != nil || !enabled || cfg.KickAfterMinutes <= 0 {
			// Feature off or auto-kick disabled: forget the pending set + index.
			_ = d.Cache.Delete(ctx, keyPending+guildID)
			_ = d.Cache.DeleteHashField(ctx, keyGuilds, guildID)
			continue
		}
		for uid, raw := range pending {
			deadline, _ := strconv.ParseInt(raw, 10, 64)
			if deadline == 0 || now < deadline {
				continue
			}
			// Past deadline: kick only if still unverified (still holds the role).
			if stillUnverified(ctx, d, guildID, uid, cfg) {
				_ = d.Discord.Kick(guildID, uid, "verification: not verified in time")
				publishVerification(ctx, d, event.TypeVerificationFailed, guildID, event.VerificationFailed{
					GuildID: guildID,
					User:    event.User{ID: uid},
					Reason:  "timed_out",
					Kicked:  true,
				})
			}
			_ = d.Cache.DeleteHashField(ctx, keyPending+guildID, uid)
		}
	}
}

// stillUnverified reports whether a member still holds the unverified role (so
// we don't kick someone who verified via another path). If the role isn't
// configured we treat presence-past-deadline as enough.
func stillUnverified(ctx context.Context, d plugin.Deps, guildID, userID string, cfg Config) bool {
	if cfg.UnverifiedRole == "" {
		return true
	}
	m, err := d.Discord.GuildMember(guildID, userID)
	if err != nil || m == nil {
		return false // gone already; nothing to kick.
	}
	for _, r := range m.Roles {
		if r == cfg.UnverifiedRole {
			return true
		}
	}
	return false
}

// ── helpers ──────────────────────────────────────────────────

func userIDOf(c *interactions.Context) string {
	if c.User.ID != "" {
		return c.User.ID
	}
	if c.I.Member != nil {
		return c.I.Member.User.ID
	}
	return ""
}

// getString reads a plain string key, mapping a miss to ("", ErrMiss).
func getString(ctx context.Context, d plugin.Deps, key string) (string, error) {
	var v string
	if err := d.Cache.GetJSON(ctx, key, &v); err != nil {
		return "", err
	}
	return v, nil
}

func guildName(ctx context.Context, d plugin.Deps, gid int64) string {
	if g, err := d.Store.Guilds.Get(ctx, gid); err == nil && g.Name != "" {
		return g.Name
	}
	return "the server"
}

func boolPtr(b bool) *bool { return &b }
