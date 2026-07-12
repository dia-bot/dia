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
	tv := viewOf(t)
	tv.reason = reason
	tv.closerID = actor.ID
	sc := ticketScope(c.GuildID, guildName(c.Ctx, d, gid), openerUser(t), cat, &tv).withActor(actor)
	_, _ = c.FollowupContent(sysMsg(cfg.Messages.Closed, "Ticket closed.", sc))
	return nil
}

// performClose runs the full close flow: it is interaction-independent so the
// close button, close-request accepts and the auto-close sweep all share it.
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
			p.postClosedMessage(ctx, d, cfg, cat, t, actor, actorID, reason, transcriptURL)
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
	p.runTicketAutomation(ctx, d, t.GuildID, gName, cat.OnCloseAutomation, "ticket_closed", openerUser(t), nil, t, cat, event.FormatID(actorID))

	if cat.Feedback.Enabled {
		p.sendFeedbackPrompt(ctx, d, cat, t, reason)
	}
}

// postClosedMessage posts the closed-state message in a channel-mode ticket:
// the category's composed Closed spec (falling back to the built-in card) plus
// the system Reopen / Delete / Transcript row (restyled by cat.Buttons; the
// optional buttons honour Hide).
func (p *Plugin) postClosedMessage(ctx context.Context, d plugin.Deps, cfg Config, cat CategoryConfig, t store.Ticket, actor event.User, actorID int64, reason, transcriptURL string) {
	tv := viewOf(t)
	tv.reason = reason
	if actorID != 0 {
		tv.closerID = event.FormatID(actorID)
	}
	sc := ticketScope(event.FormatID(t.GuildID), guildName(ctx, d, t.GuildID), openerUser(t), cat, &tv).withActor(actor)

	var content string
	var embeds []*discordgo.MessageEmbed
	var rows []discordgo.MessageComponent
	if !cat.Closed.Empty() {
		routes := map[string]specRoute{
			"reopen":     {ID: reopenButtonID(t.ID)},
			"delete":     {ID: deleteButtonID(t.ID)},
			"transcript": {ID: transcriptButtonID(t.ID)},
		}
		content, embeds = renderSpec(cat.Closed, sc, colorClosed)
		rows = renderSpecRows(cat.Closed, sc, t.ID, routes)
		if len(rows) > 5 {
			rows = rows[:5]
		}
	}
	if content == "" && len(embeds) == 0 {
		desc := "This ticket has been closed."
		if actorID != 0 {
			desc = "Closed by <@" + event.FormatID(actorID) + ">."
		}
		if reason != "" {
			desc += "\n**Reason:** " + trimTo(reason, 500)
		}
		embeds = []*discordgo.MessageEmbed{{Title: "Ticket closed", Description: desc, Color: colorClosed}}
	}

	// The classic system row stands in only when the composition renders no
	// buttons of its own.
	if len(rows) == 0 {
		var row discordgo.ActionsRow
		if !cat.Buttons.Reopen.Hide {
			row.Components = append(row.Components, systemButton(cat.Buttons.Reopen, "Reopen", "", discordgo.SuccessButton, reopenButtonID(t.ID)))
		}
		if !cat.Buttons.Delete.Hide {
			row.Components = append(row.Components, systemButton(cat.Buttons.Delete, "Delete", "", discordgo.DangerButton, deleteButtonID(t.ID)))
		}
		if transcriptURL != "" {
			tb := systemButton(cat.Buttons.Transcript, "Transcript", "", discordgo.LinkButton, "")
			tb.Style, tb.CustomID, tb.URL = discordgo.LinkButton, "", transcriptURL
			row.Components = append(row.Components, tb)
		} else if !cat.Buttons.Transcript.Hide {
			row.Components = append(row.Components, systemButton(cat.Buttons.Transcript, "Transcript", "", discordgo.SecondaryButton, transcriptButtonID(t.ID)))
		}
		if len(row.Components) > 0 {
			rows = append(rows, row)
		}
	}

	_, _ = d.Discord.SendMessage(event.FormatID(t.ChannelID), &discordgo.MessageSend{
		Content:         content,
		Embeds:          embeds,
		Components:      rows,
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	})
}

