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
	db        *pgxpool.Pool
	hub       *ws.Hub
	push      *PushHandler
	uploadDir string
	baseURL   string
}

func NewMessages(db *pgxpool.Pool, hub *ws.Hub, push *PushHandler, uploadDir, baseURL string) *MessagesHandler {
	return &MessagesHandler{db: db, hub: hub, push: push, uploadDir: uploadDir, baseURL: baseURL}
}

func (h *MessagesHandler) List(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	serverID := chi.URLParam(r, "serverID")
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

	// $1=channelID, $2=serverID, $3=userID (for reaction `mine` flag), $4=beforeTime (optional)
	const listQuery = `
		SELECT m.id, m.channel_id, m.content, m.edited_at, m.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url,
		       COALESCE(top_role.color, '') as role_color,
		       r.id, r.content, ru.display_name,
		       COALESCE((
		           SELECT json_agg(
		               json_build_object('emoji', rxn.emoji, 'count', rxn.cnt, 'mine', rxn.mine)
		               ORDER BY rxn.first_used
		           )
		           FROM (
		               SELECT emoji,
		                      COUNT(*) AS cnt,
		                      BOOL_OR(user_id = $3::uuid) AS mine,
		                      MIN(created_at) AS first_used
		               FROM message_reactions
		               WHERE message_id = m.id
		               GROUP BY emoji
		           ) rxn
		       ), '[]'::json) AS reactions
		FROM messages m
		JOIN users u ON u.id = m.author_id
		LEFT JOIN LATERAL (
			SELECT sr.color FROM space_role_members srm
			JOIN space_roles sr ON sr.id = srm.role_id
			WHERE srm.server_id = $2 AND srm.user_id = u.id
			  AND sr.color != '' AND sr.is_everyone = FALSE
			ORDER BY sr.position DESC LIMIT 1
		) top_role ON true
		LEFT JOIN messages r ON r.id = m.reply_to_id
		LEFT JOIN users ru ON ru.id = r.author_id
		WHERE m.channel_id = $1`

	var rows interface{ Next() bool; Scan(...any) error; Close() }
	var err error
	if beforeTime != nil {
		rows, err = h.db.Query(r.Context(), listQuery+` AND m.created_at < $4 ORDER BY m.created_at DESC LIMIT 50`, channelID, serverID, userID, *beforeTime)
	} else {
		rows, err = h.db.Query(r.Context(), listQuery+` ORDER BY m.created_at DESC LIMIT 50`, channelID, serverID, userID)
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
		var replyID, replyContent, replyAuthor *string
		var reactionsRaw json.RawMessage
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.Content, &m.EditedAt, &m.CreatedAt,
			&m.Author.ID, &m.Author.DisplayName, &m.Author.Bio, &m.Author.AvatarURL,
			&m.Author.RoleColor,
			&replyID, &replyContent, &replyAuthor,
			&reactionsRaw); err != nil {
			continue
		}
		if replyID != nil {
			m.ReplyTo = &models.MessageReply{ID: *replyID, Content: *replyContent, AuthorName: *replyAuthor}
		}
		if err := json.Unmarshal(reactionsRaw, &m.Reactions); err != nil || m.Reactions == nil {
			m.Reactions = []models.Reaction{}
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

	var canSend bool
	h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM server_members sm
			JOIN channels c ON c.server_id = sm.server_id
			WHERE sm.server_id = $1 AND sm.user_id = $2 AND c.id = $3
			AND (
				sm.role IN ('owner', 'admin')
				OR NOT EXISTS (SELECT 1 FROM channel_role_permissions WHERE channel_id = c.id)
				OR EXISTS (
					SELECT 1 FROM channel_role_permissions crp
					JOIN space_roles sr ON sr.id = crp.role_id
					WHERE crp.channel_id = c.id AND crp.can_write = true
					AND (sr.is_everyone = true
						OR EXISTS (SELECT 1 FROM space_role_members WHERE server_id=$1 AND user_id=$2 AND role_id=sr.id))
				)
			)
		)`, serverID, userID, channelID).Scan(&canSend)
	if !canSend {
		writeErr(w, http.StatusForbidden, "no write access to this channel")
		return
	}

	var body struct {
		Content   string `json:"content"`
		ReplyToID string `json:"reply_to_id"`
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
	var replyToID *string
	if body.ReplyToID != "" {
		replyToID = &body.ReplyToID
	}
	err := h.db.QueryRow(r.Context(), `
		WITH ins AS (
			INSERT INTO messages (channel_id, author_id, content, reply_to_id)
			VALUES ($1, $2, $3, $4)
			RETURNING id, channel_id, content, reply_to_id, edited_at, created_at, author_id
		)
		SELECT ins.id, ins.channel_id, ins.content, ins.edited_at, ins.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url,
		       COALESCE((
		           SELECT sr.color FROM space_role_members srm
		           JOIN space_roles sr ON sr.id = srm.role_id
		           WHERE srm.server_id = $5 AND srm.user_id = u.id
		             AND sr.color != '' AND sr.is_everyone = FALSE
		           ORDER BY sr.position DESC LIMIT 1
		       ), '')
		FROM ins JOIN users u ON u.id = ins.author_id
	`, channelID, userID, body.Content, replyToID, serverID).Scan(
		&m.ID, &m.ChannelID, &m.Content, &m.EditedAt, &m.CreatedAt,
		&m.Author.ID, &m.Author.DisplayName, &m.Author.Bio, &m.Author.AvatarURL,
		&m.Author.RoleColor,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "send failed")
		return
	}

	m.Reactions = []models.Reaction{}

	// Populate reply_to info if applicable
	if replyToID != nil {
		var reply models.MessageReply
		h.db.QueryRow(r.Context(), `
			SELECT m.id, m.content, u.display_name
			FROM messages m JOIN users u ON u.id = m.author_id
			WHERE m.id = $1
		`, *replyToID).Scan(&reply.ID, &reply.Content, &reply.AuthorName)
		if reply.ID != "" {
			m.ReplyTo = &reply
		}
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
	serverID := chi.URLParam(r, "serverID")
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
			  AND channel_id IN (SELECT id FROM channels WHERE server_id = $4)
			RETURNING id, channel_id, content, edited_at, created_at, author_id
		)
		SELECT upd.id, upd.channel_id, upd.content, upd.edited_at, upd.created_at,
		       u.id, u.display_name, u.bio, u.avatar_url
		FROM upd JOIN users u ON u.id = upd.author_id
	`, body.Content, messageID, userID, serverID).Scan(
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
	channelID := chi.URLParam(r, "channelID")
	userID := middleware.UserID(r)

	// Read content first so we can clean up any uploaded media after deletion
	var content string
	h.db.QueryRow(r.Context(), `SELECT content FROM messages WHERE id=$1`, messageID).Scan(&content)

	// Try author delete first
	tag, err := h.db.Exec(r.Context(), `DELETE FROM messages WHERE id=$1 AND author_id=$2`, messageID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		// Allow server admin/owner to delete any message
		var role string
		h.db.QueryRow(r.Context(), `
			SELECT sm.role FROM messages m
			JOIN channels c ON c.id = m.channel_id
			JOIN server_members sm ON sm.server_id = c.server_id AND sm.user_id = $2
			WHERE m.id = $1
		`, messageID, userID).Scan(&role)
		if role != "owner" && role != "admin" {
			writeErr(w, http.StatusForbidden, "not your message")
			return
		}
		if _, err := h.db.Exec(r.Context(), `DELETE FROM messages WHERE id=$1`, messageID); err != nil {
			writeErr(w, http.StatusInternalServerError, "delete failed")
			return
		}
	}

	DeleteUploadedFile(h.uploadDir, h.baseURL, content)

	payload, _ := json.Marshal(map[string]string{"id": messageID, "channel_id": channelID})
	h.hub.Broadcast("channel:"+channelID, ws.Event{Type: "message.delete", Payload: payload})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *MessagesHandler) AddReaction(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	channelID := chi.URLParam(r, "channelID")
	serverID := chi.URLParam(r, "serverID")
	emoji := chi.URLParam(r, "emoji")
	userID := middleware.UserID(r)

	if len([]rune(emoji)) > 16 || emoji == "" {
		writeErr(w, http.StatusBadRequest, "invalid emoji")
		return
	}

	var ok bool
	h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM server_members sm
			JOIN messages msg ON msg.channel_id = $3 AND msg.id = $4
			WHERE sm.server_id = $1 AND sm.user_id = $2
		)`, serverID, userID, channelID, messageID).Scan(&ok)
	if !ok {
		writeErr(w, http.StatusForbidden, "not found")
		return
	}

	h.db.Exec(r.Context(), `
		INSERT INTO message_reactions (message_id, user_id, emoji)
		VALUES ($1, $2, $3) ON CONFLICT DO NOTHING
	`, messageID, userID, emoji)

	h.broadcastReaction(channelID, messageID, emoji, userID, "add")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *MessagesHandler) RemoveReaction(w http.ResponseWriter, r *http.Request) {
	messageID := chi.URLParam(r, "messageID")
	channelID := chi.URLParam(r, "channelID")
	emoji := chi.URLParam(r, "emoji")
	userID := middleware.UserID(r)

	h.db.Exec(r.Context(), `
		DELETE FROM message_reactions WHERE message_id = $1 AND user_id = $2 AND emoji = $3
	`, messageID, userID, emoji)

	h.broadcastReaction(channelID, messageID, emoji, userID, "remove")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *MessagesHandler) broadcastReaction(channelID, messageID, emoji, userID, action string) {
	payload, _ := json.Marshal(map[string]string{
		"message_id": messageID,
		"channel_id": channelID,
		"emoji":      emoji,
		"user_id":    userID,
		"action":     action,
	})
	h.hub.Broadcast("channel:"+channelID, ws.Event{Type: "reaction.toggle", Payload: payload})
}
