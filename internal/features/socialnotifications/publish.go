package socialnotifications

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/eventbus"
)

// Publish emits one SocialUpdate envelope on the event bus. Both ingestion
// paths use it — the API's webhook handlers (Twitch/Kick/YouTube pushes) and
// the worker's pollers (RSS/Bluesky) — so every update reaches the announce
// handler and the automations "social_update" trigger the same way. The dedup
// id makes a redelivered webhook (or a poll racing a push) publish once.
func Publish(ctx context.Context, bus eventbus.Bus, log *slog.Logger, upd event.SocialUpdate) {
	if bus == nil {
		return
	}
	data, err := json.Marshal(upd)
	if err != nil {
		return
	}
	envBytes, err := json.Marshal(event.Envelope{
		Type:    event.TypeSocialUpdate,
		GuildID: upd.GuildID,
		TS:      time.Now().UnixMilli(),
		Data:    data,
	})
	if err != nil {
		return
	}
	subject := event.Subject(event.TypeSocialUpdate, upd.GuildID)
	dedup := fmt.Sprintf("social:%d:%s:%s", upd.SubscriptionID, upd.Kind, upd.ItemID)
	if err := bus.Publish(ctx, subject, envBytes, dedup); err != nil {
		log.Warn("social: publish update failed", "subject", subject, "err", err)
	}
}
