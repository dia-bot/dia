package bot

import (
	"context"
	"encoding/json"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
)

// handleCore keeps Postgres (guilds) and the Redis guild snapshot in sync with
// gateway state. This is what makes the dashboard's channel/role dropdowns live.
func (b *Bot) handleCore(ctx context.Context, env *event.Envelope) {
	switch env.Type {
	case event.TypeGuildCreate, event.TypeGuildUpdate:
		var g event.Guild
		if json.Unmarshal(env.Data, &g) != nil || g.Unavailable {
			return
		}
		if err := b.deps.Store.Guilds.Upsert(ctx, toStoreGuild(g)); err != nil {
			b.log.Warn("guild upsert", "guild", g.ID, "err", err)
		}
		if err := b.gstate.PutGuild(ctx, g); err != nil {
			b.log.Warn("guild snapshot", "guild", g.ID, "err", err)
		}

	case event.TypeGuildDelete:
		var gd event.GuildDelete
		if json.Unmarshal(env.Data, &gd) != nil || gd.Unavailable {
			return // outage, not a real removal
		}
		if id, ok := event.ParseID(gd.ID); ok {
			_ = b.deps.Store.Guilds.MarkLeft(ctx, id)
		}
		_ = b.gstate.RemoveGuild(ctx, gd.ID)

	case event.TypeChannelCreate, event.TypeChannelUpdate:
		var ce event.ChannelEvent
		if json.Unmarshal(env.Data, &ce) == nil && ce.ID != "" {
			_ = b.gstate.UpsertChannel(ctx, env.GuildID, ce.Channel)
		}

	case event.TypeChannelDelete:
		var ce event.ChannelEvent
		if json.Unmarshal(env.Data, &ce) == nil && ce.ID != "" {
			_ = b.gstate.DeleteChannel(ctx, env.GuildID, ce.ID)
		}

	case event.TypeRoleCreate, event.TypeRoleUpdate:
		var re event.RoleEvent
		if json.Unmarshal(env.Data, &re) == nil && re.Role.ID != "" {
			_ = b.gstate.UpsertRole(ctx, re.GuildID, re.Role)
		}

	case event.TypeRoleDelete:
		var re event.RoleEvent
		if json.Unmarshal(env.Data, &re) == nil && re.RoleID != "" {
			_ = b.gstate.DeleteRole(ctx, re.GuildID, re.RoleID)
		}

	case event.TypeMemberAdd:
		var ma event.MemberAdd
		if json.Unmarshal(env.Data, &ma) == nil && ma.MemberCount > 0 {
			b.updateMemberCount(ctx, ma.GuildID, ma.MemberCount)
		}

	case event.TypeMemberRemove:
		var mr event.MemberRemove
		if json.Unmarshal(env.Data, &mr) == nil && mr.MemberCount > 0 {
			b.updateMemberCount(ctx, mr.GuildID, mr.MemberCount)
		}
	}
}

func (b *Bot) updateMemberCount(ctx context.Context, guildID string, count int) {
	_ = b.gstate.SetMemberCount(ctx, guildID, count)
	if id, ok := event.ParseID(guildID); ok {
		_ = b.deps.Store.Guilds.UpdateMemberCount(ctx, id, count)
	}
}

func toStoreGuild(g event.Guild) store.Guild {
	id, _ := event.ParseID(g.ID)
	owner, _ := event.ParseID(g.OwnerID)
	return store.Guild{
		ID:          id,
		Name:        g.Name,
		Icon:        g.Icon,
		OwnerID:     owner,
		MemberCount: g.MemberCount,
	}
}
