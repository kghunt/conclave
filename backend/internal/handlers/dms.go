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
	db        *pgxpool.Pool
	hub       *ws.Hub
	push      *PushHandler
	uploadDir string
	baseURL   string
}

func NewDMs(db *pgxpool.Pool, hub *ws.Hub, push *PushHandler, uploadDir, baseURL string) *DMsHandler {
	return &DMsHandler{db: db, hub: hub, push: push, uploadDir: uploadDir, baseURL: baseURL}
}

func (h *DMsHandler) ListConversations(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	rows, err := h.db.Query(r.Context(), `
		SELECT dc.id, dc.created_at,
		       GREATEST(MAX(dm.created_at), dc.created_at) AS last_message_at,
		       u.id, u.display_name, u.bio, u.avatar_url,
		       COUNT(dm.id) FILTER (
		           WHERE dm.sender_id != $1
		             AND dm.created_at > CASE WHEN dc.user1_id = $1 THEN dc.user1_read_at ELSE dc.user2_read_at END
		       )::int AS unread_count
		FROM dm_conversations dc
		JOIN users u ON u.id = CASE WHEN dc.user1_id = $1 THEN dc.user2_id ELSE dc.user1_id END
		LEFT JOIN direct_messages dm ON dm.conversation_id = dc.id
		WHERE dc.user1_id = $1 OR dc.user2_id = $1
		GROUP BY dc.id, dc.created_at, u.id, u.display_name, u.bio, u.avatar_url, dc.user1_read_at, dc.user2_read_at
		ORDER BY last_message_at DESC
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
		if err := rows.Scan(&c.ID, &c.CreatedAt, &c.LastMessageAt, &c.OtherUser.ID, &c.OtherUser.DisplayName, &c.OtherUser.Bio, &c.OtherUser.AvatarURL, &c.UnreadCount); err != nil {
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

	var areFriends bool
	h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM friendships
			WHERE status = 'accepted'
			  AND ((requester_id = $1 AND addressee_id = $2)
			    OR (requester_id = $2 AND addressee_id = $1))
		)`, userID, otherID).Scan(&areFriends)
	if !areFriends {
		writeErr(w, http.StatusForbidden, "you can only DM friends")
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
		SELECT dm.id, dm.conversation_id, dm.content, dm.edited_at, dm.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url,
		       COALESCE(
		           json_agg(
		               json_build_object('emoji', rxn.emoji, 'count', rxn.cnt, 'mine', rxn.mine)
		               ORDER BY rxn.emoji
		           ) FILTER (WHERE rxn.emoji IS NOT NULL),
		           '[]'
		       ) AS reactions
		FROM direct_messages dm
		JOIN users u ON u.id = dm.sender_id
		LEFT JOIN (
		    SELECT message_id, emoji,
		           COUNT(*) AS cnt,
		           BOOL_OR(user_id = $2) AS mine
		    FROM dm_message_reactions
		    GROUP BY message_id, emoji
		) rxn ON rxn.message_id = dm.id
		WHERE dm.conversation_id = $1
		GROUP BY dm.id, u.id
		ORDER BY dm.created_at DESC LIMIT 50
	`, convID, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	messages := make([]models.DirectMessage, 0)
	for rows.Next() {
		var m models.DirectMessage
		m.Sender = &models.User{}
		var reactionsJSON []byte
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Content, &m.EditedAt, &m.CreatedAt,
			&m.Sender.ID, &m.Sender.DisplayName, &m.Sender.Bio, &m.Sender.AvatarURL,
			&reactionsJSON); err != nil {
			continue
		}
		json.Unmarshal(reactionsJSON, &m.Reactions)
		if m.Reactions == nil {
			m.Reactions = []models.Reaction{}
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

	var content string
	h.db.QueryRow(r.Context(), `SELECT content FROM direct_messages WHERE id=$1 AND sender_id=$2`, messageID, userID).Scan(&content)

	tag, err := h.db.Exec(r.Context(), `DELETE FROM direct_messages WHERE id=$1 AND sender_id=$2`, messageID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusForbidden, "not your message")
		return
	}

	DeleteUploadedFile(h.uploadDir, h.baseURL, content)

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

	var recipientID string
	h.db.QueryRow(r.Context(), `SELECT CASE WHEN user1_id=$1 THEN user2_id ELSE user1_id END FROM dm_conversations WHERE id=$2`, userID, convID).Scan(&recipientID)
	if recipientID != "" {
		h.hub.Broadcast("user:"+recipientID, ws.Event{Type: "dm.new", Payload: payload})
	}

	if h.push.enabled() {
		if recipientID == "" {
			h.db.QueryRow(r.Context(), `SELECT CASE WHEN user1_id=$1 THEN user2_id ELSE user1_id END FROM dm_conversations WHERE id=$2`, userID, convID).Scan(&recipientID)
		}
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

func (h *DMsHandler) EditMessage(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "convID")
	messageID := chi.URLParam(r, "messageID")
	userID := middleware.UserID(r)

	var body struct {
		Content string `json:"content"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Content == "" {
		writeErr(w, http.StatusBadRequest, "content required")
		return
	}
	if len(body.Content) > 4000 {
		writeErr(w, http.StatusBadRequest, "message too long")
		return
	}

	var m models.DirectMessage
	m.Sender = &models.User{}
	err := h.db.QueryRow(r.Context(), `
		WITH upd AS (
			UPDATE direct_messages SET content = $1, edited_at = NOW()
			WHERE id = $2 AND sender_id = $3 AND conversation_id = $4
			RETURNING id, conversation_id, content, edited_at, created_at, sender_id
		)
		SELECT upd.id, upd.conversation_id, upd.content, upd.edited_at, upd.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url
		FROM upd JOIN users u ON u.id = upd.sender_id
	`, body.Content, messageID, userID, convID).Scan(
		&m.ID, &m.ConversationID, &m.Content, &m.EditedAt, &m.CreatedAt,
		&m.Sender.ID, &m.Sender.DisplayName, &m.Sender.Bio, &m.Sender.AvatarURL,
	)
	if err != nil {
		writeErr(w, http.StatusForbidden, "not your message")
		return
	}
	m.Reactions = []models.Reaction{}

	payload, _ := json.Marshal(m)
	h.hub.Broadcast("dm:"+convID, ws.Event{Type: "dm.edit", Payload: payload})
	writeJSON(w, http.StatusOK, m)
}

func (h *DMsHandler) AddReaction(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "convID")
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")
	userID := middleware.UserID(r)

	if len([]rune(emoji)) > 16 || emoji == "" {
		writeErr(w, http.StatusBadRequest, "invalid emoji")
		return
	}

	var isParticipant bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM dm_conversations WHERE id=$1 AND (user1_id=$2 OR user2_id=$2))`, convID, userID).Scan(&isParticipant)
	if !isParticipant {
		writeErr(w, http.StatusForbidden, "not a participant")
		return
	}

	h.db.Exec(r.Context(), `
		INSERT INTO dm_message_reactions (message_id, user_id, emoji) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, messageID, userID, emoji)

	h.broadcastDMReaction(convID, messageID, emoji, userID, "add")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *DMsHandler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	convID := chi.URLParam(r, "convID")
	messageID := chi.URLParam(r, "messageID")
	emoji := chi.URLParam(r, "emoji")
	userID := middleware.UserID(r)

	h.db.Exec(r.Context(), `
		DELETE FROM dm_message_reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3
	`, messageID, userID, emoji)

	h.broadcastDMReaction(convID, messageID, emoji, userID, "remove")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *DMsHandler) broadcastDMReaction(convID, messageID, emoji, userID, action string) {
	payload, _ := json.Marshal(map[string]string{
		"message_id":      messageID,
		"conversation_id": convID,
		"emoji":           emoji,
		"user_id":         userID,
		"action":          action,
	})
	h.hub.Broadcast("dm:"+convID, ws.Event{Type: "dm.reaction.toggle", Payload: payload})
}

func (h *DMsHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	convID := chi.URLParam(r, "convID")
	h.db.Exec(r.Context(), `
		UPDATE dm_conversations
		SET user1_read_at = CASE WHEN user1_id = $1 THEN NOW() ELSE user1_read_at END,
		    user2_read_at = CASE WHEN user2_id = $1 THEN NOW() ELSE user2_read_at END
		WHERE id = $2 AND (user1_id = $1 OR user2_id = $1)
	`, userID, convID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
