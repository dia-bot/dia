package tickets

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Permission masks for ticket channel overwrites.
const (
	permMember int64 = discordgo.PermissionViewChannel | discordgo.PermissionSendMessages |
		discordgo.PermissionReadMessageHistory | discordgo.PermissionAttachFiles | discordgo.PermissionEmbedLinks
	permStaff int64 = permMember | discordgo.PermissionManageMessages
)

// handleOpen runs when a member clicks an "open" button or picks a category from
// the panel select. It validates access + limits, then either shows the pre-open
// form (a modal must be the interaction's first response) or opens the ticket.
func (p *Plugin) handleOpen(c *interactions.Context, d plugin.Deps, panelID, categoryID string) error {
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil {
		return err
	}
	if !enabled {
		return c.RespondEphemeral("Tickets are not available right now.")
	}
	panel, err := d.Store.Tickets.GetPanel(c.Ctx, gid, panelID)
	if err != nil || !panel.Enabled {
		return c.RespondEphemeral("This ticket panel is no longer available.")
	}
	cat, ok := DecodePanel(panel.Config).Category(categoryID)
	if !ok {
		return c.RespondEphemeral("This ticket option is no longer available.")
	}

	if deny := p.precheckOpen(c, d, cfg, cat); deny != "" {
		return c.RespondEphemeral(deny)
	}

	// A category with a form collects it first; the modal must be the first
	// response (you can't defer then open a modal).
	if len(cat.Form) > 0 {
		return c.RespondModal(formModalID(panelID, categoryID), formTitle(cat), formRows(cat.Form))
	}

	if err := c.Defer(true); err != nil {
		return err
	}
	return p.createAndOpen(c, d, cfg, panel, cat, nil)
}

// handleFormSubmit continues the open flow after the pre-open form is submitted.
func (p *Plugin) handleFormSubmit(c *interactions.Context, d plugin.Deps, panelID, categoryID string) error {
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil {
		return err
	}
	if !enabled {
		return c.RespondEphemeral("Tickets are not available right now.")
	}
	panel, err := d.Store.Tickets.GetPanel(c.Ctx, gid, panelID)
	if err != nil {
		return c.RespondEphemeral("This ticket panel is no longer available.")
	}
	cat, ok := DecodePanel(panel.Config).Category(categoryID)
	if !ok {
		return c.RespondEphemeral("This ticket option is no longer available.")
	}
	answers := map[string]string{}
	for _, f := range cat.Form {
		if v := strings.TrimSpace(c.ModalValue(f.ID)); v != "" {
			answers[f.ID] = v
		}
	}
	if err := c.Defer(true); err != nil {
		return err
	}
	return p.createAndOpen(c, d, cfg, panel, cat, answers)
}

// precheckOpen enforces blacklist, required-role and open-ticket limits before
// creating anything. It returns a denial message (the admin's override from
// cfg.Messages, or the built-in default), or "" when the member may open.
func (p *Plugin) precheckOpen(c *interactions.Context, d plugin.Deps, cfg Config, cat CategoryConfig) string {
	opener := interactionUser(c)
	member := c.I.Member
	gid, _ := event.ParseID(c.GuildID)
	// The scope (with its guild-name lookup) is only built when actually denying.
	deny := func(custom, def string) string {
		sc := ticketScope(c.GuildID, guildName(c.Ctx, d, gid), opener, cat, &ticketView{})
		return sysMsg(custom, def, sc)
	}
	if sliceContains(cfg.BlacklistUserIDs, opener.ID) || memberHasAnyRole(member, cfg.BlacklistRoleIDs) {
		return deny(cfg.Messages.Blacklisted, "You're not allowed to open tickets on this server.")
	}
	if len(cat.RequiredRoleIDs) > 0 && !memberHasAnyRole(member, cat.RequiredRoleIDs) {
		return deny(cfg.Messages.MissingRole, "You don't have the role needed to open this type of ticket.")
	}
	openerID, _ := event.ParseID(opener.ID)
	if cfg.MaxOpenPerUser > 0 {
		if n, err := d.Store.Tickets.CountOpenByOpener(c.Ctx, gid, openerID, ""); err == nil && n >= cfg.MaxOpenPerUser {
			return deny(cfg.Messages.ServerLimit, fmt.Sprintf("You already have %d open tickets. Please close one before opening another.", cfg.MaxOpenPerUser))
		}
	}
	if cat.MaxOpenPerUser > 0 {
		if n, err := d.Store.Tickets.CountOpenByOpener(c.Ctx, gid, openerID, cat.ID); err == nil && n >= cat.MaxOpenPerUser {
			return deny(cfg.Messages.CategoryLimit, "You already have an open ticket of this type.")
		}
	}
	return ""
}

