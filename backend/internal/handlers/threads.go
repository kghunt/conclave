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

type ThreadsHandler struct {
	db  *pgxpool.Pool
	hub *ws.Hub
}

func NewThreads(db *pgxpool.Pool, hub *ws.Hub) *ThreadsHandler {
	return &ThreadsHandler{db: db, hub: hub}
}

func (h *ThreadsHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	channelID := chi.URLParam(r, "channelID")
	userID := middleware.UserID(r)

	var hasAccess bool
	h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM server_members sm
			JOIN channels c ON c.server_id = sm.server_id
			WHERE sm.server_id = $1 AND sm.user_id = $2 AND c.id = $3
		)`, serverID, userID, channelID).Scan(&hasAccess)
	if !hasAccess {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT t.id, t.channel_id, t.title,
		       u.id, u.display_name, u.avatar_url,
		       t.created_at, t.last_message_at, t.message_count
		FROM threads t
		JOIN users u ON u.id = t.created_by
		WHERE t.channel_id = $1
		ORDER BY t.last_message_at DESC
	`, channelID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	threads := make([]models.Thread, 0)
	for rows.Next() {
		var t models.Thread
		t.CreatedBy = &models.User{}
		if err := rows.Scan(&t.ID, &t.ChannelID, &t.Title,
			&t.CreatedBy.ID, &t.CreatedBy.DisplayName, &t.CreatedBy.AvatarURL,
			&t.CreatedAt, &t.LastMessageAt, &t.MessageCount); err != nil {
			continue
		}
		threads = append(threads, t)
	}
	writeJSON(w, http.StatusOK, threads)
}

func (h *ThreadsHandler) Create(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	channelID := chi.URLParam(r, "channelID")
	userID := middleware.UserID(r)

	var hasAccess bool
	h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM server_members sm
			JOIN channels c ON c.server_id = sm.server_id
			WHERE sm.server_id = $1 AND sm.user_id = $2 AND c.id = $3
		)`, serverID, userID, channelID).Scan(&hasAccess)
	if !hasAccess {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	var body struct {
		Title          string `json:"title"`
		InitialMessage string `json:"initial_message"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Title == "" {
		writeErr(w, http.StatusBadRequest, "title required")
		return
	}
	if len(body.Title) > 200 {
		writeErr(w, http.StatusBadRequest, "title too long (max 200 characters)")
		return
	}
	if len(body.InitialMessage) > 4000 {
		writeErr(w, http.StatusBadRequest, "message too long (max 4000 characters)")
		return
	}

	var t models.Thread
	t.CreatedBy = &models.User{}
	err := h.db.QueryRow(r.Context(), `
		WITH ins AS (
			INSERT INTO threads (channel_id, title, created_by)
			VALUES ($1, $2, $3)
			RETURNING id, channel_id, title, created_by, created_at, last_message_at, message_count
		)
		SELECT ins.id, ins.channel_id, ins.title,
		       u.id, u.display_name, u.avatar_url,
		       ins.created_at, ins.last_message_at, ins.message_count
		FROM ins JOIN users u ON u.id = ins.created_by
	`, channelID, body.Title, userID).Scan(
		&t.ID, &t.ChannelID, &t.Title,
		&t.CreatedBy.ID, &t.CreatedBy.DisplayName, &t.CreatedBy.AvatarURL,
		&t.CreatedAt, &t.LastMessageAt, &t.MessageCount,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}

	if body.InitialMessage != "" {
		h.db.Exec(r.Context(), `INSERT INTO thread_messages (thread_id, author_id, content) VALUES ($1, $2, $3)`, t.ID, userID, body.InitialMessage)
		h.db.Exec(r.Context(), `UPDATE threads SET message_count = 1, last_message_at = NOW() WHERE id = $1`, t.ID)
		t.MessageCount = 1
	}

	payload, _ := json.Marshal(t)
	h.hub.Broadcast("channel:"+channelID, ws.Event{Type: "thread.new", Payload: payload})

	writeJSON(w, http.StatusCreated, t)
}

