package event

import "encoding/json"

// InteractionType mirrors Discord's interaction type enum.
type InteractionType int

const (
	InteractionPing               InteractionType = 1
	InteractionApplicationCommand InteractionType = 2
	InteractionMessageComponent   InteractionType = 3
	InteractionAutocomplete       InteractionType = 4
	InteractionModalSubmit        InteractionType = 5
)

// Application command option types (Discord).
const (
	OptSubCommand      = 1
	OptSubCommandGroup = 2
	OptString          = 3
	OptInteger         = 4
	OptBoolean         = 5
	OptUser            = 6
	OptChannel         = 7
	OptRole            = 8
	OptMentionable     = 9
	OptNumber          = 10
	OptAttachment      = 11
)

// Component types (Discord) relevant to interaction routing.
const (
	ComponentActionRow         = 1
	ComponentButton            = 2
	ComponentStringSelect      = 3
	ComponentTextInput         = 4
	ComponentUserSelect        = 5
	ComponentRoleSelect        = 6
	ComponentMentionableSelect = 7
	ComponentChannelSelect     = 8
)

// Interaction is the normalized INTERACTION_CREATE payload. It carries
// everything the worker needs to route the interaction and to respond to it via
// the Discord REST API (ID + Token + ApplicationID).
type Interaction struct {
	ID             string          `json:"id"`
	ApplicationID  string          `json:"application_id"`
	Type           InteractionType `json:"type"`
	Token          string          `json:"token"`
	Version        int             `json:"version,omitempty"`
	GuildID        string          `json:"guild_id,omitempty"`
	ChannelID      string          `json:"channel_id,omitempty"`
	Member         *Member         `json:"member,omitempty"`
	User           *User           `json:"user,omitempty"`
	Locale         string          `json:"locale,omitempty"`
	GuildLocale    string          `json:"guild_locale,omitempty"`
	AppPermissions string          `json:"app_permissions,omitempty"`
	Data           InteractionData `json:"data"`
	Message        *MessageRef     `json:"message,omitempty"`
}

// InteractionData is the polymorphic data block; which fields are populated
// depends on Interaction.Type.
type InteractionData struct {
	// Application command (Type 2) / autocomplete (Type 4)
	ID       string              `json:"id,omitempty"`
	Name     string              `json:"name,omitempty"`
	Type     int                 `json:"type,omitempty"`
	Options  []InteractionOption `json:"options,omitempty"`
	Resolved *Resolved           `json:"resolved,omitempty"`
	TargetID string              `json:"target_id,omitempty"`

	// Message component (Type 3)
	CustomID      string   `json:"custom_id,omitempty"`
	ComponentType int      `json:"component_type,omitempty"`
	Values        []string `json:"values,omitempty"`

	// Modal submit (Type 5) — rows of submitted inputs
	Components []ModalRow `json:"components,omitempty"`
}

// InteractionOption is a command option value (recursive for sub-commands).
type InteractionOption struct {
	Name    string              `json:"name"`
	Type    int                 `json:"type"`
	Value   json.RawMessage     `json:"value,omitempty"`
	Options []InteractionOption `json:"options,omitempty"`
	Focused bool                `json:"focused,omitempty"`
}

// Resolved maps snowflake IDs referenced by command options to their objects.
type Resolved struct {
	Users    map[string]User    `json:"users,omitempty"`
	Members  map[string]Member  `json:"members,omitempty"`
	Roles    map[string]Role    `json:"roles,omitempty"`
	Channels map[string]Channel `json:"channels,omitempty"`
}

// ModalRow is an action row of modal inputs.
type ModalRow struct {
	Type       int              `json:"type"`
	Components []ModalComponent `json:"components"`
}

// ModalComponent is a single submitted modal input.
type ModalComponent struct {
	Type     int    `json:"type"`
	CustomID string `json:"custom_id"`
	Value    string `json:"value"`
}

// MessageRef is the message a component interaction originated from.
type MessageRef struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id,omitempty"`
}

// Actor returns the invoking user for either guild (Member.User) or DM (User)
// interactions, and reports whether one was found.
func (i *Interaction) Actor() (User, bool) {
	if i.Member != nil && i.Member.User.ID != "" {
		return i.Member.User, true
	}
	if i.User != nil && i.User.ID != "" {
		return *i.User, true
	}
	return User{}, false
}
