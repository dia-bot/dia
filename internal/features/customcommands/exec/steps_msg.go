package exec

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── defer_reply / reply / edit_reply / send_message / send_dm / embed_send ──

func hDeferReply(ctx context.Context, h *Halt) error {
	var spec cc.SpecDeferReply
	_ = json.Unmarshal(h.Step.Spec, &spec)
	if h.Scope.Deferred() {
		return nil
	}
	if h.Run.InteractionToken == "" {
		// No interaction context (event/scheduled trigger) — defer is a no-op.
		return nil
	}
	if err := h.Deps.Discord.Defer(refForRun(h.Run, ""), spec.Ephemeral); err != nil {
		return err
	}
	h.Scope.MarkDeferred(true)
	// Interaction tokens are valid for 15 minutes after Defer; record the cap
	// so wait/wait_for can degrade follow-ups to send_message past that.
	exp := time.Now().Add(15 * time.Minute)
	h.Run.InteractionExpires = &exp
	return nil
}

func hReply(ctx context.Context, h *Halt) error {
	var spec cc.SpecReply
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if h.Run.InteractionToken == "" {
		return errors.New("reply without an active interaction (use send_message)")
	}
	send := buildMessageSend(ctx, h, spec.Content, spec.Embeds, spec.Components, spec.Attachments, spec.AllowedMentions)
	files := send.Files

	// Discord allows exactly ONE initial response per interaction. The first
	// reply responds (or resolves the defer); every further reply on the same
	// interaction becomes a follow-up message instead of erroring.
	if h.Scope.Replied() {
		params := &discordgo.WebhookParams{
			Content:         send.Content,
			Embeds:          send.Embeds,
			Components:      send.Components,
			Files:           files,
			AllowedMentions: send.AllowedMentions,
		}
		if spec.Ephemeral {
			params.Flags = discordgo.MessageFlagsEphemeral
		}
		_, err := h.Deps.Discord.Followup(refForRun(h.Run, ""), params)
		return err
	}

	// If we've already deferred, an Edit is required.
	if h.Scope.Deferred() {
		edit := &discordgo.WebhookEdit{
			Content:         ptrString(send.Content),
			Embeds:          &send.Embeds,
			Files:           files,
			AllowedMentions: send.AllowedMentions,
		}
		if len(send.Components) > 0 {
			edit.Components = &send.Components
		}
		if _, err := h.Deps.Discord.EditResponse(refForRun(h.Run, ""), edit); err != nil {
			return err
		}
		h.Scope.MarkReplied(true)
		return nil
	}
	data := &discordgo.InteractionResponseData{
		Content:         send.Content,
		Embeds:          send.Embeds,
		Components:      send.Components,
		Files:           files,
		AllowedMentions: send.AllowedMentions,
	}
	if spec.Ephemeral {
		data.Flags = discordgo.MessageFlagsEphemeral
	}
	if err := h.Deps.Discord.Respond(refForRun(h.Run, ""), &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	}); err != nil {
		return err
	}
	h.Scope.MarkReplied(true)
	return nil
}

func hEditReply(ctx context.Context, h *Halt) error {
	var spec cc.SpecEditReply
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if h.Run.InteractionToken == "" {
		return errors.New("edit_reply without an active interaction")
	}
	send := buildMessageSend(ctx, h, spec.Content, spec.Embeds, spec.Components, spec.Attachments, spec.AllowedMentions)
	edit := &discordgo.WebhookEdit{
		Content:         ptrString(send.Content),
		Embeds:          &send.Embeds,
		Files:           send.Files,
		AllowedMentions: send.AllowedMentions,
	}
	if len(send.Components) > 0 {
		edit.Components = &send.Components
	}
	if _, err := h.Deps.Discord.EditResponse(refForRun(h.Run, ""), edit); err != nil {
		return err
	}
	h.Scope.MarkReplied(true)
	return nil
}

