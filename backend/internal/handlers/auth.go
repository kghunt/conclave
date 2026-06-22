package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/auth"
	"github.com/karl/conclave/internal/models"
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,32}$`)

// ipLimiter is a simple sliding-window rate limiter keyed by string (IP or IP+action).
type ipLimiter struct {
	mu      sync.Mutex
	windows map[string][]time.Time
}

func newIPLimiter() *ipLimiter {
	return &ipLimiter{windows: make(map[string][]time.Time)}
}

func (l *ipLimiter) allow(key string, max int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-window)
	ts := l.windows[key]
	j := 0
	for _, t := range ts {
		if t.After(cutoff) {
			ts[j] = t
			j++
		}
	}
	ts = ts[:j]
	if len(ts) >= max {
		l.windows[key] = ts
		return false
	}
	l.windows[key] = append(ts, now)
	return true
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.Index(fwd, ","); i != -1 {
			return strings.TrimSpace(fwd[:i])
		}
		return strings.TrimSpace(fwd)
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

type AuthHandler struct {
	auth          *auth.Service
	db            *pgxpool.Pool
	base          string
	frontendURL   string
	secureCookies bool
	limiter       *ipLimiter
}

func NewAuth(a *auth.Service, db *pgxpool.Pool, baseURL, frontendURL string) *AuthHandler {
	return &AuthHandler{
		auth:          a,
		db:            db,
		base:          baseURL,
		frontendURL:   frontendURL,
		secureCookies: strings.HasPrefix(baseURL, "https://"),
		limiter:       newIPLimiter(),
	}
}

func (h *AuthHandler) getSetting(ctx context.Context, key string) string {
	var val string
	h.db.QueryRow(ctx, `SELECT value FROM settings WHERE key = $1`, key).Scan(&val)
	return val
}

func (h *AuthHandler) setTokenCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if h.getSetting(r.Context(), "google_auth_enabled") == "false" {
		http.Redirect(w, r, h.frontendURL+"/login?error=google_disabled", http.StatusTemporaryRedirect)
		return
	}
	if !h.auth.GoogleEnabled() {
		http.Redirect(w, r, h.frontendURL+"/login?error=google_not_configured", http.StatusTemporaryRedirect)
		return
	}
	state := randomState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		MaxAge:   300,
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, h.auth.AuthURL(state), http.StatusTemporaryRedirect)
}

func (h *AuthHandler) Callback(w http.ResponseWriter, r *http.Request) {
	if h.getSetting(r.Context(), "google_auth_enabled") == "false" {
		writeErr(w, http.StatusForbidden, "Google auth is disabled")
		return
	}
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		writeErr(w, http.StatusBadRequest, "invalid state")
		return
	}

	gu, err := h.auth.ExchangeCode(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "oauth exchange failed")
		return
	}

	user, err := h.upsertUser(r.Context(), gu)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "user upsert failed")
		return
	}

	token, err := h.auth.IssueToken(user.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token issue failed")
		return
	}

	h.setTokenCookie(w, token)
	http.Redirect(w, r, h.frontendURL, http.StatusTemporaryRedirect)
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.getSetting(ctx, "local_auth_enabled") == "false" {
		writeErr(w, http.StatusForbidden, "Local auth is disabled on this instance")
		return
	}
	regMode := h.getSetting(ctx, "registration_mode")
	if regMode == "" {
		regMode = "invite"
	}
	if regMode == "closed" {
		writeErr(w, http.StatusForbidden, "Registration is closed on this instance")
		return
	}

	// 5 registrations per IP per hour
	if !h.limiter.allow(clientIP(r)+":reg", 5, time.Hour) {
		writeErr(w, http.StatusTooManyRequests, "Too many registration attempts — try again later")
		return
	}

	var body struct {
		Username   string `json:"username"`
		Password   string `json:"password"`
		InviteCode string `json:"invite_code"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if !usernameRe.MatchString(body.Username) {
		writeErr(w, http.StatusBadRequest, "Username must be 2–32 characters: letters, numbers, _ and - only")
		return
	}
	if len(body.Password) < 8 {
		writeErr(w, http.StatusBadRequest, "Password must be at least 8 characters")
		return
	}

	var inviteID string
	if regMode == "invite" {
		if body.InviteCode == "" {
			writeErr(w, http.StatusBadRequest, "An invite code is required to register")
			return
		}
		err := h.db.QueryRow(ctx, `
			SELECT id FROM registration_invites
			WHERE code = $1
			  AND (expires_at IS NULL OR expires_at > NOW())
			  AND (max_uses IS NULL OR use_count < max_uses)
		`, body.InviteCode).Scan(&inviteID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "Invalid or expired invite code")
			return
		}
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Failed to process password")
		return
	}

	var u models.User
	err = h.db.QueryRow(ctx, `
		INSERT INTO users (username, password_hash, display_name, email)
		VALUES ($1, $2, $1, $1 || '@conclave.local')
		RETURNING id, email, display_name, bio, avatar_url, created_at, updated_at
	`, body.Username, hash).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "unique") || strings.Contains(err.Error(), "duplicate") {
			writeErr(w, http.StatusConflict, "Username is already taken")
			return
		}
		writeErr(w, http.StatusInternalServerError, "Failed to create account")
		return
	}

	if inviteID != "" {
		h.db.Exec(ctx, `UPDATE registration_invites SET use_count = use_count + 1 WHERE id = $1`, inviteID)
	}

	token, err := h.auth.IssueToken(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Failed to issue token")
		return
	}
	h.setTokenCookie(w, token)
	writeJSON(w, http.StatusCreated, &u)
}

func (h *AuthHandler) LocalLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.getSetting(ctx, "local_auth_enabled") == "false" {
		writeErr(w, http.StatusForbidden, "Local auth is disabled on this instance")
		return
	}

	// 10 attempts per IP per 15 minutes
	if !h.limiter.allow(clientIP(r)+":login", 10, 15*time.Minute) {
		writeErr(w, http.StatusTooManyRequests, "Too many login attempts — try again later")
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	var u models.User
	var hash string
	err := h.db.QueryRow(ctx, `
		SELECT id, email, display_name, bio, avatar_url, created_at, updated_at, password_hash
		FROM users
		WHERE username = $1 AND password_hash IS NOT NULL AND instance_banned = false
	`, body.Username).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt, &hash,
	)
	if err != nil {
		// Run bcrypt anyway to prevent timing-based user enumeration
		auth.CheckPassword("$2a$10$aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", body.Password)
		writeErr(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}
	if !auth.CheckPassword(hash, body.Password) {
		writeErr(w, http.StatusUnauthorized, "Invalid username or password")
		return
	}

	token, err := h.auth.IssueToken(u.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "Failed to issue token")
		return
	}
	h.setTokenCookie(w, token)
	writeJSON(w, http.StatusOK, &u)
}

func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		MaxAge:   -1,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *AuthHandler) upsertUser(ctx context.Context, gu *auth.GoogleUser) (*models.User, error) {
	var u models.User
	err := h.db.QueryRow(ctx, `
		INSERT INTO users (google_id, email, display_name, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (google_id) DO UPDATE
		  SET email = EXCLUDED.email,
		      display_name = CASE WHEN users.display_name = '' THEN EXCLUDED.display_name ELSE users.display_name END,
		      updated_at = NOW()
		RETURNING id, email, display_name, bio, avatar_url, created_at, updated_at
	`, gu.ID, gu.Email, gu.Name, gu.Picture).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt,
	)
	return &u, err
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
