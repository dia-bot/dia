package tickets

import (
	"context"
	"errors"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// brandColor is the rose accent used as the embed fallback color.
const brandColor = 0xff6363

// maxCategories is Discord's hard cap on both buttons (5 rows x 5) and select
// options (25) in one message.
const maxCategories = 25

// ErrPanelNoCategories is returned by PostPanel when a panel has no categories to
// open (nothing for members to click).
var ErrPanelNoCategories = errors.New("panel has no ticket categories")

// buildPanelMessage assembles a panel's channel message: the templated content +
// embed plus one open control per category (buttons, or a single select for the
// "select" style). Nothing pings. postChannelID is where it will live (drives
// {{ .Channel.* }} in the panel copy).
func buildPanelMessage(panel store.TicketPanel, pc PanelConfig, guildID, guildName, postChannelID string) *discordgo.MessageSend {
	sc := panelScope(guildID, guildName, postChannelID)
	send := &discordgo.MessageSend{
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	}
	if body := render(pc.Content, sc); body != "" {
		send.Content = body
	}
	for _, e := range pc.Embeds {
		if em := renderEmbed(e, sc, brandColor); em != nil {
			send.Embeds = append(send.Embeds, em)
		}
	}
	send.Components = panelComponents(panel, pc, sc)
	if send.Content == "" && len(send.Embeds) == 0 {
		send.Content = "Open a ticket using the options below."
	}
	return send
}

// panelComponents renders a panel's open controls.
func panelComponents(panel store.TicketPanel, pc PanelConfig, sc scope) []discordgo.MessageComponent {
	cats := pc.Categories
	if len(cats) > maxCategories {
		cats = cats[:maxCategories]
	}
	if len(cats) == 0 {
		return nil
	}

	if panel.Style == "select" {
		placeholder := render(pc.SelectPlaceholder, sc)
		if placeholder == "" {
			placeholder = "Open a ticket"
		}
		sel := discordgo.SelectMenu{
			MenuType:    discordgo.StringSelectMenu,
			CustomID:    selectMenuID(panel.ID),
			Placeholder: placeholder,
		}
		for _, cat := range cats {
			opt := discordgo.SelectMenuOption{
				Label:       optionLabel(cat),
				Value:       cat.ID,
				Description: render(cat.Description, sc),
			}
			if cat.Emoji != "" {
				opt.Emoji = ticketEmoji(cat.Emoji)
			}
			sel.Options = append(sel.Options, opt)
		}
		return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{sel}}}
	}

	// Buttons: up to 5 per row.
	var out []discordgo.MessageComponent
	var row discordgo.ActionsRow
	for _, cat := range cats {
		btn := discordgo.Button{
			Label:    optionLabel(cat),
			Style:    catButtonStyle(cat),
			CustomID: openButtonID(panel.ID, cat.ID),
		}
		if cat.Emoji != "" {
			btn.Emoji = ticketEmoji(cat.Emoji)
		}
		row.Components = append(row.Components, btn)
		if len(row.Components) == 5 {
			out = append(out, row)
			row = discordgo.ActionsRow{}
		}
	}
	if len(row.Components) > 0 {
		out = append(out, row)
	}
	return out
}

func optionLabel(cat CategoryConfig) string {
	if cat.Label != "" {
		return cat.Label
	}
	return "Open ticket"
}

// catButtonStyle defaults a panel button to primary (rather than the secondary
// default of a plain button) so panels read as calls to action.
func catButtonStyle(cat CategoryConfig) discordgo.ButtonStyle {
	if cat.ButtonStyle == "" {
		return discordgo.PrimaryButton
	}
	return buttonStyle(cat.ButtonStyle)
}

// PostPanel builds a panel and posts it to channelID, recording the message id
// so it can be re-published. Guild-scoped: GetPanel returns store.ErrNotFound for
// a panel owned by another guild. Shared by the /tickets command and the
// dashboard publish endpoint.
func PostPanel(ctx context.Context, dc *discord.Client, st *store.Store, guildID, channelID, panelID string) (string, error) {
	gid, _ := event.ParseID(guildID)
	panel, err := st.Tickets.GetPanel(ctx, gid, panelID)
	if err != nil {
		return "", err
	}
	pc := DecodePanel(panel.Config)
	if len(pc.Categories) == 0 {
		return "", ErrPanelNoCategories
	}
	guildName := ""
	if g, err := st.Guilds.Get(ctx, gid); err == nil {
		guildName = g.Name
	}
	msg, err := dc.SendMessage(channelID, buildPanelMessage(panel, pc, guildID, guildName, channelID))
	if err != nil {
		return "", err
	}
	chID, _ := event.ParseID(msg.ChannelID)
	if chID == 0 {
		chID, _ = event.ParseID(channelID)
	}
	msgID, _ := event.ParseID(msg.ID)
	if err := st.Tickets.SetPanelMessage(ctx, gid, panel.ID, chID, msgID); err != nil {
		return "", err
	}
	return msg.ID, nil
}