func hSendMessage(ctx context.Context, h *Halt) error {
	var spec cc.SpecSendMessage
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil || channelID == "" {
		return errors.New("send_message: invalid channel")
	}
	send := buildMessageSend(ctx, h, spec.Content, spec.Embeds, spec.Components, spec.Attachments, spec.AllowedMentions)
	if replyID, _ := cc.EvalSnowflake(ctx, spec.ReplyTo, h.Scope); replyID != "" {
		send.Reference = &discordgo.MessageReference{MessageID: replyID, ChannelID: channelID}
	}
	msg, err := h.Deps.Discord.SendMessage(channelID, send)
	if err != nil {
		return err
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, map[string]any{
			"id":         msg.ID,
			"channel_id": msg.ChannelID,
		})
	}
	h.SetOutput(map[string]any{"message_id": msg.ID, "channel_id": msg.ChannelID})
	return nil
}

func hSendDM(ctx context.Context, h *Halt) error {
	var spec cc.SpecSendDM
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, err := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if err != nil || userID == "" {
		return errors.New("send_dm: invalid user")
	}
	send := buildMessageSend(ctx, h, spec.Content, spec.Embeds, spec.Components, spec.Attachments, spec.AllowedMentions)
	_, err = h.Deps.Discord.SendDM(userID, send)
	return err
}

func hEmbedSend(ctx context.Context, h *Halt) error {
	var spec cc.SpecEmbedSend
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil || channelID == "" {
		return errors.New("embed_send: invalid channel")
	}
	// Through buildMessageSend so queued attachments drain here (not onto the
	// NEXT message) and the AllowedMentions default applies like every sender.
	send := buildMessageSend(ctx, h, "", []cc.EmbedSpec{spec.Embed}, nil, nil, spec.AllowedMentions)
	msg, err := h.Deps.Discord.SendMessage(channelID, send)
	if err != nil {
		return err
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, map[string]any{"id": msg.ID, "channel_id": msg.ChannelID})
	}
	h.SetOutput(map[string]any{"message_id": msg.ID, "channel_id": msg.ChannelID})
	return nil
}

// ── modal_open ───────────────────────────────────────────────────────────────

