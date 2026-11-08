package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/frogg24/file-share/internal/gateway"
	"github.com/frogg24/file-share/internal/platform/token"
)

type healthResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

func main() {
	verifier, err := token.NewVerifier("secrets/auth-public.key")
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              "localhost:8080",
		Handler:           newMux(verifier),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Starting gateway on http://localhost:8080")

	err = server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func newMux(verifier *token.Verifier) http.Handler {
	return newMuxWithAuth(verifier, &url.URL{
		Scheme: "http",
		Host:   "127.0.0.1:8081",
	})
}

func newMuxWithAuth(verifier *token.Verifier, authAddress *url.URL) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)

	authProxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(authAddress)
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("Auth service unavailable: %v", err)

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)

			err = json.NewEncoder(w).Encode(map[string]string{
				"error": "сервис авторизации недоступен",
			})
			if err != nil {
				log.Printf("Failed to write proxy error: %v", err)
			}
		},
	}

	mux.Handle("POST /api/auth/register", authProxy)
	mux.Handle("POST /api/auth/login", authProxy)
	mux.Handle("POST /api/auth/logout", authProxy)

	authenticator := gateway.NewAuthenticator(verifier)
	mux.Handle(
		"GET /api/me",
		authenticator.RequireAuth(http.HandlerFunc(meHandler)),
	)

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/index.html")
	})

	return gateway.RequireSameOrigin(mux)
}

func meHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := gateway.UserID(r.Context())
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		if err := json.NewEncoder(w).Encode(map[string]string{"error": "внутренняя ошибка сервера"}); err != nil {
			log.Printf("Failed to write me error: %v", err)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]string{"user_id": userID}); err != nil {
		log.Printf("Failed to write me response: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	response := healthResponse{
		Status:  "ok",
		Service: "gateway",
	}

	w.Header().Set("Content-Type", "application/json")

	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		log.Printf("Failed to write health response: %v", err)
	}
}
