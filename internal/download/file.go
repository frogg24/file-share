package download

import "time"

type File struct {
	ID           string
	OwnerID      string
	OriginalName string
	Size         int64
	Content      []byte
	CreatedAt    time.Time
	DeletedAt    *time.Time
}
