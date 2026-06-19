package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
	lkauth "github.com/livekit/protocol/auth"
)

type VoiceHandler struct {
	db            *pgxpool.Pool
	livekitURL    string
	livekitKey    string
	livekitSecret string
}

func NewVoice(db *pgxpool.Pool, url, key, secret string) *VoiceHandler {
	return &VoiceHandler{db: db, livekitURL: url, livekitKey: key, livekitSecret: secret}
}

func (h *VoiceHandler) Token(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	channelID := r.URL.Query().Get("channel")
	if channelID == "" {
		writeErr(w, http.StatusBadRequest, "channel required")
		return
	}

	// Verify the user is a member of the channel's server and the channel is a voice channel
	var ok bool
	h.db.QueryRow(r.Context(), `
		SELECT EXISTS(
			SELECT 1 FROM channels c
			JOIN server_members sm ON sm.server_id = c.server_id
			WHERE c.id = $1 AND sm.user_id = $2 AND c.type = 'voice'
		)
	`, channelID, userID).Scan(&ok)
	if !ok {
		writeErr(w, http.StatusForbidden, "not a member or not a voice channel")
		return
	}

	var displayName, avatarURL string
	h.db.QueryRow(r.Context(), `SELECT display_name, COALESCE(avatar_url, '') FROM users WHERE id = $1`, userID).
		Scan(&displayName, &avatarURL)

	meta, _ := json.Marshal(map[string]string{"avatar_url": avatarURL})

	at := lkauth.NewAccessToken(h.livekitKey, h.livekitSecret).
		AddGrant(&lkauth.VideoGrant{
			RoomJoin: true,
			Room:     "channel:" + channelID,
		}).
		SetIdentity(userID).
		SetName(displayName).
		SetMetadata(string(meta)).
		SetValidFor(4 * time.Hour)

	token, err := at.ToJWT()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token": token,
		"url":   h.livekitURL,
	})
}