func hModalOpen(ctx context.Context, h *Halt) error {
	var spec cc.SpecModalOpen
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if h.Run.InteractionToken == "" {
		return errors.New("modal_open requires an interaction context")
	}
	if h.Scope.Deferred() {
		return errors.New("modal_open cannot follow a defer (Discord constraint)")
	}
	customID := h.Engine.routePrefix + h.Run.ID + ":" + templated(ctx, h, spec.CustomIDSuffix)
	title := templated(ctx, h, spec.Title)
	rows := make([]discordgo.MessageComponent, 0, len(spec.Fields))
	for _, f := range spec.Fields {
		style := discordgo.TextInputShort
		if strings.EqualFold(f.Style, "paragraph") {
			style = discordgo.TextInputParagraph
		}
		required := f.Required
		rows = append(rows, discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.TextInput{
				CustomID:    f.CustomID,
				Label:       templated(ctx, h, f.Label),
				Style:       style,
				Required:    &required,
				MinLength:   f.MinLength,
				MaxLength:   f.MaxLength,
				Placeholder: templated(ctx, h, f.Placeholder),
				Value:       templated(ctx, h, f.Value),
			},
		}})
	}
	if err := h.Deps.Discord.Respond(refForRun(h.Run, ""), &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID:   customID,
			Title:      title,
			Components: rows,
		},
	}); err != nil {
		return err
	}

	// Pause the run until the modal is submitted.
	timeout := 5 * time.Minute
	if spec.Timeout != "" {
		if d, err := time.ParseDuration(spec.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	if max := h.Engine.maxWaitFor; max > 0 && timeout > max {
		timeout = max
	}
	resume := time.Now().Add(timeout)
	h.Run.markDurable()
	// The modal was shown to whoever drove this interaction (the clicker on
	// a component resume); only they can submit it.
	actor := h.Run.ActorID
	if actor == "" {
		actor = h.Run.InvokerID
	}
	return &PauseError{
		Kind:             "wait_for",
		ResumeAt:         &resume,
		AwaitingCustomID: customID,
		AwaitingUserID:   actor,
		AwaitingKind:     "modal",
	}
}

// ── reactions / pins / delete ────────────────────────────────────────────────

func hMessageDelete(ctx context.Context, h *Halt) error {
	var spec cc.SpecMessageOp
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, _ := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	messageID, _ := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	if channelID == "" || messageID == "" {
		return errors.New("message_delete: channel and message required")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	return h.Deps.Discord.DeleteMessage(channelID, messageID, reason)
}

func hReactAdd(ctx context.Context, h *Halt) error {
	var spec cc.SpecReact
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, _ := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	messageID, _ := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	if channelID == "" || messageID == "" {
		return errors.New("react_add: channel and message required")
	}
	return h.Deps.Discord.AddReaction(channelID, messageID, spec.Emoji)
}

func hReactRemove(ctx context.Context, h *Halt) error {
	var spec cc.SpecReact
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, _ := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	messageID, _ := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	if channelID == "" || messageID == "" {
		return errors.New("react_remove: channel and message required")
	}
	if userID, _ := cc.EvalSnowflake(ctx, spec.User, h.Scope); userID != "" {
		return h.Deps.Discord.RemoveUserReaction(channelID, messageID, spec.Emoji, userID)
	}
	return h.Deps.Discord.RemoveOwnReaction(channelID, messageID, spec.Emoji)
}

func hPinAdd(ctx context.Context, h *Halt) error {
	return pinOp(ctx, h, true)
}
func hPinRemove(ctx context.Context, h *Halt) error {
	return pinOp(ctx, h, false)
}

func pinOp(ctx context.Context, h *Halt, pin bool) error {
	var spec cc.SpecMessageOp
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, _ := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	messageID, _ := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	if channelID == "" || messageID == "" {
		return errors.New("pin: channel and message required")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	if pin {
		return h.Deps.Discord.PinMessage(channelID, messageID, reason)
	}
	return h.Deps.Discord.UnpinMessage(channelID, messageID, reason)
}

// ── Helpers for building a discordgo.MessageSend from a step's content/embeds
// /components/attachments. Templates render here once so a send/edit reads
// the same final values regardless of which surface it ends up on. ──────────

// allowedMentions translates the spec's opt-in flags into Discord's parse list.
// nil keeps the safe default (only user mentions ping); all-false suppresses
// every mention.
func allowedMentions(m *cc.MsgMentions) *discordgo.MessageAllowedMentions {
	if m == nil {
		return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{
			discordgo.AllowedMentionTypeUsers,
		}}
	}
	parse := []discordgo.AllowedMentionType{}
	if m.Users {
		parse = append(parse, discordgo.AllowedMentionTypeUsers)
	}
	if m.Roles {
		parse = append(parse, discordgo.AllowedMentionTypeRoles)
	}
	if m.Everyone {
		parse = append(parse, discordgo.AllowedMentionTypeEveryone)
	}
	return &discordgo.MessageAllowedMentions{Parse: parse}
}

func buildMessageSend(ctx context.Context, h *Halt, content string, embeds []cc.EmbedSpec, components []cc.ComponentRow, atts []cc.AttachmentRef, mentions *cc.MsgMentions) *discordgo.MessageSend {
	send := &discordgo.MessageSend{}
	if rendered, _ := cc.EvalTemplated(ctx, content, h.Scope); rendered != "" {
		send.Content = rendered
	}
	for _, em := range embeds {
		send.Embeds = append(send.Embeds, renderEmbed(ctx, h, em))
	}
	if len(components) > 0 {
		send.Components = renderComponents(ctx, h, components)
	}
	// Attachments: pending (queued by image_attach) first, then explicit.
	// Variable refs read bytes from scope; URL refs are downloaded (capped).
	for _, a := range h.Scope.DrainAttachments() {
		if f := attachmentFrom(h, a.FromVar, a.Filename); f != nil {
			send.Files = append(send.Files, f)
		} else if f := attachmentFromURL(ctx, h, a.URL, a.Filename); f != nil {
			send.Files = append(send.Files, f)
		}
	}
	for _, a := range atts {
		if f := attachmentFrom(h, a.FromVar, a.Filename); f != nil {
			send.Files = append(send.Files, f)
		} else if f := attachmentFromURL(ctx, h, a.URL, a.Filename); f != nil {
			send.Files = append(send.Files, f)
		}
	}
	// Mentions: nil keeps the safe default (only user pings); a spec can opt
	// roles / @everyone in, or suppress everything.
	send.AllowedMentions = allowedMentions(mentions)
	return send
}

// attachmentFromURL downloads a (templated) URL attachment, capped at 8 MiB.
// Failures degrade to skipping the file — the message still sends.
func attachmentFromURL(ctx context.Context, h *Halt, rawURL, filename string) *discordgo.File {
	if rawURL == "" {
		return nil
	}
	u := templated(ctx, h, rawURL)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil
	}
	resp, err := h.Deps.HTTP.Do(ctx, req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	const maxAttachment = 8 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAttachment+1))
	if err != nil || len(data) == 0 || len(data) > maxAttachment {
		return nil
	}
	name := filename
	if name == "" {
		if i := strings.LastIndex(u, "/"); i >= 0 && i < len(u)-1 {
			name = strings.SplitN(u[i+1:], "?", 2)[0]
		}
	}
	if name == "" {
		name = "attachment"
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = http.DetectContentType(data)
	}
	return &discordgo.File{Name: name, ContentType: ct, Reader: bytes.NewReader(data)}
}

