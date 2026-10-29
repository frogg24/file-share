package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/frogg24/file-share/internal/auth"
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

	repository := auth.NewRepository(pool)
	service := auth.NewService(repository)
	handler := auth.NewHandler(service)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/auth/register", handler.Register)

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
