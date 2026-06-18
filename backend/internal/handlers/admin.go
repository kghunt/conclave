package handlers

import (
	"crypto/subtle"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
)

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

type AdminHandler struct {
	db                 *pgxpool.Pool
	instanceAdminEmail string
}

func NewAdmin(db *pgxpool.Pool, instanceAdminEmail string) *AdminHandler {
	return &AdminHandler{db: db, instanceAdminEmail: instanceAdminEmail}
}

func (h *AdminHandler) isAdmin(r *http.Request) bool {
	if h.instanceAdminEmail == "" {
		return false
	}
	userID := middleware.UserID(r)
	var email string
	h.db.QueryRow(r.Context(), `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
	return subtle.ConstantTimeCompare([]byte(email), []byte(h.instanceAdminEmail)) == 1
}

func (h *AdminHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}

	rows, err := h.db.Query(r.Context(), `SELECT key, value FROM settings ORDER BY key`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		settings[k] = v
	}
	writeJSON(w, http.StatusOK, settings)
}

func (h *AdminHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}

	var body map[string]string
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	intKeys := map[string]bool{
		"message_retention_days":        true,
		"inactive_space_retention_days": true,
		"max_video_size_mb":             true,
	}
	themeKeys := map[string]bool{
		"theme_accent": true, "theme_bg": true, "theme_sidebar": true,
		"theme_panel": true, "theme_input": true, "theme_border": true,
		"theme_text": true, "theme_text_muted": true,
	}
	boolKeys := map[string]bool{
		"allow_user_space_creation": true,
	}

	for k, v := range body {
		switch {
		case intKeys[k]:
			n, err := strconv.Atoi(v)
			if err != nil || n < 0 {
				writeErr(w, http.StatusBadRequest, k+" must be a non-negative integer")
				return
			}
		case themeKeys[k]:
			if v != "" && !hexColorRe.MatchString(v) {
				writeErr(w, http.StatusBadRequest, k+" must be a hex colour like #rrggbb")
				return
			}
		case boolKeys[k]:
			if v != "true" && v != "false" && v != "" {
				writeErr(w, http.StatusBadRequest, k+" must be true or false")
				return
			}
		default:
			continue
		}
		if v == "" {
			h.db.Exec(r.Context(), `DELETE FROM settings WHERE key = $1`, k)
		} else {
			h.db.Exec(r.Context(), `
				INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, NOW())
				ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
			`, k, v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AdminHandler) RunRetention(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}
	go runRetention(r.Context(), h.db)
	writeJSON(w, http.StatusOK, map[string]string{"status": "retention job started"})
}

func (h *AdminHandler) GetTheme(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(r.Context(), `SELECT key, value FROM settings WHERE key LIKE 'theme_%'`)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	defer rows.Close()
	theme := map[string]string{}
	for rows.Next() {
		var k, v string
		rows.Scan(&k, &v)
		theme[strings.TrimPrefix(k, "theme_")] = v
	}
	writeJSON(w, http.StatusOK, theme)
}

func (h *AdminHandler) IsInstanceAdmin(email string) bool {
	return h.instanceAdminEmail != "" && subtle.ConstantTimeCompare([]byte(email), []byte(h.instanceAdminEmail)) == 1
}

// BanInstanceUser bans a user from the entire instance.
func (h *AdminHandler) BanInstanceUser(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}
	targetID := chi.URLParam(r, "userID")
	// Prevent self-ban
	if targetID == middleware.UserID(r) {
		writeErr(w, http.StatusBadRequest, "cannot ban yourself")
		return
	}
	h.db.Exec(r.Context(), `UPDATE users SET instance_banned = true WHERE id = $1`, targetID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// UnbanInstanceUser lifts an instance-wide ban.
func (h *AdminHandler) UnbanInstanceUser(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}
	targetID := chi.URLParam(r, "userID")
	h.db.Exec(r.Context(), `UPDATE users SET instance_banned = false WHERE id = $1`, targetID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ListInstanceUsers returns all users with their ban status for the admin panel.
func (h *AdminHandler) ListInstanceUsers(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT id, display_name, email, avatar_url, instance_banned, created_at
		FROM users ORDER BY display_name
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type userRow struct {
		ID             string `json:"id"`
		DisplayName    string `json:"display_name"`
		Email          string `json:"email"`
		AvatarURL      string `json:"avatar_url"`
		InstanceBanned bool   `json:"instance_banned"`
		CreatedAt      string `json:"created_at"`
	}
	users := make([]userRow, 0)
	for rows.Next() {
		var u userRow
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.Email, &u.AvatarURL, &u.InstanceBanned, &u.CreatedAt); err != nil {
			continue
		}
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, users)
}

// GetConfig returns public instance-wide config flags (no auth required).
func (h *AdminHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	var allowVal string
	h.db.QueryRow(r.Context(), `SELECT value FROM settings WHERE key = 'allow_user_space_creation'`).Scan(&allowVal)
	allowCreate := allowVal != "false"
	videoLimit := videoSizeLimitMB(r.Context(), h.db)
	writeJSON(w, http.StatusOK, map[string]any{
		"allow_user_space_creation": allowCreate,
		"max_video_size_mb":         videoLimit,
	})
}
