package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchangeName  = "fileshare.events"
	catalogQueue  = "catalog.events"
	downloadQueue = "download.files"
)

type RabbitMQ struct {
	connection *amqp.Connection
	channel    *amqp.Channel
}

func NewRabbitMQ(address, user, password string) (*RabbitMQ, error) {
	connectionURL := url.URL{
		Scheme: "amqp",
		Host:   address,
		User:   url.UserPassword(user, password),
		Path:   "/",
	}

	connection, err := amqp.DialConfig(
		connectionURL.String(),
		amqp.Config{
			Recovery: &amqp.Recovery{
				ReconnectionConfig: &amqp.ReconnectionConfig{
					MaxRetryCount: 60,
					RetryInterval: 5 * time.Second,
				},
			},
		},
	)

	if err != nil {
		return nil, err
	}
	channel, err := connection.Channel()
	if err != nil {
		connection.Close()
		return nil, err
	}
	err = channel.ExchangeDeclare(
		exchangeName,
		"topic",
		true,  // сохранять exchange при перезапуске RabbitMQ
		false, // не удалять автоматически
		false, // разрешить отправку от наших сервисов
		false, // дождаться ответа RabbitMQ
		nil,
	)
	if err != nil {
		connection.Close()
		return nil, err
	}
	_, err = channel.QueueDeclare(
		catalogQueue,
		true,  // сохранять очередь при перезапуске RabbitMQ
		false, // не удалять без получателя
		false, // очередь не привязана к одному подключению
		false, // дождаться ответа RabbitMQ
		nil,
	)
	if err != nil {
		connection.Close()
		return nil, err
	}
	for _, key := range []string{"file.*", "user.*"} {
		err = channel.QueueBind(
			catalogQueue,
			key,
			exchangeName,
			false,
			nil,
		)
		if err != nil {
			connection.Close()
			return nil, err
		}
	}

	_, err = channel.QueueDeclare(
		downloadQueue,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		connection.Close()
		return nil, err
	}

	for _, key := range []string{"file.replica.*", "file.deleted"} {
		err = channel.QueueBind(
			downloadQueue,
			key,
			exchangeName,
			false,
			nil,
		)
		if err != nil {
			connection.Close()
			return nil, err
		}
	}

	return &RabbitMQ{
		connection: connection,
		channel:    channel,
	}, nil
}
func (r *RabbitMQ) Close() error {
	return r.connection.Close()
}
func (r *RabbitMQ) Publish(ctx context.Context, eventType string, data any) error {
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}

	return r.channel.PublishWithContext(ctx, exchangeName, eventType, false, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    uuid.NewString(),
			Type:         eventType,
			Timestamp:    time.Now().UTC(),
			Body:         body,
		},
	)
}

func (r *RabbitMQ) Consume(ctx context.Context, queueName string, handle func(context.Context, amqp.Delivery) error) error {
	channel, err := r.connection.Channel()
	if err != nil {
		return err
	}
	defer channel.Close()

	if err := channel.Qos(1, 0, false); err != nil {
		return err
	}

	messages, err := channel.ConsumeWithContext(
		ctx, queueName, "", false, false, false, false, nil,
	)
	if err != nil {
		return err
	}

	for message := range messages {
		if err := handle(ctx, message); err != nil {
			return err
		}

		if err := message.Ack(false); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("RabbitMQ consumer for %s stopped", queueName)
}
