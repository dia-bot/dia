package exec

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand"
	"strings"
	"time"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── Message ops: edit / fetch / purge / crosspost / clear reactions ──────────

func hMessageEdit(ctx context.Context, h *Halt) error {
	var spec cc.SpecMessageEdit
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}

	// Target "reply": edit the command's own interaction reply in place.
	if spec.Target == "reply" {
		if h.Run.InteractionToken == "" {
			return errors.New("message_edit: no reply to edit (no active interaction)")
		}
		send := buildMessageSend(ctx, h, spec.Content, spec.Embeds, spec.Components, nil)
		edit := &discordgo.WebhookEdit{
			Content: ptrString(send.Content),
			Embeds:  &send.Embeds,
			Files:   send.Files,
		}
		if len(send.Components) > 0 {
			edit.Components = &send.Components
		}
		_, err := h.Deps.Discord.EditResponse(refForRun(h.Run, ""), edit)
		return err
	}

	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil || channelID == "" {
		return errors.New("message_edit: invalid channel")
	}
	messageID, err := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	if err != nil || messageID == "" {
		return errors.New("message_edit: invalid message")
	}
	edit := &discordgo.MessageEdit{}
	if spec.Content != "" {
		edit.Content = ptrString(templated(ctx, h, spec.Content))
	}
	if len(spec.Embeds) > 0 {
		embeds := make([]*discordgo.MessageEmbed, 0, len(spec.Embeds))
		for _, em := range spec.Embeds {
			embeds = append(embeds, renderEmbed(ctx, h, em))
		}
		edit.Embeds = &embeds
	}
	if len(spec.Components) > 0 {
		comps := renderComponents(ctx, h, spec.Components)
		edit.Components = &comps
	}
	_, err = h.Deps.Discord.EditMessage(channelID, messageID, edit)
	return err
}

func hMessageFetch(ctx context.Context, h *Halt) error {
	var spec cc.SpecMessageFetch
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil || channelID == "" {
		return errors.New("message_fetch: invalid channel")
	}
	messageID, err := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	if err != nil || messageID == "" {
		return errors.New("message_fetch: invalid message")
	}
	msg, err := h.Deps.Discord.FetchMessage(channelID, messageID)
	if err != nil {
		return err
	}
	out := messageToScope(msg)
	if spec.Into != "" {
		h.Scope.Set(spec.Into, out)
	}
	h.SetOutput(out)
	return nil
}

func hMessagePurge(ctx context.Context, h *Halt) error {
	var spec cc.SpecMessagePurge
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil || channelID == "" {
		return errors.New("message_purge: invalid channel")
	}
	limit := spec.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	fromUser, err := cc.EvalSnowflake(ctx, spec.FromUser, h.Scope)
	if err != nil {
		return errors.New("message_purge: invalid from_user")
	}
	contains := strings.ToLower(templated(ctx, h, spec.Contains))

	msgs, err := h.Deps.Discord.ListMessages(channelID, limit)
	if err != nil {
		return err
	}
	// Bulk delete refuses messages older than 14 days — skip them quietly.
	cutoff := time.Now().Add(-14 * 24 * time.Hour).Add(5 * time.Minute)
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m == nil || m.Author == nil {
			continue
		}
		if ts, terr := discordgo.SnowflakeTimestamp(m.ID); terr == nil && ts.Before(cutoff) {
			continue
		}
		if fromUser != "" && m.Author.ID != fromUser {
			continue
		}
		if spec.BotsOnly && !m.Author.Bot {
			continue
		}
		if contains != "" && !strings.Contains(strings.ToLower(m.Content), contains) {
			continue
		}
		ids = append(ids, m.ID)
	}
	if len(ids) > 0 {
		reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
		if err := h.Deps.Discord.BulkDeleteMessages(channelID, ids, reason); err != nil {
			return err
		}
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, len(ids))
	}
	h.SetOutput(map[string]any{"deleted": len(ids)})
	return nil
}

func hMessageCrosspost(ctx context.Context, h *Halt) error {
	var spec cc.SpecMessageCrosspost
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil || channelID == "" {
		return errors.New("message_crosspost: invalid channel")
	}
	messageID, err := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	if err != nil || messageID == "" {
		return errors.New("message_crosspost: invalid message")
	}
	return h.Deps.Discord.CrosspostMessage(channelID, messageID)
}

func hReactClear(ctx context.Context, h *Halt) error {
	var spec cc.SpecReactClear
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil || channelID == "" {
		return errors.New("react_clear: invalid channel")
	}
	messageID, err := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	if err != nil || messageID == "" {
		return errors.New("react_clear: invalid message")
	}
	return h.Deps.Discord.ClearReactions(channelID, messageID, spec.Emoji)
}

// ── Members / voice / threads / invites ──────────────────────────────────────

