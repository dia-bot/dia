package event

// Control plane: the reverse channel from the Go services to the Elixir
// gateway, used to run customers' custom bots. Unlike the JetStream event
// stream (gateway → Go, durable), the control plane is core NATS (latest-wins,
// reconciled on an interval and on gateway hello), so a missed message
// self-heals.
//
// This file is the single source of truth for the control contract; the Elixir
// side (gateway/lib/dia_gateway/control.ex) mirrors these subjects and shapes.

const (
	// SubjectBotCommand carries BotCommand messages (Go → gateway): start,
	// stop or restyle a custom bot's gateway connection.
	SubjectBotCommand = "dia.control.bots"
	// SubjectGatewayStatus carries GatewayStatus messages (gateway → Go):
	// gateway hello and per-bot connection state.
	SubjectGatewayStatus = "dia.control.gateway"
)

// Bot command actions.
const (
	BotActionEnsure   = "ensure"   // ensure a connection for app_id is running with the given token/intents/presence
	BotActionRemove   = "remove"   // tear down the connection for app_id
	BotActionPresence = "presence" // update only the presence of a running bot
)

// Presence status values (Discord gateway status strings).
const (
	StatusOnline    = "online"
	StatusIdle      = "idle"
	StatusDND       = "dnd"
	StatusInvisible = "invisible"
)

// Activity types (Discord activity type ints); ActivityNone means "no activity".
const (
	ActivityNone      = -1
	ActivityPlaying   = 0
	ActivityStreaming = 1
	ActivityListening = 2
	ActivityWatching  = 3
	ActivityCompeting = 5
)

// Presence is the status + activity a custom bot broadcasts on its own gateway
// connection (which the shared bot cannot do per guild).
type Presence struct {
	Status       string `json:"status"`        // online|idle|dnd|invisible
	ActivityType int    `json:"activity_type"` // -1 none, else a Discord activity type
	ActivityText string `json:"activity_text"`
	ActivityURL  string `json:"activity_url"` // streaming url (activity_type = 1)
}

// BotCommand is a Go → gateway control message.
type BotCommand struct {
	Action   string    `json:"action"`             // ensure|remove|presence
	AppID    string    `json:"app_id"`             // the custom application id (connection key)
	Token    string    `json:"token,omitempty"`    // decrypted bot token (ensure only; internal network)
	Intents  int       `json:"intents,omitempty"`  // gateway intents (ensure only)
	Presence *Presence `json:"presence,omitempty"` // ensure + presence
}

// Gateway status events.
const (
	GatewayEventReady    = "ready"     // gateway (re)started; Go should replay all ensures
	GatewayEventBotState = "bot_state" // a custom bot's connection state changed
)

// Custom-bot connection states reported by the gateway.
const (
	BotStateConnecting   = "connecting"
	BotStateReady        = "ready"
	BotStateError        = "error"
	BotStateDisconnected = "disconnected"
)

// GatewayStatus is a gateway → Go control message.
type GatewayStatus struct {
	Event string `json:"event"` // ready|bot_state
	AppID string `json:"app_id,omitempty"`
	State string `json:"state,omitempty"` // bot_state: connecting|ready|error|disconnected
	Error string `json:"error,omitempty"`
}
