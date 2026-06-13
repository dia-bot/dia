package exec

import (
	"context"
	"errors"
	"strings"
	"time"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── Roles / members ──────────────────────────────────────────────────────────

func hRoleAdd(ctx context.Context, h *Halt) error    { return roleOp(ctx, h, true) }
func hRoleRemove(ctx context.Context, h *Halt) error { return roleOp(ctx, h, false) }

func roleOp(ctx context.Context, h *Halt, add bool) error {
	var spec cc.SpecRole
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, err := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if err != nil || userID == "" {
		return errors.New("role: invalid user")
	}
	roleID, err := cc.EvalSnowflake(ctx, spec.Role, h.Scope)
	if err != nil || roleID == "" {
		return errors.New("role: invalid role")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	if add {
		return h.Deps.Discord.AddRole(h.Run.GuildID, userID, roleID, reason)
	}
	return h.Deps.Discord.RemoveRole(h.Run.GuildID, userID, roleID, reason)
}

func hMemberNickname(ctx context.Context, h *Halt) error {
	var spec cc.SpecMember
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, _ := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if userID == "" {
		return errors.New("member_nickname: user required")
	}
	nick, _ := cc.EvalTemplated(ctx, spec.Nickname, h.Scope)
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	return h.Deps.Discord.SetNickname(h.Run.GuildID, userID, nick, reason)
}

func hMemberKick(ctx context.Context, h *Halt) error {
	var spec cc.SpecMember
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, _ := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if userID == "" {
		return errors.New("member_kick: user required")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	return h.Deps.Discord.Kick(h.Run.GuildID, userID, reason)
}

func hMemberBan(ctx context.Context, h *Halt) error {
	var spec cc.SpecMember
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, _ := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if userID == "" {
		return errors.New("member_ban: user required")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	days := spec.DeleteMessageDays
	if days < 0 {
		days = 0
	}
	if days > 7 {
		days = 7
	}
	return h.Deps.Discord.Ban(h.Run.GuildID, userID, reason, days)
}

func hMemberUnban(ctx context.Context, h *Halt) error {
	var spec cc.SpecMember
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, _ := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if userID == "" {
		return errors.New("member_unban: user required")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	return h.Deps.Discord.Unban(h.Run.GuildID, userID, reason)
}

func hMemberTimeout(ctx context.Context, h *Halt) error {
	var spec cc.SpecMember
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, _ := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if userID == "" {
		return errors.New("member_timeout: user required")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	var until *time.Time
	if spec.Duration != "" {
		d, err := time.ParseDuration(spec.Duration)
		if err != nil {
			return err
		}
		if d > 0 {
			t := time.Now().Add(d)
			until = &t
		}
	}
	return h.Deps.Discord.Timeout(h.Run.GuildID, userID, until, reason)
}

// ── Channels / threads / voice ───────────────────────────────────────────────

func hChannelCreate(ctx context.Context, h *Halt) error {
	var spec cc.SpecChannelCreate
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	name, _ := cc.EvalTemplated(ctx, spec.Name, h.Scope)
	parent, _ := cc.EvalSnowflake(ctx, spec.Parent, h.Scope)
	data := &discordgo.GuildChannelCreateData{
		Name:             name,
		Type:             channelTypeFromString(spec.Type),
		Topic:            templated(ctx, h, spec.Topic),
		NSFW:             spec.NSFW,
		RateLimitPerUser: spec.RateLimitPerUser,
		ParentID:         parent,
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	ch, err := h.Deps.Discord.CreateChannel(h.Run.GuildID, data, reason)
	if err != nil {
		return err
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, map[string]any{"id": ch.ID, "name": ch.Name, "type": int(ch.Type)})
	}
	return nil
}

func hChannelEdit(ctx context.Context, h *Halt) error {
	var spec cc.SpecChannelEdit
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, _ := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if channelID == "" {
		return errors.New("channel_edit: channel required")
	}
	edit := &discordgo.ChannelEdit{
		Name:             templated(ctx, h, spec.Name),
		Topic:            templated(ctx, h, spec.Topic),
		RateLimitPerUser: spec.RateLimitPerUser,
		NSFW:             spec.NSFW,
	}
	if parent, _ := cc.EvalSnowflake(ctx, spec.Parent, h.Scope); parent != "" {
		edit.ParentID = parent
	}
	if spec.Locked != nil {
		edit.Locked = spec.Locked
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	_, err := h.Deps.Discord.EditChannel(channelID, edit, reason)
	return err
}

func hChannelDelete(ctx context.Context, h *Halt) error {
	var spec cc.SpecChannelDelete
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, _ := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if channelID == "" {
		return errors.New("channel_delete: channel required")
	}
	reason, _ := cc.EvalTemplated(ctx, spec.Reason, h.Scope)
	return h.Deps.Discord.DeleteChannel(channelID, reason)
}

func hThreadCreate(ctx context.Context, h *Halt) error {
	var spec cc.SpecThreadCreate
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	channelID, _ := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if channelID == "" {
		return errors.New("thread_create: channel required")
	}
	messageID, _ := cc.EvalSnowflake(ctx, spec.Message, h.Scope)
	name, _ := cc.EvalTemplated(ctx, spec.Name, h.Scope)
	archive := spec.AutoArchiveMin
	if archive <= 0 {
		archive = 60
	}
	ch, err := h.Deps.Discord.StartThread(channelID, messageID, name, archive, spec.Private, spec.Invitable)
	if err != nil {
		return err
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, map[string]any{"id": ch.ID, "name": ch.Name})
	}
	return nil
}

func hThreadArchive(ctx context.Context, h *Halt) error {
	var spec cc.SpecThreadArchive
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	threadID, _ := cc.EvalSnowflake(ctx, spec.Thread, h.Scope)
	if threadID == "" {
		return errors.New("thread_archive: thread required")
	}
	return h.Deps.Discord.ArchiveThread(threadID, spec.Locked)
}

func hVoiceMove(ctx context.Context, h *Halt) error {
	var spec cc.SpecVoiceMove
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	userID, _ := cc.EvalSnowflake(ctx, spec.User, h.Scope)
	if userID == "" {
		return errors.New("voice_move: user required")
	}
	channelID, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil {
		// An eval error must NOT degrade to "disconnect them".
		return errors.New("voice_move: invalid channel expression")
	}
	return h.Deps.Discord.MoveVoice(h.Run.GuildID, userID, channelID)
}

func channelTypeFromString(s string) discordgo.ChannelType {
	switch strings.ToLower(s) {
	case "text", "":
		return discordgo.ChannelTypeGuildText
	case "voice":
		return discordgo.ChannelTypeGuildVoice
	case "category":
		return discordgo.ChannelTypeGuildCategory
	case "announcement", "news":
		return discordgo.ChannelTypeGuildNews
	case "forum":
		return discordgo.ChannelTypeGuildForum
	case "stage":
		return discordgo.ChannelTypeGuildStageVoice
	}
	return discordgo.ChannelTypeGuildText
}
