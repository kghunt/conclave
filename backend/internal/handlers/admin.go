package handlers

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
)

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
	return email == h.instanceAdminEmail
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

	allowed := map[string]bool{
		"message_retention_days":        true,
		"inactive_space_retention_days": true,
	}

	for k, v := range body {
		if !allowed[k] {
			continue
		}
		// validate it's a non-negative integer
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeErr(w, http.StatusBadRequest, k+" must be a non-negative integer")
			return
		}
		h.db.Exec(r.Context(), `
			INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, NOW())
			ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()
		`, k, v)
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

// IsInstanceAdmin returns true if the given email matches the configured admin email.
func (h *AdminHandler) IsInstanceAdmin(email string) bool {
	return h.instanceAdminEmail != "" && email == h.instanceAdminEmail
}
