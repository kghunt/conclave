package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
	"github.com/karl/conclave/internal/models"
)

type UsersHandler struct {
	db                 *pgxpool.Pool
	avatarDir          string
	baseURL            string
	instanceAdminEmail string
}

func NewUsers(db *pgxpool.Pool, avatarDir, baseURL, instanceAdminEmail string) *UsersHandler {
	os.MkdirAll(avatarDir, 0755)
	return &UsersHandler{db: db, avatarDir: avatarDir, baseURL: baseURL, instanceAdminEmail: instanceAdminEmail}
}

func (h *UsersHandler) Me(w http.ResponseWriter, r *http.Request) {
	u, err := h.fetchUser(r.Context(), middleware.UserID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	u.IsInstanceAdmin = h.instanceAdminEmail != "" && u.Email == h.instanceAdminEmail
	writeJSON(w, http.StatusOK, u)
}

func (h *UsersHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	u, err := h.fetchPublicUser(r.Context(), chi.URLParam(r, "userID"))
	if err != nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *UsersHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	var body struct {
		DisplayName  string `json:"display_name"`
		Bio          string `json:"bio"`
		CustomStatus string `json:"custom_status"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.DisplayName) > 32 {
		writeErr(w, http.StatusBadRequest, "display name too long (max 32 characters)")
		return
	}
	if len(body.Bio) > 500 {
		writeErr(w, http.StatusBadRequest, "bio too long (max 500 characters)")
		return
	}
	if len(body.CustomStatus) > 128 {
		writeErr(w, http.StatusBadRequest, "custom status too long (max 128 characters)")
		return
	}

	var u models.User
	err := h.db.QueryRow(r.Context(), `
		UPDATE users SET display_name = $1, bio = $2, custom_status = $3, updated_at = NOW()
		WHERE id = $4
		RETURNING id, email, display_name, bio, custom_status, avatar_url, created_at, updated_at
	`, body.DisplayName, body.Bio, body.CustomStatus, userID).Scan(
		&u.ID, &u.Email, &u.DisplayName, &u.Bio, &u.CustomStatus, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (h *UsersHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)

	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "file too large")
		return
	}

	file, header, err := r.FormFile("avatar")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedImageExt[ext] {
		writeErr(w, http.StatusBadRequest, "unsupported file type")
		return
	}
	if err := validateMIME(file, ext); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	filename := uuid.New().String() + ext
	dest := filepath.Join(h.avatarDir, filename)
	out, err := os.Create(dest)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "save failed")
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		os.Remove(dest)
		writeErr(w, http.StatusInternalServerError, "save failed")
		return
	}

	avatarURL := fmt.Sprintf("%s/avatars/%s", h.baseURL, filename)
	if _, err = h.db.Exec(r.Context(), `UPDATE users SET avatar_url = $1, updated_at = NOW() WHERE id = $2`, avatarURL, userID); err != nil {
		writeErr(w, http.StatusInternalServerError, "db update failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"avatar_url": avatarURL})
}

func (h *UsersHandler) fetchUser(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	err := h.db.QueryRow(ctx, `
		SELECT id, email, display_name, bio, custom_status, avatar_url, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Bio, &u.CustomStatus, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (h *UsersHandler) fetchPublicUser(ctx context.Context, id string) (*models.User, error) {
	var u models.User
	err := h.db.QueryRow(ctx, `
		SELECT id, display_name, bio, custom_status, avatar_url, created_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.DisplayName, &u.Bio, &u.CustomStatus, &u.AvatarURL, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
