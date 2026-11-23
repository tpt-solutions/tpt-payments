package sandbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
)

func authorize(t *testing.T, ctx context.Context, s *Sandbox, pmID string) (*adapter.AuthResponse, error) {
	t.Helper()
	return s.Authorize(ctx, adapter.AuthRequest{
		Amount: 1000, Currency: "USD", PaymentMethodID: pmID, MerchantID: "merch_test",
	})
}

func TestScenario_DeclineTokensReturnTheirCardCode(t *testing.T) {
	s := New(Config{})
	cases := map[string]string{
		"pm_card_declined":           "card_declined",
		"pm_card_insufficient_funds": "insufficient_funds",
		"pm_card_expired":            "expired_card",
		"pm_card_cvc_incorrect":      "incorrect_cvc",
	}
	for pm, code := range cases {
		_, err := authorize(t, context.Background(), s, pm)
		var cardErr *adapter.CardError
		require.True(t, errors.As(err, &cardErr), "%s should be a card error", pm)
		assert.Equal(t, code, cardErr.Code, pm)
	}
}

// A network error is not a card decision, so it must not be reported as one.
func TestScenario_NetworkErrorIsNotACardDecision(t *testing.T) {
	s := New(Config{})
	_, err := authorize(t, context.Background(), s, "pm_card_network_error")
	require.Error(t, err)
	var cardErr *adapter.CardError
	assert.False(t, errors.As(err, &cardErr))
}

// Ordinary payment methods still succeed, as before scenarios existed.
func TestScenario_OrdinaryMethodsStillSucceed(t *testing.T) {
	s := New(Config{})
	resp, err := authorize(t, context.Background(), s, "pm_card_visa")
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ChargeID)
}

// A slow scenario respects the caller's deadline: the call returns when the deadline passes, not
// after the full delay.
func TestScenario_SlowHonoursDeadline(t *testing.T) {
	s := New(Config{})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := authorize(t, ctx, s, "pm_card_slow")
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
}

// Every scenario has a documented behaviour: a decline code or a network error, or a delay.
func TestScenario_TableIsComplete(t *testing.T) {
	for name, sc := range Scenarios {
		assert.True(t, sc.DeclineCode != "" || sc.NetworkError || sc.Delay > 0, name)
	}
}
