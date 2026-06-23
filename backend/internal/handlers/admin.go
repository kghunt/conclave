package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
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
		"google_auth_enabled":       true,
		"local_auth_enabled":        true,
	}
	enumKeys := map[string][]string{
		"registration_mode": {"open", "invite", "closed"},
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
		case enumKeys[k] != nil:
			valid := false
			for _, opt := range enumKeys[k] {
				if v == opt {
					valid = true
					break
				}
			}
			if !valid && v != "" {
				writeErr(w, http.StatusBadRequest, k+" must be one of: "+strings.Join(enumKeys[k], ", "))
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
		SELECT id, display_name, COALESCE(email, ''), COALESCE(avatar_url, ''), instance_banned, created_at::text
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
	ctx := r.Context()
	getSetting := func(key string) string {
		var val string
		h.db.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&val)
		return val
	}

	regMode := getSetting("registration_mode")
	if regMode == "" {
		regMode = "invite"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"allow_user_space_creation": getSetting("allow_user_space_creation") != "false",
		"max_video_size_mb":         videoSizeLimitMB(ctx, h.db),
		"google_auth_enabled":       getSetting("google_auth_enabled") != "false",
		"local_auth_enabled":        getSetting("local_auth_enabled") != "false",
		"registration_mode":         regMode,
		"desktop_download_url":      getSetting("desktop_download_url"),
	})
}

// ListRegistrationInvites returns all registration invite codes.
func (h *AdminHandler) ListRegistrationInvites(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT id, code, max_uses, use_count, expires_at::text, created_at::text
		FROM registration_invites
		ORDER BY created_at DESC
	`)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	type inviteRow struct {
		ID        string  `json:"id"`
		Code      string  `json:"code"`
		MaxUses   *int    `json:"max_uses"`
		UseCount  int     `json:"use_count"`
		ExpiresAt *string `json:"expires_at"`
		CreatedAt string  `json:"created_at"`
	}
	invites := make([]inviteRow, 0)
	for rows.Next() {
		var inv inviteRow
		if err := rows.Scan(&inv.ID, &inv.Code, &inv.MaxUses, &inv.UseCount, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
			continue
		}
		invites = append(invites, inv)
	}
	writeJSON(w, http.StatusOK, invites)
}

// CreateRegistrationInvite generates a new registration invite code.
func (h *AdminHandler) CreateRegistrationInvite(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}
	var body struct {
		MaxUses      *int `json:"max_uses"`
		ExpiresInDays *int `json:"expires_in_days"`
	}
	decodeJSON(r, &body)

	code := randomInviteCode()
	type inviteRow struct {
		ID        string  `json:"id"`
		Code      string  `json:"code"`
		MaxUses   *int    `json:"max_uses"`
		UseCount  int     `json:"use_count"`
		ExpiresAt *string `json:"expires_at"`
		CreatedAt string  `json:"created_at"`
	}
	var inv inviteRow
	var expiresAt *string
	if body.ExpiresInDays != nil && *body.ExpiresInDays > 0 {
		err := h.db.QueryRow(r.Context(), `
			INSERT INTO registration_invites (code, created_by, max_uses, expires_at)
			VALUES ($1, $2, $3, NOW() + make_interval(days => $4))
			RETURNING id, code, max_uses, use_count, expires_at::text, created_at::text
		`, code, middleware.UserID(r), body.MaxUses, *body.ExpiresInDays).Scan(
			&inv.ID, &inv.Code, &inv.MaxUses, &inv.UseCount, &expiresAt, &inv.CreatedAt,
		)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to create invite")
			return
		}
	} else {
		err := h.db.QueryRow(r.Context(), `
			INSERT INTO registration_invites (code, created_by, max_uses)
			VALUES ($1, $2, $3)
			RETURNING id, code, max_uses, use_count, expires_at::text, created_at::text
		`, code, middleware.UserID(r), body.MaxUses).Scan(
			&inv.ID, &inv.Code, &inv.MaxUses, &inv.UseCount, &expiresAt, &inv.CreatedAt,
		)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "failed to create invite")
			return
		}
	}
	inv.ExpiresAt = expiresAt
	writeJSON(w, http.StatusCreated, inv)
}

// DeleteRegistrationInvite removes a registration invite code.
func (h *AdminHandler) DeleteRegistrationInvite(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		writeErr(w, http.StatusForbidden, "instance admin only")
		return
	}
	id := chi.URLParam(r, "id")
	h.db.Exec(r.Context(), `DELETE FROM registration_invites WHERE id = $1`, id)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GenerateUserInvite lets any logged-in user create a single-use 24-hour
// registration invite code. Limited to one per user per 24 hours.
func (h *AdminHandler) GenerateUserInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := middleware.UserID(r)

	// Check registration mode — if closed or local auth disabled, no point generating a code.
	getSetting := func(key string) string {
		var val string
		h.db.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&val)
		return val
	}
	if getSetting("local_auth_enabled") == "false" {
		writeErr(w, http.StatusForbidden, "Local auth is disabled on this instance")
		return
	}
	regMode := getSetting("registration_mode")
	if regMode == "closed" {
		writeErr(w, http.StatusForbidden, "Registration is closed on this instance")
		return
	}

	// Rate limit: one invite per user per 24 hours.
	var recentCount int
	h.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM registration_invites
		WHERE created_by = $1 AND created_at > NOW() - INTERVAL '24 hours'
	`, userID).Scan(&recentCount)
	if recentCount > 0 {
		writeErr(w, http.StatusTooManyRequests, "You can only generate one invite code per day")
		return
	}

	code := randomInviteCode()
	type inviteRow struct {
		ID        string  `json:"id"`
		Code      string  `json:"code"`
		MaxUses   *int    `json:"max_uses"`
		UseCount  int     `json:"use_count"`
		ExpiresAt *string `json:"expires_at"`
		CreatedAt string  `json:"created_at"`
	}
	var inv inviteRow
	var expiresAt *string
	maxUses := 1
	if err := h.db.QueryRow(ctx, `
		INSERT INTO registration_invites (code, created_by, max_uses, expires_at)
		VALUES ($1, $2, 1, NOW() + INTERVAL '1 day')
		RETURNING id, code, max_uses, use_count, expires_at::text, created_at::text
	`, code, userID).Scan(&inv.ID, &inv.Code, &maxUses, &inv.UseCount, &expiresAt, &inv.CreatedAt); err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to create invite")
		return
	}
	inv.MaxUses = &maxUses
	inv.ExpiresAt = expiresAt
	writeJSON(w, http.StatusCreated, inv)
}

func randomInviteCode() string {
	b := make([]byte, 9)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
