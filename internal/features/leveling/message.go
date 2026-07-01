package leveling

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// levelVars is the substitution context for a level-up announcement. It mirrors
// welcome's Vars: a Go-template context (.User / .Guild) plus the {token}
// shorthands, so a message can use {{ }} logic as well as the {user.mention} /
// {level} tokens the rank-card picker already documents.
type levelVars struct {
	user    event.User
	guildID string
	server  string
	level   int
	lookup  templating.Lookup // read-only guild data for getRole/getChannel; nil in previews
}

func (v levelVars) displayName() string {
	if v.user.GlobalName != "" {
		return v.user.GlobalName
	}
	return v.user.Username
}

// tokens returns the {token} shorthand map applied after the template pass.
func (v levelVars) tokens() map[string]string {
	return map[string]string{
		"{user.mention}": "<@" + v.user.ID + ">",
		"{user.name}":    v.user.Username,
		"{username}":     v.user.Username,
		"{user.id}":      v.user.ID,
		"{user.avatar}":  discord.AvatarURL(v.user.ID, v.user.Avatar, 256),
		"{user}":         v.displayName(),
		"{server}":       v.server,
		"{level}":        strconv.Itoa(v.level),
	}
}

// tmplContext is the data root (.) for the template engine.
func (v levelVars) tmplContext() *templating.Context {
	u := templating.User{
		ID:         v.user.ID,
		Username:   v.user.Username,
		GlobalName: v.user.GlobalName,
		Avatar:     discord.AvatarURL(v.user.ID, v.user.Avatar, 256),
		Bot:        v.user.Bot,
	}
	return &templating.Context{
		User:   u,
		Member: templating.Member{User: u},
		Guild:  templating.Guild{ID: v.guildID, Name: v.server},
	}
}

// render runs the pure template engine (logic + functions) then the {token}
// shorthands, matching welcome's message rendering.
func (v levelVars) render(s string) string {
	if s == "" {
		return ""
	}
	return templating.RenderMessage(context.Background(), s, v.tmplContext(), v.lookup, v.tokens())
}

// hasLevelUp reports whether the rich message has anything to render.
func hasLevelUp(m LevelUpMsg) bool {
	return strings.TrimSpace(m.Content) != "" || len(m.Embeds) > 0
}

// buildLevelUp renders the rich level-up message (content + embeds) for one
// LevelUpMsg, or returns nil when it renders to nothing.
func buildLevelUp(msg LevelUpMsg, v levelVars) *discordgo.MessageSend {
	send := &discordgo.MessageSend{}
	if c := v.render(msg.Content); c != "" {
		send.Content = c
	}
	for _, e := range msg.Embeds {
		if em := buildEmbed(e, v); em != nil {
			send.Embeds = append(send.Embeds, em)
		}
	}
	if send.Content == "" && len(send.Embeds) == 0 {
		return nil
	}
	return send
}

// buildEmbed renders one templated EmbedSpec into a Discord embed, or nil when
// it would be empty.
func buildEmbed(e cc.EmbedSpec, v levelVars) *discordgo.MessageEmbed {
	em := &discordgo.MessageEmbed{
		Title:       v.render(e.Title),
		Description: v.render(e.Description),
		URL:         v.render(e.URL),
		Color:       colorInt(e.Color, 0xB244FC),
	}
	if e.AuthorName != "" {
		em.Author = &discordgo.MessageEmbedAuthor{Name: v.render(e.AuthorName), IconURL: v.render(e.AuthorIcon), URL: v.render(e.AuthorURL)}
	}
	if t := v.render(e.Thumbnail); t != "" {
		em.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: t}
	}
	if u := v.render(e.ImageURL); u != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: u}
	}
	if e.FooterText != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: v.render(e.FooterText), IconURL: v.render(e.FooterIcon)}
	}
	for _, f := range e.Fields {
		if f.Name == "" && f.Value == "" {
			continue
		}
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name: v.render(f.Name), Value: v.render(f.Value), Inline: f.Inline,
		})
	}
	if e.Timestamp {
		em.Timestamp = time.Now().Format(time.RFC3339)
	}
	if em.Title == "" && em.Description == "" && len(em.Fields) == 0 &&
		em.Author == nil && em.Image == nil && em.Thumbnail == nil && em.Footer == nil {
		return nil
	}
	return em
}

// colorInt converts a #RRGGBB string to a Discord embed color int.
func colorInt(hex string, fallback int) int {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return fallback
	}
	n, err := strconv.ParseInt(hex, 16, 32)
	if err != nil {
		return fallback
	}
	return int(n)
}
