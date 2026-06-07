// Package welcome posts configurable welcome/goodbye messages — plain content,
// a full embed, and/or a rendered card image — when members join or leave.
package welcome

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/tmpllookup"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Plugin implements the welcome feature.
type Plugin struct{}

// New returns the welcome plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Welcome",
		Description: "Greet joining members and bid farewell to leaving ones with custom messages, embeds and card images.",
		Category:    plugin.CategoryEngagement,
	}
}

// Init wires the join/leave handlers and the /welcome test command.
func (*Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	reg.OnEvent(event.TypeMemberAdd, func(ctx context.Context, env *event.Envelope) error {
		return handleJoin(ctx, d, env)
	})
	reg.OnEvent(event.TypeMemberRemove, func(ctx context.Context, env *event.Envelope) error {
		return handleLeave(ctx, d, env)
	})

	reg.Command(&interactions.Command{
		Def: interactions.AdminOnly(interactions.Slash("welcome",
			"Preview and manage the welcome message",
			interactions.SubCommand("test", "Send a test welcome message for yourself"),
		)),
		Handler: func(c *interactions.Context) error { return handleTest(c, d) },
	})
	return nil
}

func handleJoin(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	ma, err := plugin.DecodeData[event.MemberAdd](env)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(ma.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled || !cfg.Welcome.Enabled {
		return err
	}
	name, count := guildInfo(ctx, d, gid, ma.MemberCount)
	v := Vars{user: ma.Member.User, guildID: ma.GuildID, server: name, count: count, lookup: tmpllookup.New(ctx, d.GuildState, ma.GuildID)}
	return sendConfigured(ctx, d, cfg.Welcome, v)
}

func handleLeave(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	mr, err := plugin.DecodeData[event.MemberRemove](env)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(mr.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled || !cfg.Goodbye.Enabled {
		return err
	}
	name, count := guildInfo(ctx, d, gid, mr.MemberCount)
	v := Vars{user: mr.User, guildID: mr.GuildID, server: name, count: count, lookup: tmpllookup.New(ctx, d.GuildState, mr.GuildID)}
	return sendConfigured(ctx, d, cfg.Goodbye, v)
}

func handleTest(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil {
		return err
	}
	if !enabled || !cfg.Welcome.Enabled || cfg.Welcome.ChannelID == "" {
		return c.RespondEphemeral("Welcome is disabled or has no channel set. Configure it on the dashboard first.")
	}
	if err := c.Defer(true); err != nil {
		return err
	}
	name, count := guildInfo(c.Ctx, d, gid, 0)
	v := Vars{user: c.User, guildID: c.GuildID, server: name, count: count, lookup: tmpllookup.New(c.Ctx, d.GuildState, c.GuildID)}
	if err := sendConfigured(c.Ctx, d, cfg.Welcome, v); err != nil {
		_, e := c.FollowupContent("Failed to send test welcome: " + err.Error())
		return e
	}
	_, err = c.FollowupContent("✅ Sent a test welcome to <#" + cfg.Welcome.ChannelID + ">.")
	return err
}

// sendConfigured posts one configured message (optionally DMing the member),
// then sending the channel message built by BuildMessage.
func sendConfigured(ctx context.Context, d plugin.Deps, mc MessageConfig, v Vars) error {
	if mc.DM.Enabled && mc.DM.Content != "" {
		if ch, err := d.Discord.Session().UserChannelCreate(v.user.ID); err == nil {
			_, _ = d.Discord.SendMessage(ch.ID, &discordgo.MessageSend{Content: v.render(mc.DM.Content)})
		}
	}
	if mc.ChannelID == "" {
		return nil
	}
	send, err := BuildMessage(ctx, d.Imaging, mc, v)
	if err != nil {
		return err
	}
	_, err = d.Discord.SendMessage(mc.ChannelID, send)
	return err
}

// BuildMessage composes the channel message (content + optional embed + optional
// card image) for one MessageConfig. Exported so the dashboard's Test endpoint
// reuses the exact same rendering the bot uses at runtime.
func BuildMessage(ctx context.Context, img *imaging.Renderer, mc MessageConfig, v Vars) (*discordgo.MessageSend, error) {
	send := &discordgo.MessageSend{}

	cardAttached := false
	if mc.Card.Enabled && img != nil {
		if png, err := renderCard(ctx, img, mc.Card, v); err == nil {
			send.Files = []*discordgo.File{{Name: "card.png", ContentType: "image/png", Reader: bytes.NewReader(png)}}
			cardAttached = true
		}
	}

	if c := v.render(mc.Content); c != "" {
		send.Content = c
	}
	for _, e := range mc.Embeds {
		if !e.Enabled {
			continue
		}
		send.Embeds = append(send.Embeds, buildEmbed(e, v, cardAttached))
	}
	if !mc.PingUser {
		// Render mentions as text without pinging anyone.
		send.AllowedMentions = &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	}
	return send, nil
}

func buildEmbed(e EmbedConfig, v Vars, cardAttached bool) *discordgo.MessageEmbed {
	em := &discordgo.MessageEmbed{
		Title:       v.render(e.Title),
		URL:         e.URL,
		Description: v.render(e.Description),
		Color:       colorInt(e.Color, 0xB244FC),
	}
	if e.AuthorName != "" {
		em.Author = &discordgo.MessageEmbedAuthor{Name: v.render(e.AuthorName), IconURL: v.apply(e.AuthorIcon)}
	}
	if t := v.apply(e.Thumbnail); t != "" {
		em.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: t}
	}
	if e.FooterText != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: v.render(e.FooterText), IconURL: v.apply(e.FooterIcon)}
	}
	for _, f := range e.Fields {
		if f.Name == "" && f.Value == "" {
			continue
		}
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name: v.render(f.Name), Value: v.render(f.Value), Inline: f.Inline,
		})
	}
	// Image: the literal token {card} embeds the generated card; any other value
	// is treated as a URL. (A card with no embed referencing it shows standalone.)
	if u := strings.TrimSpace(e.ImageURL); u == "{card}" {
		if cardAttached {
			em.Image = &discordgo.MessageEmbedImage{URL: "attachment://card.png"}
		}
	} else if u != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: v.apply(u)}
	}
	if e.Timestamp {
		em.Timestamp = time.Now().Format(time.RFC3339)
	}
	return em
}

func renderCard(ctx context.Context, img *imaging.Renderer, card CardConfig, v Vars) ([]byte, error) {
	// Card Studio layout is the primary path; the legacy preset model only
	// renders for configs created before the studio existed.
	if card.Layout != nil {
		return img.RenderLayout(ctx, *card.Layout, v.Map())
	}
	return img.RenderWelcome(ctx, imaging.WelcomeInput{
		Background:   card.Background,
		AccentColor:  card.AccentColor,
		TextColor:    card.TextColor,
		SubTextColor: card.SubTextColor,
		AvatarURL:    discord.AvatarURL(v.user.ID, v.user.Avatar, 256),
		Title:        v.apply(card.Title),
		Subtitle:     v.apply(card.Subtitle),
		Footer:       v.apply(card.Footer),
	})
}

func guildInfo(ctx context.Context, d plugin.Deps, guildID int64, fallbackCount int) (name string, count int) {
	if g, err := d.Store.Guilds.Get(ctx, guildID); err == nil {
		name, count = g.Name, g.MemberCount
	}
	if fallbackCount > 0 {
		count = fallbackCount
	}
	if name == "" {
		name = "the server"
	}
	return name, count
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
