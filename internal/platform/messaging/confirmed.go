package messaging

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const confirmedPublisherTimeout = 10 * time.Second

// ConfirmedPublisher owns a non-recovering connection for the S2 outbox.
// Publications are serialized; after an in-flight failure, create a new publisher.
// A confirmation proves broker acceptance, not consumer processing.
type ConfirmedPublisher struct {
	connection *amqp.Connection
	channel    *amqp.Channel
	transport  net.Conn
	returns    <-chan amqp.Return
	publishing chan struct{}
	closed     chan struct{}
	closeOnce  sync.Once
	closeErr   error
}

// NewConfirmedPublisher bounds the entire connection, handshake and topology
// setup to ten seconds. Reconnection is the outbox worker's responsibility.
func NewConfirmedPublisher(address, user, password string) (*ConfirmedPublisher, error) {
	connectionURL := url.URL{
		Scheme: "amqp",
		Host:   address,
		User:   url.UserPassword(user, password),
		Path:   "/",
	}
	// Parse errors may contain the credential-bearing URL; do not expose them.
	if _, err := amqp.ParseURI(connectionURL.String()); err != nil {
		return nil, errors.New("invalid RabbitMQ connection address")
	}

	ctx, cancel := context.WithTimeout(context.Background(), confirmedPublisherTimeout)
	defer cancel()

	var transport net.Conn
	var stopSetup func() bool
	defer func() {
		if stopSetup != nil {
			stopSetup()
		}
	}()
	connection, err := amqp.DialConfig(connectionURL.String(), amqp.Config{
		Recovery: nil,
		Dial: func(network, address string) (net.Conn, error) {
			var err error
			transport, err = (&net.Dialer{}).DialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			// The SDK clears handshake deadlines and its Publish context does not
			// interrupt socket I/O. Closing the transport also bounds setup RPCs.
			stopSetup = context.AfterFunc(ctx, func() { _ = transport.Close() })
			return transport, nil
		},
	})
	if err != nil {
		if transport != nil {
			_ = transport.Close()
		}
		return nil, fmt.Errorf("connect confirmed RabbitMQ publisher: %w", err)
	}

	publisher := &ConfirmedPublisher{
		connection: connection,
		transport:  transport,
		publishing: make(chan struct{}, 1),
		closed:     make(chan struct{}),
	}
	channel, err := connection.Channel()
	if err != nil {
		_ = publisher.Close()
		return nil, fmt.Errorf("open confirmed RabbitMQ channel: %w", err)
	}
	publisher.channel = channel
	if err := declareTopology(channel); err != nil {
		_ = publisher.Close()
		return nil, fmt.Errorf("declare RabbitMQ topology: %w", err)
	}
	// Buffer one return for our one in-flight message; dispatch must not block
	// before it can deliver the following publisher confirmation.
	publisher.returns = channel.NotifyReturn(make(chan amqp.Return, 1))
	if err := channel.Confirm(false); err != nil {
		_ = publisher.Close()
		return nil, fmt.Errorf("enable RabbitMQ publisher confirms: %w", err)
	}
	if !stopSetup() || ctx.Err() != nil {
		_ = publisher.Close()
		return nil, context.DeadlineExceeded
	}
	return publisher, nil
}

// Publish sends the supplied JSON bytes unchanged, using the stable outbox ID.
// It succeeds only after an ack with no basic.return. Each call has a ten-second
// upper bound (including waiting for another call), or the earlier ctx deadline.
// An ambiguous outcome can still have reached the broker: retry the same ID and
// make consumers idempotent. In-flight errors close this publisher before reuse.
func (p *ConfirmedPublisher) Publish(ctx context.Context, messageID, eventType string, payload []byte) (err error) {
	if len(messageID) == 0 || len(messageID) > 255 {
		return errors.New("RabbitMQ message ID must contain 1 to 255 bytes")
	}
	if len(eventType) == 0 || len(eventType) > 255 {
		return errors.New("RabbitMQ event type must contain 1 to 255 bytes")
	}
	ctx, cancel := context.WithTimeout(ctx, confirmedPublisherTimeout)
	defer cancel()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.closed:
		return amqp.ErrClosed
	case p.publishing <- struct{}{}:
	}
	defer func() { <-p.publishing }()
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-p.closed:
		return amqp.ErrClosed
	default:
	}

	// In v1.16.0 the SDK checks ctx only before writing. Force a blocked write
	// to wake on cancellation, without starting an unbounded publish goroutine.
	stopCancel := context.AfterFunc(ctx, func() { _ = p.Close() })
	defer func() {
		stopCancel()
		if err != nil {
			_ = p.Close()
		}
	}()

	confirmation, err := p.channel.PublishWithDeferredConfirmWithContext(
		ctx, exchangeName, eventType, true, false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    messageID,
			Type:         eventType,
			Timestamp:    time.Now().UTC(),
			Body:         payload,
		},
	)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("publish RabbitMQ outbox message: %w", err)
	}
	if confirmation == nil {
		return errors.New("RabbitMQ publisher confirm mode is not enabled")
	}
	acked, err := confirmation.WaitContext(ctx)
	if err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// RabbitMQ sends basic.return before ack/nack. The SDK dispatches each
	// return into this buffered listener before completing DeferredConfirmation.
	// Check AFTER waiting, not with a select that might choose ack over return.
	select {
	case returned, ok := <-p.returns:
		if !ok {
			return amqp.ErrClosed
		}
		return fmt.Errorf("RabbitMQ returned outbox message (code %d)", returned.ReplyCode)
	default:
	}
	if !acked {
		if p.channel.IsClosed() || p.connection.IsClosed() {
			return amqp.ErrClosed
		}
		return errors.New("RabbitMQ nacked outbox message")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if !stopCancel() {
		return ctx.Err()
	}
	select {
	case <-p.closed:
		return amqp.ErrClosed
	default:
		return nil
	}
}

// Close is safe concurrently with Publish and with repeated Close calls.
// Close the socket first to interrupt blocked writes; no close handshake may
// hold up outbox shutdown. Successful Publish calls already waited for confirms.
func (p *ConfirmedPublisher) Close() error {
	p.closeOnce.Do(func() {
		close(p.closed)
		p.closeErr = p.transport.Close()
		if errors.Is(p.closeErr, net.ErrClosed) {
			p.closeErr = nil
		}
		// Finish SDK cleanup even if its reader has not observed socket closure.
		_ = p.connection.CloseDeadline(time.Now())
	})
	return p.closeErr
}
