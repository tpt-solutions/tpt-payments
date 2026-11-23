package webhook

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// JetStream transport.
//
//	outbox (webhook_deliveries, Postgres)
//	   -- Relay: publish the delivery ID, de-duplicated by message ID -->
//	stream WEBHOOKS (work queue, file storage)
//	   -- Consumer: pull, attempt via Worker.Attempt, ack or NAK with the retry delay -->
//	HTTP delivery
//
// Postgres stays the source of truth: the message carries only the delivery ID, and each attempt
// re-reads the row. A message lost in transit is recovered because the relay republishes every
// row that has no published_at.

const (
	// StreamName is the JetStream stream that holds delivery IDs.
	StreamName = "WEBHOOKS"
	// DeliverSubject is the subject the relay publishes to and the consumer reads from.
	DeliverSubject = "webhooks.deliver"
	// ConsumerName is the durable pull consumer shared by all workers.
	ConsumerName = "webhook-workers"
	// duplicateWindow is how long the stream remembers a message ID. Re-publishing a delivery
	// within this window is a no-op. It is longer than any relay pass.
	duplicateWindow = 2 * time.Minute
	// ackWait is how long a delivered-but-unacknowledged message is held before redelivery. It
	// must exceed the HTTP client's timeout.
	ackWait = 2 * time.Minute
)

// EnsureStream creates or updates the stream and the consumer. It is safe to call on every start.
func EnsureStream(ctx context.Context, js jetstream.JetStream, maxAttempts int) error {
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:       StreamName,
		Subjects:   []string{DeliverSubject},
		Retention:  jetstream.WorkQueuePolicy,
		Storage:    jetstream.FileStorage,
		Duplicates: duplicateWindow,
	}); err != nil {
		return fmt.Errorf("create webhook stream: %w", err)
	}
	if _, err := js.CreateOrUpdateConsumer(ctx, StreamName, jetstream.ConsumerConfig{
		Durable:       ConsumerName,
		FilterSubject: DeliverSubject,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       ackWait,
		DeliverPolicy: jetstream.DeliverAllPolicy,
		// The worker counts attempts in Postgres and stops at 15 or 72 hours. MaxDeliver is a
		// backstop for a message that keeps failing without reaching the worker's own limit.
		MaxDeliver: maxAttempts + 5,
	}); err != nil {
		return fmt.Errorf("create webhook consumer: %w", err)
	}
	return nil
}

// Relay publishes pending deliveries from the outbox to the stream.
type Relay struct {
	queue Queue
	js    jetstream.JetStream
	batch int
}

// Queue is the part of the delivery store the JetStream transport uses. Worker implements it
// over Postgres. Tests use an in-memory implementation.
type Queue interface {
	Unpublished(ctx context.Context, limit int) ([]string, error)
	MarkPublished(ctx context.Context, id string) error
	Attempt(ctx context.Context, id string) (Outcome, error)
}

// NewRelay returns a relay that publishes up to batch deliveries per pass.
func NewRelay(queue Queue, js jetstream.JetStream, batch int) *Relay {
	if batch <= 0 {
		batch = 100
	}
	return &Relay{queue: queue, js: js, batch: batch}
}

// RunOnce publishes one batch and returns how many deliveries it put on the stream.
func (r *Relay) RunOnce(ctx context.Context) (int, error) {
	ids, err := r.queue.Unpublished(ctx, r.batch)
	if err != nil {
		return 0, err
	}
	published := 0
	for _, id := range ids {
		// The message ID is the delivery ID, so a retry after a crash between publish and
		// MarkPublished is dropped by the stream rather than delivered twice.
		if _, err := r.js.Publish(ctx, DeliverSubject, []byte(id), jetstream.WithMsgID(id)); err != nil {
			return published, fmt.Errorf("publish delivery %s: %w", id, err)
		}
		if err := r.queue.MarkPublished(ctx, id); err != nil {
			return published, err
		}
		published++
	}
	return published, nil
}

// Run publishes every interval until ctx is cancelled.
func (r *Relay) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := r.RunOnce(ctx); err != nil {
				slog.Error("webhook relay pass failed", "err", err)
			}
		}
	}
}

// Consumer pulls delivery IDs from the stream and attempts them.
type Consumer struct {
	queue    Queue
	consumer jetstream.Consumer
	batch    int
	wait     time.Duration
}

// NewConsumer returns a consumer for the shared durable consumer on the stream.
func NewConsumer(ctx context.Context, queue Queue, js jetstream.JetStream, batch int) (*Consumer, error) {
	c, err := js.Consumer(ctx, StreamName, ConsumerName)
	if err != nil {
		return nil, fmt.Errorf("open webhook consumer: %w", err)
	}
	if batch <= 0 {
		batch = 10
	}
	return &Consumer{queue: queue, consumer: c, batch: batch, wait: 5 * time.Second}, nil
}

// RunOnce fetches one batch, attempts each message, and settles it. It returns how many messages
// it handled.
func (c *Consumer) RunOnce(ctx context.Context) (int, error) {
	batch, err := c.consumer.Fetch(c.batch, jetstream.FetchMaxWait(c.wait))
	if err != nil {
		return 0, fmt.Errorf("fetch webhook messages: %w", err)
	}
	handled := 0
	for msg := range batch.Messages() {
		c.handle(ctx, msg)
		handled++
	}
	if err := batch.Error(); err != nil && !errors.Is(err, context.Canceled) {
		return handled, fmt.Errorf("webhook batch: %w", err)
	}
	return handled, nil
}

// handle attempts one message and settles it:
//   - Done: acknowledged, the message is removed
//   - not Done: NAK with the retry delay, so the stream redelivers it after RetryAfter
//   - attempt error (database unreachable): NAK with a short delay, so nothing is lost
func (c *Consumer) handle(ctx context.Context, msg jetstream.Msg) {
	id := string(msg.Data())
	out, err := c.queue.Attempt(ctx, id)
	if err != nil {
		slog.Error("webhook attempt failed; will retry", "delivery_id", id, "err", err)
		_ = msg.NakWithDelay(10 * time.Second)
		return
	}
	if out.Done {
		_ = msg.Ack()
		return
	}
	_ = msg.NakWithDelay(out.RetryAfter)
}

// Run consumes until ctx is cancelled. A failed fetch backs off briefly and tries again.
func (c *Consumer) Run(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		if _, err := c.RunOnce(ctx); err != nil && ctx.Err() == nil {
			slog.Error("webhook consumer pass failed", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}
}
