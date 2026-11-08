package gateway_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/frogg24/file-share/internal/gateway"
	"github.com/frogg24/file-share/internal/platform/token"
)

func TestRequireAuthRejectsMissingToken(t *testing.T) {
	authenticator := gateway.NewAuthenticator(nil)

	nextCalled := false

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	handler := authenticator.RequireAuth(next)

	request := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}

	if nextCalled {
		t.Fatal("protected handler was called without a token")
	}
}

func newTestVerifier(t *testing.T) *token.Verifier {
	t.Helper()

	publicKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	publicPath := filepath.Join(t.TempDir(), "public.key")

	err = os.WriteFile(publicPath, publicKey, 0600)
	if err != nil {
		t.Fatal(err)
	}

	verifier, err := token.NewVerifier(publicPath)
	if err != nil {
		t.Fatal(err)
	}

	return verifier
}
