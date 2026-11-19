package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/frogg24/file-share/internal/platform/httpauth"
	"github.com/frogg24/file-share/internal/platform/messaging"
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
	for _, name := range []string{
		"UPLOAD_DB_USER", "UPLOAD_DB_PASSWORD", "UPLOAD_DB_NAME",
		"RABBITMQ_ADDR", "RABBITMQ_USER", "RABBITMQ_PASSWORD",
	} {
		if os.Getenv(name) == "" {
			return fmt.Errorf("не задана переменная %s", name)
		}
	}

	appCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	config, err := pgxpool.ParseConfig("host=localhost port=5434 sslmode=disable")
	if err != nil {
		return err
	}
	config.ConnConfig.User = os.Getenv("UPLOAD_DB_USER")
	config.ConnConfig.Password = os.Getenv("UPLOAD_DB_PASSWORD")
	config.ConnConfig.Database = os.Getenv("UPLOAD_DB_NAME")
	dbCtx, cancel := context.WithTimeout(appCtx, 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(dbCtx, config)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(dbCtx); err != nil {
		return err
	}
	if _, err := pool.Exec(dbCtx, `
		SELECT id, event_type, payload, created_at, published_at
		FROM outbox_messages LIMIT 0
	`); err != nil {
		return fmt.Errorf("проверка outbox; примените миграцию upload/000002: %w", err)
	}
	log.Println("Connected to upload database; outbox is ready")

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
	mux.Handle("DELETE /api/upload/files/{id}", authenticator.RequireAuth(http.HandlerFunc(handler.Delete)))
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "upload"}); err != nil {
			log.Printf("Failed to write upload health response: %v", err)
		}
	})

	brokerAddress := os.Getenv("RABBITMQ_ADDR")
	brokerUser := os.Getenv("RABBITMQ_USER")
	brokerPassword := os.Getenv("RABBITMQ_PASSWORD")
	workerCtx, stopWorker := context.WithCancel(appCtx)
	workerDone := make(chan struct{})
	go func() {
		defer close(workerDone)
		upload.RunOutbox(workerCtx, repository, func() (upload.OutboxPublisher, error) {
			publisher, err := messaging.NewConfirmedPublisher(brokerAddress, brokerUser, brokerPassword)
			if err != nil {
				return nil, err
			}
			return publisher, nil
		})
	}()
	defer func() {
		stopWorker()
		<-workerDone
	}()

	server := &http.Server{
		Addr:              "127.0.0.1:8082",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() {
		log.Println("Upload service listening on http://127.0.0.1:8082")
		serverErrors <- server.ListenAndServe()
	}()
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-appCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
