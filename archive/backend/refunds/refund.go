// tpt-payments/backend/refunds/refund.go
package refunds

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RefundStatus represents the current status of a refund
type RefundStatus string

const (
	RefundStatusPending   RefundStatus = "pending"
	RefundStatusSucceeded RefundStatus = "succeeded"
	RefundStatusFailed    RefundStatus = "failed"
	RefundStatusCanceled  RefundStatus = "canceled"
)

// Refund represents a refund of a previously successful payment
type Refund struct {
	ID              string          `json:"id"`
	PaymentIntentID string          `json:"payment_intent_id"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	Status          RefundStatus    `json:"status"`
	Reason          string          `json:"reason,omitempty"`
	Description     string          `json:"description,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       int64           `json:"created_at"`
	UpdatedAt       int64           `json:"updated_at"`
	SucceededAt     int64           `json:"succeeded_at,omitempty"`
	FailureReason   string          `json:"failure_reason,omitempty"`
}

// NewRefund creates a new refund for a payment intent
func NewRefund(paymentIntentID string, amount decimal.Decimal, currency string, reason ...string) *Refund {
	ref := &Refund{
		ID:              "re_" + uuid.NewString(),
		PaymentIntentID: paymentIntentID,
		Amount:          amount,
		Currency:        currency,
		Status:          RefundStatusPending,
		CreatedAt:       time.Now().UTC().Unix(),
		UpdatedAt:       time.Now().UTC().Unix(),
		Metadata:        make(map[string]string),
	}

	if len(reason) > 0 {
		ref.Reason = reason[0]
	}

	return ref
}

// MarkSucceeded marks the refund as successfully processed
func (r *Refund) MarkSucceeded() {
	r.Status = RefundStatusSucceeded
	r.SucceededAt = time.Now().UTC().Unix()
	r.UpdatedAt = time.Now().UTC().Unix()
}

// MarkFailed marks the refund as failed with a reason
func (r *Refund) MarkFailed(reason string) {
	r.Status = RefundStatusFailed
	r.FailureReason = reason
	r.UpdatedAt = time.Now().UTC().Unix()
}

// Cancel cancels a pending refund
func (r *Refund) Cancel() error {
	if r.Status != RefundStatusPending {
		return nil
	}
	r.Status = RefundStatusCanceled
	r.UpdatedAt = time.Now().UTC().Unix()
	return nil
}

// IsFinal returns true if the refund is in a terminal state
func (r *Refund) IsFinal() bool {
	return r.Status == RefundStatusSucceeded || r.Status == RefundStatusFailed || r.Status == RefundStatusCanceled
}