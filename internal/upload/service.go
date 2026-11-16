package upload

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/frogg24/file-share/internal/events"
	"github.com/google/uuid"
)

type UploadInput struct {
	OwnerID      string
	OriginalName string
	Content      []byte
}

const MaxFileSize int64 = 1_000_000

type EventPublisher interface {
	Publish(ctx context.Context, eventType string, data any) error
}

type Service struct {
	repository *Repository
	publisher  EventPublisher
}

func NewService(
	repository *Repository,
	publisher EventPublisher,
) *Service {
	return &Service{
		repository: repository,
		publisher:  publisher,
	}
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

	event := events.FileCreated{
		FileID:       file.ID,
		OwnerID:      file.OwnerID,
		OriginalName: file.OriginalName,
		Size:         file.Size,
		CreatedAt:    file.CreatedAt,
	}

	err = s.publisher.Publish(ctx, events.FileCreatedType, event)
	if err != nil {
		log.Printf("File %s saved, but event publication failed: %v", file.ID, err)
	} else {
		log.Printf("Sent file.created: file_id=%s", file.ID)
	}

	replica := events.FileReplicaCreated{
		FileID:       file.ID,
		OwnerID:      file.OwnerID,
		OriginalName: file.OriginalName,
		Size:         file.Size,
		Content:      file.Content,
		CreatedAt:    file.CreatedAt,
	}

	err = s.publisher.Publish(ctx, events.FileReplicaCreatedType, replica)
	if err != nil {
		log.Printf("File %s saved, but replica event publication failed: %v", file.ID, err)
	} else {
		log.Printf("Sent file.replica.created: file_id=%s", file.ID)
	}

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

	event := events.FileDeleted{
		FileID:       file.ID,
		OwnerID:      file.OwnerID,
		OriginalName: file.OriginalName,
		Size:         file.Size,
		CreatedAt:    file.CreatedAt,
		DeletedAt:    *file.DeletedAt,
	}

	if err := s.publisher.Publish(ctx, events.FileDeletedType, event); err != nil {
		return fmt.Errorf(
			"%w: file_id=%s: %v",
			ErrEventPublicationFailed,
			file.ID,
			err,
		)
	}

	log.Printf("Sent file.deleted: file_id=%s", file.ID)
	return nil
}
