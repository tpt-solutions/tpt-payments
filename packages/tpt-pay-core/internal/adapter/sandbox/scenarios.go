package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
)

// Scenario is a scripted outcome for a magic payment method. A merchant's test suite can ask for
// a specific failure by name, without knowing card numbers.
type Scenario struct {
	// DeclineCode, when set, makes the authorization fail with a card error of this code.
	DeclineCode string
	// Message is the card error message. A default is used when empty.
	Message string
	// NetworkError, when true, makes the call fail without a card decision: the outcome is
	// unknown, which the payment service leaves for reconciliation.
	NetworkError bool
	// Delay holds the call before it answers. The call honours the request's deadline, so a
	// slow scenario can exercise timeouts.
	Delay time.Duration
}

// Scenarios are the magic payment method IDs. They are part of the public test surface: changing
// one is a breaking change for merchants' test suites.
var Scenarios = map[string]Scenario{
	"pm_card_declined": {
		DeclineCode: "card_declined", Message: "Your card was declined.",
	},
	"pm_card_insufficient_funds": {
		DeclineCode: "insufficient_funds", Message: "Your card has insufficient funds.",
	},
	"pm_card_expired": {
		DeclineCode: "expired_card", Message: "Your card has expired.",
	},
	"pm_card_cvc_incorrect": {
		DeclineCode: "incorrect_cvc", Message: "Your card's security code is incorrect.",
	},
	"pm_card_network_error": {
		NetworkError: true,
	},
	"pm_card_slow": {
		Delay: 2 * time.Second,
	},
}

// applyScenario runs a scripted outcome for pmID, if there is one. It returns handled=false for
// ordinary payment methods, which then go through the card-number checks and succeed.
func applyScenario(ctx context.Context, pmID string) (handled bool, err error) {
	sc, ok := Scenarios[pmID]
	if !ok {
		return false, nil
	}
	if sc.Delay > 0 {
		timer := time.NewTimer(sc.Delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return true, ctx.Err()
		}
	}
	if sc.NetworkError {
		return true, fmt.Errorf("sandbox: simulated network error")
	}
	if sc.DeclineCode != "" {
		msg := sc.Message
		if msg == "" {
			msg = "Your card was declined."
		}
		return true, &adapter.CardError{Code: sc.DeclineCode, Message: msg}
	}
	return false, nil
}
