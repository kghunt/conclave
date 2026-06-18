package ws

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID string
	rooms  map[string]bool // channel IDs or "dm:conversationID"
	mu     sync.Mutex
}

type PresenceChange struct {
	UserID string
	Status string // "online" | "away" | "offline"
}

type Hub struct {
	clients    map[*Client]bool
	rooms      map[string]map[*Client]bool // room -> clients
	register   chan *Client
	unregister chan *Client
	broadcast  chan roomMessage
	mu         sync.RWMutex

	userConns    map[string]int    // userId → active connection count
	userPresence map[string]string // userId → "online"|"away" (absent = offline)
	PresenceChanges chan PresenceChange

	voiceRooms map[string]map[string]*Client // channelID → userID → *Client
	voiceMu    sync.RWMutex
}

type roomMessage struct {
	room    string
	payload []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:         make(map[*Client]bool),
		rooms:           make(map[string]map[*Client]bool),
		register:        make(chan *Client, 64),
		unregister:      make(chan *Client, 64),
		broadcast:       make(chan roomMessage, 256),
		userConns:       make(map[string]int),
		userPresence:    make(map[string]string),
		PresenceChanges: make(chan PresenceChange, 256),
		voiceRooms:      make(map[string]map[string]*Client),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			prev := h.userConns[c.userID]
			h.userConns[c.userID]++
			if prev == 0 {
				h.userPresence[c.userID] = "online"
			}
			h.mu.Unlock()
			if prev == 0 {
				select { case h.PresenceChanges <- PresenceChange{c.userID, "online"}: default: }
			}

		case c := <-h.unregister:
			h.mu.Lock()
			emitOffline := false
			if h.clients[c] {
				delete(h.clients, c)
				for room := range c.rooms {
					delete(h.rooms[room], c)
				}
				close(c.send)
				h.userConns[c.userID]--
				if h.userConns[c.userID] == 0 {
					delete(h.userPresence, c.userID)
					emitOffline = true
				}
			}
			h.mu.Unlock()
			if emitOffline {
				select { case h.PresenceChanges <- PresenceChange{c.userID, "offline"}: default: }
			}
			// Clean up voice rooms on disconnect
			h.voiceMu.Lock()
			var leftChannels []string
			for channelID, peers := range h.voiceRooms {
				if _, ok := peers[c.userID]; ok {
					delete(peers, c.userID)
					if len(peers) == 0 {
						delete(h.voiceRooms, channelID)
					}
					leftChannels = append(leftChannels, channelID)
				}
			}
			h.voiceMu.Unlock()
			for _, channelID := range leftChannels {
				inner, _ := json.Marshal(map[string]string{"channel_id": channelID, "user_id": c.userID})
				data, _ := json.Marshal(Event{Type: "voice.left", Payload: inner})
				select { case h.broadcast <- roomMessage{room: "channel:" + channelID, payload: data}: default: }
			}

		case msg := <-h.broadcast:
			h.mu.RLock()
			clients := h.rooms[msg.room]
			h.mu.RUnlock()
			for c := range clients {
				select {
				case c.send <- msg.payload:
				default:
					h.unregister <- c
				}
			}
		}
	}
}

func (h *Hub) Broadcast(room string, event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("ws marshal error: %v", err)
		return
	}
	h.broadcast <- roomMessage{room: room, payload: data}
}

func (h *Hub) Subscribe(c *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[room] == nil {
		h.rooms[room] = make(map[*Client]bool)
	}
	h.rooms[room][c] = true
	c.mu.Lock()
	c.rooms[room] = true
	c.mu.Unlock()
}

func (h *Hub) Unsubscribe(c *Client, room string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.rooms[room], c)
	c.mu.Lock()
	delete(c.rooms, room)
	c.mu.Unlock()
}

func (h *Hub) NewClient(conn *websocket.Conn, userID string) *Client {
	c := &Client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
		rooms:  make(map[string]bool),
	}
	h.register <- c
	return c
}

func (c *Client) WritePump() {
	defer c.conn.Close()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

func (c *Client) ReadPump(onEvent func(c *Client, event Event)) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(65536)
	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var event Event
		if err := json.Unmarshal(msg, &event); err != nil {
			continue
		}
		onEvent(c, event)
	}
}

func (c *Client) UserID() string { return c.userID }

func (c *Client) SendRaw(data []byte) {
	select {
	case c.send <- data:
	default:
		c.hub.unregister <- c
	}
}

func (h *Hub) SetPresence(userID, status string) {
	h.mu.Lock()
	if h.userConns[userID] > 0 {
		h.userPresence[userID] = status
	}
	h.mu.Unlock()
}

func (h *Hub) GetStatus(userID string) string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if s, ok := h.userPresence[userID]; ok {
		return s
	}
	return "offline"
}

func (c *Client) HasRoom(room string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rooms[room]
}

// VoiceJoin adds a client to a voice room and returns the existing peers' userIDs.
func (h *Hub) VoiceJoin(channelID string, c *Client) []string {
	h.voiceMu.Lock()
	defer h.voiceMu.Unlock()
	if h.voiceRooms[channelID] == nil {
		h.voiceRooms[channelID] = make(map[string]*Client)
	}
	existing := make([]string, 0, len(h.voiceRooms[channelID]))
	for uid := range h.voiceRooms[channelID] {
		existing = append(existing, uid)
	}
	h.voiceRooms[channelID][c.userID] = c
	return existing
}

// VoiceLeave removes a client from a voice room.
func (h *Hub) VoiceLeave(channelID string, c *Client) {
	h.voiceMu.Lock()
	defer h.voiceMu.Unlock()
	if peers, ok := h.voiceRooms[channelID]; ok {
		delete(peers, c.userID)
		if len(peers) == 0 {
			delete(h.voiceRooms, channelID)
		}
	}
}

// VoicePeers returns a snapshot of userIDs currently in a voice channel.
func (h *Hub) VoicePeers(channelID string) []string {
	h.voiceMu.RLock()
	defer h.voiceMu.RUnlock()
	peers := h.voiceRooms[channelID]
	out := make([]string, 0, len(peers))
	for uid := range peers {
		out = append(out, uid)
	}
	return out
}

// VoiceAllPeers returns a snapshot of all voice rooms: channelID → []userID.
func (h *Hub) VoiceAllPeers() map[string][]string {
	h.voiceMu.RLock()
	defer h.voiceMu.RUnlock()
	out := make(map[string][]string, len(h.voiceRooms))
	for channelID, peers := range h.voiceRooms {
		uids := make([]string, 0, len(peers))
		for uid := range peers {
			uids = append(uids, uid)
		}
		out[channelID] = uids
	}
	return out
}

// VoiceSendTo delivers a raw message directly to a specific user in a voice channel.
func (h *Hub) VoiceSendTo(channelID, toUserID string, data []byte) bool {
	h.voiceMu.RLock()
	c, ok := h.voiceRooms[channelID][toUserID]
	h.voiceMu.RUnlock()
	if !ok {
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		h.unregister <- c
		return false
	}
}

// BroadcastExcept sends to all clients in a room except the given one.
func (h *Hub) BroadcastExcept(room string, exclude *Client, event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	h.mu.RLock()
	snapshot := make([]*Client, 0, len(h.rooms[room]))
	for c := range h.rooms[room] {
		snapshot = append(snapshot, c)
	}
	h.mu.RUnlock()
	for _, c := range snapshot {
		if c == exclude {
			continue
		}
		select {
		case c.send <- data:
		default:
			h.unregister <- c
		}
	}
}
