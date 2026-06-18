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

type Hub struct {
	clients    map[*Client]bool
	rooms      map[string]map[*Client]bool // room -> clients
	register   chan *Client
	unregister chan *Client
	broadcast  chan roomMessage
	mu         sync.RWMutex
}

type roomMessage struct {
	room    string
	payload []byte
}

func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		rooms:      make(map[string]map[*Client]bool),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		broadcast:  make(chan roomMessage, 256),
	}
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = true
			h.mu.Unlock()

		case c := <-h.unregister:
			h.mu.Lock()
			if h.clients[c] {
				delete(h.clients, c)
				for room := range c.rooms {
					delete(h.rooms[room], c)
				}
				close(c.send)
			}
			h.mu.Unlock()

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
