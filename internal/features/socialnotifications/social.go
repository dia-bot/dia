// Package socialnotifications announces activity from followed social
// accounts — Twitch/Kick streams going live, new YouTube videos, Bluesky posts
// and RSS entries — in a configured channel. Push providers are ingested by
// the API's webhook endpoints and polled ones by this plugin's poller; both
// publish SOCIAL_UPDATE envelopes that the announce handler here consumes, so
// automations can trigger off the exact same events.
package socialnotifications

import (
	"context"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/social"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Plugin implements the social notifications feature.
type Plugin struct {
	tmpl    *templating.Engine
	clients *social.Clients
}

// New returns the social notifications plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Social Alerts",
		Description: "Announce when followed creators go live on Twitch or Kick, upload to YouTube, or post on Bluesky and RSS feeds.",
		Category:    plugin.CategoryEngagement,
	}
}

// Init wires the announce handler and the background workers: the poller for
// keyless providers (RSS, Bluesky) and the sync worker that reconciles push
// subscriptions (Twitch EventSub, Kick webhooks, YouTube WebSub leases).
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.tmpl = templating.New()
	p.clients = social.NewClients(d.Config)

	reg.OnEvent(event.TypeSocialUpdate, func(ctx context.Context, env *event.Envelope) error {
		return p.handleUpdate(ctx, d, env)
	})
	reg.Worker("social-poller", func(ctx context.Context) { p.pollLoop(ctx, d) })
	reg.Worker("social-sync", func(ctx context.Context) { p.syncLoop(ctx, d) })
	return nil
}

// platformNames maps provider keys to their user-facing names.
var platformNames = map[string]string{
	social.ProviderTwitch:  "Twitch",
	social.ProviderYouTube: "YouTube",
	social.ProviderKick:    "Kick",
	social.ProviderBluesky: "Bluesky",
	social.ProviderRSS:     "RSS",
}

// embedColors gives each provider its brand color.
var embedColors = map[string]int{
	social.ProviderTwitch:  0x9146FF,
	social.ProviderYouTube: 0xFF0000,
	social.ProviderKick:    0x53FC18,
	social.ProviderBluesky: 0x0085FF,
	social.ProviderRSS:     0xFF6363,
}

// defaultTemplate is the announcement line used when a subscription has no
// custom template. All values render through the standard Go template engine.
func defaultTemplate(kind string) string {
	switch kind {
	case social.KindLiveStart:
		return "🔴 **{{ .Account }}** is now live{{ if .Game }} playing **{{ .Game }}**{{ end }}{{ if .Title }}: {{ .Title }}{{ end }}"
	case social.KindNewVideo:
		return "▶️ **{{ .Account }}** uploaded a new video: **{{ .Title }}**"
	default: // new_post
		return "📣 **{{ .Account }}** posted{{ if .Title }}: {{ .Title }}{{ end }}"
	}
}

// handleUpdate announces one social update. live_end never announces (it only
// exists as an automations trigger); everything else respects the feature
// toggle and the subscription's own switches.
func (p *Plugin) handleUpdate(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	upd, err := plugin.DecodeData[event.SocialUpdate](env)
	if err != nil {
		return err
	}
	if upd.Kind == social.KindLiveEnd {
		return nil
	}
	sub, ok, err := d.Store.Social.GetByID(ctx, upd.SubscriptionID)
	if err != nil || !ok || !sub.Enabled {
		return err
	}
	_, enabled, err := plugin.LoadConfig[Config](ctx, d, sub.GuildID, FeatureKey)
	if err != nil || !enabled {
		return err
	}
	send := BuildAnnouncement(ctx, p.tmpl, sub, upd)
	if send == nil {
		return nil
	}
	_, err = d.Discord.SendMessage(event.FormatID(sub.ChannelID), send)
	if err != nil {
		d.Log.Warn("social: announce failed", "guild", upd.GuildID, "provider", upd.Provider, "err", err)
	}
	return err
}

// BuildAnnouncement composes the announcement: an optional role ping, the
// templated line, and either a rich embed or a bare link (Discord then unfurls
// it). Exported so the dashboard's Test endpoint sends exactly what the bot
// would at runtime.
func BuildAnnouncement(ctx context.Context, tmpl *templating.Engine, sub store.SocialSubscription, upd event.SocialUpdate) *discordgo.MessageSend {
	data := map[string]any{
		"Account":  upd.AccountName,
		"Platform": platformNames[upd.Provider],
		"Kind":     upd.Kind,
		"Title":    upd.Title,
		"URL":      upd.URL,
		"Game":     upd.Category,
	}
	src := sub.Template
	if strings.TrimSpace(src) == "" {
		src = defaultTemplate(upd.Kind)
	}
	line, err := tmpl.RenderCard(ctx, src, data)
	if err != nil || strings.TrimSpace(line) == "" {
		line, _ = tmpl.RenderCard(ctx, defaultTemplate(upd.Kind), data)
	}

	send := &discordgo.MessageSend{
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	}
	if sub.PingRoleID != 0 {
		if sub.PingRoleID == sub.GuildID { // the @everyone role
			line = "@everyone " + line
			send.AllowedMentions.Parse = append(send.AllowedMentions.Parse, discordgo.AllowedMentionTypeEveryone)
		} else {
			rid := event.FormatID(sub.PingRoleID)
			line = "<@&" + rid + "> " + line
			send.AllowedMentions.Roles = []string{rid}
		}
	}

	if !sub.Embed {
		if upd.URL != "" {
			line += "\n" + upd.URL
		}
		send.Content = line
		return send
	}

	send.Content = line
	em := &discordgo.MessageEmbed{
		Title: upd.Title,
		URL:   upd.URL,
		Color: embedColors[upd.Provider],
		Author: &discordgo.MessageEmbedAuthor{
			Name: upd.AccountName,
			URL:  upd.AccountURL,
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: platformNames[upd.Provider]},
		Timestamp: time.Now().Format(time.RFC3339),
	}
	if em.Title == "" {
		em.Title = upd.AccountName
	}
	if upd.Description != "" {
		em.Description = truncate(upd.Description, 400)
	}
	if upd.Category != "" {
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{Name: "Category", Value: upd.Category, Inline: true})
	}
	if upd.Thumbnail != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: upd.Thumbnail}
	}
	send.Embeds = []*discordgo.MessageEmbed{em}
	return send
}

// truncate clips s to at most n runes, appending an ellipsis when cut.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
