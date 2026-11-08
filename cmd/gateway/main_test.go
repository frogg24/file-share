package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/frogg24/file-share/internal/auth"
	"github.com/frogg24/file-share/internal/platform/token"
)

func newGatewayTestServer(t *testing.T) (*httptest.Server, *auth.TokenIssuer) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	publicPath := filepath.Join(dir, "public.key")
	privatePath := filepath.Join(dir, "private.key")
	if err := os.WriteFile(publicPath, publicKey, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privatePath, privateKey, 0600); err != nil {
		t.Fatal(err)
	}
	verifier, err := token.NewVerifier(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := auth.NewTokenIssuer(privatePath, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newMux(verifier))
	t.Cleanup(server.Close)
	return server, issuer
}

func TestMeRejectsUnauthorizedRequests(t *testing.T) {
	server, _ := newGatewayTestServer(t)
	for _, tc := range []struct {
		name    string
		headers []string
	}{
		{"missing_token", nil},
		{"invalid_token", []string{"Bearer not-a-jwt"}},
		{"duplicate_authorization", []string{"Bearer not-a-jwt", "Bearer another"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-User-ID", "attacker-controlled-id")
			for _, header := range tc.headers {
				req.Header.Add("Authorization", header)
			}
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.StatusCode)
			}
			if resp.Header.Get("WWW-Authenticate") != "Bearer" {
				t.Error("missing Bearer challenge")
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Error == "" {
				t.Error("expected a JSON error message")
			}
		})
	}
}

func TestMeReturnsVerifiedUserID(t *testing.T) {
	server, issuer := newGatewayTestServer(t)
	const expectedID = "verified-test-user"
	rawToken, err := issuer.Issue(expectedID)
	if err != nil {
		t.Fatal(err)
	}
	for _, scheme := range []string{"Bearer", "bearer"} {
		t.Run(scheme, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", scheme+" "+rawToken)
			req.Header.Set("X-User-ID", "attacker-controlled-id")
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("expected 200, got %d", resp.StatusCode)
			}
			var body struct {
				UserID string `json:"user_id"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.UserID != expectedID {
				t.Fatalf("expected verified ID %q, got %q", expectedID, body.UserID)
			}
		})
	}
}

func TestMeRejectsModifiedSignature(t *testing.T) {
	server, issuer := newGatewayTestServer(t)
	rawToken, err := issuer.Issue("verified-test-user")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(rawToken, ".")
	if parts[2][0] == 'A' {
		parts[2] = "B" + parts[2][1:]
	} else {
		parts[2] = "A" + parts[2][1:]
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+strings.Join(parts, "."))
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 for modified signature, got %d", resp.StatusCode)
	}
}

func TestHealthRemainsPublic(t *testing.T) {
	server, _ := newGatewayTestServer(t)
	resp, err := server.Client().Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
