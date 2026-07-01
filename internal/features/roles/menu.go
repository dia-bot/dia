package roles

import (
	"encoding/json"
	"strings"

	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Menu modes describe how selecting an option mutates the member's roles.
const (
	// ModeToggle adds the chosen role if absent, removes it if present.
	ModeToggle = "toggle"
	// ModeUnique removes every other option role on the menu, then adds the
	// chosen one (radio-button behaviour).
	ModeUnique = "unique"
	// ModeVerify only ever adds the chosen role (never removes).
	ModeVerify = "verify"
)

// Option is one selectable entry of a reaction-role menu. It is the element type
// of ReactionRoleMenu.Options (a JSONB array) and is authored on the dashboard.
type Option struct {
	RoleID      string `json:"role_id"`
	Label       string `json:"label"`
	Emoji       string `json:"emoji,omitempty"`
	Description string `json:"description,omitempty"`
}

// decodeOptions unmarshals a menu's Options JSONB array.
func decodeOptions(raw json.RawMessage) ([]Option, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var opts []Option
	if err := json.Unmarshal(raw, &opts); err != nil {
		return nil, err
	}
	return opts, nil
}

// optionByRole finds the option matching a role ID (ok=false if not on the menu).
func optionByRole(opts []Option, roleID string) (Option, bool) {
	for _, o := range opts {
		if o.RoleID == roleID {
			return o, true
		}
	}
	return Option{}, false
}

// componentEmoji parses a dashboard-authored emoji string into a ComponentEmoji.
// Supports both unicode emoji ("🎉") and custom-emoji mentions
// ("<:name:123>" / "<a:name:123>"). Returns nil when empty.
func componentEmoji(s string) *discordgo.ComponentEmoji {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		body := strings.Trim(s, "<>")
		animated := strings.HasPrefix(body, "a:")
		body = strings.TrimPrefix(body, "a:")
		parts := strings.Split(body, ":")
		if len(parts) == 2 {
			return &discordgo.ComponentEmoji{Name: parts[0], ID: parts[1], Animated: animated}
		}
	}
	return &discordgo.ComponentEmoji{Name: s}
}

// buildMenuMessage assembles a menu's channel message: an embed when the menu
// has a title, otherwise a plain prompt, plus the option components. Shared by
// the /reactionroles post command and the dashboard post endpoint.
func buildMenuMessage(menu store.ReactionRoleMenu, opts []Option) *discordgo.MessageSend {
	send := &discordgo.MessageSend{Components: buildComponents(menu, opts)}
	if menu.Title != "" {
		send.Embeds = []*discordgo.MessageEmbed{{
			Title:       menu.Title,
			Description: menuDescription(opts),
			Color:       0xB244FC,
		}}
	} else {
		send.Content = "Pick your roles:"
	}
	return send
}

// buildComponents renders a menu's options into message components: buttons for
// small menus (<=5 options) and a string select for larger ones. The custom_ids
// follow the "rr:" routing convention handled by this package.
func buildComponents(menu store.ReactionRoleMenu, opts []Option) []discordgo.MessageComponent {
	id := menu.ID
	if len(opts) <= 5 {
		row := discordgo.ActionsRow{}
		for _, o := range opts {
			label := o.Label
			if label == "" {
				label = "Role"
			}
			row.Components = append(row.Components, discordgo.Button{
				Label:    label,
				Style:    discordgo.SecondaryButton,
				Emoji:    componentEmoji(o.Emoji),
				CustomID: buttonID(id, o.RoleID),
			})
		}
		return []discordgo.MessageComponent{row}
	}

	min := 0
	sel := discordgo.SelectMenu{
		MenuType:    discordgo.StringSelectMenu,
		CustomID:    selectID(id),
		Placeholder: "Select your roles",
		MinValues:   &min,
		MaxValues:   len(opts),
	}
	for _, o := range opts {
		label := o.Label
		if label == "" {
			label = "Role"
		}
		sel.Options = append(sel.Options, discordgo.SelectMenuOption{
			Label:       label,
			Value:       o.RoleID,
			Description: o.Description,
			Emoji:       componentEmoji(o.Emoji),
		})
	}
	return []discordgo.MessageComponent{discordgo.ActionsRow{Components: []discordgo.MessageComponent{sel}}}
}
