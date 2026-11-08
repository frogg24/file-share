package session_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/frogg24/file-share/internal/platform/session"
)

func TestSetCookie(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "local_http"
		if secure {
			name = "https"
		}
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			lifetime := 2 * time.Minute
			if err := session.SetCookie(recorder, "test-token", lifetime, secure); err != nil {
				t.Fatal(err)
			}
			cookies := recorder.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("expected one cookie, got %d", len(cookies))
			}
			cookie := cookies[0]
			if cookie.Name != session.CookieName || cookie.Value != "test-token" {
				t.Error("wrong cookie name or value")
			}
			if !cookie.HttpOnly {
				t.Error("cookie must be HttpOnly")
			}
			if cookie.SameSite != http.SameSiteStrictMode {
				t.Error("cookie must use SameSite=Strict")
			}
			if cookie.Secure != secure {
				t.Error("wrong Secure flag")
			}
			if cookie.Path != "/" || cookie.Domain != "" {
				t.Error("cookie must be host-only with path /")
			}
			if cookie.MaxAge != int(lifetime/time.Second) {
				t.Errorf("wrong lifetime %d", cookie.MaxAge)
			}
			if !cookie.Expires.After(time.Now()) {
				t.Error("cookie must have a future expiry")
			}
			if recorder.Header().Get("Cache-Control") != "no-store" {
				t.Error("cookie response must not be cached")
			}
		})
	}
}

func TestSetCookieRejectsInvalidLifetime(t *testing.T) {
	for _, ttl := range []time.Duration{0, -time.Minute, time.Millisecond} {
		recorder := httptest.NewRecorder()
		if err := session.SetCookie(recorder, "test-token", ttl, false); err == nil {
			t.Fatal("expected invalid lifetime error")
		}
		if len(recorder.Result().Cookies()) != 0 {
			t.Fatal("invalid lifetime must not create a cookie")
		}
	}
}

func TestSetCookieRejectsEmptyToken(t *testing.T) {
	recorder := httptest.NewRecorder()
	if err := session.SetCookie(recorder, "", time.Minute, false); err == nil {
		t.Fatal("expected empty token error")
	}
}

func TestClearCookie(t *testing.T) {
	recorder := httptest.NewRecorder()
	session.ClearCookie(recorder, false)
	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected deletion cookie, got %d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != session.CookieName || cookie.Path != "/" || cookie.Domain != "" {
		t.Error("deletion must match original cookie scope")
	}
	if cookie.Value != "" || cookie.MaxAge >= 0 || !cookie.Expires.Before(time.Now()) {
		t.Error("cookie is not expired")
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Error("deletion must preserve security attributes")
	}
}
