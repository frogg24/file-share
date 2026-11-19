package upload

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/frogg24/file-share/internal/events"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create commits the file and both outgoing notifications together.
func (r *Repository) Create(ctx context.Context, file *File) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	const query = `
		INSERT INTO files (id, owner_id, original_name, size, content)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`
	if err := tx.QueryRow(ctx, query,
		file.ID, file.OwnerID, file.OriginalName, file.Size, file.Content,
	).Scan(&file.CreatedAt); err != nil {
		return err
	}

	metadata := events.FileCreated{
		FileID: file.ID, OwnerID: file.OwnerID,
		OriginalName: file.OriginalName, Size: file.Size, CreatedAt: file.CreatedAt,
	}
	if err := enqueueMessage(ctx, tx, events.FileCreatedType, metadata); err != nil {
		return err
	}
	replica := events.FileReplicaCreated{
		FileID: file.ID, OwnerID: file.OwnerID,
		OriginalName: file.OriginalName, Size: file.Size,
		Content: file.Content, CreatedAt: file.CreatedAt,
	}
	if err := enqueueMessage(ctx, tx, events.FileReplicaCreatedType, replica); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) MarkDeleted(ctx context.Context, fileID, ownerID string) (*File, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	const query = `
		UPDATE files
		SET deleted_at = COALESCE(deleted_at, clock_timestamp())
		WHERE id = $1 AND owner_id = $2
		RETURNING id, owner_id, original_name, size, created_at, deleted_at
	`
	var file File
	err = tx.QueryRow(ctx, query, fileID, ownerID).Scan(
		&file.ID, &file.OwnerID, &file.OriginalName,
		&file.Size, &file.CreatedAt, &file.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, err
	}

	event := events.FileDeleted{
		FileID: file.ID, OwnerID: file.OwnerID,
		OriginalName: file.OriginalName, Size: file.Size,
		CreatedAt: file.CreatedAt, DeletedAt: *file.DeletedAt,
	}
	// A repeated DELETE retains the timestamp and queues a safe repeat event.
	// This also preserves manual retry behavior for pre-outbox deletions.
	if err := enqueueMessage(ctx, tx, events.FileDeletedType, event); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &file, nil
}

func enqueueMessage(ctx context.Context, tx pgx.Tx, eventType string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_messages (id, event_type, payload)
		VALUES ($1, $2, $3)
	`, uuid.NewString(), eventType, payload)
	return err
}
