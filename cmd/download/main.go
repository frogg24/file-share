package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/frogg24/file-share/internal/download"
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
		"DOWNLOAD_DB_USER", "DOWNLOAD_DB_PASSWORD", "DOWNLOAD_DB_NAME",
		"RABBITMQ_ADDR", "RABBITMQ_USER", "RABBITMQ_PASSWORD",
	} {
		if os.Getenv(name) == "" {
			return fmt.Errorf("не задана переменная %s", name)
		}
	}
	config, err := pgxpool.ParseConfig("host=127.0.0.1 port=5435 sslmode=disable")
	if err != nil {
		return err
	}
	config.ConnConfig.User = os.Getenv("DOWNLOAD_DB_USER")
	config.ConnConfig.Password = os.Getenv("DOWNLOAD_DB_PASSWORD")
	config.ConnConfig.Database = os.Getenv("DOWNLOAD_DB_NAME")
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
	log.Println("Connected to download database")
	broker, err := messaging.NewRabbitMQ(
		os.Getenv("RABBITMQ_ADDR"),
		os.Getenv("RABBITMQ_USER"),
		os.Getenv("RABBITMQ_PASSWORD"),
	)
	if err != nil {
		return fmt.Errorf("подключение RabbitMQ: %w", err)
	}
	defer broker.Close()
	repository := download.NewRepository(pool)
	log.Println("Download service waiting for file replicas")
	go func() {
		err := broker.Consume(
			context.Background(),
			"download.files",
			func(ctx context.Context, message amqp.Delivery) error {
				if message.Type != events.FileReplicaCreatedType {
					return fmt.Errorf("неподдерживаемое событие %q", message.Type)
				}
				var event events.FileReplicaCreated
				if err := json.Unmarshal(message.Body, &event); err != nil {
					return fmt.Errorf("чтение копии файла: %w", err)
				}
				file := download.File{
					ID:           event.FileID,
					OwnerID:      event.OwnerID,
					OriginalName: event.OriginalName,
					Size:         event.Size,
					Content:      event.Content,
					CreatedAt:    event.CreatedAt,
				}
				if err := repository.SaveReplica(ctx, &file); err != nil {
					return fmt.Errorf("сохранение копии %s: %w", file.ID, err)
				}
				log.Printf("Replica saved: file_id=%s", file.ID)
				return nil
			},
		)
		if err != nil {
			log.Printf("File replica consumer stopped: %v", err)
		}
	}()
	handler := download.NewHandler(repository)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/download/files/{id}", handler.Download)
	server := &http.Server{
		Addr:              "127.0.0.1:8083",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	log.Println("Download service listening on http://127.0.0.1:8083")
	return server.ListenAndServe()
}
