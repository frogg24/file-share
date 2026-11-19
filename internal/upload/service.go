package upload

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"
)

type UploadInput struct {
	OwnerID      string
	OriginalName string
	Content      []byte
}

const MaxFileSize int64 = 1_000_000

type Service struct {
	repository *Repository
}

func NewService(repository *Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Upload(ctx context.Context, input UploadInput) (*File, error) {
	input.OriginalName = strings.TrimSpace(input.OriginalName)

	if input.OriginalName == "" || input.OwnerID == "" {
		return nil, ErrInvalidInput
	}

	if input.Content == nil {
		input.Content = []byte{}
	}

	if int64(len(input.Content)) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	var file File
	file.ID = uuid.NewString()
	file.OwnerID = input.OwnerID
	file.OriginalName = input.OriginalName
	file.Size = int64(len(input.Content))
	file.Content = input.Content

	err := s.repository.Create(ctx, &file)
	if err != nil {
		return nil, err
	}

	log.Printf("File saved and notifications queued: file_id=%s", file.ID)
	return &file, nil
}

func (s *Service) Delete(ctx context.Context, fileID, ownerID string) error {
	if len(fileID) != 36 || ownerID == "" {
		return ErrInvalidInput
	}
	if _, err := uuid.Parse(fileID); err != nil {
		return ErrInvalidInput
	}

	file, err := s.repository.MarkDeleted(ctx, fileID, ownerID)
	if err != nil {
		return err
	}

	log.Printf("File deleted and notification queued: file_id=%s", file.ID)
	return nil
}
