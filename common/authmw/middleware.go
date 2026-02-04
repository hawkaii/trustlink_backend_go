package authmw

import (
	"context"
	"net/http"
	"strings"

	"github.com/trustlink/common/firebaseapp"
	"github.com/trustlink/common/httpx"
	"github.com/trustlink/common/log"
	"go.uber.org/zap"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	// UserIDKey is the context key for storing user ID
	UserIDKey contextKey = "userID"
)

// AuthMiddleware validates Firebase ID tokens and injects uid into context
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			log.Warn("Missing Authorization header", zap.String("path", r.URL.Path))
			httpx.Unauthorized(w, "Missing Authorization header")
			return
		}

		// Check for Bearer token
		if !strings.HasPrefix(authHeader, "Bearer ") {
			log.Warn("Invalid Authorization header format", zap.String("path", r.URL.Path))
			httpx.Unauthorized(w, "Invalid Authorization header format")
			return
		}

		// Extract token
		idToken := strings.TrimPrefix(authHeader, "Bearer ")
		if idToken == "" {
			log.Warn("Empty token", zap.String("path", r.URL.Path))
			httpx.Unauthorized(w, "Empty token")
			return
		}

		// Verify token with Firebase
		authClient := firebaseapp.GetAuthClient()
		token, err := authClient.VerifyIDToken(r.Context(), idToken)
		if err != nil {
			log.Warn("Invalid token", zap.Error(err), zap.String("path", r.URL.Path))
			httpx.Unauthorized(w, "Invalid or expired token")
			return
		}

		// Inject UID into context
		ctx := context.WithValue(r.Context(), UserIDKey, token.UID)

		log.Debug("Authenticated request",
			zap.String("uid", token.UID),
			zap.String("path", r.URL.Path))

		// Continue to next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUserID extracts the user ID from request context
func GetUserID(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(UserIDKey).(string)
	return uid, ok
}
