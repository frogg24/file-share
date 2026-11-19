package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/frogg24/file-share/internal/auth"
	"github.com/frogg24/file-share/internal/platform/httpauth"
	"github.com/frogg24/file-share/internal/platform/messaging"
	"github.com/frogg24/file-share/internal/platform/token"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal(err)
	}

	config, err := pgxpool.ParseConfig(
		"host=localhost port=5433 sslmode=disable",
	)
	if err != nil {
		log.Fatal(err)
	}

	config.ConnConfig.User = os.Getenv("AUTH_DB_USER")
	config.ConnConfig.Password = os.Getenv("AUTH_DB_PASSWORD")
	config.ConnConfig.Database = os.Getenv("AUTH_DB_NAME")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	err = pool.Ping(ctx)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connected to auth database")

	broker, err := messaging.NewRabbitMQ(
		os.Getenv("RABBITMQ_ADDR"),
		os.Getenv("RABBITMQ_USER"),
		os.Getenv("RABBITMQ_PASSWORD"),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer broker.Close()

	repository := auth.NewRepository(pool)
	service := auth.NewService(repository, broker)
	tokenIssuer, err := auth.NewTokenIssuer(
		"secrets/auth-private.key",
		time.Hour,
	)
	if err != nil {
		log.Fatal(err)
	}
	// Local HTTP development defaults to false. Enable this for HTTPS.
	secureCookies := false
	if value, present := os.LookupEnv("AUTH_COOKIE_SECURE"); present {
		secureCookies, err = strconv.ParseBool(value)
		if err != nil {
			log.Fatal("AUTH_COOKIE_SECURE must be true or false")
		}
	}
	handler := auth.NewHandler(service, tokenIssuer, secureCookies)
	verifier, err := token.NewVerifier("secrets/auth-public.key")
	if err != nil {
		log.Fatal(err)
	}

	authenticator := httpauth.NewAuthenticator(verifier)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/register", handler.Register)
	mux.HandleFunc("POST /api/auth/login", handler.Login)
	mux.HandleFunc("POST /api/auth/logout", handler.Logout)
	mux.Handle(
		"GET /api/auth/profile",
		authenticator.RequireAuth(http.HandlerFunc(handler.Profile)),
	)
	mux.Handle(
		"PATCH /api/auth/profile",
		authenticator.RequireAuth(http.HandlerFunc(handler.UpdateProfile)),
	)

	server := &http.Server{
		Addr:              "127.0.0.1:8081",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Auth service listening on http://127.0.0.1:8081")

	err = server.ListenAndServe()
	if err != nil {
		log.Printf("Auth server stopped: %v", err)
	}

}
