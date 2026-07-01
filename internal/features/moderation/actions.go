package moderation

import (
	"bytes"
	"context"
	"strings"
	"text/template"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// plainMessage wraps content for a plain (no-embed) channel send.
func plainMessage(content string) *discordgo.MessageSend {
	return &discordgo.MessageSend{Content: content}
}

// hitContext carries everything an action needs about a single rule firing. It
// is assembled by the engine from the gateway event and the matched rule.
type hitContext struct {
	GuildID   int64
	GuildName string
	Rule      AutomodRule
	Trigger   RuleTrigger
	Reason    string
	User      event.User
	Member    *event.Member
	ChannelID string // trigger channel ("" for member-surface hits)
	MessageID string // offending message ("" for member-surface hits)
	Content   string // offending content (message surface)
	OnMessage bool   // true for message-surface hits (delete allowed)

	modCfg Config // moderation feature config (DMOnAction, LogChannel)
}

// applyActions runs the rule's actions in order against Discord + the case log,
// returning the list of action-type strings actually applied and the total
// escalation points the actions awarded.
func applyActions(ctx context.Context, d plugin.Deps, h hitContext) (applied []string, points int) {
	guildID := event.FormatID(h.GuildID)
	for _, a := range h.Rule.Actions {
		reason := actionReason(h, a)
		switch a.Type {
		case ActionDelete:
			if !h.OnMessage || h.MessageID == "" {
				continue
			}
			if err := d.Discord.DeleteMessage(h.ChannelID, h.MessageID, reason); err != nil {
				d.Log.Warn("automod: delete failed", "channel", h.ChannelID, "msg", h.MessageID, "err", err)
				continue
			}
			applied = append(applied, ActionDelete)

		case ActionWarn:
			recordCase(ctx, d, h, "warn", reason, 0, nil)
			if h.modCfg.DMOnAction && h.User.ID != "" {
				_ = d.Discord.SendDM(h.User.ID, "You have been warned. Reason: "+reason)
			}
			applied = append(applied, ActionWarn)

		case ActionTimeout:
			secs := a.Duration
			if secs <= 0 {
				secs = 600
			}
			until := time.Now().Add(time.Duration(secs) * time.Second)
			if err := d.Discord.Timeout(guildID, h.User.ID, &until, reason); err != nil {
				d.Log.Warn("automod: timeout failed", "user", h.User.ID, "err", err)
				continue
			}
			recordCase(ctx, d, h, "timeout", reason, secs, &until)
			applied = append(applied, ActionTimeout)

		case ActionKick:
			if err := d.Discord.Kick(guildID, h.User.ID, reason); err != nil {
				d.Log.Warn("automod: kick failed", "user", h.User.ID, "err", err)
				continue
			}
			recordCase(ctx, d, h, "kick", reason, 0, nil)
			applied = append(applied, ActionKick)

		case ActionBan:
			days := a.DeleteDays
			if days < 0 {
				days = 0
			}
			if days > 7 {
				days = 7
			}
			var expiresAt *time.Time
			secs := 0
			if a.Duration > 0 {
				t := time.Now().Add(time.Duration(a.Duration) * time.Second)
				expiresAt = &t
				secs = a.Duration
			}
			if err := d.Discord.Ban(guildID, h.User.ID, reason, days); err != nil {
				d.Log.Warn("automod: ban failed", "user", h.User.ID, "err", err)
				continue
			}
			recordCase(ctx, d, h, "ban", reason, secs, expiresAt)
			applied = append(applied, ActionBan)

		case ActionAddRole:
			if a.RoleID == "" {
				continue
			}
			if err := d.Discord.AddRole(guildID, h.User.ID, a.RoleID, reason); err != nil {
				d.Log.Warn("automod: add role failed", "user", h.User.ID, "role", a.RoleID, "err", err)
				continue
			}
			applied = append(applied, ActionAddRole)

		case ActionRemoveRole:
			if a.RoleID == "" {
				continue
			}
			if err := d.Discord.RemoveRole(guildID, h.User.ID, a.RoleID, reason); err != nil {
				d.Log.Warn("automod: remove role failed", "user", h.User.ID, "role", a.RoleID, "err", err)
				continue
			}
			applied = append(applied, ActionRemoveRole)

		case ActionSendMessage:
			channel := a.Channel
			if channel == "" {
				channel = h.ChannelID
			}
			if channel == "" {
				continue
			}
			body := renderTemplate(a.Message, h)
			if strings.TrimSpace(body) == "" {
				continue
			}
			sent, err := sendPlain(d, channel, body)
			if err != nil {
				d.Log.Warn("automod: send_message failed", "channel", channel, "err", err)
				continue
			}
			applied = append(applied, ActionSendMessage)
			if a.DeleteAfter > 0 && sent != "" {
				scheduleDelete(ctx, d, channel, sent, a.DeleteAfter)
			}

		case ActionDM:
			if h.User.ID == "" {
				continue
			}
			body := renderTemplate(a.Message, h)
			if strings.TrimSpace(body) == "" {
				continue
			}
			if err := d.Discord.SendDM(h.User.ID, body); err != nil {
				d.Log.Warn("automod: dm failed", "user", h.User.ID, "err", err)
				continue
			}
			applied = append(applied, ActionDM)

		case ActionAddPoints:
			if a.Points > 0 {
				points += a.Points
				applied = append(applied, ActionAddPoints)
			}

		case ActionRunAutomation:
			// Launch a flow by id, with the same .Event scope the "automod_action"
			// trigger exposes, so a rule can hand off to any automation.
			if runAutomationByID(ctx, d, h, a.AutomationID, "automod_rule", map[string]any{
				"rule_id":      h.Rule.ID,
				"rule_name":    h.Rule.Name,
				"trigger_type": h.Trigger.Type,
				"reason":       h.Reason,
				"content":      truncate(h.Content, 300),
				"message_id":   h.MessageID,
				"channel_id":   h.ChannelID,
			}) {
				applied = append(applied, ActionRunAutomation)
			}
		}
	}
	return applied, points
}

// actionReason picks the per-action reason override, else an auto reason built
// from the rule + trigger.
func actionReason(h hitContext, a RuleAction) string {
	if strings.TrimSpace(a.Reason) != "" {
		return a.Reason
	}
	if strings.TrimSpace(h.Reason) != "" {
		return "[Automod] " + h.Reason
	}
	return "[Automod] " + triggerLabel(h.Trigger.Type)
}

// recordCase writes an automod mod-log case (moderator_id 0 = automod). Timeout
// and temp-ban cases carry ExpiresAt so the mod-expiry worker can reverse them.
func recordCase(ctx context.Context, d plugin.Deps, h hitContext, action, reason string, durSecs int, expiresAt *time.Time) {
	uid, _ := event.ParseID(h.User.ID)
	prefixed := reason
	if !strings.HasPrefix(prefixed, "[Automod]") {
		prefixed = "[Automod] " + reason
	}
	_, err := d.Store.Moderation.CreateCase(ctx, store.ModCase{
		GuildID:         h.GuildID,
		UserID:          uid,
		ModeratorID:     0,
		Action:          action,
		Reason:          prefixed,
		DurationSeconds: durSecs,
		ExpiresAt:       expiresAt,
		Active:          true,
	})
	if err != nil {
		d.Log.Warn("automod: create case failed", "action", action, "err", err)
	}
}

// sendPlain posts a plain-content message and returns the new message id.
func sendPlain(d plugin.Deps, channelID, content string) (string, error) {
	msg, err := d.Discord.SendMessage(channelID, plainMessage(content))
	if err != nil {
		return "", err
	}
	if msg == nil {
		return "", nil
	}
	return msg.ID, nil
}

// scheduleDelete best-effort deletes a message after delay seconds using the
// engine's context so a shutdown cancels the timer.
func scheduleDelete(ctx context.Context, d plugin.Deps, channelID, messageID string, delay int) {
	go func() {
		timer := time.NewTimer(time.Duration(delay) * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			_ = d.Discord.DeleteMessage(channelID, messageID, "Automod auto-delete")
		}
	}()
}

// ── Templating (self-contained; does not pull in customcommands scope) ──

// tmplScope is the small value set exposed to send_message / dm templates.
type tmplScope struct {
	User    tmplUser
	Guild   tmplGuild
	Channel tmplChannel
	Rule    string
	Reason  string
	Content string
}

type tmplUser struct {
	ID       string
	Username string
	Mention  string
}
type tmplGuild struct {
	ID   string
	Name string
}
type tmplChannel struct {
	ID      string
	Mention string
}

// renderTemplate renders raw as a Go text/template against the hit scope. On any
// parse/exec error it falls back to the raw string so a typo never blocks the
// action.
func renderTemplate(raw string, h hitContext) string {
	if !strings.Contains(raw, "{{") {
		return raw
	}
	t, err := template.New("automod").Option("missingkey=zero").Parse(raw)
	if err != nil {
		return raw
	}
	uid := h.User.ID
	scope := tmplScope{
		User: tmplUser{
			ID:       uid,
			Username: userName(h.User),
			Mention:  mentionTag(uid),
		},
		Guild:   tmplGuild{ID: event.FormatID(h.GuildID), Name: h.GuildName},
		Channel: tmplChannel{ID: h.ChannelID, Mention: channelTag(h.ChannelID)},
		Rule:    h.Rule.Name,
		Reason:  h.Reason,
		Content: h.Content,
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, scope); err != nil {
		return raw
	}
	return buf.String()
}

func mentionTag(id string) string {
	if id == "" {
		return ""
	}
	return "<@" + id + ">"
}

func channelTag(id string) string {
	if id == "" {
		return ""
	}
	return "<#" + id + ">"
}
