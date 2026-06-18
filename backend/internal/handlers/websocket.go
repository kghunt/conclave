package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/auth"
	ws "github.com/karl/conclave/internal/ws"
)

type WSHandler struct {
	hub            *ws.Hub
	auth           *auth.Service
	db             *pgxpool.Pool
	allowedOrigins map[string]bool
}

func NewWS(hub *ws.Hub, a *auth.Service, db *pgxpool.Pool, baseURL, frontendURL string) *WSHandler {
	origins := map[string]bool{baseURL: true}
	if frontendURL != "" && frontendURL != baseURL {
		origins[frontendURL] = true
	}
	return &WSHandler{hub: hub, auth: a, db: db, allowedOrigins: origins}
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	claims, err := h.auth.TokenFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return origin == "" || h.allowedOrigins[origin]
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	client := h.hub.NewClient(conn, claims.UserID)
	go client.WritePump()
	client.ReadPump(h.onEvent)
}

func (h *WSHandler) onEvent(c *ws.Client, event ws.Event) {
	switch event.Type {
	case "subscribe":
		var body struct {
			Room string `json:"room"`
		}
		if err := json.Unmarshal(event.Payload, &body); err == nil && body.Room != "" {
			if h.canSubscribe(c.UserID(), body.Room) {
				h.hub.Subscribe(c, body.Room)
			}
		}
	case "unsubscribe":
		var body struct {
			Room string `json:"room"`
		}
		if err := json.Unmarshal(event.Payload, &body); err == nil && body.Room != "" {
			h.hub.Unsubscribe(c, body.Room)
		}
	case "typing":
		var body struct {
			Room string `json:"room"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.Room == "" {
			return
		}
		if !c.HasRoom(body.Room) {
			return
		}
		var displayName string
		h.db.QueryRow(context.Background(), `SELECT display_name FROM users WHERE id = $1`, c.UserID()).Scan(&displayName)
		payload, _ := json.Marshal(map[string]string{
			"user_id":      c.UserID(),
			"display_name": displayName,
			"room":         body.Room,
		})
		h.hub.BroadcastExcept(body.Room, c, ws.Event{Type: "typing", Payload: payload})
	}
}

func (h *WSHandler) canSubscribe(userID, room string) bool {
	ctx := context.Background()
	if strings.HasPrefix(room, "channel:") {
		channelID := room[len("channel:"):]
		var ok bool
		h.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM channels c
				JOIN server_members sm ON sm.server_id = c.server_id
				WHERE c.id = $1 AND sm.user_id = $2
			)
		`, channelID, userID).Scan(&ok)
		return ok
	}
	if strings.HasPrefix(room, "dm:") {
		convID := room[len("dm:"):]
		var ok bool
		h.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM dm_conversations
				WHERE id = $1 AND (user1_id = $2 OR user2_id = $2)
			)
		`, convID, userID).Scan(&ok)
		return ok
	}
	if strings.HasPrefix(room, "user:") {
		return room == "user:"+userID
	}
	if strings.HasPrefix(room, "server:") {
		serverID := room[len("server:"):]
		var ok bool
		h.db.QueryRow(ctx, `
			SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id = $1 AND user_id = $2)
		`, serverID, userID).Scan(&ok)
		return ok
	}
	return false
}
