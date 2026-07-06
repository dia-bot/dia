package tickets

import (
	"context"
	"fmt"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── /ticket command (runs inside a ticket channel) ───────────

func (p *Plugin) handleTicketCommand(c *interactions.Context, d plugin.Deps) error {
	sub := c.Subcommand()
	if len(sub) == 0 {
		return c.RespondEphemeral("Unknown subcommand.")
	}
	gid, _ := event.ParseID(c.GuildID)
	chID, _ := event.ParseID(c.I.ChannelID)
	t, err := d.Store.Tickets.GetTicketByChannel(c.Ctx, gid, chID)
	if err != nil {
		return c.RespondEphemeral("This command can only be used inside a ticket channel.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	actor := interactionUser(c)
	actorID, _ := event.ParseID(actor.ID)
	staff := isStaffMember(cfg, cat, c.I.Member)
	opener := actorID == t.OpenerID

	switch sub[0] {
	case "close":
		if !staff && !opener {
			return c.RespondEphemeral("Only staff or the ticket opener can close this ticket.")
		}
		if err := c.Defer(true); err != nil {
			return err
		}
		reason := c.Options().String("reason")
		p.performClose(c.Ctx, d, cfg, cat, t, actor, actorID, reason, "command")
		_, _ = c.FollowupContent("Ticket closed.")
		return nil

	case "claim", "unclaim":
		if !staff {
			return c.RespondEphemeral("Only staff can claim tickets.")
		}
		claim := sub[0] == "claim"
		if err := p.applyClaim(c.Ctx, d, cfg, cat, t, actor, actorID, claim); err != nil {
			return c.RespondEphemeral("Couldn't update the claim.")
		}
		if claim {
			return c.RespondEphemeral("You claimed this ticket.")
		}
		return c.RespondEphemeral("You released this ticket.")

	case "add":
		if !staff {
			return c.RespondEphemeral("Only staff can add members to a ticket.")
		}
		return p.addMember(c, d, t)

	case "remove":
		if !staff {
			return c.RespondEphemeral("Only staff can remove members from a ticket.")
		}
		return p.removeMember(c, d, t)

	case "rename":
		if !staff {
			return c.RespondEphemeral("Only staff can rename a ticket.")
		}
		return p.renameTicket(c, d, t)

	case "note":
		if !staff {
			return c.RespondEphemeral("Only staff can add notes.")
		}
		body := c.Options().String("text")
		if _, err := d.Store.Tickets.AddNote(c.Ctx, store.TicketNote{
			TicketID: t.ID, GuildID: gid, AuthorID: actorID, Body: body,
		}); err != nil {
			return c.RespondEphemeral("Couldn't save the note.")
		}
		recordEvent(c.Ctx, d, t.ID, gid, actorID, "note_added", nil)
		return c.RespondEphemeral("Note saved (only staff and the dashboard can see it).")

	case "transcript":
		if !staff {
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
	return c.RespondEphemeral("Unknown subcommand.")
}

// ── Claim ────────────────────────────────────────────────────

// handleClaim handles the Claim / Unclaim button: it updates the claim and
// rebuilds the opening message in place so the button reflects the new state.
func (p *Plugin) handleClaim(c *interactions.Context, d plugin.Deps, ticketID string, claim bool) error {
	gid, _ := event.ParseID(c.GuildID)
	t, err := d.Store.Tickets.GetTicket(c.Ctx, gid, ticketID)
	if err != nil {
		return c.RespondEphemeral("This ticket no longer exists.")
	}
	cfg, cat := p.resolveTicketConfig(c.Ctx, d, gid, t)
	if !isStaffMember(cfg, cat, c.I.Member) {
		return c.RespondEphemeral("Only staff can claim tickets.")
	}
	actor := interactionUser(c)
	actorID, _ := event.ParseID(actor.ID)
	if err := p.applyClaim(c.Ctx, d, cfg, cat, t, actor, actorID, claim); err != nil {
		return c.RespondEphemeral("Couldn't update the claim.")
	}
	var claimedBy int64
	if claim {
		claimedBy = actorID
	}
	tv := ticketView{id: t.ID, number: t.Number, subject: t.Subject, channelID: event.FormatID(t.ChannelID)}
	parts := p.buildOpening(cfg, cat, tv, openerUser(t), c.GuildID, guildName(c.Ctx, d, gid), claimedBy, false)
	return c.UpdateMessage(&discordgo.InteractionResponseData{
		Content:         parts.content,
		Embeds:          parts.embeds,
		Components:      parts.components,
		AllowedMentions: parts.allowed,
	})
}

// applyClaim persists the claim, logs it and (on claim) publishes TICKET_CLAIMED.
func (p *Plugin) applyClaim(ctx context.Context, d plugin.Deps, cfg Config, cat CategoryConfig, t store.Ticket, actor event.User, actorID int64, claim bool) error {
	claimedBy := int64(0)
	if claim {
		claimedBy = actorID
	}
	if err := d.Store.Tickets.SetClaim(ctx, t.GuildID, t.ID, claimedBy); err != nil {
		return err
	}
	if claim {
		recordEvent(ctx, d, t.ID, t.GuildID, actorID, "claimed", nil)
		payload := ticketPayload(event.TypeTicketClaimed, t, cat, openerUser(t), nil)
		payload.ActorID = actor.ID
		payload.ClaimedBy = actor.ID
		publishTicket(ctx, d, event.TypeTicketClaimed, payload)
		postLog(d, cfg, logEmbed("Ticket claimed", colorClaimed, t, actorID))
	} else {
		recordEvent(ctx, d, t.ID, t.GuildID, actorID, "unclaimed", nil)
	}
	return nil
}

// ── Add / remove / rename members ────────────────────────────

func (p *Plugin) addMember(c *interactions.Context, d plugin.Deps, t store.Ticket) error {
	userID := c.Options().Snowflake("user")
	if userID == "" {
		return c.RespondEphemeral("Please choose a member to add.")
	}
	chID := event.FormatID(t.ChannelID)
	var err error
	if t.IsThread {
		err = d.Discord.ThreadAddMember(chID, userID)
	} else {
		err = d.Discord.SetMemberPermission(chID, userID, permMember, 0, "ticket: member added")
	}
	if err != nil {
		return c.RespondEphemeral("Couldn't add that member: " + err.Error())
	}
	uid, _ := event.ParseID(userID)
	actorID, _ := event.ParseID(interactionUser(c).ID)
	_ = d.Store.Tickets.AddParticipant(c.Ctx, t.ID, uid, "added", actorID)
	recordEvent(c.Ctx, d, t.ID, t.GuildID, actorID, "user_added", map[string]any{"user_id": userID})
	_, _ = d.Discord.SendMessage(chID, &discordgo.MessageSend{
		Content:         fmt.Sprintf("Added <@%s> to the ticket.", userID),
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}, Users: []string{userID}},
	})
	return c.RespondEphemeral("Added <@" + userID + "> to the ticket.")
}

func (p *Plugin) removeMember(c *interactions.Context, d plugin.Deps, t store.Ticket) error {
	userID := c.Options().Snowflake("user")
	if userID == "" {
		return c.RespondEphemeral("Please choose a member to remove.")
	}
	if userID == event.FormatID(t.OpenerID) {
		return c.RespondEphemeral("You can't remove the ticket opener.")
	}
	chID := event.FormatID(t.ChannelID)
	var err error
	if t.IsThread {
		err = d.Discord.Session().ThreadMemberRemove(chID, userID)
	} else {
		err = d.Discord.ClearMemberPermission(chID, userID, "ticket: member removed")
	}
	if err != nil {
		return c.RespondEphemeral("Couldn't remove that member: " + err.Error())
	}
	uid, _ := event.ParseID(userID)
	actorID, _ := event.ParseID(interactionUser(c).ID)
	_ = d.Store.Tickets.RemoveParticipant(c.Ctx, t.ID, uid)
	recordEvent(c.Ctx, d, t.ID, t.GuildID, actorID, "user_removed", map[string]any{"user_id": userID})
	return c.RespondEphemeral("Removed <@" + userID + "> from the ticket.")
}

func (p *Plugin) renameTicket(c *interactions.Context, d plugin.Deps, t store.Ticket) error {
	name := slugChannel(c.Options().String("name"))
	if name == "" {
		return c.RespondEphemeral("Please provide a name.")
	}
	// Channel renames sit in Discord's strict 2-per-10-minutes bucket, so the REST
	// call can block past the 3s ack window; defer first, then follow up.
	if err := c.Defer(true); err != nil {
		return err
	}
	if _, err := d.Discord.EditChannel(event.FormatID(t.ChannelID), &discordgo.ChannelEdit{Name: name}, "ticket: renamed"); err != nil {
		_, _ = c.FollowupContent("Couldn't rename the channel (Discord limits renames to twice per 10 minutes).")
		return nil
	}
	actorID, _ := event.ParseID(interactionUser(c).ID)
	recordEvent(c.Ctx, d, t.ID, t.GuildID, actorID, "renamed", map[string]any{"name": name})
	_, _ = c.FollowupContent("Renamed the ticket to #" + name + ".")
	return nil
}
