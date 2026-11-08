package gateway

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// RequireSameOrigin rejects cross-origin browser requests that can change state.
// CLI clients without browser Origin/Fetch Metadata headers remain supported.
func RequireSameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		allowed := !strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site")
		origins := r.Header.Values("Origin")
		if len(origins) > 1 {
			allowed = false
		} else if len(origins) == 1 {
			origin, err := url.Parse(origins[0])
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}
			if err != nil || origin.Scheme != scheme ||
				!strings.EqualFold(origin.Host, r.Host) || origin.User != nil ||
				origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
				allowed = false
			}
		}

		if !allowed {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			if err := json.NewEncoder(w).Encode(map[string]string{"error": "запрос с другого сайта запрещён"}); err != nil {
				log.Printf("Failed to write origin error: %v", err)
			}
			return
		}

		next.ServeHTTP(w, r)
	})
}