func attachmentFrom(h *Halt, fromVar, filename string) *discordgo.File {
	if fromVar == "" {
		return nil
	}
	blob, ok := h.Scope.ImageBlob(fromVar)
	if !ok {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(blob.Bytes)
	if err != nil {
		return nil
	}
	name := filename
	if name == "" {
		name = blob.Filename
	}
	if name == "" {
		name = "attachment.png"
	}
	ct := blob.ContentType
	if ct == "" {
		ct = "image/png"
	}
	return &discordgo.File{Name: name, ContentType: ct, Reader: bytes.NewReader(data)}
}

func renderEmbed(ctx context.Context, h *Halt, em cc.EmbedSpec) *discordgo.MessageEmbed {
	out := &discordgo.MessageEmbed{
		Title:       templated(ctx, h, em.Title),
		Description: templated(ctx, h, em.Description),
		URL:         templated(ctx, h, em.URL),
		Color:       colorFromHex(em.Color, 0xB244FC),
	}
	if em.AuthorName != "" || em.AuthorIcon != "" || em.AuthorURL != "" {
		out.Author = &discordgo.MessageEmbedAuthor{
			Name:    templated(ctx, h, em.AuthorName),
			IconURL: templated(ctx, h, em.AuthorIcon),
			URL:     templated(ctx, h, em.AuthorURL),
		}
	}
	if em.Thumbnail != "" {
		out.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: templated(ctx, h, em.Thumbnail)}
	}
	if em.ImageURL != "" {
		out.Image = &discordgo.MessageEmbedImage{URL: templated(ctx, h, em.ImageURL)}
	}
	if em.FooterText != "" || em.FooterIcon != "" {
		out.Footer = &discordgo.MessageEmbedFooter{
			Text:    templated(ctx, h, em.FooterText),
			IconURL: templated(ctx, h, em.FooterIcon),
		}
	}
	if em.Timestamp {
		out.Timestamp = time.Now().Format(time.RFC3339)
	}
	for _, f := range em.Fields {
		out.Fields = append(out.Fields, &discordgo.MessageEmbedField{
			Name: templated(ctx, h, f.Name), Value: templated(ctx, h, f.Value), Inline: f.Inline,
		})
	}
	return out
}

func renderComponents(ctx context.Context, h *Halt, rows []cc.ComponentRow) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, 0, len(rows))
	for _, row := range rows {
		comps := make([]discordgo.MessageComponent, 0, len(row.Components))
		for _, c := range row.Components {
			comps = append(comps, renderComponent(ctx, h, c))
		}
		if len(comps) > 0 {
			out = append(out, discordgo.ActionsRow{Components: comps})
		}
	}
	return out
}

