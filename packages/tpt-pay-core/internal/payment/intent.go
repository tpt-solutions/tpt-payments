// Package payment implements the PaymentIntent lifecycle.
package payment

import (
	"time"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
)

// Status enumerates all valid PaymentIntent statuses.
type Status string

const (
	StatusRequiresPaymentMethod Status = "requires_payment_method"
	StatusRequiresConfirmation  Status = "requires_confirmation"
	StatusRequiresAction        Status = "requires_action"
	StatusProcessing            Status = "processing"
	StatusRequiresCapture       Status = "requires_capture"
	StatusSucceeded             Status = "succeeded"
	StatusCanceled              Status = "canceled"
)

// CaptureMethod controls when a charge is captured.
type CaptureMethod string

const (
	CaptureAutomatic CaptureMethod = "automatic" // capture immediately on confirm
	CaptureManual    CaptureMethod = "manual"    // authorize now, capture later
)

// Scope identifies whose data a request may touch: the merchant, and the mode
// (live or test) of the API key that made the request.
type Scope struct {
	MerchantID string
	Livemode   bool
}

// Intent is a PaymentIntent resource.
type Intent struct {
	ID                 string
	MerchantID         string
	Amount             int64  // smallest currency unit, e.g. cents
	Currency           string // ISO 4217
	Status             Status
	CaptureMethod      CaptureMethod
	ConfirmationMethod string
	PaymentMethodID    string
	AdapterChargeID    string
	AmountCaptured     int64 // set when the charge is captured; may be less than Amount
	Livemode           bool
	CanceledAt         *time.Time
	CancellationReason string
	LedgerTxnID        string
	Metadata           map[string]string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// validTransitions maps each status to the set of statuses it can move to.
var validTransitions = map[Status][]Status{
	StatusRequiresPaymentMethod: {StatusRequiresConfirmation, StatusCanceled},
	StatusRequiresConfirmation:  {StatusRequiresAction, StatusProcessing, StatusRequiresCapture, StatusCanceled},
	StatusRequiresAction:        {StatusRequiresConfirmation, StatusCanceled},
	StatusProcessing:            {StatusSucceeded, StatusRequiresPaymentMethod, StatusRequiresCapture},
	StatusRequiresCapture:       {StatusSucceeded, StatusCanceled},
	StatusSucceeded:             {}, // terminal
	StatusCanceled:              {}, // terminal
}

// CanTransition reports whether transitioning from current → next is valid.
func CanTransition(current, next Status) bool {
	for _, allowed := range validTransitions[current] {
		if allowed == next {
			return true
		}
	}
	return false
}

// AssertTransition returns an APIError if the transition is invalid.
func AssertTransition(current, next Status, action string) *apierrors.APIError {
	if !CanTransition(current, next) {
		return apierrors.IntentInvalidState(string(current), action)
	}
	return nil
}

// IsTerminal reports whether the status is a terminal state.
func IsTerminal(s Status) bool {
	return s == StatusSucceeded || s == StatusCanceled
}

// CreateParams are the parameters for creating a PaymentIntent.
type CreateParams struct {
	Amount             int64
	Currency           string
	CaptureMethod      CaptureMethod
	ConfirmationMethod string
	Metadata           map[string]string
}

// ConfirmParams are the parameters for confirming a PaymentIntent.
type ConfirmParams struct {
	PaymentMethodID string
}

// CaptureParams are the parameters for manually capturing a PaymentIntent.
type CaptureParams struct {
	AmountToCapture int64 // 0 means capture the full authorized amount
}

// CancelParams are the parameters for canceling a PaymentIntent.
type CancelParams struct {
	Reason string
}
