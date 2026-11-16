package upload

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, file *File) error {
	const query = `
	INSERT INTO files (id, owner_id, original_name, size, content)
	VALUES ($1, $2, $3, $4, $5)
	RETURNING created_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		file.ID,
		file.OwnerID,
		file.OriginalName,
		file.Size,
		file.Content,
	).Scan(
		&file.CreatedAt,
	)

	return err
}

func (r *Repository) MarkDeleted(ctx context.Context, fileID, ownerID string) (*File, error) {
	const query = `
		UPDATE files
		SET deleted_at = COALESCE(deleted_at, clock_timestamp())
		WHERE id = $1 AND owner_id = $2
		RETURNING id, owner_id, original_name, size, created_at, deleted_at
	`

	var file File

	err := r.db.QueryRow(ctx, query, fileID, ownerID).Scan(
		&file.ID,
		&file.OwnerID,
		&file.OriginalName,
		&file.Size,
		&file.CreatedAt,
		&file.DeletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, err
	}

	return &file, nil
}
