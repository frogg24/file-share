//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/frogg24/file-share/internal/auth"
	"github.com/frogg24/file-share/internal/platform/session"
	"github.com/frogg24/file-share/internal/platform/token"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// Uses the existing development database and a supplied test account.
// It reads user data but does not create or delete accounts or touch real keys.
func TestSessionCookieLifecycle(t *testing.T) {
	login, password := os.Getenv("FILE_SHARE_TEST_LOGIN"), os.Getenv("FILE_SHARE_TEST_PASSWORD")
	if login == "" || password == "" {
		t.Skip("set FILE_SHARE_TEST_LOGIN and FILE_SHARE_TEST_PASSWORD for an existing test account")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := godotenv.Load(filepath.Join(root, ".env")); err != nil {
		t.Fatal("cannot load development database configuration")
	}
	config, err := pgxpool.ParseConfig("host=localhost port=5433 sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.User = os.Getenv("AUTH_DB_USER")
	config.ConnConfig.Password = os.Getenv("AUTH_DB_PASSWORD")
	config.ConnConfig.Database = os.Getenv("AUTH_DB_NAME")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatal("development database is unavailable")
	}
	repository := auth.NewRepository(pool)
	storedUser, err := repository.FindByLogin(ctx, login)
	if err != nil {
		t.Fatal("test account is not available")
	}

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	privatePath, publicPath := filepath.Join(dir, "private.key"), filepath.Join(dir, "public.key")
	if err := os.WriteFile(privatePath, privateKey, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(publicPath, publicKey, 0600); err != nil {
		t.Fatal(err)
	}
	issuer, err := auth.NewTokenIssuer(privatePath, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := token.NewVerifier(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	handler := auth.NewHandler(auth.NewService(repository), issuer, false)
	authMux := http.NewServeMux()
	authMux.HandleFunc("POST /api/auth/login", handler.Login)
	authMux.HandleFunc("POST /api/auth/logout", handler.Logout)
	authServer := httptest.NewServer(authMux)
	t.Cleanup(authServer.Close)
	authURL, err := url.Parse(authServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	gatewayServer := httptest.NewServer(newMuxWithAuth(verifier, authURL))
	t.Cleanup(gatewayServer.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}

	getMe := func(c *http.Client, expectedStatus int) {
		t.Helper()
		resp, err := c.Get(gatewayServer.URL + "/api/me")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != expectedStatus {
			t.Fatalf("GET /api/me: expected %d, got %d", expectedStatus, resp.StatusCode)
		}
		if expectedStatus == http.StatusOK {
			var body struct {
				UserID string `json:"user_id"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.UserID != storedUser.ID {
				t.Fatal("returned user ID does not match the authenticated database user")
			}
		}
	}
	post := func(path string, payload any, origin string) *http.Response {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPost, gatewayServer.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", origin)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	getMe(client, http.StatusUnauthorized)
	resp := post("/api/auth/login", map[string]string{"login": login, "password": password}, gatewayServer.URL)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("login expected 200, got %d", resp.StatusCode)
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 {
		resp.Body.Close()
		t.Fatalf("expected one session cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != session.CookieName || cookie.Value == "" || !cookie.HttpOnly || cookie.Secure || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" || cookie.Domain != "" || cookie.MaxAge <= 0 || !cookie.Expires.After(time.Now()) {
		resp.Body.Close()
		t.Fatal("session cookie has incorrect attributes")
	}
	var loginBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&loginBody); err != nil {
		resp.Body.Close()
		t.Fatal(err)
	}
	resp.Body.Close()
	if _, found := loginBody["access_token"]; found {
		t.Fatal("JWT must not be exposed in the login JSON")
	}
	if _, found := loginBody["token_type"]; found {
		t.Fatal("cookie login must not advertise a bearer JSON response")
	}
	getMe(client, http.StatusOK)
	reloadedClient := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	getMe(reloadedClient, http.StatusOK)
	t.Log("real PostgreSQL login -> proxied HttpOnly cookie -> authenticated subsequent requests: PASS")

	resp = post("/api/auth/logout", nil, "https://evil.example")
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin logout expected 403, got %d", resp.StatusCode)
	}
	getMe(client, http.StatusOK)

	resp = post("/api/auth/logout", nil, gatewayServer.URL)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout expected 200, got %d", resp.StatusCode)
	}
	cleared := resp.Cookies()
	if len(cleared) != 1 || cleared[0].Name != session.CookieName || cleared[0].MaxAge >= 0 {
		t.Fatal("logout did not expire the session cookie")
	}
	getMe(client, http.StatusUnauthorized)
	resp = post("/api/auth/logout", nil, gatewayServer.URL)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal("logout must be idempotent")
	}
	t.Log("cross-origin logout rejected; same-origin logout removes access; repeated logout succeeds: PASS")

	resp = post("/api/auth/login", map[string]string{"login": login, "password": "definitely-wrong-test-password"}, gatewayServer.URL)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password expected 401, got %d", resp.StatusCode)
	}
	if len(resp.Cookies()) != 0 {
		t.Fatal("failed login must not issue a session cookie")
	}
	getMe(client, http.StatusUnauthorized)

	now := time.Now()
	expired := jwt.NewWithClaims(jwt.SigningMethodEdDSA, jwt.RegisteredClaims{
		Subject: storedUser.ID, Issuer: "file-share-auth", Audience: jwt.ClaimStrings{"file-share"},
		IssuedAt: jwt.NewNumericDate(now.Add(-time.Hour)), ExpiresAt: jwt.NewNumericDate(now.Add(-time.Minute)),
	})
	rawExpired, err := expired.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse(gatewayServer.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(baseURL, []*http.Cookie{{Name: session.CookieName, Value: rawExpired, Path: "/", MaxAge: 60}})
	getMe(client, http.StatusUnauthorized)
	t.Log("wrong password does not create a cookie; expired JWT is rejected even in an unexpired cookie: PASS")
}
