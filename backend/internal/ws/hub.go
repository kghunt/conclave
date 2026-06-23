package ws

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// VoiceSubParticipant carries display info for sub-channel state broadcasts.
type VoiceSubParticipant struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	AvatarURL   string `json:"avatar_url"`
}

// VoiceSub is an ephemeral breakout room nested inside a voice channel.
type VoiceSub struct {
	ID           string
	ChannelID    string
	ServerID     string
	Name         string
	CreatorID    string
	Participants map[string]*Client              // userID → *Client
	PeerInfo     map[string]VoiceSubParticipant  // userID → display info
}

func newSubID() string {
	b := make([]byte, 6)
	rand.Read(b)
	return hex.EncodeToString(b)
}

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
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
	rooms  map[string]bool
	mu     sync.Mutex
}

type PresenceChange struct {
	UserID string
	Status string // "online" | "away" | "offline"
}

type GameStatusChange struct {
	UserID string
	Game   string
}

type gameStatusEntry struct {
	game      string
	updatedAt time.Time
}

type Hub struct {
	clients    map[*Client]bool
	rooms      map[string]map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan roomMessage
	mu         sync.RWMutex

	userConns       map[string]int
	userPresence    map[string]string
	PresenceChanges chan PresenceChange

	gameStatus        map[string]gameStatusEntry
	gameStatusMu      sync.RWMutex
	GameStatusChanges chan GameStatusChange

	voiceRooms     map[string]map[string]*Client // channelID → userID → *Client
	voiceServerMap map[string]string             // channelID → serverID
	voiceSubs      map[string]map[string]*VoiceSub // channelID → subID → *VoiceSub
	voiceMu        sync.RWMutex
}

type roomMessage struct {
	room    string
	payload []byte
}

func NewHub() *Hub {
	h := &Hub{
		clients:           make(map[*Client]bool),
		rooms:             make(map[string]map[*Client]bool),
		register:          make(chan *Client, 64),
		unregister:        make(chan *Client, 64),
		broadcast:         make(chan roomMessage, 512),
		userConns:         make(map[string]int),
		userPresence:      make(map[string]string),
		PresenceChanges:   make(chan PresenceChange, 256),
		gameStatus:        make(map[string]gameStatusEntry),
		GameStatusChanges: make(chan GameStatusChange, 256),
		voiceRooms:        make(map[string]map[string]*Client),
		voiceServerMap:    make(map[string]string),
		voiceSubs:         make(map[string]map[string]*VoiceSub),
	}
	go h.runGameStatusCleaner()
	return h
}

// runGameStatusCleaner clears statuses with no heartbeat in 90 seconds.
func (h *Hub) runGameStatusCleaner() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		h.gameStatusMu.Lock()
		now := time.Now()
		for userID, entry := range h.gameStatus {
			if now.Sub(entry.updatedAt) > 90*time.Second {
				delete(h.gameStatus, userID)
				select {
				case h.GameStatusChanges <- GameStatusChange{UserID: userID, Game: ""}:
				default:
				}
			}
		}
		h.gameStatusMu.Unlock()
	}
}

func (h *Hub) SetGameStatus(userID, game string) {
	h.gameStatusMu.Lock()
	if game == "" {
		delete(h.gameStatus, userID)
	} else {
		h.gameStatus[userID] = gameStatusEntry{game: game, updatedAt: time.Now()}
	}
	h.gameStatusMu.Unlock()
	select {
	case h.GameStatusChanges <- GameStatusChange{UserID: userID, Game: game}:
	default:
	}
}

func (h *Hub) GetAllGameStatuses() map[string]string {
	h.gameStatusMu.RLock()
	defer h.gameStatusMu.RUnlock()
	out := make(map[string]string, len(h.gameStatus))
	for uid, e := range h.gameStatus {
		out[uid] = e.game
	}
	return out
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
				select {
				case h.PresenceChanges <- PresenceChange{c.userID, "online"}:
				default:
				}
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
				select {
				case h.PresenceChanges <- PresenceChange{c.userID, "offline"}:
				default:
				}
				// Clear game status when the user goes fully offline.
				h.gameStatusMu.Lock()
				if _, had := h.gameStatus[c.userID]; had {
					delete(h.gameStatus, c.userID)
					h.gameStatusMu.Unlock()
					select {
					case h.GameStatusChanges <- GameStatusChange{UserID: c.userID, Game: ""}:
					default:
					}
				} else {
					h.gameStatusMu.Unlock()
				}
			}

			h.voiceMu.Lock()
			type leftRoom struct{ channelID, serverID string }
			var leftRooms []leftRoom
			for channelID, peers := range h.voiceRooms {
				if _, ok := peers[c.userID]; ok {
					delete(peers, c.userID)
					serverID := h.voiceServerMap[channelID]
					if len(peers) == 0 {
						delete(h.voiceRooms, channelID)
						delete(h.voiceServerMap, channelID)
					}
					leftRooms = append(leftRooms, leftRoom{channelID, serverID})
				}
			}
			h.voiceMu.Unlock()

			if len(leftRooms) > 0 {
				userID := c.userID
				go func() {
					for _, lr := range leftRooms {
						inner, _ := json.Marshal(map[string]string{"channel_id": lr.channelID, "user_id": userID})
						evt, _ := json.Marshal(Event{Type: "voice.left", Payload: inner})
						h.broadcast <- roomMessage{room: "channel:" + lr.channelID, payload: evt}
						if lr.serverID != "" {
							h.broadcast <- roomMessage{room: "server:" + lr.serverID, payload: evt}
						}
					}
				}()
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
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) ReadPump(onEvent func(c *Client, event Event)) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(65536)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
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

