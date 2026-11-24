package sandbox

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
)

// outcomes runs n authorizations and reports which ones failed.
func outcomes(t *testing.T, s *Sandbox, n int) []bool {
	t.Helper()
	failed := make([]bool, n)
	for i := range failed {
		_, err := authorize(t, context.Background(), s, "pm_card_visa")
		failed[i] = err != nil
	}
	return failed
}

func TestChaos_ZeroRateNeverFails(t *testing.T) {
	s := New(Config{ChaosRate: 0, ChaosSeed: 7})
	for _, failed := range outcomes(t, s, 200) {
		assert.False(t, failed)
	}
}

func TestChaos_RateOneAlwaysFailsWithANetworkError(t *testing.T) {
	s := New(Config{ChaosRate: 1, ChaosSeed: 7})
	_, err := authorize(t, context.Background(), s, "pm_card_visa")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errChaos))
	// A chaos failure is not a card decision, so it must not look like one to the caller.
	var cardErr *adapter.CardError
	assert.False(t, errors.As(err, &cardErr))
}

func TestChaos_SameSeedFailsTheSameCalls(t *testing.T) {
	a := outcomes(t, New(Config{ChaosRate: 0.4, ChaosSeed: 42}), 300)
	b := outcomes(t, New(Config{ChaosRate: 0.4, ChaosSeed: 42}), 300)
	assert.Equal(t, a, b)
}

func TestChaos_ObservedRateTracksTheConfiguredRate(t *testing.T) {
	const n = 4000
	failed := 0
	for _, f := range outcomes(t, New(Config{ChaosRate: 0.3, ChaosSeed: 99}), n) {
		if f {
			failed++
		}
	}
	rate := float64(failed) / n
	assert.InDelta(t, 0.3, rate, 0.03, "observed failure rate %.3f", rate)
}

func TestChaos_AppliesToEveryAdapterCall(t *testing.T) {
	s := New(Config{ChaosRate: 1, ChaosSeed: 1})
	ctx := context.Background()
	_, err := s.Capture(ctx, adapter.CaptureRequest{ChargeID: "ch_x"})
	assert.ErrorIs(t, err, errChaos)
	_, err = s.Void(ctx, adapter.VoidRequest{ChargeID: "ch_x"})
	assert.ErrorIs(t, err, errChaos)
	_, err = s.Refund(ctx, adapter.RefundRequest{ChargeID: "ch_x", Amount: 1})
	assert.ErrorIs(t, err, errChaos)
}
