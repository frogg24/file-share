package events

import "time"

const FileCreatedType = "file.created"
const FileReplicaCreatedType = "file.replica.created"
const FileDownloadedType = "file.downloaded"

type FileCreated struct {
	FileID       string    `json:"file_id"`
	OwnerID      string    `json:"owner_id"`
	OriginalName string    `json:"original_name"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"created_at"`
}

type FileReplicaCreated struct {
	FileID       string    `json:"file_id"`
	OwnerID      string    `json:"owner_id"`
	OriginalName string    `json:"original_name"`
	Size         int64     `json:"size"`
	Content      []byte    `json:"content"`
	CreatedAt    time.Time `json:"created_at"`
}

type FileDownloaded struct {
	DownloadID   string    `json:"download_id"`
	FileID       string    `json:"file_id"`
	UserID       *string   `json:"user_id"`
	DownloadedAt time.Time `json:"downloaded_at"`
}
