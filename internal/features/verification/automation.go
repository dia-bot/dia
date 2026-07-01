package verification

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations/runner"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
)

// onVerified fires when a member clears verification: it publishes the
// "verification_passed" event (so any automation can react) and, when the config
// names one, launches that specific automation flow directly.
func onVerified(d plugin.Deps, c *interactions.Context, cfg Config) {
	ctx := c.Ctx
	gidStr := c.GuildID
	user := interactionUser(c)
	member := c.I.Member
	publishVerification(ctx, d, event.TypeVerificationPassed, gidStr, event.VerificationPassed{
		GuildID:   gidStr,
		User:      user,
		Member:    member,
		Mode:      cfg.Mode,
		ChannelID: cfg.Channel,
	})
	if strings.TrimSpace(cfg.RunAutomation) != "" {
		runVerificationAutomation(ctx, d, gidStr, user, member, runOpts{
			AutomationID: cfg.RunAutomation,
			TriggerKind:  "verification_passed",
			ChannelID:    cfg.Channel,
			Event:        map[string]any{"mode": cfg.Mode, "channel_id": cfg.Channel},
		})
	}
}

// runOpts parameterises runVerificationAutomation so both the on-verify path
// (cfg.RunAutomation, trigger "verification_passed") and a custom-button click
// (an explicit automation id, trigger "verification_click") share one launcher.
type runOpts struct {
	// AutomationID is the saved automation to run.
	AutomationID string
	// TriggerKind labels the durable run ("verification_passed" / "verification_click").
	TriggerKind string
	// ChannelID is the run's channel (the prompt channel, or the clicked channel).
	ChannelID string
	// Event is exposed under the flow's .Event scope (e.g. {custom_id, suffix} for
	// a click, or {mode, channel_id} for a pass).
	Event map[string]any
	// InteractionID / InteractionToken let a click-launched flow reply to the
	// interaction (open a modal, edit the message); empty for the on-verify path.
	InteractionID    string
	InteractionToken string
}

// interactionUser pulls the acting user out of an interaction (the top-level
// user, falling back to the member's user for guild interactions).
func interactionUser(c *interactions.Context) event.User {
	if c.User.ID != "" {
		return c.User
	}
	if c.I.Member != nil {
		return c.I.Member.User
	}
	return c.User
}

// publishVerification publishes a verification event on the worker event stream
// (the same path AUTOMOD_ACTION uses). Best-effort.
func publishVerification(ctx context.Context, d plugin.Deps, t event.Type, gidStr string, payload any) {
	if d.Bus == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		d.Log.Warn("verification: marshal event failed", "type", t, "err", err)
		return
	}
	envBytes, err := json.Marshal(event.Envelope{Type: t, GuildID: gidStr, TS: time.Now().UnixMilli(), Data: data})
	if err != nil {
		d.Log.Warn("verification: marshal envelope failed", "type", t, "err", err)
		return
	}
	if err := d.Bus.Publish(ctx, event.Subject(t, gidStr), envBytes, ""); err != nil {
		d.Log.Warn("verification: publish failed", "type", t, "err", err)
	}
}

// runVerificationAutomation launches a saved automation flow as a durable run,
// building the scope (.User / .Member / .Guild / .Channel + a small .Event) the
// way the automations runtime does so the flow behaves identically to one wired
// on the matching trigger. opts selects which automation runs, the trigger kind,
// the .Event scope and (for a click) the interaction context.
func runVerificationAutomation(ctx context.Context, d plugin.Deps, gidStr string, user event.User, member *event.Member, opts runOpts) {
	id := strings.TrimSpace(opts.AutomationID)
	if id == "" {
		return
	}
	gid, _ := event.ParseID(gidStr)
	auto, err := d.Store.Automations.Get(ctx, gid, id)
	if err != nil {
		d.Log.Warn("verification: automation lookup failed", "automation", id, "err", err)
		return
	}
	if !auto.Enabled {
		return
	}
	var def cc.Definition
	if err := json.Unmarshal(auto.Definition, &def); err != nil {
		d.Log.Warn("verification: automation decode failed", "automation", id, "err", err)
		return
	}

	guildCtx := cc.ContextGuild{ID: gidStr, Name: "the server"}
	if row, err := d.Store.Guilds.Get(ctx, gid); err == nil {
		if row.Name != "" {
			guildCtx.Name = row.Name
		}
		guildCtx.MemberCount = row.MemberCount
	}
	ctxVars := cc.BuildContext(gidStr, opts.ChannelID, user, member, guildCtx, time.Now().UnixMilli())
	scope := cc.NewScope(d.GuildState, gidStr, ctxVars, nil, automationVarDefaults(&def))
	if opts.Event != nil {
		scope.SetEvent(opts.Event)
	}
	runner.New(d).Start(ctx, runner.Meta{
		AutomationID:     auto.ID,
		Version:          auto.Version,
		GuildID:          gidStr,
		InvokerID:        user.ID,
		ActorID:          user.ID,
		ChannelID:        opts.ChannelID,
		TriggerKind:      opts.TriggerKind,
		InteractionID:    opts.InteractionID,
		InteractionToken: opts.InteractionToken,
	}, def, scope)
}

// automationVarDefaults seeds the flow's declared-variable defaults into the run,
// mirroring the automations runtime so an on-verify flow behaves exactly as it
// would under a real "verification_passed" trigger.
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
