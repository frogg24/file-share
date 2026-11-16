package events

import "time"

const UserCreatedType = "user.created"
const UserUpdatedType = "user.updated"

type UserChanged struct {
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	UpdatedAt   time.Time `json:"updated_at"`
}
