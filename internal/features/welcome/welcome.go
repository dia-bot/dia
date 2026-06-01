// Package welcome posts configurable welcome/leave messages and renders
// per-server welcome card images when a member joins.
package welcome

import (
	"bytes"
	"context"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
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
		Description: "Greet new members with custom messages and welcome card images.",
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
	if err != nil || !enabled {
		return err
	}

	name, count := guildInfo(ctx, d, gid, ma.MemberCount)
	v := vars{user: ma.Member.User, server: name, count: count}

	// Optional DM.
	if cfg.DMMessage != "" {
		if ch, err := d.Discord.Session().UserChannelCreate(ma.Member.User.ID); err == nil {
			_, _ = d.Discord.SendMessage(ch.ID, &discordgo.MessageSend{Content: v.apply(cfg.DMMessage)})
		}
	}

	if cfg.ChannelID == "" {
		return nil
	}
	return sendWelcome(ctx, d, cfg, ma.Member, v)
}

func handleLeave(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	mr, err := plugin.DecodeData[event.MemberRemove](env)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(mr.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled || !cfg.LeaveEnabled || cfg.LeaveChannelID == "" {
		return err
	}
	name, count := guildInfo(ctx, d, gid, mr.MemberCount)
	v := vars{user: mr.User, server: name, count: count}
	_, err = d.Discord.SendMessage(cfg.LeaveChannelID, &discordgo.MessageSend{Content: v.apply(cfg.LeaveMessage)})
	return err
}

func handleTest(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil {
		return err
	}
	if !enabled || cfg.ChannelID == "" {
		return c.RespondEphemeral("Welcome is disabled or has no channel set. Configure it on the dashboard first.")
	}
	if err := c.Defer(true); err != nil {
		return err
	}
	name, count := guildInfo(c.Ctx, d, gid, 0)
	member := event.Member{User: c.User}
	v := vars{user: c.User, server: name, count: count}
	if err := sendWelcome(c.Ctx, d, cfg, member, v); err != nil {
		_, e := c.FollowupContent("Failed to send test welcome: " + err.Error())
		return e
	}
	_, err = c.FollowupContent("✅ Sent a test welcome to <#" + cfg.ChannelID + ">.")
	return err
}

// sendWelcome composes and posts the welcome message (+ optional card image).
func sendWelcome(ctx context.Context, d plugin.Deps, cfg Config, member event.Member, v vars) error {
	send := &discordgo.MessageSend{}

	if cfg.Card.Enabled && d.Imaging != nil {
		if png, err := renderCard(ctx, d, cfg.Card, member.User, v); err == nil {
			send.Files = []*discordgo.File{{Name: "welcome.png", ContentType: "image/png", Reader: bytes.NewReader(png)}}
		} else {
			d.Log.Warn("welcome card render failed", "err", err)
		}
	}

	text := v.apply(cfg.Message)
	if cfg.UseEmbed {
		embed := &discordgo.MessageEmbed{Description: text, Color: colorInt(cfg.EmbedColor, 0xB244FC)}
		if len(send.Files) > 0 {
			embed.Image = &discordgo.MessageEmbedImage{URL: "attachment://welcome.png"}
		}
		send.Embeds = []*discordgo.MessageEmbed{embed}
	} else if text != "" {
		send.Content = text
	}

	_, err := d.Discord.SendMessage(cfg.ChannelID, send)
	return err
}

func renderCard(ctx context.Context, d plugin.Deps, card CardConfig, user event.User, v vars) ([]byte, error) {
	return d.Imaging.RenderWelcome(ctx, imaging.WelcomeInput{
		Background:   card.Background,
		AccentColor:  card.AccentColor,
		TextColor:    card.TextColor,
		SubTextColor: card.SubTextColor,
		AvatarURL:    discord.AvatarURL(user.ID, user.Avatar, 256),
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

// vars holds the substitution context for message/card templates.
type vars struct {
	user   event.User
	server string
	count  int
}

func (v vars) apply(s string) string {
	if s == "" {
		return ""
	}
	name := v.user.GlobalName
	if name == "" {
		name = v.user.Username
	}
	return strings.NewReplacer(
		"{user.mention}", "<@"+v.user.ID+">",
		"{username}", v.user.Username,
		"{user}", name,
		"{server}", v.server,
		"{count}", strconv.Itoa(v.count),
	).Replace(s)
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
