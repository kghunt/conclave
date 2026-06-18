package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
	"github.com/karl/conclave/internal/models"
)

type ChannelsHandler struct {
	db *pgxpool.Pool
}

func NewChannels(db *pgxpool.Pool) *ChannelsHandler {
	return &ChannelsHandler{db: db}
}

func (h *ChannelsHandler) List(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var isMember bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id=$1 AND user_id=$2)`, serverID, userID).Scan(&isMember)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT c.id, c.server_id, c.name, c.description, c.position, c.created_at,
		  (SELECT COUNT(*) FROM messages m WHERE m.channel_id = c.id
		   AND m.created_at > COALESCE((SELECT last_read FROM read_cursors WHERE user_id=$2 AND channel_id=c.id), '1970-01-01'))
		FROM channels c
		WHERE c.server_id = $1
		ORDER BY c.position, c.name
	`, serverID, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	var channels []models.Channel
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(&c.ID, &c.ServerID, &c.Name, &c.Description, &c.Position, &c.CreatedAt, &c.UnreadCount); err != nil {
			continue
		}
		channels = append(channels, c)
	}
	writeJSON(w, http.StatusOK, channels)
}

func (h *ChannelsHandler) Create(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var role string
	h.db.QueryRow(r.Context(), `SELECT role FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, userID).Scan(&role)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}

	var c models.Channel
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO channels (server_id, name, description)
		VALUES ($1, $2, $3)
		RETURNING id, server_id, name, description, position, created_at
	`, serverID, body.Name, body.Description).Scan(
		&c.ID, &c.ServerID, &c.Name, &c.Description, &c.Position, &c.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (h *ChannelsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var role string
	h.db.QueryRow(r.Context(), `SELECT role FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, userID).Scan(&role)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	h.db.Exec(r.Context(), `DELETE FROM channels WHERE id=$1 AND server_id=$2`, channelID, serverID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ChannelsHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	channelID := chi.URLParam(r, "channelID")
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var isMember bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id=$1 AND user_id=$2)`, serverID, userID).Scan(&isMember)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	h.db.Exec(r.Context(), `
		INSERT INTO read_cursors (user_id, channel_id, last_read)
		VALUES ($1, $2, NOW())
		ON CONFLICT (user_id, channel_id) DO UPDATE SET last_read = NOW()
	`, userID, channelID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
