package automations

import "github.com/dia-bot/dia/internal/event"

// Filter is one optional trigger filter a kind supports; the dashboard renders
// the matching control and the runtime applies it before a run starts.
type Filter string

const (
	FilterChannels   Filter = "channels"    // restrict to / exclude channels
	FilterRoles      Filter = "roles"       // actor must / must not hold a role
	FilterIgnoreBots Filter = "ignore_bots" // drop bot actors
	FilterKeywords   Filter = "keywords"    // message content match
	FilterEmojis     Filter = "emojis"      // reaction emoji allowlist
	FilterRole       Filter = "role"        // single watched role (role add/remove)
	FilterCooldown   Filter = "cooldown"    // per-scope rate limit
)

// TriggerKind is one entry in the trigger catalogue: a user-facing automation
// trigger mapped to the gateway event it derives from, plus the filters it
// supports and the `.Event.*` variables it exposes (mirrored on the dashboard).
type TriggerKind struct {
	Key         string     `json:"key"`
	Label       string     `json:"label"`
	Description string     `json:"description"`
	Event       event.Type `json:"event"`
	Category    string     `json:"category"`
	Filters     []Filter   `json:"filters"`
	// Actor names what `.User` refers to for this trigger (e.g. "the member who
	// joined", "the message author"), shown in the editor.
	Actor string `json:"actor"`
	// HasChannel reports whether `.Channel` is populated for this trigger.
	HasChannel bool `json:"has_channel"`
}

// Trigger categories for the dashboard grouping.
const (
	CatMembers    = "members"
	CatRoles      = "roles"
	CatMessages   = "messages"
	CatReactions  = "reactions"
	CatVoice      = "voice"
	CatModeration = "moderation"
	CatChannels   = "channels"
	CatTickets    = "tickets"
	CatGiveaways  = "giveaways"
	CatSocial     = "social"
)

