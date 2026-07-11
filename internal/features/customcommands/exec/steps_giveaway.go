package exec

import (
	"context"
	"errors"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// hGiveawayStart starts a giveaway from a saved preset. The Giveaways feature
// owns the presets, the composed message and the draw; the engine only resolves
// the templated inputs and hands them to the injected GiveawayStarter. The new
// giveaway id is written to Into (and logged as the step output).
func hGiveawayStart(ctx context.Context, h *Halt) error {
	if h.Deps.Giveaways == nil {
		return errors.New("giveaway_start: giveaways are unavailable")
	}
	var spec cc.SpecGiveawayStart
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	prize, err := cc.EvalString(ctx, spec.Prize, h.Scope)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prize) == "" {
		return errors.New("giveaway_start: prize required")
	}
	preset, err := cc.EvalTemplated(ctx, spec.Preset, h.Scope)
	if err != nil {
		return err
	}
	channel, err := cc.EvalSnowflake(ctx, spec.Channel, h.Scope)
	if err != nil {
		return err
	}
	duration, err := cc.EvalString(ctx, spec.Duration, h.Scope)
	if err != nil {
		return err
	}
	winners, err := cc.EvalInt(ctx, spec.Winners, h.Scope)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(h.Run.GuildID)
	hostID, _ := event.ParseID(h.Run.InvokerID)
	id, err := h.Deps.Giveaways.StartGiveaway(ctx, gid, preset, prize, channel, duration, int(winners), hostID)
	if err != nil {
		return err
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, id)
	}
	h.SetOutput(map[string]any{"giveaway_id": id})
	return nil
}
