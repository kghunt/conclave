package handlers

import (
	"crypto/rand"
	"encoding/base64"
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

type ServersHandler struct {
	db *pgxpool.Pool
}

func NewServers(db *pgxpool.Pool) *ServersHandler {
	return &ServersHandler{db: db}
}

func (h *ServersHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	rows, err := h.db.Query(r.Context(), `
		SELECT s.id, s.name, s.description, s.icon_url, s.owner_id, s.is_public, s.invite_code, s.created_at, sm.role
		FROM servers s
		JOIN server_members sm ON sm.server_id = s.id AND sm.user_id = $1
		ORDER BY s.name
	`, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	var servers []models.Server
	for rows.Next() {
		var s models.Server
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID, &s.IsPublic, &s.InviteCode, &s.CreatedAt, &s.Role); err != nil {
			continue
		}
		servers = append(servers, s)
	}
	writeJSON(w, http.StatusOK, servers)
}

func (h *ServersHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		IsPublic    bool   `json:"is_public"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}

	inviteCode := randomCode(8)
	var s models.Server
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO servers (name, description, owner_id, is_public, invite_code)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, icon_url, owner_id, is_public, invite_code, created_at
	`, body.Name, body.Description, userID, body.IsPublic, inviteCode).Scan(
		&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID, &s.IsPublic, &s.InviteCode, &s.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create failed")
		return
	}

	h.db.Exec(r.Context(), `INSERT INTO server_members (server_id, user_id, role) VALUES ($1, $2, 'owner')`, s.ID, userID)

	// create a default #general channel
	h.db.Exec(r.Context(), `INSERT INTO channels (server_id, name) VALUES ($1, 'general')`, s.ID)

	s.Role = "owner"
	writeJSON(w, http.StatusCreated, s)
}

func (h *ServersHandler) Update(w http.ResponseWriter, r *http.Request) {
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
		IsPublic    *bool  `json:"is_public"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	var s models.Server
	err := h.db.QueryRow(r.Context(), `
		UPDATE servers SET
			name        = CASE WHEN $2 != '' THEN $2 ELSE name END,
			description = CASE WHEN $3 != '' THEN $3 ELSE description END,
			is_public   = COALESCE($4, is_public)
		WHERE id = $1
		RETURNING id, name, description, icon_url, owner_id, is_public, invite_code, created_at
	`, serverID, body.Name, body.Description, body.IsPublic).Scan(
		&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID, &s.IsPublic, &s.InviteCode, &s.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	s.Role = role
	writeJSON(w, http.StatusOK, s)
}

func (h *ServersHandler) UploadIcon(w http.ResponseWriter, r *http.Request, avatarDir, baseURL string) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var role string
	h.db.QueryRow(r.Context(), `SELECT role FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, userID).Scan(&role)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "file too large")
		return
	}

	file, header, err := r.FormFile("icon")
	if err != nil {
		writeErr(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".gif" && ext != ".webp" {
		writeErr(w, http.StatusBadRequest, "unsupported file type")
		return
	}

	filename := uuid.New().String() + ext
	dest := filepath.Join(avatarDir, filename)
	out, err := os.Create(dest)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "save failed")
		return
	}
	defer out.Close()
	io.Copy(out, file)

	iconURL := fmt.Sprintf("%s/avatars/%s", baseURL, filename)
	h.db.Exec(r.Context(), `UPDATE servers SET icon_url = $1 WHERE id = $2`, iconURL, serverID)

	writeJSON(w, http.StatusOK, map[string]string{"icon_url": iconURL})
}

