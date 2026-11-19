package webhook_test

// Integration tests for the webhook delivery path. They use a real database (see
// internal/testdb) and a local HTTP receiver. Skipped unless TEST_DATABASE_URL is set.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/secretbox"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/testdb"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/webhook"
)

type harness struct {
	pool     *pgxpool.Pool
	box      *secretbox.Box
	svc      *payment.Service
	scope    payment.Scope
	merchant string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	var merchantID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Hooks Merchant', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid").Scan(&merchantID))

	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	box, err := secretbox.New(base64.StdEncoding.EncodeToString(raw))
	require.NoError(t, err)

	return &harness{
		pool:     pool,
		box:      box,
		svc:      payment.NewService(pool, sandbox.Default(), ledger.New(pool)),
		scope:    payment.Scope{MerchantID: merchantID.String()},
		merchant: merchantID.String(),
	}
}

// endpoint inserts a test-mode endpoint directly, so tests can also cover addresses the
// API would refuse. It returns the endpoint ID and the plaintext signing secret.
func (h *harness) endpoint(t *testing.T, url string, events []string) (string, string) {
	t.Helper()
	secret := webhook.GenerateSecret()
	ciphertext, err := h.box.Seal([]byte(secret))
	require.NoError(t, err)
	id := "we_" + uuid.New().String()
	_, err = h.pool.Exec(context.Background(), `
		INSERT INTO webhook_endpoints
			(id, merchant_id, url, enabled_events, secret_ciphertext, secret_prefix, livemode)
		VALUES ($1, $2, $3, $4, $5, $6, FALSE)
	`, id, h.merchant, url, events, ciphertext, secret[:12])
	require.NoError(t, err)
	return id, secret
}

func (h *harness) newIntent(t *testing.T) *payment.Intent {
	t.Helper()
	intent, err := h.svc.Create(context.Background(), h.scope, payment.CreateParams{
		Amount: 1500, Currency: "usd",
	})
	require.NoError(t, err)
	return intent
}

func (h *harness) deliveriesFor(t *testing.T, endpointID string) (attempts int, delivered, failed bool) {
	t.Helper()
	var deliveredAt, failedAt *time.Time
	require.NoError(t, h.pool.QueryRow(context.Background(), `
		SELECT attempt_count, delivered_at, failed_at FROM webhook_deliveries
		WHERE endpoint_id = $1 ORDER BY created_at DESC LIMIT 1
	`, endpointID).Scan(&attempts, &deliveredAt, &failedAt))
	return attempts, deliveredAt != nil, failedAt != nil
}

// makeDue moves every pending delivery to now, so the next RunOnce picks it up.
func (h *harness) makeDue(t *testing.T) {
	t.Helper()
	_, err := h.pool.Exec(context.Background(),
		`UPDATE webhook_deliveries SET next_attempt_at = NOW() WHERE delivered_at IS NULL AND failed_at IS NULL`)
	require.NoError(t, err)
}

// receiver records every delivery. respond decides the status returned to the sender.
type receiver struct {
	mu       sync.Mutex
	bodies   [][]byte
	headers  []http.Header
	respond  func(n int) int
	server   *httptest.Server
	requests int
}

func newReceiver(t *testing.T, respond func(n int) int) *receiver {
	t.Helper()
	r := &receiver{respond: respond}
	r.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.requests++
		n := r.requests
		r.bodies = append(r.bodies, body)
		r.headers = append(r.headers, req.Header.Clone())
		r.mu.Unlock()
		w.WriteHeader(r.respond(n))
	}))
	t.Cleanup(r.server.Close)
	return r
}

func (r *receiver) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.requests
}

