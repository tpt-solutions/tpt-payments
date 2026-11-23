package payment_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

// succeededIntent returns a payment intent that has been automatically captured for amount.
func (e *env) succeededIntent(t *testing.T, amount int64) *payment.Intent {
	t.Helper()
	ctx := context.Background()
	intent := e.newIntent(t, amount, payment.CaptureAutomatic)
	got, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: approvedPM})
	require.NoError(t, err)
	require.Equal(t, payment.StatusSucceeded, got.Status)
	return got
}

// webhookEventCount counts deliveries of one event type across all endpoints.
func (e *env) webhookEventCount(t *testing.T, eventType string) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM webhook_deliveries WHERE event_type = $1`, eventType,
	).Scan(&n))
	return n
}

// addWebhookEndpoint enables one endpoint for the test merchant, so events are queued.
func (e *env) addWebhookEndpoint(t *testing.T) {
	t.Helper()
	_, err := e.pool.Exec(context.Background(), `
		INSERT INTO webhook_endpoints (id, merchant_id, url, secret_ciphertext, secret_prefix, livemode)
		VALUES ($1, $2, 'https://example.test/hook', 'unused', 'whsec_t', FALSE)
	`, "we_"+uuid.New().String(), e.merchantID)
	require.NoError(t, err)
}

func TestIntegration_FullRefund_ReversesLedgerAndEmitsEvent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	e.addWebhookEndpoint(t)
	intent := e.succeededIntent(t, 1000)
	require.Equal(t, int64(1000), e.merchantBalance(t))

	refund, err := e.svc.CreateRefund(ctx, e.scope(), payment.CreateRefundParams{
		PaymentIntentID: intent.ID,
		Reason:          "requested_by_customer",
	})
	require.NoError(t, err)

	assert.Equal(t, payment.RefundSucceeded, refund.Status)
	assert.Equal(t, int64(1000), refund.Amount, "omitting amount refunds the remaining balance")
	assert.NotEmpty(t, refund.AdapterRefundID)
	assert.NotEmpty(t, refund.LedgerTxnID)

	assert.Equal(t, int64(0), e.merchantBalance(t), "the merchant's balance returns to zero")
	n, err := e.lgr.VerifyIntegrity(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "refund ledger transaction must balance")
	assert.Equal(t, 1, e.webhookEventCount(t, "charge.refunded"))

	// The intent itself stays succeeded; refunds are tracked separately.
	cur, err := e.svc.Retrieve(ctx, intent.ID, e.scope())
	require.NoError(t, err)
	assert.Equal(t, payment.StatusSucceeded, cur.Status)
}

func TestIntegration_PartialRefunds_CannotExceedCapture(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.succeededIntent(t, 1000)

	_, err := e.svc.CreateRefund(ctx, e.scope(), payment.CreateRefundParams{PaymentIntentID: intent.ID, Amount: 300})
	require.NoError(t, err)

	// Over the remaining 700 is rejected, and nothing is reserved by the failed attempt.
	_, err = e.svc.CreateRefund(ctx, e.scope(), payment.CreateRefundParams{PaymentIntentID: intent.ID, Amount: 701})
	var apiErr *apierrors.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 422, apiErr.StatusCode)

	// Omitting amount refunds the remaining 700.
	rest, err := e.svc.CreateRefund(ctx, e.scope(), payment.CreateRefundParams{PaymentIntentID: intent.ID})
	require.NoError(t, err)
	assert.Equal(t, int64(700), rest.Amount)

	// Nothing is left to refund.
	_, err = e.svc.CreateRefund(ctx, e.scope(), payment.CreateRefundParams{PaymentIntentID: intent.ID})
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, "charge_already_refunded", string(apiErr.Code))
	assert.Equal(t, int64(0), e.merchantBalance(t))
}

func TestIntegration_Refund_RequiresSucceededIntent(t *testing.T) {
	e := setup(t)
	intent := e.newIntent(t, 500, payment.CaptureAutomatic)

	_, err := e.svc.CreateRefund(context.Background(), e.scope(), payment.CreateRefundParams{PaymentIntentID: intent.ID})
	var apiErr *apierrors.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.Equal(t, apierrors.CodeIntentInvalidState, apiErr.Code)
	assert.Equal(t, 0, e.ledgerTxnCount(t, intent.ID))
}

func TestIntegration_Refund_ForeignMerchantIsNotFound(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.succeededIntent(t, 800)

	other := payment.Scope{MerchantID: uuid.New().String()}
	_, err := e.svc.CreateRefund(ctx, other, payment.CreateRefundParams{PaymentIntentID: intent.ID})
	var apiErr *apierrors.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 404, apiErr.StatusCode)

	// The refund ID is not revealed to another merchant either.
	refund, err := e.svc.CreateRefund(ctx, e.scope(), payment.CreateRefundParams{PaymentIntentID: intent.ID, Amount: 100})
	require.NoError(t, err)
	_, err = e.svc.RetrieveRefund(ctx, refund.ID, other)
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 404, apiErr.StatusCode)
}

func TestIntegration_Refund_RejectsUnknownReason(t *testing.T) {
	e := setup(t)
	intent := e.succeededIntent(t, 800)

	_, err := e.svc.CreateRefund(context.Background(), e.scope(), payment.CreateRefundParams{
		PaymentIntentID: intent.ID, Reason: "because",
	})
	var apiErr *apierrors.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, "reason", apiErr.Param)
}

// Concurrent refunds on one intent must not over-refund: with 1000 captured and 20 workers
// asking for 100 each, exactly 10 succeed.
func TestIntegration_ConcurrentRefunds_NeverExceedCapture(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.succeededIntent(t, 1000)

	const workers = 20
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			refund, err := e.svc.CreateRefund(ctx, e.scope(), payment.CreateRefundParams{
				PaymentIntentID: intent.ID, Amount: 100,
			})
			if err == nil && refund.Status == payment.RefundSucceeded {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 10, succeeded, "exactly the refundable balance worth of refunds may succeed")

	var total int64
	require.NoError(t, e.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM refunds WHERE payment_intent_id = $1 AND status = 'succeeded'`,
		intent.ID,
	).Scan(&total))
	assert.Equal(t, int64(1000), total)
	assert.Equal(t, int64(0), e.merchantBalance(t))
}
