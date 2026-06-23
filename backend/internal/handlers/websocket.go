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
	case "presence":
		var body struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil {
			return
		}
		if body.Status != "online" && body.Status != "away" {
			return
		}
		h.hub.SetPresence(c.UserID(), body.Status)
		go h.broadcastPresence(c.UserID(), body.Status)

	case "voice.join":
		var body struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.ChannelID == "" {
			return
		}
		// Verify user is a member of the channel's server
		if !c.HasRoom("channel:" + body.ChannelID) {
			return
		}
		// Look up server ID before joining so the hub can cache it for ungraceful-disconnect cleanup.
		var serverID string
		h.db.QueryRow(context.Background(), `SELECT server_id FROM channels WHERE id = $1`, body.ChannelID).Scan(&serverID)
		existingIDs := h.hub.VoiceJoin(body.ChannelID, serverID, c)

		// Fetch user info for all existing peers and send voice.state to joiner
		go func() {
			peers := h.fetchVoicePeers(existingIDs)
			statePayload, _ := json.Marshal(map[string]any{
				"channel_id": body.ChannelID,
				"peers":      peers,
			})
			data, _ := json.Marshal(ws.Event{Type: "voice.state", Payload: statePayload})
			c.SendRaw(data)

			// Fetch joiner info and broadcast voice.joined to channel and server rooms
			var joiner struct {
				UserID      string `json:"user_id"`
				DisplayName string `json:"display_name"`
				AvatarURL   string `json:"avatar_url"`
			}
			joiner.UserID = c.UserID()
			h.db.QueryRow(context.Background(), `SELECT display_name, avatar_url FROM users WHERE id = $1`, c.UserID()).Scan(&joiner.DisplayName, &joiner.AvatarURL)
			joinedPayload, _ := json.Marshal(map[string]any{"channel_id": body.ChannelID, "user": joiner})
			h.hub.BroadcastExcept("channel:"+body.ChannelID, c, ws.Event{Type: "voice.joined", Payload: joinedPayload})
			if serverID != "" {
				h.hub.BroadcastExcept("server:"+serverID, c, ws.Event{Type: "voice.joined", Payload: joinedPayload})
			}
		}()

	case "voice.leave":
		var body struct {
			ChannelID string `json:"channel_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.ChannelID == "" {
			return
		}
		h.hub.VoiceLeave(body.ChannelID, c)
		var svrID string
		h.db.QueryRow(context.Background(), `SELECT server_id FROM channels WHERE id = $1`, body.ChannelID).Scan(&svrID)
		leftPayload, _ := json.Marshal(map[string]string{"channel_id": body.ChannelID, "user_id": c.UserID()})
		h.hub.Broadcast("channel:"+body.ChannelID, ws.Event{Type: "voice.left", Payload: leftPayload})
		if svrID != "" {
			h.hub.Broadcast("server:"+svrID, ws.Event{Type: "voice.left", Payload: leftPayload})
		}

	case "call.ring":
		var body struct {
			ToUserID string `json:"to_user_id"`
			ConvID   string `json:"conv_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.ToUserID == "" || body.ConvID == "" {
			return
		}
		// Verify they are friends
		var areFriends bool
		h.db.QueryRow(context.Background(), `
			SELECT EXISTS(
				SELECT 1 FROM friendships
				WHERE ((requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1))
				AND status = 'accepted'
			)
		`, c.UserID(), body.ToUserID).Scan(&areFriends)
		if !areFriends {
			return
		}
		var displayName, avatarURL string
		h.db.QueryRow(context.Background(), `SELECT display_name, COALESCE(avatar_url,'') FROM users WHERE id=$1`, c.UserID()).
			Scan(&displayName, &avatarURL)
		ringPayload, _ := json.Marshal(map[string]string{
			"conv_id":           body.ConvID,
			"from_user_id":      c.UserID(),
			"from_display_name": displayName,
			"from_avatar_url":   avatarURL,
		})
		h.hub.Broadcast("user:"+body.ToUserID, ws.Event{Type: "call.ring", Payload: ringPayload})

	case "call.accept":
		var body struct {
			ConvID   string `json:"conv_id"`
			CallerID string `json:"caller_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.CallerID == "" {
			return
		}
		var displayName string
		h.db.QueryRow(context.Background(), `SELECT display_name FROM users WHERE id=$1`, c.UserID()).Scan(&displayName)
		acceptPayload, _ := json.Marshal(map[string]string{
			"conv_id":              body.ConvID,
			"from_user_id":        c.UserID(),
			"from_display_name":   displayName,
		})
		h.hub.Broadcast("user:"+body.CallerID, ws.Event{Type: "call.accepted", Payload: acceptPayload})

	case "call.decline":
		var body struct {
			ConvID   string `json:"conv_id"`
			CallerID string `json:"caller_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.CallerID == "" {
			return
		}
		declinePayload, _ := json.Marshal(map[string]string{"conv_id": body.ConvID})
		h.hub.Broadcast("user:"+body.CallerID, ws.Event{Type: "call.declined", Payload: declinePayload})

	case "call.end":
		var body struct {
			ConvID      string `json:"conv_id"`
			OtherUserID string `json:"other_user_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.OtherUserID == "" {
			return
		}
		endPayload, _ := json.Marshal(map[string]string{"conv_id": body.ConvID})
		h.hub.Broadcast("user:"+body.OtherUserID, ws.Event{Type: "call.ended", Payload: endPayload})

	case "call.cancel":
		var body struct {
			ToUserID string `json:"to_user_id"`
			ConvID   string `json:"conv_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.ToUserID == "" {
			return
		}
		cancelPayload, _ := json.Marshal(map[string]string{"conv_id": body.ConvID})
		h.hub.Broadcast("user:"+body.ToUserID, ws.Event{Type: "call.cancelled", Payload: cancelPayload})

	case "game.status":
		// Desktop app reports which game the user is running.
		var body struct {
			Game string `json:"game"` // empty string = no game
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil {
			return
		}
		h.hub.SetGameStatus(c.UserID(), body.Game)

	case "voice.sub.create":
		var body struct {
			ChannelID string `json:"channel_id"`
			Name      string `json:"name"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.ChannelID == "" {
			return
		}
		if !c.HasRoom("channel:" + body.ChannelID) {
			return
		}
		if body.Name == "" {
			body.Name = "Sub-channel"
		}
		var serverID, displayName, avatarURL string
		h.db.QueryRow(context.Background(), `SELECT server_id FROM channels WHERE id = $1`, body.ChannelID).Scan(&serverID)
		h.db.QueryRow(context.Background(), `SELECT display_name, COALESCE(avatar_url,'') FROM users WHERE id = $1`, c.UserID()).
			Scan(&displayName, &avatarURL)
		peer := ws.VoiceSubParticipant{UserID: c.UserID(), DisplayName: displayName, AvatarURL: avatarURL}
		sub := h.hub.VoiceSubCreate(body.ChannelID, serverID, c.UserID(), body.Name, peer, c)
		// Tell the creator which sub ID was assigned so the frontend can request a token.
		createdPayload, _ := json.Marshal(map[string]string{
			"channel_id": body.ChannelID,
			"sub_id":     sub.ID,
			"name":       sub.Name,
		})
		data, _ := json.Marshal(ws.Event{Type: "voice.sub.created", Payload: createdPayload})
		c.SendRaw(data)
		h.broadcastSubState(body.ChannelID, serverID)

	case "voice.sub.join":
		var body struct {
			ChannelID string `json:"channel_id"`
			SubID     string `json:"sub_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.ChannelID == "" || body.SubID == "" {
			return
		}
		if !c.HasRoom("channel:" + body.ChannelID) {
			return
		}
		var serverID, displayName, avatarURL string
		h.db.QueryRow(context.Background(), `SELECT server_id FROM channels WHERE id = $1`, body.ChannelID).Scan(&serverID)
		h.db.QueryRow(context.Background(), `SELECT display_name, COALESCE(avatar_url,'') FROM users WHERE id = $1`, c.UserID()).
			Scan(&displayName, &avatarURL)
		peer := ws.VoiceSubParticipant{UserID: c.UserID(), DisplayName: displayName, AvatarURL: avatarURL}
		if _, ok := h.hub.VoiceSubJoin(body.ChannelID, body.SubID, c.UserID(), peer, c); !ok {
			return
		}
		h.broadcastSubState(body.ChannelID, serverID)

	case "voice.sub.leave":
		var body struct {
			ChannelID string `json:"channel_id"`
			SubID     string `json:"sub_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.ChannelID == "" || body.SubID == "" {
			return
		}
		sub, wasClosed, _ := h.hub.VoiceSubLeave(body.ChannelID, body.SubID, c.UserID())
		serverID := ""
		if sub != nil {
			serverID = sub.ServerID
		}
		if wasClosed && serverID != "" {
			closedPayload, _ := json.Marshal(map[string]string{"channel_id": body.ChannelID, "sub_id": body.SubID})
			h.hub.Broadcast("server:"+serverID, ws.Event{Type: "voice.sub.closed", Payload: closedPayload})
		} else if serverID != "" {
			h.broadcastSubState(body.ChannelID, serverID)
		}

	case "voice.sub.close":
		var body struct {
			ChannelID string `json:"channel_id"`
			SubID     string `json:"sub_id"`
		}
		if err := json.Unmarshal(event.Payload, &body); err != nil || body.ChannelID == "" || body.SubID == "" {
			return
		}
		serverID, _ := h.hub.VoiceSubClose(body.ChannelID, body.SubID)
		if serverID != "" {
			closedPayload, _ := json.Marshal(map[string]string{"channel_id": body.ChannelID, "sub_id": body.SubID})
			h.hub.Broadcast("server:"+serverID, ws.Event{Type: "voice.sub.closed", Payload: closedPayload})
		}

	}
}

func (h *WSHandler) broadcastSubState(channelID, serverID string) {
	if serverID == "" {
		return
	}
	subs := h.hub.GetVoiceSubsSnapshot(channelID)
	payload, _ := json.Marshal(map[string]any{"channel_id": channelID, "subs": subs})
	h.hub.Broadcast("server:"+serverID, ws.Event{Type: "voice.sub.state", Payload: payload})
}

func (h *WSHandler) fetchVoicePeers(userIDs []string) []map[string]string {
	out := make([]map[string]string, 0, len(userIDs))
	for _, uid := range userIDs {
		var name, avatar string
		h.db.QueryRow(context.Background(), `SELECT display_name, avatar_url FROM users WHERE id = $1`, uid).Scan(&name, &avatar)
		out = append(out, map[string]string{"user_id": uid, "display_name": name, "avatar_url": avatar})
	}
	return out
}

func (h *WSHandler) RunPresenceBroadcaster() {
	for change := range h.hub.PresenceChanges {
		h.broadcastPresence(change.UserID, change.Status)
	}
}

func (h *WSHandler) broadcastPresence(userID, status string) {
	rows, err := h.db.Query(context.Background(),
		`SELECT server_id FROM server_members WHERE user_id = $1`, userID)
	if err != nil {
		return
	}
	defer rows.Close()
	payload, _ := json.Marshal(map[string]string{"user_id": userID, "status": status})
	for rows.Next() {
		var serverID string
		rows.Scan(&serverID)
		h.hub.Broadcast("server:"+serverID, ws.Event{Type: "presence.update", Payload: payload})
	}
}

func (h *WSHandler) RunGameStatusBroadcaster() {
	for change := range h.hub.GameStatusChanges {
		h.broadcastGameStatus(change.UserID, change.Game)
	}
}

func (h *WSHandler) broadcastGameStatus(userID, game string) {
	rows, err := h.db.Query(context.Background(),
		`SELECT server_id FROM server_members WHERE user_id = $1`, userID)
	if err != nil {
		return
	}
	defer rows.Close()
	payload, _ := json.Marshal(map[string]string{"user_id": userID, "game": game})
	evt := ws.Event{Type: "presence.game", Payload: payload}
	for rows.Next() {
		var serverID string
		rows.Scan(&serverID)
		h.hub.Broadcast("server:"+serverID, evt)
	}
	// Also send to the user's own room so their own tab sees it.
	h.hub.Broadcast("user:"+userID, evt)
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
	if strings.HasPrefix(room, "thread:") {
		threadID := room[len("thread:"):]
		var ok bool
		h.db.QueryRow(ctx, `
			SELECT EXISTS(
				SELECT 1 FROM threads t
				JOIN channels c ON c.id = t.channel_id
				JOIN server_members sm ON sm.server_id = c.server_id
				WHERE t.id = $1 AND sm.user_id = $2
			)`, threadID, userID).Scan(&ok)
		return ok
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