// createAndOpen creates the ticket channel/thread, posts the opening message and
// records the ticket. It runs after Defer(true), so it replies via Followup.
func (p *Plugin) createAndOpen(c *interactions.Context, d plugin.Deps, cfg Config, panel store.TicketPanel, cat CategoryConfig, answers map[string]string) error {
	gid, _ := event.ParseID(c.GuildID)
	opener := interactionUser(c)
	openerID, _ := event.ParseID(opener.ID)
	gName := guildName(c.Ctx, d, gid)
	// Pre-ticket scope for the customizable open-flow replies (no ticket yet).
	scPre := ticketScope(c.GuildID, gName, opener, cat, &ticketView{})

	// Cooldown (consumed only on an actual open attempt).
	if cat.CooldownSeconds > 0 && d.Cache != nil {
		key := "tkt:cd:" + panel.ID + ":" + cat.ID + ":" + opener.ID
		if ok, err := d.Cache.Reserve(c.Ctx, key, time.Duration(cat.CooldownSeconds)*time.Second); err == nil && !ok {
			_, _ = c.FollowupContent(sysMsg(cfg.Messages.Cooldown, "You're opening tickets too quickly. Please wait a moment and try again.", scPre))
			return nil
		}
	}

	subject := firstAnswer(cat, answers)
	answersJSON := json.RawMessage("{}")
	if len(answers) > 0 {
		if b, err := json.Marshal(answers); err == nil {
			answersJSON = b
		}
	}
	acMin, warnMin := 0, 0
	if cat.AutoClose.Enabled && cat.AutoClose.InactivityMinutes > 0 {
		acMin = cat.AutoClose.InactivityMinutes
		warnMin = cat.AutoClose.WarnMinutes
	}

	t, err := d.Store.Tickets.CreateTicketChecked(c.Ctx, store.Ticket{
		GuildID:          gid,
		PanelID:          panel.ID,
		CategoryID:       cat.ID,
		CategoryLabel:    cat.Label,
		OpenerID:         openerID,
		OpenerUsername:   opener.Username,
		OpenerGlobalName: opener.GlobalName,
		Subject:          subject,
		Status:           "open",
		FormAnswers:      answersJSON,
		AutoCloseMinutes: acMin,
		AutoWarnMinutes:  warnMin,
	}, cfg.MaxOpenPerUser, cat.MaxOpenPerUser)
	if errors.Is(err, store.ErrOpenLimit) {
		_, _ = c.FollowupContent(sysMsg(cfg.Messages.ServerLimit, "You've reached the maximum number of open tickets. Please close one before opening another.", scPre))
		return nil
	}
	if errors.Is(err, store.ErrCategoryLimit) {
		_, _ = c.FollowupContent(sysMsg(cfg.Messages.CategoryLimit, "You already have an open ticket of this type.", scPre))
		return nil
	}
	if err != nil {
		d.Log.Warn("tickets: create ticket row", "err", err)
		_, _ = c.FollowupContent(sysMsg(cfg.Messages.OpenFailed, "Something went wrong opening your ticket. Please try again.", scPre))
		return nil
	}

	tv := ticketView{id: t.ID, number: t.Number, subject: subject}
	sc := ticketScope(c.GuildID, gName, opener, cat, &tv)
	name := channelName(cat.NameScheme, sc, t.Number)

	ch, isThread, err := p.createTicketChannel(c, d, cfg, cat, name, opener, t.Number)
	if err != nil {
		d.Log.Warn("tickets: create channel", "err", err)
		_ = d.Store.Tickets.MarkDeleted(c.Ctx, gid, t.ID)
		_, _ = c.FollowupContent(sysMsg(cfg.Messages.OpenFailed, "I couldn't create the ticket channel. The bot may be missing the Manage Channels permission.", scPre))
		return nil
	}
	chID, _ := event.ParseID(ch.ID)
	_ = d.Store.Tickets.SetTicketChannel(c.Ctx, t.ID, chID, isThread)
	t.ChannelID = chID
	t.IsThread = isThread
	_ = d.Store.Tickets.AddParticipant(c.Ctx, t.ID, openerID, "opener", openerID)

	// Opening message with the control buttons.
	tv.channelID = ch.ID
	parts := p.buildOpening(cfg, cat, tv, opener, c.GuildID, gName, true)
	_, _ = d.Discord.SendMessage(ch.ID, &discordgo.MessageSend{
		Content:         parts.content,
		Embeds:          parts.embeds,
		Components:      parts.components,
		AllowedMentions: parts.allowed,
	})

	recordEvent(c.Ctx, d, t.ID, gid, openerID, "opened", map[string]any{"category": cat.Label, "subject": subject})
	payload := ticketPayload(event.TypeTicketOpened, t, cat, opener, c.I.Member)
	publishTicket(c.Ctx, d, event.TypeTicketOpened, payload)
	postLog(d, cfg, logEmbed("Ticket opened", colorOpened, t, openerID))
	p.runTicketAutomation(c.Ctx, d, gid, gName, cat.OnOpenAutomation, "ticket_opened", opener, c.I.Member, t, cat, opener.ID)

	scOpened := ticketScope(c.GuildID, gName, opener, cat, &tv)
	_, _ = c.FollowupContent(sysMsg(cfg.Messages.Opened, "Opened your ticket: {{ .Ticket.Channel }}", scOpened))
	return nil
}

