package handlers

import (
	"crypto/subtle"
	"net/http"
	"regexp"
	"strconv"
	"strings"

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
	}
	themeKeys := map[string]bool{
		"theme_accent": true, "theme_bg": true, "theme_sidebar": true,
		"theme_panel": true, "theme_input": true, "theme_border": true,
		"theme_text": true, "theme_text_muted": true,
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
