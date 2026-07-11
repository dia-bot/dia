package giveaway

import (
	"context"

	"github.com/dia-bot/dia/internal/event"
)

// AutomationRunner runs a saved automation on demand. It's the cycle-safe bridge
// the giveaway feature uses to fire a user's automation when an action button is
// clicked, WITHOUT importing the automations runner (which already imports this
// package). The automations runtime plugin satisfies it structurally; the worker
// injects the concrete implementation via SetAutomationRunner once both plugins
// have initialised. A nil runner just makes action buttons report "not set up".
type AutomationRunner interface {
	RunAutomation(ctx context.Context, guildID, automationID string, user event.User, member *event.Member, channelID string, eventMap map[string]any) error
}

// SetAutomationRunner injects the automations bridge used by composed action
// buttons. Called by the worker after plugin registration.
func (p *Plugin) SetAutomationRunner(r AutomationRunner) { p.autoRunner = r }
