package middleware

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/karl/conclave/internal/auth"
)

type contextKey string

const UserIDKey contextKey = "userID"

func Auth(a *auth.Service, db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, err := a.TokenFromRequest(r)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if db != nil {
				var banned bool
				db.QueryRow(r.Context(), `SELECT instance_banned FROM users WHERE id = $1`, claims.UserID).Scan(&banned)
				if banned {
					http.Error(w, "account banned", http.StatusForbidden)
					return
				}
			}
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserID(r *http.Request) string {
	v, _ := r.Context().Value(UserIDKey).(string)
	return v
}
