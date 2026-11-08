package auth_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frogg24/file-share/internal/auth"
	"github.com/frogg24/file-share/internal/platform/session"
)

func TestLogoutExpiresCookieWithoutRequiringLogin(t *testing.T) {
	for _, secure := range []bool{false, true} {
		handler := auth.NewHandler(nil, nil, secure)
		for i := 0; i < 2; i++ {
			response := httptest.NewRecorder()
			handler.Logout(response, httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", response.Code)
			}
			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("expected cookie deletion")
			}
			cookie := cookies[0]
			if cookie.Name != session.CookieName || cookie.Path != "/" || cookie.MaxAge >= 0 || cookie.Value != "" {
				t.Fatal("incorrect deletion cookie")
			}
			if !cookie.HttpOnly || cookie.Secure != secure || cookie.SameSite != http.SameSiteStrictMode {
				t.Fatal("incorrect cookie attributes")
			}
		}
	}
}

func TestFailedLoginDoesNotIssueCookie(t *testing.T) {
	handler := auth.NewHandler(auth.NewService(nil), nil, false)
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"invalid_json", "{", http.StatusBadRequest},
		{"empty_credentials", "{}", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.Login(response, httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(tc.body)))
			if response.Code != tc.status {
				t.Fatalf("expected %d, got %d", tc.status, response.Code)
			}
			if len(response.Result().Cookies()) != 0 {
				t.Fatal("failed login issued a cookie")
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("login response must not be cached")
			}
		})
	}
}
