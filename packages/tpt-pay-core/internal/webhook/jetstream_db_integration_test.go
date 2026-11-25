package webhook_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// The production delivery path: a payment writes a delivery row, the relay publishes its ID to
// JetStream, and the consumer attempts it through the Postgres-backed worker. Needs
// TEST_DATABASE_URL; the NATS server is embedded.
func TestJetStreamPath_DeliversThroughTheDatabaseOutbox(t *testing.T) {
	h := newHarness(t)
	js := startNATS(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rcv := newReceiver(t, func(int) int { return http.StatusOK })
	epID, secret := h.endpoint(t, rcv.server.URL, []string{"*"})
	h.newIntent(t) // writes a pending delivery

	worker := webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{})
	relay := webhook.NewRelay(worker, js, 100)
	published, err := relay.RunOnce(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, published, 1, "the pending delivery must be published")

	consumer, err := webhook.NewConsumer(ctx, worker, js, 10)
	require.NoError(t, err)
	deadline := time.Now().Add(10 * time.Second)
	for rcv.count() == 0 {
		require.True(t, time.Now().Before(deadline), "the receiver never got the event")
		_, err := consumer.RunOnce(ctx)
		require.NoError(t, err)
	}

	rcv.mu.Lock()
	body, header := rcv.bodies[0], rcv.headers[0].Get(webhook.SignatureHeader)
	rcv.mu.Unlock()
	assert.NoError(t, webhook.Verify(body, header, secret, time.Minute), "the event must verify with the endpoint secret")

	// The attempt was recorded in the database, not only in JetStream.
	require.Eventually(t, func() bool {
		var delivered bool
		err := h.pool.QueryRow(context.Background(), `
			SELECT delivered_at IS NOT NULL FROM webhook_deliveries WHERE endpoint_id = $1
		`, epID).Scan(&delivered)
		return err == nil && delivered
	}, 5*time.Second, 50*time.Millisecond, "the delivery should be marked delivered")
}