func hMemberFetch(ctx context.Context, h *Halt) error {
	var spec cc.SpecMemberFetch
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, err := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if err != nil || userID == "" {
		return errors.New("member_fetch: invalid user")
	}
	m, err := h.Deps.Discord.GetMember(h.Run.GuildID, userID)
	if err != nil {
		return err
	}
	out := map[string]any{
		"id":        userID,
		"nick":      m.Nick,
		"roles":     m.Roles,
		"joined_at": m.JoinedAt.Format(time.RFC3339),
		"pending":   m.Pending,
	}
	if m.User != nil {
		out["username"] = m.User.Username
		out["global_name"] = m.User.GlobalName
		out["bot"] = m.User.Bot
		out["avatar_url"] = m.User.AvatarURL("128")
		out["mention"] = "<@" + m.User.ID + ">"
	}
	if m.CommunicationDisabledUntil != nil {
		out["timed_out_until"] = m.CommunicationDisabledUntil.Format(time.RFC3339)
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, out)
	}
	h.SetOutput(out)
	return nil
}

func hVoiceSet(ctx context.Context, h *Halt) error {
	var spec cc.SpecVoiceSet
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, err := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if err != nil || userID == "" {
		return errors.New("voice_set: invalid user")
	}
	if spec.Mute == nil && spec.Deafen == nil {
		return errors.New("voice_set: nothing to change (set mute and/or deafen)")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	return h.Deps.Discord.SetVoiceState(h.Run.GuildID, userID, spec.Mute, spec.Deafen, reason)
}

func hThreadMember(ctx context.Context, h *Halt) error {
	var spec cc.SpecThreadMember
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	threadID, err := cc.EvalSnowflake(ctx, spec.Thread, h.Scope)
	if err != nil || threadID == "" {
		return errors.New("thread_member: invalid thread")
	}
	userID, err := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if err != nil || userID == "" {
		return errors.New("thread_member: invalid user")
	}
	if spec.Action == "remove" {
		return h.Deps.Discord.ThreadMemberRemove(threadID, userID)
	}
	return h.Deps.Discord.ThreadMemberAdd(threadID, userID)
}

func hInviteCreate(ctx context.Context, h *Halt) error {
	var spec cc.SpecInviteCreate
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil || channelID == "" {
		return errors.New("invite_create: invalid channel")
	}
	maxAge := 0
	if spec.MaxAge != "" {
		d, derr := time.ParseDuration(spec.MaxAge)
		if derr != nil {
			return errors.New("invite_create: invalid max_age duration")
		}
		maxAge = int(d.Seconds())
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	inv, err := h.Deps.Discord.CreateInvite(channelID, maxAge, spec.MaxUses, spec.Temporary, spec.Unique, reason)
	if err != nil {
		return err
	}
	out := map[string]any{"code": inv.Code, "url": "https://discord.gg/" + inv.Code}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, out)
	}
	h.SetOutput(out)
	return nil
}

// ── Pure data steps ──────────────────────────────────────────────────────────

func hPickRandom(ctx context.Context, h *Halt) error {
	var spec cc.SpecPickRandom
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	items, err := cc.EvalList(ctx, spec.From, h.Scope)
	if err != nil || len(items) == 0 {
		return errors.New("pick_random: 'from' is empty or not a list")
	}
	count := spec.Count
	if count <= 0 {
		count = 1
	}
	if count > len(items) {
		count = len(items)
	}
	picked := make([]any, len(items))
	copy(picked, items)
	rand.Shuffle(len(picked), func(i, j int) { picked[i], picked[j] = picked[j], picked[i] })
	picked = picked[:count]

	var out any = picked
	if count == 1 {
		out = picked[0]
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, out)
	}
	h.SetOutput(out)
	return nil
}

func hJSONParse(ctx context.Context, h *Halt) error {
	var spec cc.SpecJSONParse
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	raw, err := cc.EvalString(ctx, spec.Value, h.Scope)
	if err != nil {
		return err
	}
	var parsed any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &parsed); err != nil {
		return errors.New("json_parse: invalid JSON: " + err.Error())
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, parsed)
	}
	h.SetOutput(parsed)
	return nil
}

// messageToScope flattens a fetched message into the template-friendly shape.
func messageToScope(m *discordgo.Message) map[string]any {
	out := map[string]any{
		"id":          m.ID,
		"channel_id":  m.ChannelID,
		"content":     m.Content,
		"pinned":      m.Pinned,
		"embed_count": len(m.Embeds),
	}
	if ts, err := discordgo.SnowflakeTimestamp(m.ID); err == nil {
		out["created_at"] = ts.Format(time.RFC3339)
	}
	if m.Author != nil {
		out["author_id"] = m.Author.ID
		out["author_username"] = m.Author.Username
		out["author_bot"] = m.Author.Bot
		out["author_mention"] = "<@" + m.Author.ID + ">"
	}
	reactions := 0
	for _, r := range m.Reactions {
		if r != nil {
			reactions += r.Count
		}
	}
	out["reaction_count"] = reactions
	return out
}
