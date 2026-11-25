package sandbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
)

var start = time.Date(2030, 6, 15, 12, 0, 0, 0, time.UTC)

func newTimedSandbox(t *testing.T) (*sandbox.Sandbox, *clock.Test) {
	t.Helper()
	clk := clock.NewTest(start)
	return sandbox.New(sandbox.Config{Clock: clk}), clk
}

func TestSandbox_TimestampsFollowTheClock(t *testing.T) {
	s, clk := newTimedSandbox(t)
	resp := authorize(t, s, "pm_test_success", 1000)
	assert.True(t, resp.AuthorizedAt.Equal(start))

	_, err := clk.Advance(48 * time.Hour)
	require.NoError(t, err)
	cap, err := s.Capture(context.Background(), adapter.CaptureRequest{ChargeID: resp.ChargeID, Amount: 1000})
	require.NoError(t, err)
	assert.True(t, cap.CapturedAt.Equal(start.Add(48*time.Hour)))
	assert.Equal(t, start.Add(72*time.Hour).Format("20060102"), cap.SettleDate)
}

func TestSandbox_OutageFailsCallsInsideTheWindowOnly(t *testing.T) {
	s, clk := newTimedSandbox(t)
	require.NoError(t, s.AddOutage(sandbox.Outage{From: start.Add(time.Hour), To: start.Add(2 * time.Hour)}))

	// Before the window: succeeds.
	resp := authorize(t, s, "pm_test_success", 1000)

	// Inside the window: every call fails with a network error, not a decline.
	_, err := clk.Advance(90 * time.Minute)
	require.NoError(t, err)
	_, err = s.Authorize(context.Background(), adapter.AuthRequest{Amount: 1000, Currency: "USD", PaymentMethodID: "pm_test_success"})
	require.Error(t, err)
	var ce *adapter.CardError
	assert.False(t, errors.As(err, &ce), "an outage is not a card decision")
	assert.EqualError(t, err, "sandbox: simulated network outage")
	_, err = s.Capture(context.Background(), adapter.CaptureRequest{ChargeID: resp.ChargeID, Amount: 1000})
	assert.EqualError(t, err, "sandbox: simulated network outage")

	// The window end is exclusive.
	_, err = clk.Advance(30 * time.Minute)
	require.NoError(t, err)
	_, err = s.Capture(context.Background(), adapter.CaptureRequest{ChargeID: resp.ChargeID, Amount: 1000})
	assert.NoError(t, err)
}

func TestSandbox_OutageWithDeclineCodeDeclinesAuthorize(t *testing.T) {
	s, _ := newTimedSandbox(t)
	require.NoError(t, s.AddOutage(sandbox.Outage{
		From:        start,
		To:          start.Add(time.Hour),
		DeclineCode: "do_not_honor",
	}))
	_, err := s.Authorize(context.Background(), adapter.AuthRequest{Amount: 1000, Currency: "USD", PaymentMethodID: "pm_test_success"})
	var ce *adapter.CardError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, "do_not_honor", ce.Code)
}

func TestSandbox_RejectsEmptyOutageAndClears(t *testing.T) {
	s, _ := newTimedSandbox(t)
	assert.ErrorIs(t, s.AddOutage(sandbox.Outage{From: start, To: start}), sandbox.ErrInvalidOutage)
	assert.ErrorIs(t, s.AddOutage(sandbox.Outage{From: start, To: start.Add(-time.Second)}), sandbox.ErrInvalidOutage)

	require.NoError(t, s.AddOutage(sandbox.Outage{From: start.Add(time.Hour), To: start.Add(2 * time.Hour)}))
	require.NoError(t, s.AddOutage(sandbox.Outage{From: start, To: start.Add(time.Minute)}))
	got := s.Outages()
	require.Len(t, got, 2)
	assert.True(t, got[0].From.Equal(start), "windows are listed earliest first")
	s.ClearOutages()
	assert.Empty(t, s.Outages())
}

func TestSandbox_CardExpiresOnTheSandboxClock(t *testing.T) {
	s, clk := newTimedSandbox(t)
	tok, err := s.Tokenize(context.Background(), adapter.TokenizeRequest{})
	require.NoError(t, err)
	// Tokens expire at the end of ExpMonth in ExpYear (three years from the sandbox clock).
	require.Equal(t, 2033, tok.ExpYear)

	_, err = s.Authorize(context.Background(), adapter.AuthRequest{Amount: 1000, Currency: "USD", PaymentMethodID: tok.PaymentMethodID})
	require.NoError(t, err)

	_, err = clk.Set(time.Date(2034, 1, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	_, err = s.Authorize(context.Background(), adapter.AuthRequest{Amount: 1000, Currency: "USD", PaymentMethodID: tok.PaymentMethodID})
	var ce *adapter.CardError
	require.True(t, errors.As(err, &ce))
	assert.Equal(t, "expired_card", ce.Code)
}
