package upload

import (
	"context"
	"log"
	"time"
)

type OutboxMessage struct {
	ID        string
	EventType string
	Payload   []byte
}

// PendingMessages closes its database rows before any network publication starts.
func (r *Repository) PendingMessages(ctx context.Context, limit int) ([]OutboxMessage, error) {
	if limit < 1 {
		return nil, ErrInvalidInput
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, event_type, payload
		FROM outbox_messages
		WHERE published_at IS NULL
		ORDER BY created_at, id
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]OutboxMessage, 0)
	for rows.Next() {
		var message OutboxMessage
		if err := rows.Scan(&message.ID, &message.EventType, &message.Payload); err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (r *Repository) MarkPublished(ctx context.Context, messageID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE outbox_messages
		SET published_at = clock_timestamp()
		WHERE id = $1 AND published_at IS NULL
	`, messageID)
	return err
}

// Publish returns nil only after broker confirmation and successful routing.
// Implementations must send payload as raw JSON and retain the supplied ID.
type OutboxPublisher interface {
	Publish(ctx context.Context, messageID, eventType string, payload []byte) error
	Close() error
}

// RunOutbox belongs to the service lifetime, not to an individual HTTP request.
// A crash between broker confirmation and MarkPublished can cause a safe repeat.
func RunOutbox(ctx context.Context, repository *Repository, connect func() (OutboxPublisher, error)) {
	var publisher OutboxPublisher
	defer func() {
		if publisher != nil {
			_ = publisher.Close()
		}
	}()

	for ctx.Err() == nil {
		queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		messages, err := repository.PendingMessages(queryCtx, 20)
		cancel()

		delay := time.Second
		if err == nil && len(messages) > 0 {
			delay = 0
			for _, message := range messages {
				if ctx.Err() != nil {
					return
				}
				if publisher == nil {
					publisher, err = connect()
					if err != nil {
						publisher = nil
						break
					}
				}

				publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err = publisher.Publish(publishCtx, message.ID, message.EventType, message.Payload)
				cancel()
				if err != nil {
					_ = publisher.Close()
					publisher = nil
					break
				}

				markCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				err = repository.MarkPublished(markCtx, message.ID)
				cancel()
				if err != nil {
					break
				}
				log.Printf("Outbox published: message_id=%s type=%s", message.ID, message.EventType)
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("Upload outbox: %v; retrying in 5s", err)
			delay = 5 * time.Second
		}
		if delay > 0 && !waitOutbox(ctx, delay) {
			return
		}
	}
}

func waitOutbox(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
