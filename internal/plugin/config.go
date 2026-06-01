package plugin

import (
	"context"
	"encoding/json"

	"github.com/dia-bot/dia/internal/event"
)

// LoadConfig loads a feature's typed config for a guild. The returned config is
// the zero value (or whatever defaults the caller pre-populated) when no row
// exists; enabled reports the feature toggle.
func LoadConfig[T any](ctx context.Context, d Deps, guildID int64, feature string) (cfg T, enabled bool, err error) {
	fc, err := d.Store.Features.Get(ctx, guildID, feature)
	if err != nil {
		return cfg, false, err
	}
	if len(fc.Config) > 0 {
		if uerr := json.Unmarshal(fc.Config, &cfg); uerr != nil {
			return cfg, fc.Enabled, uerr
		}
	}
	return cfg, fc.Enabled, nil
}

// SaveConfig persists a feature's typed config + enabled flag for a guild.
func SaveConfig[T any](ctx context.Context, d Deps, guildID int64, feature string, enabled bool, cfg T) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return d.Store.Features.Upsert(ctx, guildID, feature, enabled, raw)
}

// DecodeData unmarshals an event envelope's payload into T.
func DecodeData[T any](env *event.Envelope) (T, error) {
	var v T
	err := json.Unmarshal(env.Data, &v)
	return v, err
}
