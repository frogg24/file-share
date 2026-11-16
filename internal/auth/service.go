package auth

import (
	"context"
	"errors"
	"log"
	"strings"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
	"github.com/frogg24/file-share/internal/events"
	"github.com/google/uuid"
)

type RegisterInput struct {
	Login       string
	Password    string
	DisplayName string
}

type LoginInput struct {
	Login    string
	Password string
}

type EventPublisher interface {
	Publish(ctx context.Context, eventType string, data any) error
}

type Service struct {
	repository *Repository
	publisher  EventPublisher
}

func NewService(repository *Repository, publisher EventPublisher) *Service {
	return &Service{
		repository: repository,
		publisher:  publisher,
	}
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (*User, error) {
	input.Login = strings.TrimSpace(input.Login)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Login = strings.ToLower(input.Login)

	if input.DisplayName == "" {
		return nil, ErrInvalidInput
	}

	if input.Login == "" {
		return nil, ErrInvalidInput
	}

	passwordLength := utf8.RuneCountInString(input.Password)
	if passwordLength < 6 || passwordLength > 128 {
		return nil, ErrInvalidInput
	}

	passwordHash, err := argon2id.CreateHash(
		input.Password,
		argon2id.DefaultParams,
	)
	if err != nil {
		return nil, err
	}

	var user User
	user.ID = uuid.NewString()
	user.Login = input.Login
	user.DisplayName = input.DisplayName
	user.PasswordHash = passwordHash

	err = s.repository.Create(ctx, &user)
	if err != nil {
		return nil, err
	}

	event := events.UserChanged{
		UserID:      user.ID,
		DisplayName: user.DisplayName,
		UpdatedAt:   user.UpdatedAt,
	}

	if err := s.publisher.Publish(ctx, events.UserCreatedType, event); err != nil {
		log.Printf("User %s saved, but event publication failed: %v", user.ID, err)
	} else {
		log.Printf("Sent user.created: user_id=%s", user.ID)
	}

	return &user, nil
}

func (s *Service) Login(ctx context.Context, input LoginInput) (*User, error) {
	input.Login = strings.ToLower(strings.TrimSpace(input.Login))
	if input.Login == "" || input.Password == "" {
		return nil, ErrInvalidCredentials
	}

	user, err := s.repository.FindByLogin(ctx, input.Login)
	if errors.Is(err, ErrUserNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	match, err := argon2id.ComparePasswordAndHash(
		input.Password,
		user.PasswordHash,
	)
	if err != nil {
		return nil, err
	}
	if !match {
		return nil, ErrInvalidCredentials
	}
	return user, nil
}

func (s *Service) Profile(ctx context.Context, id string) (*Profile, error) {
	if len(id) != 36 {
		return nil, ErrInvalidInput
	}

	profile, err := s.repository.FindProfileByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return profile, nil
}

func (s *Service) UpdateDisplayName(ctx context.Context, userID, displayName string) (*Profile, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, ErrInvalidInput
	}
	if _, err := uuid.Parse(userID); err != nil {
		return nil, ErrInvalidInput
	}

	profile, err := s.repository.UpdateDisplayName(ctx, userID, displayName)
	if err != nil {
		return nil, err
	}

	event := events.UserChanged{
		UserID:      profile.ID,
		DisplayName: profile.DisplayName,
		UpdatedAt:   profile.UpdatedAt,
	}

	if err := s.publisher.Publish(ctx, events.UserUpdatedType, event); err != nil {
		log.Printf("User %s updated, but event publication failed: %v", profile.ID, err)
	} else {
		log.Printf("Sent user.updated: user_id=%s", profile.ID)
	}

	return profile, nil
}
