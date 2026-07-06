package tickets

import (
	"context"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── Close ────────────────────────────────────────────────────

// handleCloseButton opens the close-reason modal (a modal must be the first
// response to the click).
func (p *Plugin) handleCloseButton(c *interactions.Context, d plugin.Deps, ticketID string) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	if t.Status != "open" {
		return c.RespondEphemeral("This ticket is already closed.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	actorID, _ := event.ParseID(interactionUser(c).ID)
	if !isStaffMember(cfg, cat, c.I.Member) && actorID != t.OpenerID {
		return c.RespondEphemeral("Only staff or the ticket opener can close this ticket.")
	}
	row := discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.TextInput{
			CustomID: "reason", Label: "Reason (optional)", Style: discordgo.TextInputParagraph,
			Required: boolPtr(false), MaxLength: 500, Placeholder: "Why is this ticket being closed?",
		},
	}}
	return c.RespondModal(closeModalID(ticketID), "Close ticket", []discordgo.MessageComponent{row})
}

// handleCloseSubmit closes the ticket after the reason modal is submitted.
func (p *Plugin) handleCloseSubmit(c *interactions.Context, d plugin.Deps, ticketID string) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	actor := interactionUser(c)
	actorID, _ := event.ParseID(actor.ID)
	if !isStaffMember(cfg, cat, c.I.Member) && actorID != t.OpenerID {
		return c.RespondEphemeral("Only staff or the ticket opener can close this ticket.")
	}
	reason := strings.TrimSpace(c.ModalValue("reason"))
	if err := c.Defer(true); err != nil {
		return err
	}
	p.performClose(c.Ctx, d, cfg, cat, t, actor, actorID, reason, "button")
	_, _ = c.FollowupContent("Ticket closed.")
	return nil
}

// performClose runs the full close flow: it is interaction-independent so the
// close button, the /ticket close command and the auto-close sweep all share it.
// actorID 0 means an automatic (system) close.
func (p *Plugin) performClose(ctx context.Context, d plugin.Deps, cfg Config, cat CategoryConfig, t store.Ticket, actor event.User, actorID int64, reason, source string) {
	ok, err := d.Store.Tickets.CloseTicket(ctx, t.GuildID, t.ID, actorID, reason)
	if err != nil {
		d.Log.Warn("tickets: close", "ticket", t.ID, "err", err)
		return
	}
	if !ok {
		return // already closed (a concurrent close won the race)
	}
	t.Status = "closed"
	t.ClosedBy = actorID
	t.CloseReason = reason
	chID := event.FormatID(t.ChannelID)

	transcriptURL := ""
	if cat.Transcript.Enabled && t.ChannelID != 0 {
		if url, err := p.generateAndPostTranscript(ctx, d, cfg, cat, t, openerUser(t)); err == nil {
			transcriptURL = url
		}
	}

	if t.ChannelID != 0 {
		if t.IsThread {
			archived, locked := true, true
			_, _ = d.Discord.EditChannel(chID, &discordgo.ChannelEdit{Archived: &archived, Locked: &locked}, "ticket: closed")
		} else {
			// Read-only for the opener; rename so the archive category reads clearly.
			_ = d.Discord.SetMemberPermission(chID, event.FormatID(t.OpenerID),
				discordgo.PermissionViewChannel|discordgo.PermissionReadMessageHistory,
				discordgo.PermissionSendMessages, "ticket: closed")
			_, _ = d.Discord.EditChannel(chID, &discordgo.ChannelEdit{Name: "closed-" + strconv.Itoa(t.Number)}, "ticket: closed")
			p.postClosedMessage(d, t, actorID, reason, transcriptURL)
		}
	}

	recordEvent(ctx, d, t.ID, t.GuildID, actorID, "closed", map[string]any{"reason": reason, "source": source})
	payload := ticketPayload(event.TypeTicketClosed, t, cat, openerUser(t), nil)
	payload.ActorID = event.FormatID(actorID)
	payload.ClosedBy = event.FormatID(actorID)
	payload.Reason = reason
	publishTicket(ctx, d, event.TypeTicketClosed, payload)

	var extra []*discordgo.MessageEmbedField
	if reason != "" {
		extra = append(extra, field("Reason", trimTo(reason, 500), false))
	}
	if source == "auto" {
		extra = append(extra, field("Trigger", "Inactivity auto-close", true))
	}
	postLog(d, cfg, logEmbed("Ticket closed", colorClosed, t, actorID, extra...))

	gName := guildName(ctx, d, t.GuildID)
	p.runCategoryAutomation(ctx, d, t.GuildID, gName, cat.OnCloseAutomation, "ticket_closed", openerUser(t), nil, t, cat, event.FormatID(actorID))

	if cat.Feedback.Enabled {
		p.sendFeedbackPrompt(d, cat, t)
	}
}

