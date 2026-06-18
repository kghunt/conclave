package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
	"github.com/karl/conclave/internal/models"
	"github.com/karl/conclave/internal/ws"
)

type DMsHandler struct {
	db   *pgxpool.Pool
	hub  *ws.Hub
	push *PushHandler
}

func NewDMs(db *pgxpool.Pool, hub *ws.Hub, push *PushHandler) *DMsHandler {
	return &DMsHandler{db: db, hub: hub, push: push}
}

func (h *DMsHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	rows, err := h.db.Query(r.Context(), `
		SELECT dc.id, dc.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url
		FROM dm_conversations dc
		JOIN users u ON u.id = CASE WHEN dc.user1_id = $1 THEN dc.user2_id ELSE dc.user1_id END
		WHERE dc.user1_id = $1 OR dc.user2_id = $1
		ORDER BY dc.created_at DESC
	`, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	convs := make([]models.DMConversation, 0)
	for rows.Next() {
		var c models.DMConversation
		c.OtherUser = &models.User{}
		if err := rows.Scan(&c.ID, &c.CreatedAt, &c.OtherUser.ID, &c.OtherUser.DisplayName, &c.OtherUser.Bio, &c.OtherUser.AvatarURL); err != nil {
			continue
		}
		convs = append(convs, c)
	}
	writeJSON(w, http.StatusOK, convs)
}

func (h *DMsHandler) GetOrCreate(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	otherID := chi.URLParam(r, "userID")

	if userID == otherID {
		writeErr(w, http.StatusBadRequest, "cannot DM yourself")
		return
	}

	u1, u2 := userID, otherID
	if u1 > u2 {
		u1, u2 = u2, u1
	}

	var conv models.DMConversation
	conv.OtherUser = &models.User{}
	err := h.db.QueryRow(r.Context(), `
		WITH ins AS (
			INSERT INTO dm_conversations (user1_id, user2_id) VALUES ($1, $2)
			ON CONFLICT (user1_id, user2_id) DO UPDATE SET user1_id = EXCLUDED.user1_id
			RETURNING id, created_at
		)
		SELECT ins.id, ins.created_at, u.id, u.display_name, u.bio, u.avatar_url
		FROM ins, users u WHERE u.id = $3
	`, u1, u2, otherID).Scan(&conv.ID, &conv.CreatedAt, &conv.OtherUser.ID, &conv.OtherUser.DisplayName, &conv.OtherUser.Bio, &conv.OtherUser.AvatarURL)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create conversation failed")
		return
	}
	writeJSON(w, http.StatusOK, conv)
}

func (h *DMsHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "convID")
	userID := middleware.UserID(r)

	var isParticipant bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM dm_conversations WHERE id=$1 AND (user1_id=$2 OR user2_id=$2))`, convID, userID).Scan(&isParticipant)
	if !isParticipant {
		writeErr(w, http.StatusForbidden, "not a participant")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT dm.id, dm.conversation_id, dm.content, dm.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url
		FROM direct_messages dm JOIN users u ON u.id = dm.sender_id
		WHERE dm.conversation_id = $1
		ORDER BY dm.created_at DESC LIMIT 50
	`, convID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	messages := make([]models.DirectMessage, 0)
	for rows.Next() {
		var m models.DirectMessage
		m.Sender = &models.User{}
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Content, &m.CreatedAt,
			&m.Sender.ID, &m.Sender.DisplayName, &m.Sender.Bio, &m.Sender.AvatarURL); err != nil {
			continue
		}
		messages = append(messages, m)
	}
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	writeJSON(w, http.StatusOK, messages)
}

func (h *DMsHandler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "convID")
	messageID := chi.URLParam(r, "messageID")
	userID := middleware.UserID(r)

	var isParticipant bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM dm_conversations WHERE id=$1 AND (user1_id=$2 OR user2_id=$2))`, convID, userID).Scan(&isParticipant)
	if !isParticipant {
		writeErr(w, http.StatusForbidden, "not a participant")
		return
	}

	tag, err := h.db.Exec(r.Context(), `DELETE FROM direct_messages WHERE id=$1 AND sender_id=$2`, messageID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusForbidden, "not your message")
		return
	}

	payload, _ := json.Marshal(map[string]string{"id": messageID, "conversation_id": convID})
	h.hub.Broadcast("dm:"+convID, ws.Event{Type: "dm.delete", Payload: payload})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *DMsHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "convID")
	userID := middleware.UserID(r)

	var isParticipant bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM dm_conversations WHERE id=$1 AND (user1_id=$2 OR user2_id=$2))`, convID, userID).Scan(&isParticipant)
	if !isParticipant {
		writeErr(w, http.StatusForbidden, "not a participant")
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Content == "" {
		writeErr(w, http.StatusBadRequest, "content required")
		return
	}
	if len(body.Content) > 4000 {
		writeErr(w, http.StatusBadRequest, "message too long (max 4000 characters)")
		return
	}

	var m models.DirectMessage
	m.Sender = &models.User{}
	err := h.db.QueryRow(r.Context(), `
		WITH ins AS (
			INSERT INTO direct_messages (conversation_id, sender_id, content) VALUES ($1, $2, $3)
			RETURNING id, conversation_id, content, created_at, sender_id
		)
		SELECT ins.id, ins.conversation_id, ins.content, ins.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url
		FROM ins JOIN users u ON u.id = ins.sender_id
	`, convID, userID, body.Content).Scan(
		&m.ID, &m.ConversationID, &m.Content, &m.CreatedAt,
		&m.Sender.ID, &m.Sender.DisplayName, &m.Sender.Bio, &m.Sender.AvatarURL,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "send failed")
		return
	}

	payload, _ := json.Marshal(m)
	h.hub.Broadcast("dm:"+convID, ws.Event{Type: "dm.new", Payload: payload})

	if h.push.enabled() {
		var recipientID string
		h.db.QueryRow(r.Context(), `SELECT CASE WHEN user1_id=$1 THEN user2_id ELSE user1_id END FROM dm_conversations WHERE id=$2`, userID, convID).Scan(&recipientID)
		content := body.Content
		if len(content) > 100 {
			content = content[:97] + "…"
		}
		go h.push.SendToUser(recipientID, PushPayload{
			Title: m.Sender.DisplayName,
			Body:  content,
			URL:   "/",
		})
	}

	writeJSON(w, http.StatusCreated, m)
}