func (h *ServersHandler) Get(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var s models.Server
	err := h.db.QueryRow(r.Context(), `
		SELECT s.id, s.name, s.description, s.icon_url, s.owner_id, s.is_public, s.invite_code, s.created_at, COALESCE(sm.role, '')
		FROM servers s
		LEFT JOIN server_members sm ON sm.server_id = s.id AND sm.user_id = $2
		WHERE s.id = $1
	`, serverID, userID).Scan(
		&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID, &s.IsPublic, &s.InviteCode, &s.CreatedAt, &s.Role,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "server not found")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *ServersHandler) Join(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var isPublic bool
	err := h.db.QueryRow(r.Context(), `SELECT is_public FROM servers WHERE id = $1`, serverID).Scan(&isPublic)
	if err != nil {
		writeErr(w, http.StatusNotFound, "server not found")
		return
	}
	if !isPublic {
		writeErr(w, http.StatusForbidden, "server requires an invite")
		return
	}

	_, err = h.db.Exec(r.Context(), `
		INSERT INTO server_members (server_id, user_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`, serverID, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "join failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ServersHandler) Leave(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var ownerID string
	h.db.QueryRow(r.Context(), `SELECT owner_id FROM servers WHERE id = $1`, serverID).Scan(&ownerID)
	if ownerID == userID {
		writeErr(w, http.StatusBadRequest, "owner cannot leave; transfer or delete server first")
		return
	}

	h.db.Exec(r.Context(), `DELETE FROM server_members WHERE server_id = $1 AND user_id = $2`, serverID, userID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ServersHandler) JoinByInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userID := middleware.UserID(r)

	var serverID string
	var maxUses *int
	var useCount int
	err := h.db.QueryRow(r.Context(), `
		SELECT server_id, max_uses, use_count FROM invites
		WHERE code = $1 AND (expires_at IS NULL OR expires_at > NOW())
	`, code).Scan(&serverID, &maxUses, &useCount)
	if err != nil {
		writeErr(w, http.StatusNotFound, "invite not found or expired")
		return
	}
	if maxUses != nil && useCount >= *maxUses {
		writeErr(w, http.StatusGone, "invite has reached its use limit")
		return
	}

	h.db.Exec(r.Context(), `UPDATE invites SET use_count = use_count + 1 WHERE code = $1`, code)
	h.db.Exec(r.Context(), `INSERT INTO server_members (server_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, serverID, userID)

	writeJSON(w, http.StatusOK, map[string]string{"server_id": serverID})
}

func (h *ServersHandler) Members(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var isMember bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id=$1 AND user_id=$2)`, serverID, userID).Scan(&isMember)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT u.id, u.email, u.display_name, u.bio, u.avatar_url, u.created_at, u.updated_at, sm.role, sm.joined_at
		FROM server_members sm JOIN users u ON u.id = sm.user_id
		WHERE sm.server_id = $1
		ORDER BY sm.role, u.display_name
	`, serverID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	var members []models.ServerMember
	for rows.Next() {
		var m models.ServerMember
		m.User = &models.User{}
		if err := rows.Scan(&m.User.ID, &m.User.Email, &m.User.DisplayName, &m.User.Bio, &m.User.AvatarURL, &m.User.CreatedAt, &m.User.UpdatedAt, &m.Role, &m.JoinedAt); err != nil {
			continue
		}
		members = append(members, m)
	}
	writeJSON(w, http.StatusOK, members)
}

func (h *ServersHandler) Delete(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var ownerID string
	if err := h.db.QueryRow(r.Context(), `SELECT owner_id FROM servers WHERE id = $1`, serverID).Scan(&ownerID); err != nil {
		writeErr(w, http.StatusNotFound, "server not found")
		return
	}
	if ownerID != userID {
		writeErr(w, http.StatusForbidden, "only the owner can delete this server")
		return
	}

	// CASCADE in the schema handles all related data (members, channels, messages, invites)
	if _, err := h.db.Exec(r.Context(), `DELETE FROM servers WHERE id = $1`, serverID); err != nil {
		writeErr(w, http.StatusInternalServerError, "delete failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ServersHandler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	targetID := chi.URLParam(r, "userID")
	callerID := middleware.UserID(r)

	var callerRole string
	h.db.QueryRow(r.Context(), `SELECT role FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, callerID).Scan(&callerRole)
	if callerRole != "owner" {
		writeErr(w, http.StatusForbidden, "only the owner can manage roles")
		return
	}
	if targetID == callerID {
		writeErr(w, http.StatusBadRequest, "cannot change your own role")
		return
	}

	var targetRole string
	h.db.QueryRow(r.Context(), `SELECT role FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, targetID).Scan(&targetRole)
	if targetRole == "owner" {
		writeErr(w, http.StatusBadRequest, "cannot change the owner's role")
		return
	}

	var body struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Role != "admin" && body.Role != "member" {
		writeErr(w, http.StatusBadRequest, "role must be admin or member")
		return
	}

	_, err := h.db.Exec(r.Context(), `UPDATE server_members SET role=$1 WHERE server_id=$2 AND user_id=$3`, body.Role, serverID, targetID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"role": body.Role})
}

func (h *ServersHandler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var role string
	h.db.QueryRow(r.Context(), `SELECT role FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, userID).Scan(&role)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	code := randomCode(10)
	var invite models.Invite
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO invites (server_id, creator_id, code)
		VALUES ($1, $2, $3)
		RETURNING id, server_id, code, expires_at, max_uses, use_count, created_at
	`, serverID, userID, code).Scan(
		&invite.ID, &invite.ServerID, &invite.Code, &invite.ExpiresAt, &invite.MaxUses, &invite.UseCount, &invite.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create invite failed")
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func randomCode(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:n]
}
