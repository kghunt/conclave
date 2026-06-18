package handlers

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
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
	"github.com/karl/conclave/internal/ws"
)

type ServersHandler struct {
	db                 *pgxpool.Pool
	hub                *ws.Hub
	instanceAdminEmail string
}

func NewServers(db *pgxpool.Pool, hub *ws.Hub, instanceAdminEmail string) *ServersHandler {
	return &ServersHandler{db: db, hub: hub, instanceAdminEmail: instanceAdminEmail}
}

func (h *ServersHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	rows, err := h.db.Query(r.Context(), `
		SELECT s.id, s.name, s.description, s.icon_url, s.owner_id, s.is_public, s.invite_code,
		       s.member_invites_enabled, s.member_invite_expiry_days, s.created_at, sm.role
		FROM servers s
		JOIN server_members sm ON sm.server_id = s.id AND sm.user_id = $1
		ORDER BY s.name
	`, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	servers := make([]models.Server, 0)
	for rows.Next() {
		var s models.Server
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID, &s.IsPublic, &s.InviteCode,
			&s.MemberInvitesEnabled, &s.MemberInviteExpiryDays, &s.CreatedAt, &s.Role); err != nil {
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

	// Check if space creation is restricted to instance admin only
	var settingVal string
	h.db.QueryRow(r.Context(), `SELECT value FROM settings WHERE key = 'allow_user_space_creation'`).Scan(&settingVal)
	if settingVal == "false" {
		var email string
		h.db.QueryRow(r.Context(), `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
		if !h.IsInstanceAdmin(email) {
			writeErr(w, http.StatusForbidden, "space creation is disabled by the administrator")
			return
		}
	}

	inviteCode := randomCode(8)
	var s models.Server
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO servers (name, description, owner_id, is_public, invite_code)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, description, icon_url, owner_id, is_public, invite_code,
		          member_invites_enabled, member_invite_expiry_days, created_at
	`, body.Name, body.Description, userID, body.IsPublic, inviteCode).Scan(
		&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID, &s.IsPublic, &s.InviteCode,
		&s.MemberInvitesEnabled, &s.MemberInviteExpiryDays, &s.CreatedAt,
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
		Name                   string `json:"name"`
		Description            string `json:"description"`
		IsPublic               *bool  `json:"is_public"`
		MemberInvitesEnabled   *bool  `json:"member_invites_enabled"`
		MemberInviteExpiryDays *int   `json:"member_invite_expiry_days"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.MemberInviteExpiryDays != nil && *body.MemberInviteExpiryDays < 1 {
		writeErr(w, http.StatusBadRequest, "expiry must be at least 1 day")
		return
	}

	var s models.Server
	err := h.db.QueryRow(r.Context(), `
		UPDATE servers SET
			name                     = CASE WHEN $2 != '' THEN $2 ELSE name END,
			description              = CASE WHEN $3 != '' THEN $3 ELSE description END,
			is_public                = COALESCE($4, is_public),
			member_invites_enabled   = COALESCE($5, member_invites_enabled),
			member_invite_expiry_days = COALESCE($6, member_invite_expiry_days)
		WHERE id = $1
		RETURNING id, name, description, icon_url, owner_id, is_public, invite_code,
		          member_invites_enabled, member_invite_expiry_days, created_at
	`, serverID, body.Name, body.Description, body.IsPublic,
		body.MemberInvitesEnabled, body.MemberInviteExpiryDays).Scan(
		&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID, &s.IsPublic, &s.InviteCode,
		&s.MemberInvitesEnabled, &s.MemberInviteExpiryDays, &s.CreatedAt,
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
	if !allowedImageExt[ext] {
		writeErr(w, http.StatusBadRequest, "unsupported file type")
		return
	}
	if err := validateMIME(file, ext); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
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
	if _, err := io.Copy(out, file); err != nil {
		os.Remove(dest)
		writeErr(w, http.StatusInternalServerError, "save failed")
		return
	}

	iconURL := fmt.Sprintf("%s/avatars/%s", baseURL, filename)
	h.db.Exec(r.Context(), `UPDATE servers SET icon_url = $1 WHERE id = $2`, iconURL, serverID)

	writeJSON(w, http.StatusOK, map[string]string{"icon_url": iconURL})
}

func (h *ServersHandler) Get(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var s models.Server
	err := h.db.QueryRow(r.Context(), `
		SELECT s.id, s.name, s.description, s.icon_url, s.owner_id, s.is_public, s.invite_code,
		       s.member_invites_enabled, s.member_invite_expiry_days, s.created_at, COALESCE(sm.role, '')
		FROM servers s
		LEFT JOIN server_members sm ON sm.server_id = s.id AND sm.user_id = $2
		WHERE s.id = $1
	`, serverID, userID).Scan(
		&s.ID, &s.Name, &s.Description, &s.IconURL, &s.OwnerID, &s.IsPublic, &s.InviteCode,
		&s.MemberInvitesEnabled, &s.MemberInviteExpiryDays, &s.CreatedAt, &s.Role,
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
	go h.broadcastMemberJoin(serverID, userID)
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
	go h.broadcastMemberLeave(serverID, userID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *ServersHandler) JoinByInvite(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	userID := middleware.UserID(r)

	// Atomically increment use_count only when within the limit — prevents TOCTOU race.
	var serverID string
	err := h.db.QueryRow(r.Context(), `
		UPDATE invites SET use_count = use_count + 1
		WHERE code = $1
		  AND (expires_at IS NULL OR expires_at > NOW())
		  AND (max_uses IS NULL OR use_count < max_uses)
		RETURNING server_id
	`, code).Scan(&serverID)
	if err != nil {
		// Either not found, expired, or use limit reached — distinguish via a read.
		var exists bool
		h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM invites WHERE code = $1 AND (expires_at IS NULL OR expires_at > NOW()))`, code).Scan(&exists)
		if !exists {
			writeErr(w, http.StatusNotFound, "invite not found or expired")
		} else {
			writeErr(w, http.StatusGone, "invite has reached its use limit")
		}
		return
	}

	h.db.Exec(r.Context(), `INSERT INTO server_members (server_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, serverID, userID)
	go h.broadcastMemberJoin(serverID, userID)
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
		SELECT u.id, u.display_name, u.bio, u.avatar_url, u.created_at, u.updated_at, sm.role, sm.joined_at
		FROM server_members sm JOIN users u ON u.id = sm.user_id
		WHERE sm.server_id = $1
		ORDER BY sm.role, u.display_name
	`, serverID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	members := make([]models.ServerMember, 0)
	for rows.Next() {
		var m models.ServerMember
		m.User = &models.User{}
		if err := rows.Scan(&m.User.ID, &m.User.DisplayName, &m.User.Bio, &m.User.AvatarURL, &m.User.CreatedAt, &m.User.UpdatedAt, &m.Role, &m.JoinedAt); err != nil {
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
		// Instance admin can also delete any space
		var email string
		h.db.QueryRow(r.Context(), `SELECT email FROM users WHERE id = $1`, userID).Scan(&email)
		if !h.IsInstanceAdmin(email) {
			writeErr(w, http.StatusForbidden, "only the owner can delete this server")
			return
		}
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
	var memberInvitesEnabled bool
	var memberInviteExpiryDays int
	err := h.db.QueryRow(r.Context(), `
		SELECT sm.role, s.member_invites_enabled, s.member_invite_expiry_days
		FROM server_members sm JOIN servers s ON s.id = sm.server_id
		WHERE sm.server_id = $1 AND sm.user_id = $2
	`, serverID, userID).Scan(&role, &memberInvitesEnabled, &memberInviteExpiryDays)
	if err != nil {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	isAdmin := role == "owner" || role == "admin"
	if !isAdmin && !memberInvitesEnabled {
		writeErr(w, http.StatusForbidden, "members cannot create invites for this space")
		return
	}

	code := randomCode(10)
	var invite models.Invite
	// Admins/owners get permanent invites; members get a time-limited one.
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO invites (server_id, creator_id, code, expires_at)
		VALUES ($1, $2, $3, CASE WHEN $4 THEN NULL ELSE NOW() + make_interval(days => $5) END)
		RETURNING id, server_id, code, expires_at, max_uses, use_count, created_at
	`, serverID, userID, code, isAdmin, memberInviteExpiryDays).Scan(
		&invite.ID, &invite.ServerID, &invite.Code, &invite.ExpiresAt, &invite.MaxUses, &invite.UseCount, &invite.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "create invite failed")
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

type serverDiscovery struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IconURL     string `json:"icon_url"`
	MemberCount int    `json:"member_count"`
	IsMember    bool   `json:"is_member"`
}

func (h *ServersHandler) Presence(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	userID := middleware.UserID(r)

	var isMember bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM server_members WHERE server_id=$1 AND user_id=$2)`, serverID, userID).Scan(&isMember)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	rows, err := h.db.Query(r.Context(), `SELECT user_id FROM server_members WHERE server_id = $1`, serverID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	result := map[string]string{}
	for rows.Next() {
		var uid string
		rows.Scan(&uid)
		result[uid] = h.hub.GetStatus(uid)
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *ServersHandler) Discover(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	q := r.URL.Query().Get("q")

	rows, err := h.db.Query(r.Context(), `
		SELECT s.id, s.name, s.description, s.icon_url,
		       COUNT(sm.user_id) AS member_count,
		       EXISTS(SELECT 1 FROM server_members WHERE server_id = s.id AND user_id = $1) AS is_member
		FROM servers s
		LEFT JOIN server_members sm ON sm.server_id = s.id
		WHERE s.is_public = true
		  AND ($2 = '' OR s.name ILIKE '%' || $2 || '%' OR s.description ILIKE '%' || $2 || '%')
		GROUP BY s.id
		ORDER BY COUNT(sm.user_id) DESC, s.created_at DESC
		LIMIT 50
	`, userID, q)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	results := make([]serverDiscovery, 0)
	for rows.Next() {
		var d serverDiscovery
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.IconURL, &d.MemberCount, &d.IsMember); err != nil {
			continue
		}
		results = append(results, d)
	}
	writeJSON(w, http.StatusOK, results)
}

func (h *ServersHandler) IsInstanceAdmin(email string) bool {
	return h.instanceAdminEmail != "" && subtle.ConstantTimeCompare([]byte(email), []byte(h.instanceAdminEmail)) == 1
}

func randomCode(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:n]
}

func (h *ServersHandler) broadcastMemberJoin(serverID, userID string) {
	payload, _ := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
	h.hub.Broadcast("server:"+serverID, ws.Event{Type: "member.join", Payload: payload})
}

func (h *ServersHandler) broadcastMemberLeave(serverID, userID string) {
	payload, _ := json.Marshal(map[string]string{"server_id": serverID, "user_id": userID})
	h.hub.Broadcast("server:"+serverID, ws.Event{Type: "member.leave", Payload: payload})
}
