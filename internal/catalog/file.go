package catalog

import "time"

type FileListItem struct {
	ID            string    `json:"id"`
	OwnerID       string    `json:"owner_id"`
	OwnerName     string    `json:"owner_name"`
	OriginalName  string    `json:"original_name"`
	Size          int64     `json:"size"`
	DownloadCount int64     `json:"download_count"`
	CreatedAt     time.Time `json:"created_at"`
}

type ListOptions struct {
	Search  string
	SortBy  string
	Desc    bool
	Limit   int
	Offset  int
	OwnerID string
}

type FileStats struct {
	FileID        string         `json:"file_id"`
	OriginalName  string         `json:"original_name"`
	DownloadCount int64          `json:"download_count"`
	Downloads     []DownloadInfo `json:"downloads"`
}

type DownloadInfo struct {
	UserName     string    `json:"user_name"`
	DownloadedAt time.Time `json:"downloaded_at"`
}