func TestDelivery_SucceededEventIsSignedAndDelivered(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rcv := newReceiver(t, func(int) int { return http.StatusOK })
	epID, secret := h.endpoint(t, rcv.server.URL, []string{"*"})

	intent := h.newIntent(t)
	_, err := h.svc.Confirm(ctx, intent.ID, h.scope, payment.ConfirmParams{PaymentMethodID: "pm_card_visa"})
	require.NoError(t, err)

	worker := webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{})
	n, err := worker.RunOnce(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, n, 2, "created and succeeded deliveries are both due")

	rcv.mu.Lock()
	defer rcv.mu.Unlock()
	var succeeded map[string]any
	for i, body := range rcv.bodies {
		header := rcv.headers[i].Get(webhook.SignatureHeader)
		require.NoError(t, webhook.Verify(body, header, secret, 5*time.Minute), "signature must verify")

		var ev struct {
			Type string `json:"type"`
			Data struct {
				Object map[string]any `json:"object"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &ev))
		if ev.Type == string(webhook.EventPaymentIntentSucceeded) {
			succeeded = ev.Data.Object
		}
		assert.NotEmpty(t, rcv.headers[i].Get("TPT-Event-Id"))
	}
	require.NotNil(t, succeeded, "payment_intent.succeeded must be delivered")
	assert.Equal(t, "succeeded", succeeded["status"])
	assert.Equal(t, float64(1500), succeeded["amount_captured"])

	attempts, delivered, _ := h.deliveriesFor(t, epID)
	assert.Equal(t, 1, attempts)
	assert.True(t, delivered)
}

func TestDelivery_FailureIsRetriedThenSucceeds(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rcv := newReceiver(t, func(n int) int {
		if n == 1 {
			return http.StatusInternalServerError
		}
		return http.StatusNoContent
	})
	epID, _ := h.endpoint(t, rcv.server.URL, []string{"payment_intent.created"})
	h.newIntent(t)

	worker := webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{})
	_, err := worker.RunOnce(ctx)
	require.NoError(t, err)

	attempts, delivered, failed := h.deliveriesFor(t, epID)
	assert.Equal(t, 1, attempts)
	assert.False(t, delivered)
	assert.False(t, failed, "one failure must not be terminal")

	// The retry is scheduled in the future, so it is not yet due.
	n, err := worker.RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "a failed delivery waits for its retry schedule")

	h.makeDue(t)
	_, err = worker.RunOnce(ctx)
	require.NoError(t, err)
	attempts, delivered, _ = h.deliveriesFor(t, epID)
	assert.Equal(t, 2, attempts)
	assert.True(t, delivered)
	assert.Equal(t, 2, rcv.count())
}

func TestDelivery_ExhaustedAttemptsMarkFailed(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rcv := newReceiver(t, func(int) int { return http.StatusBadGateway })
	epID, _ := h.endpoint(t, rcv.server.URL, []string{"payment_intent.created"})
	h.newIntent(t)

	worker := webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{MaxAttempts: 2})
	for i := 0; i < 2; i++ {
		h.makeDue(t)
		_, err := worker.RunOnce(ctx)
		require.NoError(t, err)
	}

	attempts, delivered, failed := h.deliveriesFor(t, epID)
	assert.Equal(t, 2, attempts)
	assert.False(t, delivered)
	assert.True(t, failed, "delivery must stop after max attempts")
}

func TestDelivery_OnlySubscribedEventsAreSent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rcv := newReceiver(t, func(int) int { return http.StatusOK })
	h.endpoint(t, rcv.server.URL, []string{"payment_intent.canceled"})

	intent := h.newIntent(t) // created: not subscribed
	_, err := h.svc.Confirm(ctx, intent.ID, h.scope, payment.ConfirmParams{PaymentMethodID: "pm_card_visa"})
	require.NoError(t, err)

	_, err = webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{}).RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, rcv.count(), "unsubscribed events must not be delivered")
}

func TestDelivery_CardDeclineEmitsPaymentFailed(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rcv := newReceiver(t, func(int) int { return http.StatusOK })
	h.endpoint(t, rcv.server.URL, []string{"payment_intent.payment_failed"})

	intent := h.newIntent(t)
	_, err := h.svc.Confirm(ctx, intent.ID, h.scope, payment.ConfirmParams{PaymentMethodID: sandbox.CardDecline})
	require.Error(t, err)

	_, err = webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{}).RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, rcv.count())

	var ev struct {
		Type string `json:"type"`
	}
	require.NoError(t, json.Unmarshal(rcv.bodies[0], &ev))
	assert.Equal(t, "payment_intent.payment_failed", ev.Type)
}

func TestDelivery_CancelEmitsCanceled(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rcv := newReceiver(t, func(int) int { return http.StatusOK })
	h.endpoint(t, rcv.server.URL, []string{"payment_intent.canceled"})

	intent := h.newIntent(t)
	_, err := h.svc.Cancel(ctx, intent.ID, h.scope, payment.CancelParams{Reason: "requested_by_customer"})
	require.NoError(t, err)

	_, err = webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{}).RunOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, rcv.count())
}

func TestDelivery_LinkLocalDestinationIsBlockedAtConnectTime(t *testing.T) {
	// The API refuses link-local URLs at creation. This inserts one directly to prove the
	// connect-time guard also holds if validation is ever bypassed.
	h := newHarness(t)
	ctx := context.Background()
	epID, _ := h.endpoint(t, "http://169.254.169.254/latest/meta-data", []string{"*"})
	h.newIntent(t)

	_, err := webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{}).RunOnce(ctx)
	require.NoError(t, err)

	var lastErr *string
	require.NoError(t, h.pool.QueryRow(ctx, `
		SELECT last_error FROM webhook_deliveries WHERE endpoint_id = $1 ORDER BY created_at DESC LIMIT 1
	`, epID).Scan(&lastErr))
	require.NotNil(t, lastErr)
	assert.Contains(t, *lastErr, "not allowed")
}
