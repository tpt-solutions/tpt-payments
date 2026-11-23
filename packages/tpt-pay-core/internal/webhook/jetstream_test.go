package webhook_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// startNATS runs an in-process JetStream server for one test. No database and no external
// process are needed.
func startNATS(t *testing.T) jetstream.JetStream {
	t.Helper()
	srv, err := server.NewServer(&server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	require.NoError(t, err)
	go srv.Start()
	require.True(t, srv.ReadyForConnections(10*time.Second), "nats server did not start")
	t.Cleanup(func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	})

	nc, err := nats.Connect(srv.ClientURL())
	require.NoError(t, err)
	t.Cleanup(nc.Close)
	js, err := jetstream.New(nc)
	require.NoError(t, err)
	require.NoError(t, webhook.EnsureStream(context.Background(), js, 15))
	return js
}

// fakeQueue stands in for the Postgres outbox. Attempt outcomes come from a script per ID.
type fakeQueue struct {
	mu        sync.Mutex
	ids       []string
	published map[string]bool
	attempts  map[string]int
	script    map[string][]webhook.Outcome
}

func newFakeQueue(ids ...string) *fakeQueue {
	return &fakeQueue{
		ids:       ids,
		published: map[string]bool{},
		attempts:  map[string]int{},
		script:    map[string][]webhook.Outcome{},
	}
}

func (q *fakeQueue) Unpublished(_ context.Context, limit int) ([]string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	var out []string
	for _, id := range q.ids {
		if !q.published[id] {
			out = append(out, id)
		}
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (q *fakeQueue) MarkPublished(_ context.Context, id string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.published[id] = true
	return nil
}

func (q *fakeQueue) Attempt(_ context.Context, id string) (webhook.Outcome, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := q.attempts[id]
	q.attempts[id] = n + 1
	if steps := q.script[id]; n < len(steps) {
		return steps[n], nil
	}
	return webhook.Outcome{Done: true}, nil
}

func (q *fakeQueue) attemptCount(id string) int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.attempts[id]
}

func (q *fakeQueue) unmark(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.published, id)
}

func streamMessages(t *testing.T, js jetstream.JetStream) uint64 {
	t.Helper()
	s, err := js.Stream(context.Background(), webhook.StreamName)
	require.NoError(t, err)
	info, err := s.Info(context.Background())
	require.NoError(t, err)
	return info.State.Msgs
}

func TestRelay_PublishesEachPendingDeliveryOnce(t *testing.T) {
	js := startNATS(t)
	q := newFakeQueue("dlv_a", "dlv_b")
	relay := webhook.NewRelay(q, js, 100)

	n, err := relay.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	n, err = relay.RunOnce(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n, "published deliveries are not published again")
	assert.Equal(t, uint64(2), streamMessages(t, js))
}

// A delivery published twice (for example after a crash before MarkPublished) must reach the
// stream once. The message ID is the delivery ID, and the stream drops the duplicate.
func TestRelay_RepublishedDeliveryIsDeduplicated(t *testing.T) {
	js := startNATS(t)
	q := newFakeQueue("dlv_a")
	relay := webhook.NewRelay(q, js, 100)

	_, err := relay.RunOnce(context.Background())
	require.NoError(t, err)
	q.unmark("dlv_a")
	_, err = relay.RunOnce(context.Background())
	require.NoError(t, err)

	assert.Equal(t, uint64(1), streamMessages(t, js))
}

func TestConsumer_AcksDoneAndRedeliversAfterRetryDelay(t *testing.T) {
	js := startNATS(t)
	q := newFakeQueue("dlv_ok", "dlv_retry")
	// dlv_retry fails once with a short retry delay, then succeeds.
	q.script["dlv_retry"] = []webhook.Outcome{{RetryAfter: 200 * time.Millisecond}}

	relay := webhook.NewRelay(q, js, 100)
	_, err := relay.RunOnce(context.Background())
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	consumer, err := webhook.NewConsumer(ctx, q, js, 10)
	require.NoError(t, err)
	go consumer.Run(ctx)

	require.Eventually(t, func() bool {
		return q.attemptCount("dlv_retry") == 2
	}, 15*time.Second, 50*time.Millisecond, "the retried delivery is attempted again after its delay")

	require.Eventually(t, func() bool {
		return streamMessages(t, js) == 0
	}, 15*time.Second, 50*time.Millisecond, "acknowledged and finished messages leave the stream")

	assert.Equal(t, 1, q.attemptCount("dlv_ok"), "a finished delivery is not attempted twice")
}
