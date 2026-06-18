package moderation

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// emitResult bundles what a finished hit produced, for the event payload + log.
type emitResult struct {
	Applied     []string
	Points      int
	TotalPoints int
	Escalated   string
}

// emit publishes the internal AUTOMOD_ACTION event (so Automations can react)
// and posts the alert/log embed. Both are best-effort; failures are logged.
func emit(ctx context.Context, d plugin.Deps, h hitContext, automodCfg AutomodConfig, res emitResult) {
	publishAutomodEvent(ctx, d, h, res)
	postAutomodLog(d, h, automodCfg, res)
}

// publishAutomodEvent wraps an event.AutomodAction payload in an Envelope and
// publishes it on the AUTOMOD_ACTION subject for the guild.
func publishAutomodEvent(ctx context.Context, d plugin.Deps, h hitContext, res emitResult) {
	if d.Bus == nil {
		return
	}
	gid := event.FormatID(h.GuildID)
	payload := event.AutomodAction{
		GuildID:     gid,
		RuleID:      h.Rule.ID,
		RuleName:    h.Rule.Name,
		TriggerType: h.Trigger.Type,
		Reason:      h.Reason,
		Actions:     res.Applied,
		Points:      res.Points,
		TotalPoints: res.TotalPoints,
		Escalated:   res.Escalated,
		User:        h.User,
		Member:      h.Member,
		ChannelID:   h.ChannelID,
		MessageID:   h.MessageID,
		Content:     truncate(h.Content, 300),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		d.Log.Warn("automod: marshal payload failed", "err", err)
		return
	}
	envBytes, err := json.Marshal(event.Envelope{
		Type:    event.TypeAutomodAction,
		GuildID: gid,
		TS:      time.Now().UnixMilli(),
		Data:    data,
	})
	if err != nil {
		d.Log.Warn("automod: marshal envelope failed", "err", err)
		return
	}
	subject := event.Subject(event.TypeAutomodAction, gid)
	if err := d.Bus.Publish(ctx, subject, envBytes, ""); err != nil {
		d.Log.Warn("automod: publish failed", "subject", subject, "err", err)
	}
}

// postAutomodLog posts the automod embed: to AlertChannel if configured, else to
// the moderation log channel.
func postAutomodLog(d plugin.Deps, h hitContext, automodCfg AutomodConfig, res emitResult) {
	channel := strings.TrimSpace(automodCfg.AlertChannel)
	if channel == "" {
		channel = strings.TrimSpace(h.modCfg.LogChannel)
	}
	if channel == "" {
		return
	}
	embed := automodEmbed(h, res)
	_, _ = d.Discord.SendMessage(channel, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}})
}

// automodEmbed renders the automod hit for the alert/log channel.
func automodEmbed(h hitContext, res emitResult) *discordgo.MessageEmbed {
	fields := []*discordgo.MessageEmbedField{
		{Name: "User", Value: mention(h.User.ID, userName(h.User)), Inline: true},
		{Name: "Rule", Value: nonEmpty(h.Rule.Name), Inline: true},
		{Name: "Trigger", Value: triggerLabel(h.Trigger.Type), Inline: true},
	}
	if h.ChannelID != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Channel", Value: channelTag(h.ChannelID), Inline: true})
	}
	if len(res.Applied) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Actions", Value: strings.Join(res.Applied, ", "), Inline: true})
	}
	if res.Points > 0 {
		val := "+" + strconv.Itoa(res.Points) + " (" + strconv.Itoa(res.TotalPoints) + " active)"
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Points", Value: val, Inline: true})
	}
	if res.Escalated != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Escalated", Value: actionTitle(res.Escalated), Inline: true})
	}
	if h.Reason != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Reason", Value: truncate(h.Reason, 200), Inline: false})
	}
	if h.Content != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Content", Value: truncate(h.Content, 300), Inline: false})
	}
	return &discordgo.MessageEmbed{
		Title:     "Automod — " + nonEmpty(h.Rule.Name),
		Color:     0xED4245,
		Fields:    fields,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}
