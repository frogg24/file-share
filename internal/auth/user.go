package auth

import "time"

type Profile struct {
	ID          string    `json:"id"`
	Login       string    `json:"login"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type User struct {
	ID           string
	Login        string
	PasswordHash string
	DisplayName  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
