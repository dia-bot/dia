// Package realtime is a per-guild WebSocket fan-out hub. The API feeds it guild
// state changes (channels/roles/member counts) sourced from the gateway, and it
// pushes them to dashboard clients viewing that guild — so a channel created in
// Discord shows up in the dashboard's dropdowns instantly.
package realtime

import (
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	sendBuffer = 32
)

// Hub tracks connected clients grouped by guild.
type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*Client]struct{}
	log   *slog.Logger
}

// NewHub creates a Hub.
func NewHub(log *slog.Logger) *Hub {
	return &Hub{rooms: map[string]map[*Client]struct{}{}, log: log}
}

// Broadcast sends a pre-encoded message to every client watching guildID.
func (h *Hub) Broadcast(guildID string, msg []byte) {
	h.mu.RLock()
	clients := h.rooms[guildID]
	for c := range clients {
		select {
		case c.send <- msg:
		default:
			// Slow client: drop the message rather than block the fan-out.
		}
	}
	h.mu.RUnlock()
}

// Count returns the number of connected clients (for diagnostics).
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	n := 0
	for _, r := range h.rooms {
		n += len(r)
	}
	return n
}

func (h *Hub) add(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	room := h.rooms[c.guildID]
	if room == nil {
		room = map[*Client]struct{}{}
		h.rooms[c.guildID] = room
	}
	room[c] = struct{}{}
}

func (h *Hub) remove(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room := h.rooms[c.guildID]; room != nil {
		delete(room, c)
		if len(room) == 0 {
			delete(h.rooms, c.guildID)
		}
	}
}

// Client is a single dashboard WebSocket connection scoped to one guild.
type Client struct {
	hub     *Hub
	conn    *websocket.Conn
	guildID string
	send    chan []byte
}

// Serve registers an upgraded connection for a guild and blocks until it closes.
func (h *Hub) Serve(conn *websocket.Conn, guildID string) {
	c := &Client{hub: h, conn: conn, guildID: guildID, send: make(chan []byte, sendBuffer)}
	h.add(c)
	go c.writePump()
	c.readPump() // blocks until the connection closes
}

func (c *Client) readPump() {
	defer func() {
		c.hub.remove(c)
		_ = c.conn.Close()
	}()
	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
