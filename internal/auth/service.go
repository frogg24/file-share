package auth

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
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

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
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
