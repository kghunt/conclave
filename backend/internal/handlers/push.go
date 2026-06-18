package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/middleware"
)

type PushHandler struct {
	db              *pgxpool.Pool
	vapidPublicKey  string
	vapidPrivateKey string
	vapidEmail      string
}

func NewPush(db *pgxpool.Pool, publicKey, privateKey, email string) *PushHandler {
	return &PushHandler{db: db, vapidPublicKey: publicKey, vapidPrivateKey: privateKey, vapidEmail: email}
}

func (h *PushHandler) enabled() bool {
	return h.vapidPublicKey != "" && h.vapidPrivateKey != ""
}

// GetPublicKey returns the VAPID public key so the frontend can subscribe.
func (h *PushHandler) GetPublicKey(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"public_key": h.vapidPublicKey})
}

type pushSubReq struct {
	Endpoint string `json:"endpoint"`
	P256DH   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

func (h *PushHandler) Subscribe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	var body pushSubReq
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Endpoint == "" || body.P256DH == "" || body.Auth == "" {
		writeErr(w, http.StatusBadRequest, "missing fields")
		return
	}
	_, err := h.db.Exec(r.Context(), `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, endpoint) DO UPDATE SET p256dh = $3, auth = $4
	`, userID, body.Endpoint, body.P256DH, body.Auth)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "failed to save subscription")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PushHandler) Unsubscribe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.UserID(r)
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.db.Exec(r.Context(), `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, userID, body.Endpoint)
	w.WriteHeader(http.StatusNoContent)
}

// PushPayload is the JSON body sent to the browser push service.
type PushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

type pushSub struct {
	endpoint, p256dh, auth string
}

func (h *PushHandler) querySubscriptions(ctx context.Context, query string, args ...any) []pushSub {
	rows, err := h.db.Query(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var subs []pushSub
	for rows.Next() {
		var s pushSub
		if err := rows.Scan(&s.endpoint, &s.p256dh, &s.auth); err == nil {
			subs = append(subs, s)
		}
	}
	return subs
}

func (h *PushHandler) dispatch(subs []pushSub, payload PushPayload) {
	if !h.enabled() || len(subs) == 0 {
		return
	}
	data, _ := json.Marshal(payload)
	for _, s := range subs {
		s := s
		go func() {
			resp, err := webpush.SendNotification(data, &webpush.Subscription{
				Endpoint: s.endpoint,
				Keys:     webpush.Keys{Auth: s.auth, P256dh: s.p256dh},
			}, &webpush.Options{
				Subscriber:      "mailto:" + h.vapidEmail,
				VAPIDPublicKey:  h.vapidPublicKey,
				VAPIDPrivateKey: h.vapidPrivateKey,
				TTL:             30,
			})
			if err != nil {
				log.Printf("push: %v", err)
				return
			}
			resp.Body.Close()
		}()
	}
}

// SendToUser sends a push notification to all of a user's subscribed devices.
func (h *PushHandler) SendToUser(userID string, payload PushPayload) {
	if !h.enabled() {
		return
	}
	subs := h.querySubscriptions(context.Background(),
		`SELECT endpoint, p256dh, auth FROM push_subscriptions WHERE user_id = $1`, userID)
	h.dispatch(subs, payload)
}

// SendToServerMembers sends to all server members except the sender.
func (h *PushHandler) SendToServerMembers(serverID, excludeUserID string, payload PushPayload) {
	if !h.enabled() {
		return
	}
	subs := h.querySubscriptions(context.Background(), `
		SELECT ps.endpoint, ps.p256dh, ps.auth
		FROM push_subscriptions ps
		JOIN server_members sm ON sm.user_id = ps.user_id
		WHERE sm.server_id = $1 AND ps.user_id != $2
	`, serverID, excludeUserID)
	h.dispatch(subs, payload)
}
