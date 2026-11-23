package sandbox_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
)

func newSandbox() *sandbox.Sandbox {
	return sandbox.New(sandbox.Config{}) // zero latency in tests
}

func authorize(t *testing.T, s *sandbox.Sandbox, pmID string, amount int64) *adapter.AuthResponse {
	t.Helper()
	resp, err := s.Authorize(context.Background(), adapter.AuthRequest{
		Amount:          amount,
		Currency:        "USD",
		PaymentMethodID: pmID,
		MerchantID:      "merch_test",
	})
	require.NoError(t, err)
	return resp
}

func TestSandbox_Authorize_Success(t *testing.T) {
	s := newSandbox()
	resp, err := s.Authorize(context.Background(), adapter.AuthRequest{
		Amount:          1000,
		Currency:        "USD",
		PaymentMethodID: "pm_test_success",
		MerchantID:      "merch_test",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ChargeID)
	assert.NotEmpty(t, resp.AuthCode)
	assert.NotEmpty(t, resp.NetworkTxnID)
	assert.False(t, resp.AuthorizedAt.IsZero())
}

func TestSandbox_Authorize_Decline(t *testing.T) {
	tests := []struct {
		name     string
		pmID     string
		wantCode string
	}{
		{"generic decline", sandbox.CardDecline, "card_declined"},
		{"insufficient funds", sandbox.CardInsufficientFunds, "insufficient_funds"},
		{"expired card", sandbox.CardExpired, "expired_card"},
		{"bad CVC", sandbox.CardCVCFail, "incorrect_cvc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := newSandbox()
			_, err := s.Authorize(context.Background(), adapter.AuthRequest{
				Amount:          1000,
				Currency:        "USD",
				PaymentMethodID: tc.pmID,
			})
			require.Error(t, err)
			var cardErr *adapter.CardError
			require.ErrorAs(t, err, &cardErr)
			assert.Equal(t, tc.wantCode, cardErr.Code)
		})
	}
}

func TestSandbox_FullCycle_AuthCaptureRefund(t *testing.T) {
	s := newSandbox()

	auth, err := s.Authorize(context.Background(), adapter.AuthRequest{
		Amount:          5000,
		Currency:        "USD",
		PaymentMethodID: "pm_good",
		MerchantID:      "merch_test",
	})
	require.NoError(t, err)

	cap, err := s.Capture(context.Background(), adapter.CaptureRequest{
		ChargeID: auth.ChargeID,
		Amount:   5000,
	})
	require.NoError(t, err)
	assert.Equal(t, auth.ChargeID, cap.ChargeID)

	refund, err := s.Refund(context.Background(), adapter.RefundRequest{
		ChargeID: auth.ChargeID,
		Amount:   5000,
		Reason:   "requested_by_customer",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5000), refund.Amount)
}

func TestSandbox_PartialCapture(t *testing.T) {
	s := newSandbox()
	auth := authorize(t, s, "pm_good", 1000)

	_, err := s.Capture(context.Background(), adapter.CaptureRequest{
		ChargeID: auth.ChargeID,
		Amount:   700,
	})
	require.NoError(t, err)
}

func TestSandbox_VoidBeforeCapture(t *testing.T) {
	s := newSandbox()
	auth := authorize(t, s, "pm_good", 1000)

	_, err := s.Void(context.Background(), adapter.VoidRequest{ChargeID: auth.ChargeID})
	require.NoError(t, err)

	// Can't capture after void.
	_, err = s.Capture(context.Background(), adapter.CaptureRequest{ChargeID: auth.ChargeID, Amount: 1000})
	require.Error(t, err)
}

func TestSandbox_CannotVoidCapturedCharge(t *testing.T) {
	s := newSandbox()
	auth := authorize(t, s, "pm_good", 1000)
	_, err := s.Capture(context.Background(), adapter.CaptureRequest{ChargeID: auth.ChargeID, Amount: 1000})
	require.NoError(t, err)

	_, err = s.Void(context.Background(), adapter.VoidRequest{ChargeID: auth.ChargeID})
	require.Error(t, err)
}

func TestSandbox_OverRefundFails(t *testing.T) {
	s := newSandbox()
	auth := authorize(t, s, "pm_good", 1000)
	_, err := s.Capture(context.Background(), adapter.CaptureRequest{ChargeID: auth.ChargeID, Amount: 1000})
	require.NoError(t, err)

	// Full refund
	_, err = s.Refund(context.Background(), adapter.RefundRequest{ChargeID: auth.ChargeID, Amount: 1000})
	require.NoError(t, err)

	// Second refund should fail
	_, err = s.Refund(context.Background(), adapter.RefundRequest{ChargeID: auth.ChargeID, Amount: 1})
	require.Error(t, err)
}

func TestSandbox_ForceDeclineConfig(t *testing.T) {
	s := sandbox.New(sandbox.Config{ForceDeclineCode: "insufficient_funds"})
	_, err := s.Authorize(context.Background(), adapter.AuthRequest{
		Amount:          1000,
		Currency:        "USD",
		PaymentMethodID: "pm_would_normally_succeed",
	})
	require.Error(t, err)
	var cardErr *adapter.CardError
	require.ErrorAs(t, err, &cardErr)
	assert.Equal(t, "insufficient_funds", cardErr.Code)
}
