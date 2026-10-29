package auth

import "time"

type User struct {
	ID           string
	Login        string
	PasswordHash string
	DisplayName  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