// Triggers is the closed catalogue of automation triggers. Adding one here is
// all that's needed for the API/dashboard to offer it; the runtime maps Event →
// handler and builds the scope per Key.
var Triggers = []TriggerKind{
	// Members
	{Key: "member_join", Label: "Member joins", Description: "A member joins the server.", Event: event.TypeMemberAdd, Category: CatMembers, Actor: "the member who joined", Filters: []Filter{FilterIgnoreBots, FilterCooldown}},
	{Key: "member_leave", Label: "Member leaves", Description: "A member leaves, is kicked, or is banned.", Event: event.TypeMemberRemove, Category: CatMembers, Actor: "the member who left", Filters: []Filter{FilterCooldown}},
	{Key: "member_update", Label: "Member updated", Description: "A member's roles, nickname or boost status changes.", Event: event.TypeMemberUpdate, Category: CatMembers, Actor: "the updated member", Filters: []Filter{FilterCooldown}},
	{Key: "verification_passed", Label: "Member verified", Description: "A member passes verification (button or captcha).", Event: event.TypeVerificationPassed, Category: CatMembers, Actor: "the verified member", Filters: []Filter{FilterCooldown}},
	{Key: "verification_failed", Label: "Verification failed", Description: "A member fails the captcha, or is removed for not verifying in time.", Event: event.TypeVerificationFailed, Category: CatMembers, Actor: "the member who failed", Filters: []Filter{FilterCooldown}},
	{Key: "level_up", Label: "Member levels up", Description: "A member reaches a new level.", Event: event.TypeLevelUp, Category: CatMembers, Actor: "the member who leveled up", HasChannel: true, Filters: []Filter{FilterChannels, FilterCooldown}},

	// Roles (derived from member updates)
	{Key: "role_added", Label: "Role added", Description: "A specific role is granted to a member (use for boost detection: watch the Server Booster role).", Event: event.TypeMemberUpdate, Category: CatRoles, Actor: "the member who got the role", Filters: []Filter{FilterRole, FilterCooldown}},
	{Key: "role_removed", Label: "Role removed", Description: "A specific role is removed from a member.", Event: event.TypeMemberUpdate, Category: CatRoles, Actor: "the member who lost the role", Filters: []Filter{FilterRole, FilterCooldown}},
	{Key: "reaction_role_pick", Label: "Reaction role picked", Description: "A member picks roles from a reaction-role menu.", Event: event.TypeReactionRolePick, Category: CatRoles, Actor: "the member who picked", HasChannel: true, Filters: []Filter{FilterChannels, FilterCooldown}},

	// Messages
	{Key: "message_create", Label: "Message sent", Description: "A message is posted in the server.", Event: event.TypeMessageCreate, Category: CatMessages, Actor: "the message author", HasChannel: true, Filters: []Filter{FilterChannels, FilterRoles, FilterIgnoreBots, FilterKeywords, FilterCooldown}},
	{Key: "message_edit", Label: "Message edited", Description: "A message is edited.", Event: event.TypeMessageUpdate, Category: CatMessages, Actor: "the message author", HasChannel: true, Filters: []Filter{FilterChannels, FilterIgnoreBots, FilterKeywords}},
	{Key: "message_delete", Label: "Message deleted", Description: "A message is deleted.", Event: event.TypeMessageDelete, Category: CatMessages, Actor: "(no actor)", HasChannel: true, Filters: []Filter{FilterChannels}},

	// Reactions
	{Key: "reaction_add", Label: "Reaction added", Description: "Someone reacts to a message.", Event: event.TypeReactionAdd, Category: CatReactions, Actor: "the member who reacted", HasChannel: true, Filters: []Filter{FilterChannels, FilterEmojis, FilterIgnoreBots, FilterCooldown}},
	{Key: "reaction_remove", Label: "Reaction removed", Description: "Someone removes a reaction.", Event: event.TypeReactionRemove, Category: CatReactions, Actor: "the member who un-reacted", HasChannel: true, Filters: []Filter{FilterChannels, FilterEmojis}},

	// Voice
	{Key: "voice_join", Label: "Joins voice", Description: "A member joins a voice channel.", Event: event.TypeVoiceStateUpdate, Category: CatVoice, Actor: "the member", HasChannel: true, Filters: []Filter{FilterChannels, FilterIgnoreBots, FilterCooldown}},
	{Key: "voice_leave", Label: "Leaves voice", Description: "A member leaves a voice channel.", Event: event.TypeVoiceStateUpdate, Category: CatVoice, Actor: "the member", HasChannel: true, Filters: []Filter{FilterChannels, FilterCooldown}},
	{Key: "voice_move", Label: "Switches voice channel", Description: "A member moves between voice channels.", Event: event.TypeVoiceStateUpdate, Category: CatVoice, Actor: "the member", HasChannel: true, Filters: []Filter{FilterCooldown}},

	// Moderation
	{Key: "ban_add", Label: "Member banned", Description: "A user is banned from the server.", Event: event.TypeBanAdd, Category: CatModeration, Actor: "the banned user", Filters: []Filter{FilterCooldown}},
	{Key: "ban_remove", Label: "Member unbanned", Description: "A user is unbanned.", Event: event.TypeBanRemove, Category: CatModeration, Actor: "the unbanned user", Filters: []Filter{FilterCooldown}},
	{Key: "automod_action", Label: "Automod action taken", Description: "An automod rule fires on a member (keyword, spam, escalation, and more).", Event: event.TypeAutomodAction, Category: CatModeration, Actor: "the flagged member", HasChannel: true, Filters: []Filter{FilterIgnoreBots, FilterCooldown}},
	{Key: "moderation_action", Label: "Moderation action taken", Description: "A moderator runs /ban, /kick, /timeout, /warn or /note.", Event: event.TypeModerationAction, Category: CatModeration, Actor: "the actioned member", Filters: []Filter{FilterCooldown}},
	{Key: "raid_alert", Label: "Anti-raid mode changes", Description: "The server enters or leaves anti-raid mode (branch on .Event.active).", Event: event.TypeRaidAlert, Category: CatModeration, Actor: "(no actor)", Filters: []Filter{FilterCooldown}},

	// Tickets
	{Key: "ticket_opened", Label: "Ticket opened", Description: "A member opens a support ticket.", Event: event.TypeTicketOpened, Category: CatTickets, Actor: "the member who opened the ticket", HasChannel: true, Filters: []Filter{FilterCooldown}},
	{Key: "ticket_claimed", Label: "Ticket claimed", Description: "A staff member claims a ticket.", Event: event.TypeTicketClaimed, Category: CatTickets, Actor: "the ticket opener", HasChannel: true, Filters: []Filter{FilterCooldown}},
	{Key: "ticket_closed", Label: "Ticket closed", Description: "A ticket is closed (by staff, the opener, or auto-close).", Event: event.TypeTicketClosed, Category: CatTickets, Actor: "the ticket opener", HasChannel: true, Filters: []Filter{FilterCooldown}},
	{Key: "ticket_close_requested", Label: "Ticket close requested", Description: "Staff ask the opener to confirm closing a ticket (.Event.actor_id is the requester).", Event: event.TypeTicketCloseRequested, Category: CatTickets, Actor: "the ticket opener", HasChannel: true, Filters: []Filter{FilterCooldown}},
	{Key: "ticket_reopened", Label: "Ticket reopened", Description: "Staff reopen a closed ticket (.Event.actor_id is the reopener).", Event: event.TypeTicketReopened, Category: CatTickets, Actor: "the ticket opener", HasChannel: true, Filters: []Filter{FilterCooldown}},
	{Key: "ticket_rated", Label: "Ticket rated", Description: "A member rates their closed ticket (branch on .Event.rating).", Event: event.TypeTicketRated, Category: CatTickets, Actor: "the ticket opener", Filters: []Filter{FilterCooldown}},

	// Channels & threads
	{Key: "channel_create", Label: "Channel created", Description: "A channel is created.", Event: event.TypeChannelCreate, Category: CatChannels, Actor: "(no actor)", Filters: nil},
	{Key: "channel_delete", Label: "Channel deleted", Description: "A channel is deleted.", Event: event.TypeChannelDelete, Category: CatChannels, Actor: "(no actor)", Filters: nil},
	{Key: "thread_create", Label: "Thread created", Description: "A thread is created.", Event: event.TypeThreadCreate, Category: CatChannels, Actor: "(no actor)", HasChannel: true, Filters: nil},

	// Giveaways
	{Key: "giveaway_ended", Label: "Giveaway ends", Description: "A giveaway is drawn (natural end, manual end, or reroll). .User is the first winner; loop .Event.winner_ids for all winners.", Event: event.TypeGiveawayEnded, Category: CatGiveaways, Actor: "the first winner (if any)", HasChannel: true, Filters: []Filter{FilterChannels, FilterCooldown}},
	{Key: "giveaway_entry", Label: "Giveaway entered", Description: "A member clicks a giveaway's Enter button. Branch on .Event.outcome (entered, left, denied, blocked). .User is the member who clicked.", Event: event.TypeGiveawayEntered, Category: CatGiveaways, Actor: "the member who clicked Enter", HasChannel: true, Filters: []Filter{FilterChannels, FilterIgnoreBots, FilterCooldown}},

	// Social
	{Key: "social_update", Label: "Social account update", Description: "A followed social account goes live or posts (branch on .Event.kind: live_start, live_end, new_video, new_post — and .Event.provider).", Event: event.TypeSocialUpdate, Category: CatSocial, Actor: "(no actor)", Filters: []Filter{FilterCooldown}},
}

