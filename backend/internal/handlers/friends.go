package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
	"github.com/karl/conclave/internal/models"
	"github.com/karl/conclave/internal/ws"
)

type FriendsHandler struct {
	db  *pgxpool.Pool
	hub *ws.Hub
}

func NewFriends(db *pgxpool.Pool, hub *ws.Hub) *FriendsHandler {
	return &FriendsHandler{db: db, hub: hub}
}

type friendEntry struct {
	User  *models.User `json:"user"`
	Since time.Time    `json:"since"`
}

func (h *FriendsHandler) List(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	rows, err := h.db.Query(r.Context(), `
		SELECT u.id, u.display_name, u.bio, u.avatar_url, u.created_at, u.updated_at, f.created_at
		FROM friendships f
		JOIN users u ON u.id = CASE WHEN f.requester_id = $1 THEN f.addressee_id ELSE f.requester_id END
		WHERE (f.requester_id = $1 OR f.addressee_id = $1) AND f.status = 'accepted'
		ORDER BY u.display_name
	`, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	friends := make([]friendEntry, 0)
	for rows.Next() {
		var e friendEntry
		e.User = &models.User{}
		if err := rows.Scan(&e.User.ID, &e.User.DisplayName, &e.User.Bio, &e.User.AvatarURL,
			&e.User.CreatedAt, &e.User.UpdatedAt, &e.Since); err == nil {
			friends = append(friends, e)
		}
	}
	writeJSON(w, http.StatusOK, friends)
}

func (h *FriendsHandler) Requests(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	rows, err := h.db.Query(r.Context(), `
		SELECT u.id, u.display_name, u.bio, u.avatar_url, u.created_at, u.updated_at, f.created_at
		FROM friendships f
		JOIN users u ON u.id = f.requester_id
		WHERE f.addressee_id = $1 AND f.status = 'pending'
		ORDER BY f.created_at DESC
	`, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	requests := make([]friendEntry, 0)
	for rows.Next() {
		var e friendEntry
		e.User = &models.User{}
		if err := rows.Scan(&e.User.ID, &e.User.DisplayName, &e.User.Bio, &e.User.AvatarURL,
			&e.User.CreatedAt, &e.User.UpdatedAt, &e.Since); err == nil {
			requests = append(requests, e)
		}
	}
	writeJSON(w, http.StatusOK, requests)
}

func (h *FriendsHandler) SendRequest(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	targetID := chi.URLParam(r, "userID")

	if userID == targetID {
		writeErr(w, http.StatusBadRequest, "cannot add yourself")
		return
	}

	// If the target already sent us a request, auto-accept it
	var reverseID string
	h.db.QueryRow(r.Context(), `
		SELECT id FROM friendships WHERE requester_id = $1 AND addressee_id = $2 AND status = 'pending'
	`, targetID, userID).Scan(&reverseID)
	if reverseID != "" {
		h.db.Exec(r.Context(), `UPDATE friendships SET status = 'accepted' WHERE id = $1`, reverseID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
		return
	}

	// Check for existing relationship
	var existingStatus string
	h.db.QueryRow(r.Context(), `
		SELECT status FROM friendships
		WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)
	`, userID, targetID).Scan(&existingStatus)
	if existingStatus == "accepted" {
		writeErr(w, http.StatusConflict, "already friends")
		return
	}
	if existingStatus == "pending" {
		writeErr(w, http.StatusConflict, "request already sent")
		return
	}

	_, err := h.db.Exec(r.Context(), `
		INSERT INTO friendships (requester_id, addressee_id) VALUES ($1, $2)
	`, userID, targetID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to send request")
		return
	}

	// Notify the target in real-time that they have a new friend request
	var requester models.User
	h.db.QueryRow(r.Context(), `SELECT id, display_name, bio, avatar_url, created_at, updated_at FROM users WHERE id = $1`, userID).
		Scan(&requester.ID, &requester.DisplayName, &requester.Bio, &requester.AvatarURL, &requester.CreatedAt, &requester.UpdatedAt)
	if payload, err := json.Marshal(requester); err == nil {
		h.hub.Broadcast("user:"+targetID, ws.Event{Type: "friend.request", Payload: payload})
	}

	writeJSON(w, http.StatusCreated, map[string]string{"status": "pending"})
}

func (h *FriendsHandler) Sent(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	rows, err := h.db.Query(r.Context(), `
		SELECT u.id, u.display_name, u.bio, u.avatar_url, u.created_at, u.updated_at, f.created_at
		FROM friendships f
		JOIN users u ON u.id = f.addressee_id
		WHERE f.requester_id = $1 AND f.status = 'pending'
		ORDER BY f.created_at DESC
	`, userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()
	sent := make([]friendEntry, 0)
	for rows.Next() {
		var e friendEntry
		e.User = &models.User{}
		if err := rows.Scan(&e.User.ID, &e.User.DisplayName, &e.User.Bio, &e.User.AvatarURL,
			&e.User.CreatedAt, &e.User.UpdatedAt, &e.Since); err == nil {
			sent = append(sent, e)
		}
	}
	writeJSON(w, http.StatusOK, sent)
}

func (h *FriendsHandler) Accept(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	requesterID := chi.URLParam(r, "userID")

	tag, err := h.db.Exec(r.Context(), `
		UPDATE friendships SET status = 'accepted'
		WHERE requester_id = $1 AND addressee_id = $2 AND status = 'pending'
	`, requesterID, userID)
	if err != nil || tag.RowsAffected() == 0 {
		writeErr(w, http.StatusNotFound, "request not found")
		return
	}

	// Notify the requester in real-time that their request was accepted
	var accepter models.User
	h.db.QueryRow(r.Context(), `SELECT id, display_name, bio, avatar_url, created_at, updated_at FROM users WHERE id = $1`, userID).
		Scan(&accepter.ID, &accepter.DisplayName, &accepter.Bio, &accepter.AvatarURL, &accepter.CreatedAt, &accepter.UpdatedAt)
	if payload, err := json.Marshal(accepter); err == nil {
		h.hub.Broadcast("user:"+requesterID, ws.Event{Type: "friend.accepted", Payload: payload})
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *FriendsHandler) Remove(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	otherID := chi.URLParam(r, "userID")

	h.db.Exec(r.Context(), `
		DELETE FROM friendships
		WHERE (requester_id = $1 AND addressee_id = $2) OR (requester_id = $2 AND addressee_id = $1)
	`, userID, otherID)
	w.WriteHeader(http.StatusNoContent)
}

func (h *FriendsHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	q := r.URL.Query().Get("q")
	if len(q) < 2 {
		writeErr(w, http.StatusBadRequest, "query must be at least 2 characters")
		return
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT id, display_name, bio, avatar_url, created_at, updated_at
		FROM users
		WHERE LOWER(display_name) LIKE LOWER($1) AND id != $2
		LIMIT 20
	`, "%"+q+"%", userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "search failed")
		return
	}
	defer rows.Close()

	users := make([]models.User, 0)
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.Bio, &u.AvatarURL, &u.CreatedAt, &u.UpdatedAt); err == nil {
			users = append(users, u)
		}
	}
	writeJSON(w, http.StatusOK, users)
}
