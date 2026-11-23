package webhook_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// A finished delivery can be replayed: it is pending again, with the same event ID, and the worker
// sends it once more.
func TestInspector_ReplayResendsADeliveredEvent(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rcv := newReceiver(t, func(int) int { return http.StatusOK })
	epID, _ := h.endpoint(t, rcv.server.URL, []string{"payment_intent.succeeded"})

	intent := h.newIntent(t)
	_, err := h.svc.Confirm(ctx, intent.ID, h.scope, payment.ConfirmParams{PaymentMethodID: "pm_card_visa"})
	require.NoError(t, err)

	worker := webhook.NewWorker(h.pool, h.box, webhook.WorkerConfig{})
	_, err = worker.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, rcv.count())

	recs, _, err := webhook.ListDeliveries(ctx, h.pool, webhook.DeliveryFilter{
		MerchantID: h.merchant, Limit: 10, EndpointID: epID,
	})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "delivered", recs[0].Status)
	eventID := recs[0].EventID

	require.NoError(t, webhook.Replay(ctx, h.pool, h.merchant, false, recs[0].ID))
	_, err = worker.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, rcv.count(), "the replayed event is sent again")

	recs, _, err = webhook.ListDeliveries(ctx, h.pool, webhook.DeliveryFilter{MerchantID: h.merchant, Limit: 10, EndpointID: epID})
	require.NoError(t, err)
	assert.Equal(t, eventID, recs[0].EventID, "a replay keeps the event ID so receivers can de-duplicate")
}

// A delivery belonging to another merchant cannot be replayed or listed.
func TestInspector_IsScopedToTheMerchant(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	rcv := newReceiver(t, func(int) int { return http.StatusOK })
	epID, _ := h.endpoint(t, rcv.server.URL, []string{"*"})
	intent := h.newIntent(t)
	_, err := h.svc.Confirm(ctx, intent.ID, h.scope, payment.ConfirmParams{PaymentMethodID: "pm_card_visa"})
	require.NoError(t, err)

	recs, _, err := webhook.ListDeliveries(ctx, h.pool, webhook.DeliveryFilter{MerchantID: h.merchant, Limit: 10, EndpointID: epID})
	require.NoError(t, err)
	require.NotEmpty(t, recs)

	stranger := "00000000-0000-0000-0000-000000000001"
	err = webhook.Replay(ctx, h.pool, stranger, false, recs[0].ID)
	assert.ErrorIs(t, err, webhook.ErrDeliveryNotFound)

	page, _, err := webhook.ListDeliveries(ctx, h.pool, webhook.DeliveryFilter{MerchantID: stranger, Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, page)
}

func TestInspector_MalformedIDsAreNotFoundNotErrors(t *testing.T) {
	h := newHarness(t)
	err := webhook.Replay(context.Background(), h.pool, h.merchant, false, "not-a-uuid")
	assert.ErrorIs(t, err, webhook.ErrDeliveryNotFound)
}
