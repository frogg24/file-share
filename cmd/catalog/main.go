package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/frogg24/file-share/internal/catalog"
	"github.com/frogg24/file-share/internal/events"
	"github.com/frogg24/file-share/internal/platform/messaging"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	amqp "github.com/rabbitmq/amqp091-go"
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
		"CATALOG_DB_USER", "CATALOG_DB_PASSWORD", "CATALOG_DB_NAME",
		"RABBITMQ_ADDR", "RABBITMQ_USER", "RABBITMQ_PASSWORD",
	} {
		if os.Getenv(name) == "" {
			return fmt.Errorf("не задана переменная %s", name)
		}
	}
	config, err := pgxpool.ParseConfig("host=127.0.0.1 port=5436 sslmode=disable")
	if err != nil {
		return err
	}
	config.ConnConfig.User = os.Getenv("CATALOG_DB_USER")
	config.ConnConfig.Password = os.Getenv("CATALOG_DB_PASSWORD")
	config.ConnConfig.Database = os.Getenv("CATALOG_DB_NAME")
	dbCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(dbCtx, config)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(dbCtx); err != nil {
		return err
	}
	log.Println("Connected to catalog database")
	broker, err := messaging.NewRabbitMQ(
		os.Getenv("RABBITMQ_ADDR"),
		os.Getenv("RABBITMQ_USER"),
		os.Getenv("RABBITMQ_PASSWORD"),
	)
	if err != nil {
		return fmt.Errorf("подключение RabbitMQ: %w", err)
	}
	defer broker.Close()
	repository := catalog.NewRepository(pool)
	log.Println("Catalog service waiting for events")
	go func() {
		err := broker.Consume(context.Background(), "catalog.events",
			func(ctx context.Context, message amqp.Delivery) error {
				switch message.Type {
				case events.FileCreatedType:
					var event events.FileCreated
					if err := json.Unmarshal(message.Body, &event); err != nil {
						return err
					}
					if err := repository.ApplyFileCreated(ctx, event); err != nil {
						return err
					}
					log.Printf("Applied file.created: file_id=%s", event.FileID)
				case events.FileDownloadedType:
					var event events.FileDownloaded
					if err := json.Unmarshal(message.Body, &event); err != nil {
						return err
					}
					if err := repository.ApplyFileDownloaded(ctx, event); err != nil {
						return err
					}
					log.Printf("Applied file.downloaded: download_id=%s, file_id=%s", event.DownloadID, event.FileID)
				default:
					return fmt.Errorf("неподдерживаемое событие %q", message.Type)
				}
				return nil
			},
		)
		if err != nil {
			log.Printf("Catalog consumer stopped: %v", err)
		}
	}()
	service := catalog.NewService(repository)
	handler := catalog.NewHandler(service)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/catalog/files", handler.List)
	server := &http.Server{
		Addr:              "127.0.0.1:8084",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Println("Catalog service listening on http://127.0.0.1:8084")
	return server.ListenAndServe()
}
