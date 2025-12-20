package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// UserIDFromContext extracts the user ID from the context
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDKey).(uuid.UUID)
	return userID, ok
}

// AuthMiddleware validates JWT tokens and adds user ID to context
func AuthMiddleware(jwt *JWTService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Parse Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				next.ServeHTTP(w, r)
				return
			}

			// Validate token
			userID, err := jwt.ValidateToken(parts[1])
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Add user ID to context
			ctx := context.WithValue(r.Context(), userIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
