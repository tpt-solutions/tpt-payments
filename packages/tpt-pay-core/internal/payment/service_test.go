package payment_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

func TestStateMachine_ValidTransitions(t *testing.T) {
	tests := []struct {
		from   payment.Status
		to     payment.Status
		action string
		valid  bool
	}{
		{payment.StatusRequiresPaymentMethod, payment.StatusRequiresConfirmation, "attach_method", true},
		{payment.StatusRequiresPaymentMethod, payment.StatusCanceled, "cancel", true},
		{payment.StatusRequiresConfirmation, payment.StatusProcessing, "confirm", true},
		{payment.StatusRequiresConfirmation, payment.StatusRequiresCapture, "confirm_manual", true},
		{payment.StatusRequiresCapture, payment.StatusSucceeded, "capture", true},
		{payment.StatusRequiresCapture, payment.StatusCanceled, "cancel", true},
		{payment.StatusProcessing, payment.StatusSucceeded, "succeed", true},
		// Invalid transitions.
		{payment.StatusSucceeded, payment.StatusCanceled, "cancel", false},
		{payment.StatusCanceled, payment.StatusSucceeded, "reopen", false},
		{payment.StatusRequiresCapture, payment.StatusProcessing, "reprocess", false},
		{payment.StatusSucceeded, payment.StatusProcessing, "reprocess", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.from)+"→"+string(tc.to), func(t *testing.T) {
			got := payment.CanTransition(tc.from, tc.to)
			assert.Equal(t, tc.valid, got)
		})
	}
}

func TestIsTerminal(t *testing.T) {
	assert.True(t, payment.IsTerminal(payment.StatusSucceeded))
	assert.True(t, payment.IsTerminal(payment.StatusCanceled))
	assert.False(t, payment.IsTerminal(payment.StatusProcessing))
	assert.False(t, payment.IsTerminal(payment.StatusRequiresCapture))
	assert.False(t, payment.IsTerminal(payment.StatusRequiresPaymentMethod))
}

func TestAssertTransition_ReturnsAPIError(t *testing.T) {
	// Attempting to capture from a non-capture state should return an APIError.
	err := payment.AssertTransition(payment.StatusSucceeded, payment.StatusSucceeded, "capture")
	assert.NotNil(t, err)
	assert.Contains(t, err.Message, "succeeded")
}

func TestCreateParams_DefaultCaptureMethod(t *testing.T) {
	var p payment.CreateParams
	p.Amount = 100
	p.Currency = "USD"
	// CaptureMethod defaults to automatic in the service layer.
	assert.Equal(t, payment.CaptureMethod(""), p.CaptureMethod)
}
