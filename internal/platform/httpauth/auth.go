package httpauth

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/frogg24/file-share/internal/platform/session"
	"github.com/frogg24/file-share/internal/platform/token"
)

type Authenticator struct {
	verifier *token.Verifier
}

func NewAuthenticator(verifier *token.Verifier) *Authenticator {
	return &Authenticator{verifier: verifier}
}

type userIDContextKey struct{}

// UserID returns the ID from a verified token, never from client headers.
func UserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey{}).(string)
	return userID, ok && userID != ""
}

func (a *Authenticator) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		var rawToken string
		headers := r.Header.Values("Authorization")
		if len(headers) > 0 {
			// An explicit Authorization header takes priority. Never fall back
			// to a cookie when the supplied header is malformed or invalid.
			if len(headers) != 1 {
				writeUnauthorized(w)
				return
			}
			parts := strings.Fields(headers[0])
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeUnauthorized(w)
				return
			}
			rawToken = parts[1]
		} else {
			count := 0
			for _, cookie := range r.Cookies() {
				if cookie.Name == session.CookieName {
					rawToken = cookie.Value
					count++
				}
			}
			if count != 1 || rawToken == "" {
				writeUnauthorized(w)
				return
			}
		}

		if a == nil || a.verifier == nil {
			writeUnauthorized(w)
			return
		}

		userID, err := a.verifier.Verify(rawToken)
		if err != nil {
			writeUnauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), userIDContextKey{}, userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)

	err := json.NewEncoder(w).Encode(map[string]string{
		"error": "требуется авторизация",
	})
	if err != nil {
		log.Printf("Failed to write auth error: %v", err)
	}
}
