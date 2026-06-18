package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
	"github.com/karl/conclave/internal/models"
	"github.com/karl/conclave/internal/ws"
)

type RolesHandler struct {
	db  *pgxpool.Pool
	hub *ws.Hub
}

func NewRoles(db *pgxpool.Pool, hub *ws.Hub) *RolesHandler {
	return &RolesHandler{db: db, hub: hub}
}

func (h *RolesHandler) callerRole(r *http.Request, serverID string) string {
	userID := middleware.UserID(r)
	var role string
	h.db.QueryRow(r.Context(), `SELECT role FROM server_members WHERE server_id=$1 AND user_id=$2`, serverID, userID).Scan(&role)
	return role
}

func (h *RolesHandler) ListRoles(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	if h.callerRole(r, serverID) == "" {
		writeErr(w, http.StatusForbidden, "not a member")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, server_id, name, color, is_everyone, position, created_at
		FROM space_roles WHERE server_id = $1
		ORDER BY is_everyone DESC, position, name
	`, serverID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	roles := make([]models.SpaceRole, 0)
	for rows.Next() {
		var sr models.SpaceRole
		if err := rows.Scan(&sr.ID, &sr.ServerID, &sr.Name, &sr.Color, &sr.IsEveryone, &sr.Position, &sr.CreatedAt); err != nil {
			continue
		}
		roles = append(roles, sr)
	}
	writeJSON(w, http.StatusOK, roles)
}

func (h *RolesHandler) CreateRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	role := h.callerRole(r, serverID)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	var body struct {
		Name  string `json:"name"`
		Color string `json:"color"`
	}
	if err := decodeJSON(r, &body); err != nil || body.Name == "" {
		writeErr(w, http.StatusBadRequest, "name required")
		return
	}
	if len(body.Name) > 50 {
		writeErr(w, http.StatusBadRequest, "name too long (max 50)")
		return
	}

	var maxPos int
	h.db.QueryRow(r.Context(), `SELECT COALESCE(MAX(position), 0) FROM space_roles WHERE server_id = $1`, serverID).Scan(&maxPos)

	var sr models.SpaceRole
	err := h.db.QueryRow(r.Context(), `
		INSERT INTO space_roles (server_id, name, color, position)
		VALUES ($1, $2, $3, $4)
		RETURNING id, server_id, name, color, is_everyone, position, created_at
	`, serverID, body.Name, body.Color, maxPos+1).Scan(
		&sr.ID, &sr.ServerID, &sr.Name, &sr.Color, &sr.IsEveryone, &sr.Position, &sr.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusConflict, "role name already exists")
		return
	}

	payload, _ := json.Marshal(sr)
	h.hub.Broadcast("server:"+serverID, ws.Event{Type: "role.new", Payload: payload})
	writeJSON(w, http.StatusCreated, sr)
}

func (h *RolesHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	roleID := chi.URLParam(r, "roleID")
	role := h.callerRole(r, serverID)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	var isEveryone bool
	h.db.QueryRow(r.Context(), `SELECT is_everyone FROM space_roles WHERE id=$1 AND server_id=$2`, roleID, serverID).Scan(&isEveryone)
	if isEveryone {
		writeErr(w, http.StatusBadRequest, "cannot edit the everyone role")
		return
	}

	var body struct {
		Name  *string `json:"name"`
		Color *string `json:"color"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name != nil && len(*body.Name) > 50 {
		writeErr(w, http.StatusBadRequest, "name too long (max 50)")
		return
	}

	var sr models.SpaceRole
	err := h.db.QueryRow(r.Context(), `
		UPDATE space_roles
		SET name  = COALESCE($3, name),
		    color = COALESCE($4, color)
		WHERE id = $1 AND server_id = $2
		RETURNING id, server_id, name, color, is_everyone, position, created_at
	`, roleID, serverID, body.Name, body.Color).Scan(
		&sr.ID, &sr.ServerID, &sr.Name, &sr.Color, &sr.IsEveryone, &sr.Position, &sr.CreatedAt,
	)
	if err != nil {
		writeErr(w, http.StatusNotFound, "role not found")
		return
	}

	payload, _ := json.Marshal(sr)
	h.hub.Broadcast("server:"+serverID, ws.Event{Type: "role.updated", Payload: payload})
	writeJSON(w, http.StatusOK, sr)
}

func (h *RolesHandler) DeleteRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	roleID := chi.URLParam(r, "roleID")
	role := h.callerRole(r, serverID)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	var isEveryone bool
	h.db.QueryRow(r.Context(), `SELECT is_everyone FROM space_roles WHERE id=$1 AND server_id=$2`, roleID, serverID).Scan(&isEveryone)
	if isEveryone {
		writeErr(w, http.StatusBadRequest, "cannot delete the everyone role")
		return
	}

	h.db.Exec(r.Context(), `DELETE FROM space_roles WHERE id=$1 AND server_id=$2`, roleID, serverID)

	payload, _ := json.Marshal(map[string]string{"id": roleID, "server_id": serverID})
	h.hub.Broadcast("server:"+serverID, ws.Event{Type: "role.deleted", Payload: payload})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *RolesHandler) AssignRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	targetUserID := chi.URLParam(r, "userID")
	roleID := chi.URLParam(r, "roleID")
	role := h.callerRole(r, serverID)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	var isEveryone bool
	err := h.db.QueryRow(r.Context(), `SELECT is_everyone FROM space_roles WHERE id=$1 AND server_id=$2`, roleID, serverID).Scan(&isEveryone)
	if err != nil {
		writeErr(w, http.StatusNotFound, "role not found")
		return
	}
	if isEveryone {
		writeErr(w, http.StatusBadRequest, "cannot assign the everyone role")
		return
	}

	h.db.Exec(r.Context(), `
		INSERT INTO space_role_members (server_id, user_id, role_id) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING
	`, serverID, targetUserID, roleID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *RolesHandler) RemoveRole(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	targetUserID := chi.URLParam(r, "userID")
	roleID := chi.URLParam(r, "roleID")
	role := h.callerRole(r, serverID)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	h.db.Exec(r.Context(), `DELETE FROM space_role_members WHERE server_id=$1 AND user_id=$2 AND role_id=$3`, serverID, targetUserID, roleID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *RolesHandler) ListChannelPerms(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	channelID := chi.URLParam(r, "channelID")
	role := h.callerRole(r, serverID)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	// Verify channel belongs to this server
	var chExists bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM channels WHERE id=$1 AND server_id=$2)`, channelID, serverID).Scan(&chExists)
	if !chExists {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}

	// Return all roles with their overrides (or default values if no override)
	rows, err := h.db.Query(r.Context(), `
		SELECT sr.id, sr.name, sr.color, sr.is_everyone,
		       COALESCE(crp.can_view, TRUE),
		       COALESCE(crp.can_write, TRUE),
		       crp.role_id IS NOT NULL as has_override
		FROM space_roles sr
		LEFT JOIN channel_role_permissions crp ON crp.role_id = sr.id AND crp.channel_id = $2
		WHERE sr.server_id = $1
		ORDER BY sr.is_everyone DESC, sr.position, sr.name
	`, serverID, channelID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type permRow struct {
		models.ChannelPermission
		IsEveryone  bool `json:"is_everyone"`
		HasOverride bool `json:"has_override"`
	}
	perms := make([]permRow, 0)
	for rows.Next() {
		var p permRow
		if err := rows.Scan(&p.RoleID, &p.RoleName, &p.Color, &p.IsEveryone, &p.CanView, &p.CanWrite, &p.HasOverride); err != nil {
			continue
		}
		perms = append(perms, p)
	}
	writeJSON(w, http.StatusOK, perms)
}

func (h *RolesHandler) SetChannelPerm(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	channelID := chi.URLParam(r, "channelID")
	roleID := chi.URLParam(r, "roleID")
	role := h.callerRole(r, serverID)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	var chExists, roleExists bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM channels WHERE id=$1 AND server_id=$2)`, channelID, serverID).Scan(&chExists)
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM space_roles WHERE id=$1 AND server_id=$2)`, roleID, serverID).Scan(&roleExists)
	if !chExists || !roleExists {
		writeErr(w, http.StatusNotFound, "channel or role not found")
		return
	}

	var body struct {
		CanView  bool `json:"can_view"`
		CanWrite bool `json:"can_write"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}

	_, err := h.db.Exec(r.Context(), `
		INSERT INTO channel_role_permissions (channel_id, role_id, can_view, can_write)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (channel_id, role_id) DO UPDATE
		  SET can_view = EXCLUDED.can_view,
		      can_write = EXCLUDED.can_write
	`, channelID, roleID, body.CanView, body.CanWrite)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "set permission failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *RolesHandler) DeleteChannelPerm(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "serverID")
	channelID := chi.URLParam(r, "channelID")
	roleID := chi.URLParam(r, "roleID")
	role := h.callerRole(r, serverID)
	if role != "owner" && role != "admin" {
		writeErr(w, http.StatusForbidden, "admin required")
		return
	}

	var chExists bool
	h.db.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM channels WHERE id=$1 AND server_id=$2)`, channelID, serverID).Scan(&chExists)
	if !chExists {
		writeErr(w, http.StatusNotFound, "channel not found")
		return
	}

	h.db.Exec(r.Context(), `DELETE FROM channel_role_permissions WHERE channel_id=$1 AND role_id=$2`, channelID, roleID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
