package tickets

import (
	"strconv"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// handleCloseRequest runs /ticket closerequest: a staff member asks the opener
// to confirm the close instead of closing on them mid-conversation. The request
// message pings the opener with Accept / Keep-open buttons; an optional delay
// auto-accepts after the deadline (the sweep closes it). Re-running replaces
// the pending request.
func (p *Plugin) handleCloseRequest(c *interactions.Context, d plugin.Deps, cfg Config, cat CategoryConfig, t store.Ticket) error {
	actor := interactionUser(c)
	actorID, _ := event.ParseID(actor.ID)
	reason := c.Options().String("reason")
	delayHours := int(c.Options().Int("delay"))

	// Posting the request + log is several REST calls; ack first.
	if err := c.Defer(true); err != nil {
		return err
	}

	var deadline *time.Time
	if delayHours > 0 {
		dl := time.Now().Add(time.Duration(delayHours) * time.Hour)
		deadline = &dl
	}
	ok, err := d.Store.Tickets.SetCloseRequest(c.Ctx, t.GuildID, t.ID, actorID, reason, deadline)
	if err != nil || !ok {
		_, _ = c.FollowupContent("Couldn't create the close request (is the ticket still open?).")
		return nil
	}

	tv := viewOf(t)
	tv.reason = reason
	tv.deadline = deadline
	sc := ticketScope(c.GuildID, guildName(c.Ctx, d, t.GuildID), openerUser(t), cat, &tv).withActor(actor)

	var content string
	var embeds []*discordgo.MessageEmbed
	var rows []discordgo.MessageComponent
	if !cat.CloseRequest.Empty() {
		content, embeds = renderSpec(cat.CloseRequest, sc, brandColor)
		rows = renderSpecRows(cat.CloseRequest, sc, t.ID)
		if len(rows) > 4 {
			rows = rows[:4]
		}
	}
	if content == "" && len(embeds) == 0 {
		desc := "<@" + actor.ID + "> would like to close this ticket."
		if reason != "" {
			desc += "\n**Reason:** " + trimTo(reason, 500)
		}
		if deadline != nil {
			desc += "\n\nIt closes automatically " + sc.Ticket.Deadline + " unless you keep it open."
		}
		embeds = []*discordgo.MessageEmbed{{Title: "Close this ticket?", Description: desc, Color: brandColor}}
	}
	// The opener must be pinged or the request is easy to miss.
	openerMention := "<@" + event.FormatID(t.OpenerID) + ">"
	if content == "" {
		content = openerMention
	} else {
		content = openerMention + "\n" + content
	}

	rows = append(rows, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Style: discordgo.SuccessButton, Label: "Accept & close", CustomID: closeReqAcceptID(t.ID)},
		discordgo.Button{Style: discordgo.SecondaryButton, Label: "Keep open", CustomID: closeReqDenyID(t.ID)},
	}})

	_, sendErr := d.Discord.SendMessage(event.FormatID(t.ChannelID), &discordgo.MessageSend{
		Content:         content,
		Embeds:          embeds,
		Components:      rows,
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}, Users: []string{event.FormatID(t.OpenerID)}},
	})
	if sendErr != nil {
		_, _ = d.Store.Tickets.ClearCloseRequest(c.Ctx, t.GuildID, t.ID)
		_, _ = c.FollowupContent("Couldn't post the close request.")
		return nil
	}

	recordEvent(c.Ctx, d, t.ID, t.GuildID, actorID, "close_requested", map[string]any{"reason": reason, "delay_hours": delayHours})
	payload := ticketPayload(event.TypeTicketCloseRequested, t, cat, openerUser(t), nil)
	payload.ActorID = actor.ID
	payload.Reason = reason
	publishTicket(c.Ctx, d, event.TypeTicketCloseRequested, payload)
	postLog(d, cfg, logEmbed("Close requested", colorClaimed, t, actorID))
	_, _ = c.FollowupContent("Close request sent to the opener.")
	return nil
}

// handleCloseReqAccept closes the ticket when the opener (or staff) accepts a
// pending close request.
func (p *Plugin) handleCloseReqAccept(c *interactions.Context, d plugin.Deps, ticketID string) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	if t.Status != "open" || t.CloseRequestedBy == 0 {
		return c.RespondEphemeral("This close request is no longer active.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	actor := interactionUser(c)
	actorID, _ := event.ParseID(actor.ID)
	if actorID != t.OpenerID && !isStaffMember(cfg, cat, c.I.Member) {
		return c.RespondEphemeral("Only the ticket opener or staff can respond to this.")
	}
	// Strip the buttons in place, then run the shared close flow.
	_ = c.UpdateMessage(&discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "Close request accepted",
			Description: "Accepted by <@" + actor.ID + ">.",
			Color:       colorClosed,
		}},
		Components: []discordgo.MessageComponent{},
	})
	p.performClose(c.Ctx, d, cfg, cat, t, actor, actorID, t.CloseRequestReason, "close_request")
	return nil
}

// handleCloseReqDeny keeps the ticket open when the opener (or staff) declines
// a pending close request.
func (p *Plugin) handleCloseReqDeny(c *interactions.Context, d plugin.Deps, ticketID string) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	actor := interactionUser(c)
	actorID, _ := event.ParseID(actor.ID)
	if actorID != t.OpenerID && !isStaffMember(cfg, cat, c.I.Member) {
		return c.RespondEphemeral("Only the ticket opener or staff can respond to this.")
	}
	ok, err := d.Store.Tickets.ClearCloseRequest(c.Ctx, gid, t.ID)
	if err != nil || !ok {
		return c.RespondEphemeral("This close request is no longer active.")
	}
	recordEvent(c.Ctx, d, t.ID, gid, actorID, "close_request_denied", nil)
	return c.UpdateMessage(&discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "Ticket stays open",
			Description: "<@" + actor.ID + "> kept the ticket open.",
			Color:       colorReopened,
		}},
		Components: []discordgo.MessageComponent{},
	})
}

// handleActionButton fires the saved automation a composed ticket button points
// at (tkt:act:<ticketID>:<suffix>). The mapping lives on the category's message
// specs (welcome / closed / close-request share one suffix namespace). An
// unwired button is acknowledged silently so decorative buttons stay harmless.
func (p *Plugin) handleActionButton(c *interactions.Context, d plugin.Deps, ticketID, suffix string) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	_, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	autoID := ""
	for _, spec := range []MessageSpec{cat.Welcome, cat.Closed, cat.CloseRequest, cat.Feedback.Message} {
		if id := spec.ButtonActions[suffix]; id != "" {
			autoID = id
			break
		}
	}
	if autoID == "" {
		return c.DeferUpdate()
	}
	if err := c.DeferUpdate(); err != nil {
		return err
	}
	clicker := interactionUser(c)
	gName := guildName(c.Ctx, d, gid)
	p.runTicketAutomation(c.Ctx, d, gid, gName, autoID, "ticket_button", clicker, c.I.Member, t, cat, clicker.ID)
	return nil
}

// closeRequestDelayChoices offers the common auto-accept windows.
func closeRequestDelayChoices() []*discordgo.ApplicationCommandOptionChoice {
	hours := []int{1, 2, 6, 12, 24, 48, 72}
	out := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(hours))
	for _, h := range hours {
		label := strconv.Itoa(h) + " hours"
		if h == 1 {
			label = "1 hour"
		}
		out = append(out, interactions.Choice(label, h))
	}
	return out
}
