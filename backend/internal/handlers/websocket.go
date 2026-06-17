package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/karl/conclave/internal/auth"
	ws "github.com/karl/conclave/internal/ws"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

type WSHandler struct {
	hub  *ws.Hub
	auth *auth.Service
}

func NewWS(hub *ws.Hub, a *auth.Service) *WSHandler {
	return &WSHandler{hub: hub, auth: a}
}

func (h *WSHandler) Handle(w http.ResponseWriter, r *http.Request) {
	claims, err := h.auth.TokenFromRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
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
			h.hub.Subscribe(c, body.Room)
		}
	case "unsubscribe":
		var body struct {
			Room string `json:"room"`
		}
		if err := json.Unmarshal(event.Payload, &body); err == nil && body.Room != "" {
			h.hub.Unsubscribe(c, body.Room)
		}
	}
}
