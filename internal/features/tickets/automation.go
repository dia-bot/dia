package tickets

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations/runner"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
)

// runTicketAutomation launches a saved automation (by id) as a durable run for
// a ticket, exactly like verification's automation hook. It backs both the
// per-category on-open / on-close hooks and composed action buttons. The scope
// mirrors the ticket_* triggers (.User is the passed user, .Event.* carries the
// ticket fields), so a saved automation behaves identically whether it is fired
// by the built-in trigger, a category hook, or a button click.
func (p *Plugin) runTicketAutomation(ctx context.Context, d plugin.Deps, gid int64, gName, automationID, triggerKind string, opener event.User, member *event.Member, t store.Ticket, cat CategoryConfig, actorID string) {
	automationID = strings.TrimSpace(automationID)
	if automationID == "" || p.runner == nil {
		return
	}
	auto, err := d.Store.Automations.Get(ctx, gid, automationID)
	if err != nil || !auto.Enabled {
		return
	}
	var def cc.Definition
	if json.Unmarshal(auto.Definition, &def) != nil {
		return
	}
	guildID := event.FormatID(gid)
	channelID := event.FormatID(t.ChannelID)
	guildCtx := cc.ContextGuild{ID: guildID, Name: gName}
	if g, gerr := d.Store.Guilds.Get(ctx, gid); gerr == nil {
		guildCtx.MemberCount = g.MemberCount
	}
	ctxVars := cc.BuildContext(guildID, channelID, opener, member, guildCtx, time.Now().UnixMilli())
	scope := cc.NewScope(d.GuildState, guildID, ctxVars, nil, automationVarDefaults(&def))
	scope.SetEvent(ticketEventMap(t, cat, actorID))
	p.runner.Start(ctx, runner.Meta{
		AutomationID: auto.ID,
		Version:      auto.Version,
		GuildID:      guildID,
		InvokerID:    opener.ID,
		ActorID:      opener.ID,
		ChannelID:    channelID,
		TriggerKind:  triggerKind,
	}, def, scope)
}

// ticketEventMap builds the .Event.* variables a ticket flow sees. It matches the
// keys the automations runtime exposes for the ticket_* triggers (runtime.go) and
// the web variable picker (TICKET_EVENT_VARS) so a per-category hook and a
// trigger-fired automation are indistinguishable to the flow.
func ticketEventMap(t store.Ticket, cat CategoryConfig, actorID string) map[string]any {
	m := map[string]any{
		"ticket_id":      t.ID,
		"number":         t.Number,
		"panel_id":       t.PanelID,
		"category_id":    t.CategoryID,
		"category_label": catLabel(t, cat),
		"subject":        t.Subject,
		"channel_id":     event.FormatID(t.ChannelID),
		"actor_id":       actorID,
		"claimed_by":     "",
		"closed_by":      "",
		"reason":         t.CloseReason,
		"rating":         t.Rating,
	}
	if t.ClaimedBy != 0 {
		m["claimed_by"] = event.FormatID(t.ClaimedBy)
	}
	if t.ClosedBy != 0 {
		m["closed_by"] = event.FormatID(t.ClosedBy)
	}
	return m
}

// automationVarDefaults seeds declared variables' defaults into the scope
// (mirrors the automations runtime's defaultVars).
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
