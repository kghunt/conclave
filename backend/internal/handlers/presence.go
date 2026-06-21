package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
	"github.com/karl/conclave/internal/ws"
)

type PresenceHandler struct {
	db  *pgxpool.Pool
	hub *ws.Hub
}

func NewPresence(db *pgxpool.Pool, hub *ws.Hub) *PresenceHandler {
	return &PresenceHandler{db: db, hub: hub}
}

// GenerateToken creates (or replaces) a presence token for the authenticated user.
// The token is returned once and stored plaintext — it's a 32-byte random value
// with 256 bits of entropy, used only by the local desktop companion app.
func (h *PresenceHandler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)

	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	token := hex.EncodeToString(b)

	// Replace any existing token for this user.
	h.db.Exec(r.Context(), `DELETE FROM presence_tokens WHERE user_id = $1`, userID)
	if _, err := h.db.Exec(r.Context(),
		`INSERT INTO presence_tokens (user_id, token) VALUES ($1, $2)`,
		userID, token,
	); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to store token")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"token": token})
}

// RevokeToken deletes the presence token for the authenticated user.
func (h *PresenceHandler) RevokeToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	h.db.Exec(r.Context(), `DELETE FROM presence_tokens WHERE user_id = $1`, userID)
	// Clear any active game status.
	h.hub.SetGameStatus(userID, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// HasToken returns whether the user has a token and whether the app recently checked in.
// active = heartbeat received within the last 2 minutes.
func (h *PresenceHandler) HasToken(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	var hasToken bool
	var active bool
	h.db.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM presence_tokens WHERE user_id = $1),
		        EXISTS(SELECT 1 FROM presence_tokens WHERE user_id = $1 AND last_heartbeat_at > NOW() - INTERVAL '2 minutes')`,
		userID,
	).Scan(&hasToken, &active)
	writeJSON(w, http.StatusOK, map[string]any{"has_token": hasToken, "active": active})
}

// Heartbeat is called by the desktop app (Bearer token auth) to report the
// current game status. An empty "game" field clears the status.
func (h *PresenceHandler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	token := bearerToken(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "missing token")
		return
	}

	var userID string
	if err := h.db.QueryRow(r.Context(),
		`SELECT user_id FROM presence_tokens WHERE token = $1`, token,
	).Scan(&userID); err != nil {
		writeErr(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var body struct {
		Game string `json:"game"`
	}
	decodeJSON(r, &body)

	h.db.Exec(r.Context(), `UPDATE presence_tokens SET last_heartbeat_at = NOW() WHERE token = $1`, token)
	h.hub.SetGameStatus(userID, body.Game)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}
