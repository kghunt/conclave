package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
	"github.com/karl/conclave/internal/models"
	"github.com/karl/conclave/internal/ws"
)

var mentionRe = regexp.MustCompile(`@(\w+)`)

type MessagesHandler struct {
	db   *pgxpool.Pool
	hub  *ws.Hub
	push *PushHandler
}

func NewMessages(db *pgxpool.Pool, hub *ws.Hub, push *PushHandler) *MessagesHandler {
	return &MessagesHandler{db: db, hub: hub, push: push}
}

func (h *MessagesHandler) List(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var isMember bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id=$1 AND user_id=$2)`, serverID, userID).Scan(&isMember)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	var beforeTime *time.Time
	if raw := r.URL.Query().Get("before"); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			t, err = time.Parse(time.RFC3339, raw)
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid 'before' parameter: expected RFC3339 timestamp")
			return
		}
		beforeTime = &t
	}

	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	if beforeTime != nil {
		rows, err = h.db.Query(r.Context(), `
			SELECT m.id, m.channel_id, m.content, m.edited_at, m.created_at,
			       u.id, u.display_name, u.bio, u.avatar_url
			FROM messages m JOIN users u ON u.id = m.author_id
			WHERE m.channel_id = $1 AND m.created_at < $2
			ORDER BY m.created_at DESC LIMIT 50
		`, channelID, *beforeTime)
	} else {
		rows, err = h.db.Query(r.Context(), `
			SELECT m.id, m.channel_id, m.content, m.edited_at, m.created_at,
			       u.id, u.display_name, u.bio, u.avatar_url
			FROM messages m JOIN users u ON u.id = m.author_id
			WHERE m.channel_id = $1
			ORDER BY m.created_at DESC LIMIT 50
		`, channelID)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	messages := make([]models.Message, 0)
	for rows.Next() {
		var m models.Message
		m.Author = &models.User{}
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.Content, &m.EditedAt, &m.CreatedAt,
			&m.Author.ID, &m.Author.DisplayName, &m.Author.Bio, &m.Author.AvatarURL); err != nil {
			continue
		}
		messages = append(messages, m)
	}
	// reverse to chronological order
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	writeJSON(w, http.StatusOK, messages)
}

func (h *MessagesHandler) Send(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var isMember bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id=$1 AND user_id=$2)`, serverID, userID).Scan(&isMember)
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

	var m models.Message
	m.Author = &models.User{}
	err := h.db.QueryRow(r.Context(), `
		WITH ins AS (
			INSERT INTO messages (channel_id, author_id, content) VALUES ($1, $2, $3)
			RETURNING id, channel_id, content, edited_at, created_at, author_id
		)
		SELECT ins.id, ins.channel_id, ins.content, ins.edited_at, ins.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url
		FROM ins JOIN users u ON u.id = ins.author_id
	`, channelID, userID, body.Content).Scan(
		&m.ID, &m.ChannelID, &m.Content, &m.EditedAt, &m.CreatedAt,
		&m.Author.ID, &m.Author.DisplayName, &m.Author.Bio, &m.Author.AvatarURL,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "send failed")
		return
	}

	payload, _ := json.Marshal(m)
	h.hub.Broadcast("channel:"+channelID, ws.Event{Type: "message.new", Payload: payload})
	go h.notifyMentions(serverID, m)

	if h.push.enabled() {
		var chName, srvName string
		h.db.QueryRow(r.Context(), `SELECT c.name, s.name FROM channels c JOIN servers s ON s.id = c.server_id WHERE c.id = $1`, channelID).Scan(&chName, &srvName)
		content := body.Content
		if len(content) > 100 {
			content = content[:97] + "…"
		}
		go h.push.SendToServerMembers(serverID, userID, PushPayload{
			Title: srvName + " #" + chName,
			Body:  m.Author.DisplayName + ": " + content,
			URL:   "/",
		})
	}

	writeJSON(w, http.StatusCreated, m)
}

func (h *MessagesHandler) Edit(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	channelID := chi.URLParam(r, "channelID")
	userID := middleware.UserID(r)

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

	var m models.Message
	m.Author = &models.User{}
	err := h.db.QueryRow(r.Context(), `
		WITH upd AS (
			UPDATE messages SET content = $1, edited_at = NOW()
			WHERE id = $2 AND author_id = $3
			RETURNING id, channel_id, content, edited_at, created_at, author_id
		)
		SELECT upd.id, upd.channel_id, upd.content, upd.edited_at, upd.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url
		FROM upd JOIN users u ON u.id = upd.author_id
	`, body.Content, messageID, userID).Scan(
		&m.ID, &m.ChannelID, &m.Content, &m.EditedAt, &m.CreatedAt,
		&m.Author.ID, &m.Author.DisplayName, &m.Author.Bio, &m.Author.AvatarURL,
	)
	if err != nil {
		writeErr(w, http.StatusForbidden, "not your message or not found")
		return
	}

	payload, _ := json.Marshal(m)
	h.hub.Broadcast("channel:"+channelID, ws.Event{Type: "message.edit", Payload: payload})
	writeJSON(w, http.StatusOK, m)
}

func (h *MessagesHandler) notifyMentions(serverID string, msg models.Message) {
	matches := mentionRe.FindAllStringSubmatch(msg.Content, -1)
	if len(matches) == 0 {
		return
	}

	seen := map[string]bool{}
	handles := make([]string, 0, len(matches))
	for _, m := range matches {
		lower := strings.ToLower(m[1])
		if !seen[lower] {
			seen[lower] = true
			handles = append(handles, lower)
		}
	}

	ctx := context.Background()
	rows, err := h.db.Query(ctx, `
		SELECT u.id
		FROM users u
		JOIN server_members sm ON sm.user_id = u.id
		WHERE sm.server_id = $1
		  AND u.id != $2
		  AND LOWER(REPLACE(u.display_name, ' ', '_')) = ANY($3)
	`, serverID, msg.Author.ID, handles)
	if err != nil {
		return
	}
	defer rows.Close()

	payload, _ := json.Marshal(msg)
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) != nil {
			continue
		}
		h.hub.Broadcast("user:"+uid, ws.Event{Type: "mention.new", Payload: payload})
		if h.push.enabled() {
			content := msg.Content
			if len(content) > 100 {
				content = content[:97] + "…"
			}
			h.push.SendToUser(uid, PushPayload{
				Title: msg.Author.DisplayName + " mentioned you",
				Body:  content,
				URL:   "/",
			})
		}
	}
}

func (h *MessagesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	userID := middleware.UserID(r)

	tag, err := h.db.Exec(r.Context(), `DELETE FROM messages WHERE id=$1 AND author_id=$2`, messageID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusForbidden, "not your message")
		return
	}

	channelID := chi.URLParam(r, "channelID")
	payload, _ := json.Marshal(map[string]string{"id": messageID, "channel_id": channelID})
	h.hub.Broadcast("channel:"+channelID, ws.Event{Type: "message.delete", Payload: payload})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