func (h *ThreadsHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "threadID")
	userID := middleware.UserID(r)

	var isMember bool
	h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM threads t
			JOIN channels c ON c.id = t.channel_id
			JOIN server_members sm ON sm.server_id = c.server_id
			WHERE t.id = $1 AND sm.user_id = $2
		)`, threadID, userID).Scan(&isMember)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT tm.id, tm.thread_id, tm.content, tm.created_at, tm.edited_at,
		       u.id, u.display_name, u.avatar_url
		FROM thread_messages tm
		JOIN users u ON u.id = tm.author_id
		WHERE tm.thread_id = $1
		ORDER BY tm.created_at ASC
	`, threadID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	msgs := make([]models.ThreadMessage, 0)
	for rows.Next() {
		var m models.ThreadMessage
		m.Author = &models.User{}
		if err := rows.Scan(&m.ID, &m.ThreadID, &m.Content, &m.CreatedAt, &m.EditedAt,
			&m.Author.ID, &m.Author.DisplayName, &m.Author.AvatarURL); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	writeJSON(w, http.StatusOK, msgs)
}

func (h *ThreadsHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "threadID")
	userID := middleware.UserID(r)

	var isMember bool
	var channelID string
	h.db.QueryRow(r.Context(), `
		SELECT sm.user_id IS NOT NULL, c.id
		FROM threads t
		JOIN channels c ON c.id = t.channel_id
		LEFT JOIN server_members sm ON sm.server_id = c.server_id AND sm.user_id = $2
		WHERE t.id = $1
	`, threadID, userID).Scan(&isMember, &channelID)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member")
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

	var m models.ThreadMessage
	m.Author = &models.User{}
	err := h.db.QueryRow(r.Context(), `
		WITH ins AS (
			INSERT INTO thread_messages (thread_id, author_id, content)
			VALUES ($1, $2, $3)
			RETURNING id, thread_id, content, created_at, edited_at, author_id
		)
		SELECT ins.id, ins.thread_id, ins.content, ins.created_at, ins.edited_at,
		       u.id, u.display_name, u.avatar_url
		FROM ins JOIN users u ON u.id = ins.author_id
	`, threadID, userID, body.Content).Scan(
		&m.ID, &m.ThreadID, &m.Content, &m.CreatedAt, &m.EditedAt,
		&m.Author.ID, &m.Author.DisplayName, &m.Author.AvatarURL,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "send failed")
		return
	}

	// Update thread stats
	h.db.Exec(r.Context(), `
		UPDATE threads SET last_message_at = NOW(), message_count = message_count + 1 WHERE id = $1
	`, threadID)

	// Broadcast to thread room
	msgPayload, _ := json.Marshal(m)
	h.hub.Broadcast("thread:"+threadID, ws.Event{Type: "thread.message.new", Payload: msgPayload})

	// Broadcast updated thread to channel room so bubbles update
	var updated models.Thread
	updated.CreatedBy = &models.User{}
	h.db.QueryRow(r.Context(), `
		SELECT t.id, t.channel_id, t.title,
		       u.id, u.display_name, u.avatar_url,
		       t.created_at, t.last_message_at, t.message_count
		FROM threads t JOIN users u ON u.id = t.created_by
		WHERE t.id = $1
	`, threadID).Scan(
		&updated.ID, &updated.ChannelID, &updated.Title,
		&updated.CreatedBy.ID, &updated.CreatedBy.DisplayName, &updated.CreatedBy.AvatarURL,
		&updated.CreatedAt, &updated.LastMessageAt, &updated.MessageCount,
	)
	if updated.ID != "" {
		threadPayload, _ := json.Marshal(updated)
		h.hub.Broadcast("channel:"+channelID, ws.Event{Type: "thread.updated", Payload: threadPayload})
	}

	writeJSON(w, http.StatusCreated, m)
}
