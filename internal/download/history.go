package download

import (
	"context"
	"time"
)

type DownloadRecord struct {
	ID           string
	FileID       string
	UserID       *string
	DownloadedAt time.Time
}

func (r *Repository) RecordDownload(ctx context.Context, record *DownloadRecord) error {
	const query = `
		INSERT INTO downloads (id, file_id, user_id)
		VALUES ($1, $2, $3)
		RETURNING downloaded_at
	`

	err := r.db.QueryRow(ctx, query, record.ID, record.FileID, record.UserID).Scan(&record.DownloadedAt)

	if err != nil {
		return err
	}

	return nil
}
