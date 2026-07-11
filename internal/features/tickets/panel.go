package tickets

import (
	"context"
	"errors"
	"strings"

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

// panelComponents renders a panel's open controls. User-composed components
// (edited in the dashboard preview like a giveaway's) take precedence: each
// composed button routes by its wiring — ButtonBindings opens a category,
// ButtonActions runs a saved automation, a link opens its URL. The classic
// generated controls are used when nothing is composed; with the "select"
// style the dropdown always leads and composed rows follow it.
func panelComponents(panel store.TicketPanel, pc PanelConfig, sc scope) []discordgo.MessageComponent {
	composed := composedPanelRows(panel, pc, sc)

	cats := pc.Categories
	if len(cats) > maxCategories {
		cats = cats[:maxCategories]
	}
	if len(cats) == 0 {
		return composed
	}

	if panel.Style != "select" && len(composed) > 0 {
		return composed
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
		rows := []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{sel}}}
		rows = append(rows, composed...)
		if len(rows) > 5 {
			rows = rows[:5]
		}
		return rows
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

// composedPanelRows renders the panel's user-composed button rows, routing each
// button: bound to a category → the open handler (dropped when the category no
// longer exists); a link → its (templated) URL; anything else → the panel
// action handler, which runs the saved automation ButtonActions maps it to (or
// acknowledges silently when unwired).
func composedPanelRows(panel store.TicketPanel, pc PanelConfig, sc scope) []discordgo.MessageComponent {
	var out []discordgo.MessageComponent
	for _, row := range pc.Components {
		var comps []discordgo.MessageComponent
		for _, c := range row.Components {
			if c.Type != "" && c.Type != "button" {
				continue
			}
			label := render(c.Label, sc)
			if label == "" {
				label = "Button"
			}
			if catID := pc.ButtonBindings[c.CustomIDSuffix]; catID != "" && c.CustomIDSuffix != "" {
				if _, ok := pc.Category(catID); !ok {
					continue // the bound ticket type was deleted
				}
				btn := discordgo.Button{Label: label, Style: buttonStyle(c.Style), CustomID: openButtonID(panel.ID, catID), Disabled: c.Disabled}
				if em := ticketEmoji(c.Emoji); em != nil {
					btn.Emoji = em
				}
				comps = append(comps, btn)
				continue
			}
			if strings.EqualFold(c.Style, "link") || c.URL != "" {
				url := render(c.URL, sc)
				if url == "" {
					continue
				}
				btn := discordgo.Button{Label: label, Style: discordgo.LinkButton, URL: url, Disabled: c.Disabled}
				if em := ticketEmoji(c.Emoji); em != nil {
					btn.Emoji = em
				}
				comps = append(comps, btn)
				continue
			}
			btn := discordgo.Button{Label: label, Style: buttonStyle(c.Style), CustomID: panelActionID(panel.ID, c.CustomIDSuffix), Disabled: c.Disabled}
			if em := ticketEmoji(c.Emoji); em != nil {
				btn.Emoji = em
			}
			comps = append(comps, btn)
		}
		if len(comps) > 0 {
			out = append(out, discordgo.ActionsRow{Components: comps})
		}
	}
	if len(out) > 5 {
		out = out[:5]
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
