package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/gin-gonic/gin"
)

// handleRealtime authenticates off the session cookie, authorizes the guild,
// then upgrades to a WebSocket that streams that guild's state changes.
func (s *Server) handleRealtime(c *gin.Context) {
	gid := c.Param("id")
	sess, _, ok := s.sessionFromCookie(c)
	if !ok {
		fail(c, http.StatusUnauthorized, "not authenticated")
		return
	}
	if !canManage(sess, gid) {
		fail(c, http.StatusForbidden, "forbidden")
		return
	}
	if _, ok := event.ParseID(gid); !ok {
		fail(c, http.StatusBadRequest, "invalid guild id")
		return
	}
	if !s.botInGuild(c.Request.Context(), gid) {
		fail(c, http.StatusNotFound, "Dia is not in this server")
		return
	}
	conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return // Upgrade already wrote the error response
	}
	s.hub.Serve(conn, gid) // blocks until the socket closes
}

// realtimeSubjects are the gateway event types streamed to dashboards.
var realtimeTypes = []event.Type{
	event.TypeChannelCreate, event.TypeChannelUpdate, event.TypeChannelDelete,
	event.TypeRoleCreate, event.TypeRoleUpdate, event.TypeRoleDelete,
	event.TypeGuildUpdate, event.TypeMemberAdd, event.TypeMemberRemove,
}

// StartRealtime subscribes to guild-state events and fans them out to the hub.
func (s *Server) StartRealtime(ctx context.Context) error {
	subjects := make([]string, 0, len(realtimeTypes))
	for _, t := range realtimeTypes {
		subjects = append(subjects, event.SubjectPrefix+"."+string(t)+".>")
	}
	_, err := s.bus.Consume(ctx, eventbus.ConsumerSpec{
		Durable:        "dia-api-realtime",
		FilterSubjects: subjects,
		MaxAckPending:  512,
	}, func(ctx context.Context, msg eventbus.Msg) error {
		var env event.Envelope
		if err := json.Unmarshal(msg.Data(), &env); err != nil {
			return nil
		}
		if out := s.toBrowserMessage(&env); out != nil {
			s.hub.Broadcast(env.GuildID, out)
		}
		return nil
	})
	return err
}

// toBrowserMessage maps a gateway envelope to a compact dashboard message.
func (s *Server) toBrowserMessage(env *event.Envelope) []byte {
	type wire struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	emit := func(t string, v any) []byte {
		raw, _ := json.Marshal(v)
		b, _ := json.Marshal(wire{Type: t, Data: raw})
		return b
	}

	switch env.Type {
	case event.TypeChannelCreate, event.TypeChannelUpdate:
		var ce event.ChannelEvent
		if json.Unmarshal(env.Data, &ce) == nil {
			return emit("channel.upsert", ce.Channel)
		}
	case event.TypeChannelDelete:
		var ce event.ChannelEvent
		if json.Unmarshal(env.Data, &ce) == nil {
			return emit("channel.delete", gin.H{"id": ce.ID})
		}
	case event.TypeRoleCreate, event.TypeRoleUpdate:
		var re event.RoleEvent
		if json.Unmarshal(env.Data, &re) == nil {
			return emit("role.upsert", re.Role)
		}
	case event.TypeRoleDelete:
		var re event.RoleEvent
		if json.Unmarshal(env.Data, &re) == nil {
			return emit("role.delete", gin.H{"id": re.RoleID})
		}
	case event.TypeGuildUpdate:
		var g event.Guild
		if json.Unmarshal(env.Data, &g) == nil {
			return emit("guild.update", gin.H{"name": g.Name, "icon": g.Icon, "member_count": g.MemberCount})
		}
	case event.TypeMemberAdd:
		var m event.MemberAdd
		if json.Unmarshal(env.Data, &m) == nil && m.MemberCount > 0 {
			return emit("member.count", gin.H{"count": m.MemberCount})
		}
	case event.TypeMemberRemove:
		var m event.MemberRemove
		if json.Unmarshal(env.Data, &m) == nil && m.MemberCount > 0 {
			return emit("member.count", gin.H{"count": m.MemberCount})
		}
	}
	return nil
}
