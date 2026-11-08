package gateway_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/frogg24/file-share/internal/gateway"
)

func TestRequireAuthRejectsInvalidAuthorization(t *testing.T) {
	authenticator := gateway.NewAuthenticator(newTestVerifier(t))
	testCases := []struct {
		name   string
		header string
	}{
		{"wrong_scheme", "Basic abc"},
		{"missing_token", "Bearer"},
		{"extra_parts", "Bearer one two"},
		{"whitespace", "   "},
		{"invalid_token", "Bearer not-a-jwt"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			nextCalled := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusNoContent)
			})
			request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
			request.Header.Set("Authorization", tc.header)
			response := httptest.NewRecorder()

			authenticator.RequireAuth(next).ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", response.Code)
			}
			if nextCalled {
				t.Fatal("protected handler was called without valid authorization")
			}
			if response.Header().Get("WWW-Authenticate") != "Bearer" {
				t.Error("missing Bearer authentication challenge")
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatalf("expected JSON error: %v", err)
			}
			if body.Error == "" {
				t.Error("expected nonempty error message")
			}
		})
	}
}
