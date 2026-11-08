package main

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
)

const accessCookieName = "file_share_access"

func TestMeAcceptsCookie(t *testing.T) {
	server, issuer := newGatewayTestServer(t)
	rawToken, err := issuer.Issue("cookie-user")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: accessCookieName, Value: rawToken})
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("authenticated response must not be cached")
	}
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.UserID != "cookie-user" {
		t.Fatalf("unexpected user ID %q", body.UserID)
	}
}

func TestMeRejectsInvalidOrDuplicateCookies(t *testing.T) {
	server, issuer := newGatewayTestServer(t)
	rawToken, err := issuer.Issue("cookie-user")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		values []string
	}{
		{"invalid", []string{"not-a-jwt"}},
		{"empty", []string{""}},
		{"duplicate", []string{rawToken, rawToken}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range tc.values {
				req.AddCookie(&http.Cookie{Name: accessCookieName, Value: value})
			}
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.StatusCode)
			}
		})
	}
}

func TestMalformedBearerDoesNotFallBackToCookie(t *testing.T) {
	server, issuer := newGatewayTestServer(t)
	rawToken, err := issuer.Issue("cookie-user")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodGet, server.URL+"/api/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Basic abc")
	req.AddCookie(&http.Cookie{Name: accessCookieName, Value: rawToken})
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestCookieWorksAcrossRequestsAndNewClients(t *testing.T) {
	server, issuer := newGatewayTestServer(t)
	rawToken, err := issuer.Issue("persistent-cookie-user")
	if err != nil {
		t.Fatal(err)
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	jar.SetCookies(baseURL, []*http.Cookie{{Name: accessCookieName, Value: rawToken, Path: "/", MaxAge: 60, HttpOnly: true, SameSite: http.SameSiteStrictMode}})
	for i := 0; i < 2; i++ {
		client := &http.Client{Jar: jar}
		resp, err := client.Get(server.URL + "/api/me")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, resp.StatusCode)
		}
	}
}

func TestGatewayRejectsCrossOriginAuthRequests(t *testing.T) {
	server, _ := newGatewayTestServer(t)
	for _, origin := range []string{"https://evil.example", "null", server.URL + ".evil.example"} {
		t.Run(origin, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/login", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Origin", origin)
			req.Header.Set("Content-Type", "application/json")
			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.StatusCode)
			}
		})
	}
}

func TestGatewayRejectsCrossSiteWithoutOrigin(t *testing.T) {
	server, _ := newGatewayTestServer(t)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/api/auth/login", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Content-Type", "application/json")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}