// sendFeedbackPrompt DMs the opener a 1-5 star rating select after close. The
// message above the select is the category's composed Feedback.Message (falling
// back to the built-in card with Feedback.Prompt as its description).
func (p *Plugin) sendFeedbackPrompt(ctx context.Context, d plugin.Deps, cat CategoryConfig, t store.Ticket, reason string) {
	tv := viewOf(t)
	tv.reason = reason
	sc := ticketScope(event.FormatID(t.GuildID), guildName(ctx, d, t.GuildID), openerUser(t), cat, &tv)

	var content string
	var embeds []*discordgo.MessageEmbed
	var rows []discordgo.MessageComponent
	if !cat.Feedback.Message.Empty() {
		content, embeds = renderSpec(cat.Feedback.Message, sc, brandColor)
		rows = renderSpecRows(cat.Feedback.Message, sc, t.ID, nil)
		if len(rows) > 4 {
			rows = rows[:4]
		}
	}
	if content == "" && len(embeds) == 0 {
		prompt := render(cat.Feedback.Prompt, sc)
		if strings.TrimSpace(prompt) == "" {
			prompt = "How was your support experience?"
		}
		embeds = []*discordgo.MessageEmbed{{
			Title:       "Ticket #" + strconv.Itoa(t.Number) + " closed",
			Description: prompt,
			Color:       brandColor,
		}}
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
	rows = append(rows, discordgo.ActionsRow{Components: []discordgo.MessageComponent{sel}})

	_, _ = d.Discord.SendDMComplex(event.FormatID(t.OpenerID), &discordgo.MessageSend{
		Content:    content,
		Embeds:     embeds,
		Components: rows,
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
	t.Rating = rating
	p.runTicketAutomation(c.Ctx, d, gid, guildName(c.Ctx, d, gid), cat.OnRateAutomation, "ticket_rated", openerUser(t), nil, t, cat, event.FormatID(actorID))

	thanks := ratingStars(rating)
	if strings.TrimSpace(cat.Feedback.ThanksMessage) != "" {
		tv := viewOf(t)
		tv.rating = rating
		sc := ticketScope(guildID, guildName(c.Ctx, d, gid), openerUser(t), cat, &tv)
		if msg := strings.TrimSpace(render(cat.Feedback.ThanksMessage, sc)); msg != "" {
			thanks = msg
		}
	}
	return c.UpdateMessage(&discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "Thanks for your feedback!",
			Description: thanks,
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
	actor := interactionUser(c)
	actorID, _ := event.ParseID(actor.ID)
	chID := event.FormatID(t.ChannelID)
	if t.ChannelID != 0 && !t.IsThread {
		_ = d.Discord.SetMemberPermission(chID, event.FormatID(t.OpenerID), permMember, 0, "ticket: reopened")
		prefix := cfg.NamePrefix
		if prefix == "" {
			prefix = "ticket"
		}
		_, _ = d.Discord.EditChannel(chID, &discordgo.ChannelEdit{Name: slugChannel(prefix + "-" + strconv.Itoa(t.Number))}, "ticket: reopened")
	}
	t.Status = "open"
	t.ClosedBy = 0
	recordEvent(c.Ctx, d, t.ID, gid, actorID, "reopened", nil)
	payload := ticketPayload(event.TypeTicketReopened, t, cat, openerUser(t), nil)
	payload.ActorID = actor.ID
	publishTicket(c.Ctx, d, event.TypeTicketReopened, payload)
	postLog(d, cfg, logEmbed("Ticket reopened", colorReopened, t, actorID))
	gName := guildName(c.Ctx, d, gid)
	p.runTicketAutomation(c.Ctx, d, gid, gName, cat.OnReopenAutomation, "ticket_reopened", openerUser(t), nil, t, cat, actor.ID)

	tv := viewOf(t)
	sc := ticketScope(c.GuildID, gName, openerUser(t), cat, &tv).withActor(actor)
	return c.UpdateMessage(&discordgo.InteractionResponseData{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "Ticket reopened",
			Description: sysMsg(cfg.Messages.Reopened, "Reopened by {{ .Actor.Mention }}.", sc),
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
