package payment_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

// Three partial refunds of one intent, newest last, to page through.
func (e *env) threeRefunds(t *testing.T) (intentID string, ids []string) {
	t.Helper()
	ctx := context.Background()
	intent := e.succeededIntent(t, 1000)
	for _, amount := range []int64{100, 200, 300} {
		r, err := e.svc.CreateRefund(ctx, e.scope(), payment.CreateRefundParams{
			PaymentIntentID: intent.ID, Amount: amount,
		})
		require.NoError(t, err)
		ids = append(ids, r.ID)
	}
	return intent.ID, ids
}

func TestIntegration_ListRefunds_PagesNewestFirst(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, ids := e.threeRefunds(t)

	first, err := e.svc.ListRefunds(ctx, payment.RefundListParams{Scope: e.scope(), Limit: 2})
	require.NoError(t, err)
	require.Len(t, first.Refunds, 2)
	assert.True(t, first.HasMore)
	assert.Equal(t, ids[2], first.Refunds[0].ID, "newest first")
	assert.Equal(t, ids[1], first.Refunds[1].ID)

	second, err := e.svc.ListRefunds(ctx, payment.RefundListParams{
		Scope: e.scope(), Limit: 2, StartingAfter: first.Refunds[1].ID,
	})
	require.NoError(t, err)
	require.Len(t, second.Refunds, 1)
	assert.False(t, second.HasMore)
	assert.Equal(t, ids[0], second.Refunds[0].ID)
	assert.Equal(t, int64(100), second.Refunds[0].Amount)
}

func TestIntegration_ListRefunds_FiltersByStatusAndIntent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intentID, _ := e.threeRefunds(t)

	succeeded, err := e.svc.ListRefunds(ctx, payment.RefundListParams{
		Scope: e.scope(), Status: payment.RefundSucceeded,
	})
	require.NoError(t, err)
	assert.Len(t, succeeded.Refunds, 3)

	failed, err := e.svc.ListRefunds(ctx, payment.RefundListParams{
		Scope: e.scope(), Status: payment.RefundFailed,
	})
	require.NoError(t, err)
	assert.Empty(t, failed.Refunds)

	byIntent, err := e.svc.ListRefunds(ctx, payment.RefundListParams{
		Scope: e.scope(), PaymentIntentID: intentID,
	})
	require.NoError(t, err)
	assert.Len(t, byIntent.Refunds, 3)

	other, err := e.svc.ListRefunds(ctx, payment.RefundListParams{
		Scope: e.scope(), PaymentIntentID: "pi_does_not_exist",
	})
	require.NoError(t, err)
	assert.Empty(t, other.Refunds)
}

func TestIntegration_ListRefunds_IsScopedToTheMerchantAndMode(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	_, ids := e.threeRefunds(t)

	otherMerchant := payment.Scope{MerchantID: "00000000-0000-4000-8000-000000000001", Livemode: e.scope().Livemode}
	page, err := e.svc.ListRefunds(ctx, payment.RefundListParams{Scope: otherMerchant})
	require.NoError(t, err)
	assert.Empty(t, page.Refunds, "another merchant's refunds are never listed")

	liveMode := payment.Scope{MerchantID: e.scope().MerchantID, Livemode: !e.scope().Livemode}
	page, err = e.svc.ListRefunds(ctx, payment.RefundListParams{Scope: liveMode})
	require.NoError(t, err)
	assert.Empty(t, page.Refunds, "test-mode refunds are not listed in live mode")

	// A cursor from outside the caller's scope is refused as an invalid parameter, like a lookup.
	_, err = e.svc.ListRefunds(ctx, payment.RefundListParams{Scope: otherMerchant, StartingAfter: ids[0]})
	require.Error(t, err)
}