func renderComponent(ctx context.Context, h *Halt, c cc.Component) discordgo.MessageComponent {
	cid := ""
	if c.CustomIDSuffix != "" {
		// Templated: per-item buttons (e.g. vote_{{ .Vars.idx }}) stay unique
		// inside a loop; the run id already isolates concurrent users.
		cid = h.Engine.routePrefix + h.Run.ID + ":" + templated(ctx, h, c.CustomIDSuffix)
	}
	if c.OnClick == "none" && !strings.EqualFold(c.Style, "link") {
		// Decorative: the custom_id references no run, so clicks resolve to a
		// bare silent acknowledgement forever (suffix only keeps ids unique
		// within the message). Link buttons never carry a custom_id.
		cid = h.Engine.noopPrefix + templated(ctx, h, c.CustomIDSuffix)
	}
	switch c.Type {
	case "button":
		style := buttonStyle(c.Style)
		btn := discordgo.Button{
			CustomID: cid,
			Label:    templated(ctx, h, c.Label),
			Style:    style,
			Disabled: c.Disabled,
			URL:      templated(ctx, h, c.URL),
		}
		// Discord rejects the whole message when a button carries both: a
		// link button is its URL, everything else is its custom_id.
		if style == discordgo.LinkButton {
			btn.CustomID = ""
		} else {
			btn.URL = ""
		}
		if c.Emoji != "" {
			btn.Emoji = componentEmoji(c.Emoji)
		}
		return btn
	case "select_string":
		opts := make([]discordgo.SelectMenuOption, 0, len(c.Options))
		for _, o := range c.Options {
			so := discordgo.SelectMenuOption{
				Label:       templated(ctx, h, o.Label),
				Value:       o.Value,
				Description: templated(ctx, h, o.Description),
				Default:     o.Default,
			}
			if o.Emoji != "" {
				so.Emoji = componentEmoji(o.Emoji)
			}
			opts = append(opts, so)
		}
		return discordgo.SelectMenu{
			CustomID:    cid,
			Placeholder: templated(ctx, h, c.Placeholder),
			Options:     opts,
			MinValues:   c.MinValues,
			MaxValues:   intOrZero(c.MaxValues),
			Disabled:    c.Disabled,
			MenuType:    discordgo.StringSelectMenu,
		}
	case "select_user":
		return discordgo.SelectMenu{CustomID: cid, MenuType: discordgo.UserSelectMenu, Placeholder: templated(ctx, h, c.Placeholder)}
	case "select_role":
		return discordgo.SelectMenu{CustomID: cid, MenuType: discordgo.RoleSelectMenu, Placeholder: templated(ctx, h, c.Placeholder)}
	case "select_channel":
		return discordgo.SelectMenu{CustomID: cid, MenuType: discordgo.ChannelSelectMenu, Placeholder: templated(ctx, h, c.Placeholder)}
	}
	return discordgo.Button{Label: c.Label, CustomID: cid, Style: discordgo.SecondaryButton}
}

func buttonStyle(s string) discordgo.ButtonStyle {
	switch strings.ToLower(s) {
	case "primary":
		return discordgo.PrimaryButton
	case "success":
		return discordgo.SuccessButton
	case "danger":
		return discordgo.DangerButton
	case "link":
		return discordgo.LinkButton
	}
	return discordgo.SecondaryButton
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func templated(ctx context.Context, h *Halt, src string) string {
	out, _ := cc.EvalTemplated(ctx, src, h.Scope)
	return out
}

func colorFromHex(hex string, fallback int) int {
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

func decodeSpec(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

func ptrString(s string) *string { return &s }

// componentEmoji turns the editor's emoji string into Discord's shape:
// unicode emoji pass through as Name; custom server emojis arrive as
// "name:id" (also tolerated: "a:name:id" for animated, or the full
// "<a:name:id>" paste) and split into Name + ID + Animated.
func componentEmoji(s string) *discordgo.ComponentEmoji {
	s = strings.Trim(strings.TrimSpace(s), "<>")
	parts := strings.Split(s, ":")
	last := parts[len(parts)-1]
	if len(parts) >= 2 && isSnowflake(last) {
		e := &discordgo.ComponentEmoji{Name: parts[len(parts)-2], ID: last}
		if len(parts) >= 3 && parts[0] == "a" {
			e.Animated = true
		}
		return e
	}
	return &discordgo.ComponentEmoji{Name: s}
}

// isSnowflake reports whether s looks like a Discord id — long enough that a
// short numeric select value ("vote:1") can't be mistaken for one.
func isSnowflake(s string) bool {
	if len(s) < 15 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