// postClosedMessage posts the staff control row (reopen / delete / transcript)
// in a closed channel-mode ticket.
func (p *Plugin) postClosedMessage(d plugin.Deps, t store.Ticket, actorID int64, reason, transcriptURL string) {
	desc := "This ticket has been closed."
	if actorID != 0 {
		desc = "Closed by <@" + event.FormatID(actorID) + ">."
	}
	if reason != "" {
		desc += "\n**Reason:** " + trimTo(reason, 500)
	}
	row := discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.Button{Style: discordgo.SuccessButton, Label: "Reopen", CustomID: reopenButtonID(t.ID)},
		discordgo.Button{Style: discordgo.DangerButton, Label: "Delete", CustomID: deleteButtonID(t.ID)},
	}}
	if transcriptURL != "" {
		row.Components = append(row.Components, discordgo.Button{Style: discordgo.LinkButton, Label: "Transcript", URL: transcriptURL})
	} else {
		row.Components = append(row.Components, discordgo.Button{Style: discordgo.SecondaryButton, Label: "Transcript", CustomID: transcriptButtonID(t.ID)})
	}
	_, _ = d.Discord.SendMessage(event.FormatID(t.ChannelID), &discordgo.MessageSend{
		Embeds:          []*discordgo.MessageEmbed{{Title: "Ticket closed", Description: desc, Color: colorClosed}},
		Components:      []discordgo.MessageComponent{row},
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	})
}

// sendFeedbackPrompt DMs the opener a 1-5 star rating select after close.
func (p *Plugin) sendFeedbackPrompt(d plugin.Deps, cat CategoryConfig, t store.Ticket) {
	prompt := cat.Feedback.Prompt
	if strings.TrimSpace(prompt) == "" {
		prompt = "How was your support experience?"
	}
	sel := discordgo.SelectMenu{
		MenuType:    discordgo.StringSelectMenu,
		CustomID:    rateSelectID(event.FormatID(t.GuildID), t.ID),
		Placeholder: "Rate your experience",
	}
	labels := []string{"★ Poor", "★★ Fair", "★★★ Good", "★★★★ Great", "★★★★★ Excellent"}
	for i, label := range labels {
		sel.Options = append(sel.Options, discordgo.SelectMenuOption{Label: label, Value: strconv.Itoa(i + 1)})
	}
	_, _ = d.Discord.SendDMComplex(event.FormatID(t.OpenerID), &discordgo.MessageSend{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "Ticket #" + strconv.Itoa(t.Number) + " closed",
			Description: prompt,
			Color:       brandColor,
		}},
		Components: []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{sel}}},
	})
}

// ── Rating ───────────────────────────────────────────────────

