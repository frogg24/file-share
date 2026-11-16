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
	Search string
	SortBy string
	Desc   bool
	Limit  int
	Offset int
}