// createTicketChannel makes the private ticket channel (or thread) and returns
// it plus whether it is a thread.
func (p *Plugin) createTicketChannel(c *interactions.Context, d plugin.Deps, cfg Config, cat CategoryConfig, name string, opener event.User, number int) (*discordgo.Channel, bool, error) {
	if cat.OpenMode == OpenModeThread {
		base := c.I.ChannelID
		ch, err := d.Discord.StartThread(base, &discordgo.ThreadStart{
			Name:                name,
			Type:                discordgo.ChannelTypeGuildPrivateThread,
			AutoArchiveDuration: 1440,
			Invitable:           false,
		}, "ticket opened")
		if err != nil {
			return nil, true, err
		}
		_ = d.Discord.ThreadAddMember(ch.ID, opener.ID)
		return ch, true, nil
	}

	ch, err := d.Discord.CreateChannel(c.GuildID, discordgo.GuildChannelCreateData{
		Name:                 name,
		Type:                 discordgo.ChannelTypeGuildText,
		Topic:                fmt.Sprintf("Ticket #%d • opened by <@%s>", number, opener.ID),
		ParentID:             cat.ParentID,
		PermissionOverwrites: ticketOverwrites(c.GuildID, opener.ID, cfg.StaffRoles(cat)),
	}, "ticket opened")
	if err != nil {
		return nil, false, err
	}
	return ch, false, nil
}

// ticketOverwrites hides the channel from @everyone and grants the opener +
// support roles access.
func ticketOverwrites(guildID, openerID string, supportRoles []string) []*discordgo.PermissionOverwrite {
	ow := []*discordgo.PermissionOverwrite{
		{ID: guildID, Type: discordgo.PermissionOverwriteTypeRole, Deny: discordgo.PermissionViewChannel},
		{ID: openerID, Type: discordgo.PermissionOverwriteTypeMember, Allow: permMember},
	}
	for _, r := range supportRoles {
		if r == "" {
			continue
		}
		ow = append(ow, &discordgo.PermissionOverwrite{ID: r, Type: discordgo.PermissionOverwriteTypeRole, Allow: permStaff})
	}
	return ow
}

