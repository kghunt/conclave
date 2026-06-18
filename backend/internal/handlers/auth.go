package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/auth"
	"github.com/karl/conclave/internal/models"
)

type AuthHandler struct {
	auth          *auth.Service
	db            *pgxpool.Pool
	base          string
	frontendURL   string
	secureCookies bool
}

func NewAuth(a *auth.Service, db *pgxpool.Pool, baseURL, frontendURL string) *AuthHandler {
	return &AuthHandler{
		auth:          a,
		db:            db,
		base:          baseURL,
		frontendURL:   frontendURL,
		secureCookies: strings.HasPrefix(baseURL, "https://"),
	}
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
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

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		MaxAge:   int((30 * 24 * time.Hour).Seconds()),
		HttpOnly: true,
		Secure:   h.secureCookies,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	http.Redirect(w, r, h.frontendURL, http.StatusTemporaryRedirect)
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