func (p *Plugin) handleRate(c *interactions.Context, d plugin.Deps, guildID, ticketID string) error {
	vals := c.ComponentValues()
	if len(vals) == 0 {
		return c.DeferUpdate()
	}
	rating, _ := strconv.Atoi(vals[0])
	if rating < 1 || rating > 5 {
		return c.DeferUpdate()
	}
	gid, _ := event.ParseID(guildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	actorID, _ := event.ParseID(interactionUser(c).ID)
	if actorID != t.OpenerID {
		return c.RespondEphemeral("Only the ticket opener can rate this ticket.")
	}
	if err := d.Store.Tickets.SetRating(c.Ctx, gid, t.ID, rating, ""); err != nil {
		return c.RespondEphemeral("Couldn't record your rating.")
	}
	recordEvent(c.Ctx, d, t.ID, gid, actorID, "rated", map[string]any{"rating": rating})
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	payload := ticketPayload(event.TypeTicketRated, t, cat, openerUser(t), nil)
	payload.ActorID = event.FormatID(actorID)
	payload.Rating = rating
	publishTicket(c.Ctx, d, event.TypeTicketRated, payload)
	postLog(d, cfg, logEmbed("Ticket rated", colorRated, t, actorID, field("Rating", ratingStars(rating), true)))
	return c.UpdateMessage(&discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "Thanks for your feedback!",
			Description: ratingStars(rating),
			Color:       brandColor,
		}},
		Components: []discordgo.MessageComponent{},
	})
}

// ── Reopen / delete ──────────────────────────────────────────

func (p *Plugin) handleReopen(c *interactions.Context, d plugin.Deps, ticketID string) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	if !isStaffMember(cfg, cat, c.I.Member) {
		return c.RespondEphemeral("Only staff can reopen tickets.")
	}
	ok, err := d.Store.Tickets.ReopenTicket(c.Ctx, gid, t.ID)
	if err != nil {
		return c.RespondEphemeral("Couldn't reopen the ticket.")
	}
	if !ok {
		return c.RespondEphemeral("This ticket isn't closed.")
	}
	actorID, _ := event.ParseID(interactionUser(c).ID)
	chID := event.FormatID(t.ChannelID)
	if t.ChannelID != 0 && !t.IsThread {
		_ = d.Discord.SetMemberPermission(chID, event.FormatID(t.OpenerID), permMember, 0, "ticket: reopened")
		prefix := cfg.NamePrefix
		if prefix == "" {
			prefix = "ticket"
		}
		_, _ = d.Discord.EditChannel(chID, &discordgo.ChannelEdit{Name: slugChannel(prefix + "-" + strconv.Itoa(t.Number))}, "ticket: reopened")
	}
	recordEvent(c.Ctx, d, t.ID, gid, actorID, "reopened", nil)
	postLog(d, cfg, logEmbed("Ticket reopened", colorReopened, t, actorID))
	return c.UpdateMessage(&discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "Ticket reopened",
			Description: "Reopened by <@" + event.FormatID(actorID) + ">.",
			Color:       colorReopened,
		}},
		Components: []discordgo.MessageComponent{},
	})
}

func (p *Plugin) handleDelete(c *interactions.Context, d plugin.Deps, ticketID string) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	if !isStaffMember(cfg, cat, c.I.Member) {
		return c.RespondEphemeral("Only staff can delete tickets.")
	}
	actorID, _ := event.ParseID(interactionUser(c).ID)
	_ = c.DeferUpdate() // ack before the channel disappears
	_ = d.Store.Tickets.MarkDeleted(c.Ctx, gid, t.ID)
	recordEvent(c.Ctx, d, t.ID, gid, actorID, "deleted", nil)
	postLog(d, cfg, logEmbed("Ticket deleted", colorDeleted, t, actorID))
	if t.ChannelID != 0 {
		_ = d.Discord.DeleteChannel(event.FormatID(t.ChannelID), "ticket: deleted")
	}
	return nil
}

// handleTranscriptButton regenerates a transcript on demand.
func (p *Plugin) handleTranscriptButton(c *interactions.Context, d plugin.Deps, ticketID string) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	if !isStaffMember(cfg, cat, c.I.Member) {
		return c.RespondEphemeral("Only staff can generate a transcript.")
	}
	if err := c.Defer(true); err != nil {
		return err
	}
	if _, err := p.generateAndPostTranscript(c.Ctx, d, cfg, cat, t, openerUser(t)); err != nil {
		_, _ = c.FollowupContent("Couldn't generate the transcript.")
		return nil
	}
	_, _ = c.FollowupContent("Transcript generated.")
	return nil
}
