package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
func (r *Repository) ApplyFileCreated(ctx context.Context, event events.FileCreated) error {
	const query = `
        INSERT INTO files (
            id, owner_id, original_name, size, created_at, download_count
        )
        VALUES (
            $1, $2, $3, $4, $5,
            (SELECT count(*) FROM downloads WHERE file_id = $1)
        )
        ON CONFLICT (id) DO UPDATE SET
            owner_id = EXCLUDED.owner_id,
            original_name = EXCLUDED.original_name,
            size = EXCLUDED.size,
            created_at = EXCLUDED.created_at
    `
	_, err := r.db.Exec(ctx, query,
		event.FileID, event.OwnerID, event.OriginalName, event.Size, event.CreatedAt,
	)
	return err
}

func (r *Repository) ApplyFileDownloaded(ctx context.Context, event events.FileDownloaded) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	const insertDownload = `
        INSERT INTO downloads (id, file_id, user_id, downloaded_at)
        VALUES ($1, $2, $3, $4)
        ON CONFLICT (id) DO NOTHING
    `
	result, err := tx.Exec(ctx, insertDownload,
		event.DownloadID, event.FileID, event.UserID, event.DownloadedAt,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 1 {
		_, err = tx.Exec(ctx, `
            UPDATE files
            SET download_count = download_count + 1
            WHERE id = $1
        `, event.FileID)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) List(ctx context.Context, options ListOptions) ([]FileListItem, int64, error) {
	if options.Limit <= 0 || options.Offset < 0 {
		return nil, 0, ErrInvalidInput
	}
	var column string
	switch options.SortBy {
	case "", "name":
		column = "f.original_name"
	case "owner":
		column = "COALESCE(u.display_name, 'anonymous')"
	case "downloads":
		column = "f.download_count"
	default:
		return nil, 0, ErrInvalidInput
	}
	direction := "ASC"
	if options.Desc {
		direction = "DESC"
	}
	escapedSearch := strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
	).Replace(options.Search)
	pattern := "%" + escapedSearch + "%"
	var ownerID any
	if options.OwnerID != "" {
		ownerID = options.OwnerID
	}
	const fromWhere = `
        FROM files f
        LEFT JOIN users u ON u.id = f.owner_id
        WHERE f.deleted_at IS NULL
			AND ($2::uuid IS NULL OR f.owner_id = $2)
          	AND (f.original_name ILIKE $1 OR COALESCE(u.display_name, 'anonymous') ILIKE $1)
    `
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT count(*) "+fromWhere, pattern, ownerID).Scan(&total); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf(`
        SELECT f.id, f.owner_id, COALESCE(u.display_name, 'anonymous'),
               f.original_name, f.size, f.download_count, f.created_at
        `+fromWhere+`
        ORDER BY %s %s, f.id ASC
        LIMIT $3 OFFSET $4
    `, column, direction)
	rows, err := r.db.Query(ctx, query, pattern, ownerID, options.Limit, options.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]FileListItem, 0)
	for rows.Next() {
		var item FileListItem
		if err := rows.Scan(
			&item.ID, &item.OwnerID, &item.OwnerName,
			&item.OriginalName, &item.Size, &item.DownloadCount, &item.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *Repository) ApplyUserChanged(ctx context.Context, event events.UserChanged) error {
	const query = `
		INSERT INTO users (id, display_name, updated_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET
			display_name = EXCLUDED.display_name,
			updated_at = EXCLUDED.updated_at
		WHERE users.updated_at < EXCLUDED.updated_at
	`

	_, err := r.db.Exec(
		ctx,
		query,
		event.UserID,
		event.DisplayName,
		event.UpdatedAt,
	)
	return err
}

func (r *Repository) ApplyFileDeleted(ctx context.Context, event events.FileDeleted) error {
	const query = `
		INSERT INTO files (id, owner_id, original_name, size, created_at, deleted_at, download_count)
		VALUES ($1, $2, $3, $4, $5, $6, (SELECT count(*) FROM downloads WHERE file_id = $1))
		ON CONFLICT (id) DO UPDATE SET deleted_at = COALESCE(files.deleted_at, EXCLUDED.deleted_at)
	`

	_, err := r.db.Exec(
		ctx,
		query,
		event.FileID,
		event.OwnerID,
		event.OriginalName,
		event.Size,
		event.CreatedAt,
		event.DeletedAt,
	)
	return err
}

func (r *Repository) FileStats(ctx context.Context, fileID, ownerID string) (*FileStats, error) {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	stats := FileStats{
		Downloads: make([]DownloadInfo, 0),
	}

	const fileQuery = `
		SELECT id, original_name, download_count
		FROM files
		WHERE id = $1
		  AND owner_id = $2
		  AND deleted_at IS NULL
	`

	err = tx.QueryRow(ctx, fileQuery, fileID, ownerID).Scan(
		&stats.FileID,
		&stats.OriginalName,
		&stats.DownloadCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrFileNotFound
	}
	if err != nil {
		return nil, err
	}

	const downloadsQuery = `
		SELECT
			CASE
				WHEN d.user_id IS NULL THEN 'Гость'
				ELSE COALESCE(u.display_name, 'Имя пока неизвестно')
			END,
			d.downloaded_at
		FROM downloads d
		LEFT JOIN users u ON u.id = d.user_id
		WHERE d.file_id = $1
		ORDER BY d.downloaded_at DESC, d.id DESC
	`

	rows, err := tx.Query(ctx, downloadsQuery, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var download DownloadInfo

		if err := rows.Scan(&download.UserName, &download.DownloadedAt); err != nil {
			return nil, err
		}

		stats.Downloads = append(stats.Downloads, download)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &stats, nil
}
