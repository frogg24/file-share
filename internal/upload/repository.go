package upload

import (
	"context"

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
