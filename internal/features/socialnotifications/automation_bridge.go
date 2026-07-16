package socialnotifications

import (
	"context"

	"github.com/dia-bot/dia/internal/event"
)

// AutomationRunner runs a saved automation on demand. It's the cycle-safe
// bridge this feature uses to fire a user's automation when a subscription's
// per-kind automation or a composed action button fires, WITHOUT importing the
// automations runtime (which imports this package for the built-in flow). The
// automations runtime plugin satisfies it structurally; the worker injects the
// concrete implementation via SetAutomationRunner once both plugins have
// initialised. A nil runner makes action buttons report "not set up" and
// per-kind automations no-op.
type AutomationRunner interface {
	RunAutomation(ctx context.Context, guildID, automationID string, user event.User, member *event.Member, channelID string, eventMap map[string]any) error
}

// SetAutomationRunner injects the automations bridge. Called by the worker
// after plugin registration.
func (p *Plugin) SetAutomationRunner(r AutomationRunner) { p.autoRunner = r }