// buildOpening assembles a ticket's opening message (also used to rebuild it on
// claim/unclaim): the category's fully-composed Welcome spec, any composed
// action/link buttons, then the system Claim/Close row (restyled by
// cat.Buttons). ping controls whether support roles / the opener are actually
// pinged (true on first post, false on later edits). tv.claimerID drives the
// claimed state.
func (p *Plugin) buildOpening(cfg Config, cat CategoryConfig, tv ticketView, opener event.User, guildID, gName string, ping bool) openingParts {
	sc := ticketScope(guildID, gName, opener, cat, &tv)
	content, embeds := renderSpec(cat.Welcome, sc, brandColor)

	am := &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	if ping {
		var mentions []string
		for _, r := range cat.PingRoleIDs {
			if r != "" {
				mentions = append(mentions, "<@&"+r+">")
				am.Roles = append(am.Roles, r)
			}
		}
		if cat.PingOpener && opener.ID != "" {
			mentions = append(mentions, "<@"+opener.ID+">")
			am.Users = append(am.Users, opener.ID)
		}
		if len(mentions) > 0 {
			pref := strings.Join(mentions, " ")
			if content != "" {
				content = pref + "\n" + content
			} else {
				content = pref
			}
		}
	}

	if tv.claimerID != "" {
		claimField := field("Claimed by", "<@"+tv.claimerID+">", true)
		if len(embeds) > 0 {
			embeds[0].Fields = append(embeds[0].Fields, claimField)
		} else {
			embeds = append(embeds, &discordgo.MessageEmbed{
				Description: "Claimed by <@" + tv.claimerID + ">",
				Color:       colorClaimed,
			})
		}
	}
	// A components-only message is rejected by Discord; never post one.
	if content == "" && len(embeds) == 0 {
		content = "Ticket #" + strconv.Itoa(tv.number)
	}

	// Composed rows carry the controls: buttons bound to claim/close route to
	// the native handlers (the claim binding flips to Unclaim while claimed and
	// disappears when claiming is off). Only when nothing composed survives the
	// render does the classic system row stand in, so a ticket always has its
	// controls.
	routes := map[string]specRoute{"close": {ID: closeButtonID(tv.id)}}
	if cat.ClaimEnabled {
		if tv.claimerID == "" {
			routes["claim"] = specRoute{ID: claimButtonID(tv.id)}
		} else {
			routes["claim"] = specRoute{ID: unclaimButtonID(tv.id), Label: "Unclaim"}
		}
	}
	rows := renderSpecRows(cat.Welcome, sc, tv.id, routes)
	if len(rows) > 5 {
		rows = rows[:5]
	}
	if len(rows) == 0 {
		var row discordgo.ActionsRow
		if cat.ClaimEnabled {
			if tv.claimerID == "" {
				row.Components = append(row.Components,
					systemButton(cat.Buttons.Claim, "Claim", "🙋", discordgo.SuccessButton, claimButtonID(tv.id)))
			} else {
				row.Components = append(row.Components,
					systemButton(SystemButton{}, "Unclaim", "", discordgo.SecondaryButton, unclaimButtonID(tv.id)))
			}
		}
		row.Components = append(row.Components,
			systemButton(cat.Buttons.Close, "Close", "🔒", discordgo.DangerButton, closeButtonID(tv.id)))
		rows = []discordgo.MessageComponent{row}
	}

	return openingParts{
		content:    content,
		embeds:     embeds,
		components: rows,
		allowed:    am,
	}
}

type openingParts struct {
	content    string
	embeds     []*discordgo.MessageEmbed
	components []discordgo.MessageComponent
	allowed    *discordgo.MessageAllowedMentions
}

// ── form modal ───────────────────────────────────────────────

func formTitle(cat CategoryConfig) string {
	if cat.Label != "" {
		return trimTo("Open: "+cat.Label, 45)
	}
	return "Open a ticket"
}

func formRows(fields []FormField) []discordgo.MessageComponent {
	rows := make([]discordgo.MessageComponent, 0, len(fields))
	for i, f := range fields {
		if i >= 5 { // Discord caps a modal at 5 inputs
			break
		}
		style := discordgo.TextInputShort
		if f.Style == "paragraph" {
			style = discordgo.TextInputParagraph
		}
		ti := discordgo.TextInput{
			CustomID:    f.ID,
			Label:       trimTo(f.Label, 45),
			Style:       style,
			Placeholder: f.Placeholder,
			Required:    boolPtr(f.Required),
		}
		if f.MinLength > 0 {
			ti.MinLength = f.MinLength
		}
		if f.MaxLength > 0 {
			ti.MaxLength = f.MaxLength
		}
		rows = append(rows, discordgo.ActionsRow{Components: []discordgo.MessageComponent{ti}})
	}
	return rows
}

// firstAnswer derives a short subject from the first answered form field.
func firstAnswer(cat CategoryConfig, answers map[string]string) string {
	for _, f := range cat.Form {
		if v := answers[f.ID]; v != "" {
			return trimTo(v, 200)
		}
	}
	return ""
}

// ── small helpers ────────────────────────────────────────────

func boolPtr(b bool) *bool { return &b }

func trimTo(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func sliceContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func memberHasAnyRole(m *event.Member, roles []string) bool {
	if m == nil || len(roles) == 0 {
		return false
	}
	have := map[string]bool{}
	for _, r := range m.Roles {
		have[r] = true
	}
	for _, r := range roles {
		if have[r] {
			return true
		}
	}
	return false
}

// ticketPayload builds the automations event payload for a ticket lifecycle
// change. opener is the ticket opener; member is their guild member when known.
func ticketPayload(_ event.Type, t store.Ticket, cat CategoryConfig, opener event.User, member *event.Member) event.TicketEvent {
	return event.TicketEvent{
		GuildID:       event.FormatID(t.GuildID),
		TicketID:      t.ID,
		Number:        t.Number,
		PanelID:       t.PanelID,
		CategoryID:    t.CategoryID,
		CategoryLabel: catLabel(t, cat),
		ChannelID:     event.FormatID(t.ChannelID),
		Subject:       t.Subject,
		User:          opener,
		Member:        member,
	}
}

func catLabel(t store.Ticket, cat CategoryConfig) string {
	if cat.Label != "" {
		return cat.Label
	}
	return t.CategoryLabel
}
