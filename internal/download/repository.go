package download

import (
	"context"
	"errors"

	"github.com/frogg24/file-share/internal/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SaveReplica(ctx context.Context, file *File) error {
	const query = `
		INSERT INTO files (id, owner_id, original_name, size, content, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO NOTHING
	`
	_, err := r.db.Exec(
		ctx,
		query,
		file.ID,
		file.OwnerID,
		file.OriginalName,
		file.Size,
		file.Content,
		file.CreatedAt,
	)

	return err
}

func (r *Repository) FindByID(ctx context.Context, id string) (*File, error) {
	var file File
	const query = `
		SELECT id, owner_id, original_name, size, content, created_at 
		FROM files
		WHERE id = $1 AND deleted_at IS NULL
	`

	err := r.db.QueryRow(
		ctx,
		query,
		id,
	).Scan(
		&file.ID,
		&file.OwnerID,
		&file.OriginalName,
		&file.Size,
		&file.Content,
		&file.CreatedAt,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFileNotFound
	}

	if err != nil {
		return nil, err
	}

	return &file, nil
}

func (r *Repository) ApplyFileDeleted(ctx context.Context, event events.FileDeleted) error {
	const query = `
		INSERT INTO files (id, owner_id, original_name, size, content, created_at, deleted_at)
		VALUES ($1, $2, $3, 0, ''::bytea, $4, $5)
		ON CONFLICT (id) DO UPDATE SET deleted_at = COALESCE(files.deleted_at, EXCLUDED.deleted_at)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		event.FileID,
		event.OwnerID,
		event.OriginalName,
		event.CreatedAt,
		event.DeletedAt,
	)
	return err
}
