package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/gin-gonic/gin"
)

// featureManagerRoles is the reusable seam for per-feature dashboard delegation:
// it maps a feature key to a function that extracts that feature's configured
// manager-role IDs from its stored config JSON. A feature opts into role-based
// access simply by registering here; unlisted features stay admin-only. To
// delegate another feature later, add its key + a config extractor.
var featureManagerRoles = map[string]func(json.RawMessage) []string{
	giveaway.FeatureKey: func(raw json.RawMessage) []string {
		var c giveaway.Config
		_ = json.Unmarshal(raw, &c)
		return c.ManagerRoles
	},
}

// guildAccess is a user's resolved access to one guild's dashboard. Admins get
// everything; a non-admin gets the set of delegated feature keys their roles
// grant. It is computed once per request in the guild middleware and stashed in
// the gin context (ctxAccess) for handlers to read.
type guildAccess struct {
	Admin    bool            `json:"admin"`
	Features map[string]bool `json:"features"`
}

// can reports whether the user may manage a specific feature.
func (a guildAccess) can(feature string) bool { return a.Admin || a.Features[feature] }

// any reports whether the user may reach the guild dashboard at all.
func (a guildAccess) any() bool { return a.Admin || len(a.Features) > 0 }

const ctxAccess = "dia_guild_access"

func accessFromCtx(c *gin.Context) guildAccess {
	if v, ok := c.Get(ctxAccess); ok {
		if a, ok := v.(guildAccess); ok {
			return a
		}
	}
	return guildAccess{}
}

// memberRoleIDs returns the user's role IDs in a guild, fetched from Discord and
// cached briefly (the dashboard consults it on many requests). A short TTL keeps
// role/permission revocations taking effect quickly. Empty on any lookup miss.
func (s *Server) memberRoleIDs(ctx context.Context, gid, userID string) []string {
	key := "dash:member_roles:" + gid + ":" + userID
	var roles []string
	if err := s.cache.GetJSON(ctx, key, &roles); err == nil {
		return roles
	}
	m, err := s.discord.GuildMember(gid, userID)
	if err != nil || m == nil {
		return nil
	}
	_ = s.cache.SetJSON(ctx, key, m.Roles, 60*time.Second)
	return m.Roles
}

// accessFor resolves a user's access to a guild. Server admins/owners manage
// everything (no member lookup). Otherwise, the user's role IDs are intersected
// with each ENABLED delegatable feature's configured manager roles; a member
// lookup is only paid when some delegatable feature actually configures manager
// roles.
func (s *Server) accessFor(ctx context.Context, sess *Session, gid string) guildAccess {
	if canManage(sess, gid) {
		return guildAccess{Admin: true}
	}
	acc := guildAccess{Features: map[string]bool{}}
	if sess == nil {
		return acc
	}
	gidInt, ok := event.ParseID(gid)
	if !ok {
		return acc
	}
	configs, err := s.store.Features.GetAll(ctx, gidInt)
	if err != nil {
		return acc
	}
	// Only the delegatable features that both are enabled and configure manager
	// roles are in play; collect them before paying for a member lookup.
	want := map[string][]string{}
	for key, extract := range featureManagerRoles {
		fc, ok := configs[key]
		if !ok || !fc.Enabled {
			continue
		}
		if roles := extract(json.RawMessage(fc.Config)); len(roles) > 0 {
			want[key] = roles
		}
	}
	if len(want) == 0 {
		return acc
	}
	held := map[string]bool{}
	for _, r := range s.memberRoleIDs(ctx, gid, sess.UserID) {
		held[r] = true
	}
	for key, roles := range want {
		for _, r := range roles {
			if held[r] {
				acc.Features[key] = true
				break
			}
		}
	}
	return acc
}
