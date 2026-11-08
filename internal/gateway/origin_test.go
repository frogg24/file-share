package gateway_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/frogg24/file-share/internal/gateway"
)

func TestRequireSameOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, method, target, origin, fetchSite string
		status                                  int
	}{
		{"same_origin", "POST", "http://localhost:8080/api/auth/logout", "http://localhost:8080", "same-origin", 204},
		{"cli", "POST", "http://localhost:8080/api/auth/logout", "", "", 204},
		{"cross_origin", "POST", "http://localhost:8080/api/auth/logout", "https://evil.example", "cross-site", 403},
		{"same_site_wrong_port", "POST", "http://localhost:8080/api/auth/logout", "http://localhost:9000", "same-site", 403},
		{"null_origin", "POST", "http://localhost:8080/api/auth/logout", "null", "", 403},
		{"malformed_origin", "POST", "http://localhost:8080/api/auth/logout", "://invalid", "", 403},
		{"origin_with_path", "POST", "http://localhost:8080/api/auth/logout", "http://localhost:8080/path", "", 403},
		{"https_same_origin", "POST", "https://example.com/api/auth/logout", "https://example.com", "same-origin", 204},
		{"wrong_scheme", "POST", "https://example.com/api/auth/logout", "http://example.com", "", 403},
		{"cross_site_missing_origin", "POST", "http://localhost:8080/api/auth/logout", "", "cross-site", 403},
		{"safe_public_get", "GET", "http://localhost:8080/health", "https://evil.example", "cross-site", 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(http.StatusNoContent) })
			req := httptest.NewRequest(tc.method, tc.target, strings.NewReader("{}"))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			if tc.fetchSite != "" {
				req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			}
			response := httptest.NewRecorder()
			gateway.RequireSameOrigin(next).ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("expected %d, got %d", tc.status, response.Code)
			}
			if called != (tc.status == http.StatusNoContent) {
				t.Fatal("incorrect downstream handler invocation")
			}
		})
	}
}

func TestRequireSameOriginRejectsDuplicateOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("downstream handler must not run") })
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8080/api/auth/logout", nil)
	req.Header.Add("Origin", "http://localhost:8080")
	req.Header.Add("Origin", "http://localhost:8080")
	response := httptest.NewRecorder()
	gateway.RequireSameOrigin(next).ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", response.Code)
	}
}
