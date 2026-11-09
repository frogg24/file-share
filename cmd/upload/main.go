package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/frogg24/file-share/internal/platform/httpauth"
	"github.com/frogg24/file-share/internal/platform/token"
	"github.com/frogg24/file-share/internal/upload"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		return err
	}
	for _, name := range []string{"UPLOAD_DB_USER", "UPLOAD_DB_PASSWORD", "UPLOAD_DB_NAME"} {
		if os.Getenv(name) == "" {
			return fmt.Errorf("не задана переменная %s", name)
		}
	}
	config, err := pgxpool.ParseConfig("host=localhost port=5434 sslmode=disable")
	if err != nil {
		return err
	}
	config.ConnConfig.User = os.Getenv("UPLOAD_DB_USER")
	config.ConnConfig.Password = os.Getenv("UPLOAD_DB_PASSWORD")
	config.ConnConfig.Database = os.Getenv("UPLOAD_DB_NAME")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}
	log.Println("Connected to upload database")
	// This service verifies existing JWTs. It never reads the private key
	// and does not issue tokens or provide registration/login endpoints.
	verifier, err := token.NewVerifier("secrets/auth-public.key")
	if err != nil {
		return err
	}
	repository := upload.NewRepository(pool)
	service := upload.NewService(repository)
	handler := upload.NewHandler(service)
	authenticator := httpauth.NewAuthenticator(verifier)
	mux := http.NewServeMux()
	mux.Handle("POST /api/upload/files", authenticator.RequireAuth(http.HandlerFunc(handler.Upload)))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "upload"}); err != nil {
			log.Printf("Failed to write upload health response: %v", err)
		}
	})
	server := &http.Server{
		Addr:              "127.0.0.1:8082",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Println("Upload service listening on http://127.0.0.1:8082")
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
