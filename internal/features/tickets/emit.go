package tickets

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Log embed colors per lifecycle kind.
const (
	colorOpened   = 0x57F287
	colorClaimed  = 0x5865F2
	colorClosed   = 0xED4245
	colorReopened = 0xFEE75C
	colorDeleted  = 0x4E5058
	colorRated    = brandColor
)

// publishTicket wraps an event.TicketEvent in an Envelope and publishes it on the
// worker event stream (same path as AUTOMOD_ACTION) so automations can trigger
// off ticket lifecycle. Best-effort; failures are logged. Copied from
// moderation/emit.go publishEvent.
func publishTicket(ctx context.Context, d plugin.Deps, t event.Type, payload event.TicketEvent) {
	if d.Bus == nil {
		return
	}
	data, err := json.Marshal(payload)
	if err != nil {
		d.Log.Warn("tickets: marshal event payload failed", "type", t, "err", err)
		return
	}
	envBytes, err := json.Marshal(event.Envelope{Type: t, GuildID: payload.GuildID, TS: time.Now().UnixMilli(), Data: data})
	if err != nil {
		d.Log.Warn("tickets: marshal envelope failed", "type", t, "err", err)
		return
	}
	if err := d.Bus.Publish(ctx, event.Subject(t, payload.GuildID), envBytes, ""); err != nil {
		d.Log.Warn("tickets: publish failed", "type", t, "err", err)
	}
}

// recordEvent appends a lifecycle row to ticket_events (best-effort).
func recordEvent(ctx context.Context, d plugin.Deps, ticketID string, guildID, actorID int64, kind string, data map[string]any) {
	var raw json.RawMessage
	if len(data) > 0 {
		if b, err := json.Marshal(data); err == nil {
			raw = b
		}
	}
	if err := d.Store.Tickets.AddEvent(ctx, store.TicketEvent{
		TicketID: ticketID, GuildID: guildID, Kind: kind, ActorID: actorID, Data: raw,
	}); err != nil {
		d.Log.Debug("tickets: record event", "kind", kind, "err", err)
	}
}

// postLog sends a lifecycle embed to the configured log channel (best-effort).
func postLog(d plugin.Deps, cfg Config, embed *discordgo.MessageEmbed) {
	if cfg.LogChannel == "" || embed == nil {
		return
	}
	_, _ = d.Discord.SendMessage(cfg.LogChannel, &discordgo.MessageSend{
		Embeds:          []*discordgo.MessageEmbed{embed},
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	})
}

// logEmbed builds a lifecycle log embed for a ticket action.
func logEmbed(title string, color int, t store.Ticket, actorID int64, fields ...*discordgo.MessageEmbedField) *discordgo.MessageEmbed {
	base := []*discordgo.MessageEmbedField{
		{Name: "Ticket", Value: fmt.Sprintf("#%d", t.Number), Inline: true},
		{Name: "Opener", Value: "<@" + event.FormatID(t.OpenerID) + ">", Inline: true},
	}
	if t.CategoryLabel != "" {
		base = append(base, &discordgo.MessageEmbedField{Name: "Category", Value: t.CategoryLabel, Inline: true})
	}
	if actorID != 0 {
		base = append(base, &discordgo.MessageEmbedField{Name: "By", Value: "<@" + event.FormatID(actorID) + ">", Inline: true})
	}
	if t.ChannelID != 0 {
		base = append(base, &discordgo.MessageEmbedField{Name: "Channel", Value: "<#" + event.FormatID(t.ChannelID) + ">", Inline: true})
	}
	base = append(base, fields...)
	return &discordgo.MessageEmbed{
		Title:     title,
		Color:     color,
		Fields:    base,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// field is a tiny helper for optional log fields.
func field(name, value string, inline bool) *discordgo.MessageEmbedField {
	return &discordgo.MessageEmbedField{Name: name, Value: value, Inline: inline}
}

// ratingStars renders an N-of-5 star string.
func ratingStars(n int) string {
	if n < 0 {
		n = 0
	}
	if n > 5 {
		n = 5
	}
	stars := ""
	for i := 0; i < 5; i++ {
		if i < n {
			stars += "★"
		} else {
			stars += "☆"
		}
	}
	return stars + " (" + strconv.Itoa(n) + "/5)"
}