// triggerByKey indexes the catalogue.
var triggerByKey = func() map[string]TriggerKind {
	m := make(map[string]TriggerKind, len(Triggers))
	for _, t := range Triggers {
		m[t.Key] = t
	}
	return m
}()

// TriggerByKey returns the catalogue entry for a trigger key (ok=false if unknown).
func TriggerByKey(key string) (TriggerKind, bool) {
	t, ok := triggerByKey[key]
	return t, ok
}

// ValidTriggerKey reports whether key names a known trigger.
func ValidTriggerKey(key string) bool {
	_, ok := triggerByKey[key]
	return ok
}

// EventForTrigger returns the gateway event a trigger key derives from.
func EventForTrigger(key string) (event.Type, bool) {
	t, ok := triggerByKey[key]
	if !ok {
		return "", false
	}
	return t.Event, true
}

// SubscribedEvents returns the distinct set of gateway events the catalogue
// needs — the runtime subscribes to exactly these.
func SubscribedEvents() []event.Type {
	seen := map[event.Type]bool{}
	var out []event.Type
	for _, t := range Triggers {
		if !seen[t.Event] {
			seen[t.Event] = true
			out = append(out, t.Event)
		}
	}
	return out
}

// TriggerKeysForEvent returns every trigger key that derives from a gateway
// event (e.g. VOICE_STATE_UPDATE → voice_join, voice_leave, voice_move).
func TriggerKeysForEvent(e event.Type) []string {
	var out []string
	for _, t := range Triggers {
		if t.Event == e {
			out = append(out, t.Key)
		}
	}
	return out
}