func (h *Hub) VoiceJoin(channelID, serverID string, c *Client) []string {
	h.voiceMu.Lock()
	defer h.voiceMu.Unlock()
	if h.voiceRooms[channelID] == nil {
		h.voiceRooms[channelID] = make(map[string]*Client)
	}
	if serverID != "" {
		h.voiceServerMap[channelID] = serverID
	}
	existing := make([]string, 0, len(h.voiceRooms[channelID]))
	for uid := range h.voiceRooms[channelID] {
		existing = append(existing, uid)
	}
	h.voiceRooms[channelID][c.userID] = c
	return existing
}

func (h *Hub) VoiceLeave(channelID string, c *Client) {
	h.voiceMu.Lock()
	defer h.voiceMu.Unlock()
	if peers, ok := h.voiceRooms[channelID]; ok {
		delete(peers, c.userID)
		if len(peers) == 0 {
			delete(h.voiceRooms, channelID)
			delete(h.voiceServerMap, channelID)
		}
	}
}

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

// ── Voice sub-channel (breakout room) methods ────────────────────────────────

// VoiceSubCreate creates a new breakout room and returns it.
func (h *Hub) VoiceSubCreate(channelID, serverID, creatorID, name string, peer VoiceSubParticipant, c *Client) *VoiceSub {
	h.voiceMu.Lock()
	defer h.voiceMu.Unlock()
	if h.voiceSubs[channelID] == nil {
		h.voiceSubs[channelID] = make(map[string]*VoiceSub)
	}
	sub := &VoiceSub{
		ID:           newSubID(),
		ChannelID:    channelID,
		ServerID:     serverID,
		Name:         name,
		CreatorID:    creatorID,
		Participants: map[string]*Client{creatorID: c},
		PeerInfo:     map[string]VoiceSubParticipant{creatorID: peer},
	}
	h.voiceSubs[channelID][sub.ID] = sub
	return sub
}

// VoiceSubJoin adds a user to an existing sub. Returns (sub, ok).
func (h *Hub) VoiceSubJoin(channelID, subID, userID string, peer VoiceSubParticipant, c *Client) (*VoiceSub, bool) {
	h.voiceMu.Lock()
	defer h.voiceMu.Unlock()
	sub, ok := h.voiceSubs[channelID][subID]
	if !ok {
		return nil, false
	}
	sub.Participants[userID] = c
	sub.PeerInfo[userID] = peer
	return sub, true
}

// VoiceSubLeave removes a user from a sub. Returns (sub, wasClosed, closedParticipants).
func (h *Hub) VoiceSubLeave(channelID, subID, userID string) (sub *VoiceSub, wasClosed bool, closedParticipants []string) {
	h.voiceMu.Lock()
	defer h.voiceMu.Unlock()
	s, ok := h.voiceSubs[channelID][subID]
	if !ok {
		return nil, false, nil
	}
	delete(s.Participants, userID)
	delete(s.PeerInfo, userID)
	if len(s.Participants) == 0 {
		delete(h.voiceSubs[channelID], subID)
		if len(h.voiceSubs[channelID]) == 0 {
			delete(h.voiceSubs, channelID)
		}
		return s, true, nil
	}
	return s, false, nil
}

// VoiceSubClose force-closes a sub and returns its participant user IDs.
func (h *Hub) VoiceSubClose(channelID, subID string) (serverID string, participants []string) {
	h.voiceMu.Lock()
	defer h.voiceMu.Unlock()
	sub, ok := h.voiceSubs[channelID][subID]
	if !ok {
		return "", nil
	}
	for uid := range sub.Participants {
		participants = append(participants, uid)
	}
	serverID = sub.ServerID
	delete(h.voiceSubs[channelID], subID)
	if len(h.voiceSubs[channelID]) == 0 {
		delete(h.voiceSubs, channelID)
	}
	return serverID, participants
}

// GetVoiceSubsSnapshot returns a serialisable snapshot of all subs for a channel.
func (h *Hub) GetVoiceSubsSnapshot(channelID string) []map[string]any {
	h.voiceMu.RLock()
	defer h.voiceMu.RUnlock()
	subs := h.voiceSubs[channelID]
	out := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		peers := make([]VoiceSubParticipant, 0, len(sub.PeerInfo))
		for _, p := range sub.PeerInfo {
			peers = append(peers, p)
		}
		out = append(out, map[string]any{
			"id":           sub.ID,
			"name":         sub.Name,
			"creator_id":   sub.CreatorID,
			"participants": peers,
		})
	}
	return out
}

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
