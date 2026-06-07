// Package tmpllookup adapts the Redis-backed guild state into a
// templating.Lookup, so message templates can resolve roles/channels by name or
// id (read-only). It lives in its own package to keep internal/templating free
// of any guild-state dependency.
package tmpllookup

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/dia-bot/dia/internal/guildstate"
	"github.com/dia-bot/dia/internal/templating"
)

type lookup struct {
	ctx     context.Context
	gs      *guildstate.Store
	guildID string

	// The snapshot is fetched at most once per render (a single getRole/getChannel
	// or a thousand share one fetch), then scanned in memory.
	once  sync.Once
	roles []templating.RoleInfo
	chans []templating.ChannelInfo
}

// New returns a read-only templating.Lookup for one guild, or nil when no state
// store / guild id is available (templates then see lookups as disabled).
func New(ctx context.Context, gs *guildstate.Store, guildID string) templating.Lookup {
	if gs == nil || guildID == "" {
		return nil
	}
	return &lookup{ctx: ctx, gs: gs, guildID: guildID}
}

// load fetches the guild snapshot exactly once and sorts by id, so that when a
// guild has several roles/channels with the same name the lowest id (the oldest)
// wins deterministically across renders rather than an arbitrary map order.
func (l *lookup) load() {
	l.once.Do(func() {
		snap, err := l.gs.Snapshot(l.ctx, l.guildID)
		if err != nil {
			return
		}
		for _, r := range snap.Roles {
			l.roles = append(l.roles, templating.RoleInfo{ID: r.ID, Name: r.Name, Color: r.Color})
		}
		for _, c := range snap.Channels {
			l.chans = append(l.chans, templating.ChannelInfo{ID: c.ID, Name: c.Name, Type: c.Type})
		}
		sort.Slice(l.roles, func(i, j int) bool { return l.roles[i].ID < l.roles[j].ID })
		sort.Slice(l.chans, func(i, j int) bool { return l.chans[i].ID < l.chans[j].ID })
	})
}

func (l *lookup) Role(nameOrID string) (*templating.RoleInfo, bool) {
	l.load()
	for i := range l.roles { // exact id first — unambiguous
		if l.roles[i].ID == nameOrID {
			return &l.roles[i], true
		}
	}
	for i := range l.roles { // then case-insensitive name (lowest id wins)
		if strings.EqualFold(l.roles[i].Name, nameOrID) {
			return &l.roles[i], true
		}
	}
	return nil, false
}

func (l *lookup) Channel(nameOrID string) (*templating.ChannelInfo, bool) {
	l.load()
	want := strings.TrimPrefix(nameOrID, "#")
	for i := range l.chans {
		if l.chans[i].ID == nameOrID {
			return &l.chans[i], true
		}
	}
	for i := range l.chans {
		if strings.EqualFold(l.chans[i].Name, want) {
			return &l.chans[i], true
		}
	}
	return nil, false
}
