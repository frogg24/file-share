package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, user *User) error {
	const query = `
		INSERT INTO users (id, login, password_hash, display_name)
		VALUES ($1, $2, $3, $4)
		RETURNING created_at, updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		user.ID,
		user.Login,
		user.PasswordHash,
		user.DisplayName,
	).Scan(
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "users_login_key" {
			return ErrLoginTaken
		}

		return err
	}

	return nil

}

func (r *Repository) FindByLogin(ctx context.Context, login string) (*User, error) {

	var user User

	const query = `
		SELECT id, login, password_hash, display_name, created_at, updated_at
		FROM users
		WHERE login = $1
	`

	err := r.db.QueryRow(
		ctx,
		query,
		login,
	).Scan(
		&user.ID,
		&user.Login,
		&user.PasswordHash,
		&user.DisplayName,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err != nil {
		return nil, err
	}

	return &user, nil
}
