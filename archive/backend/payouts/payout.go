// tpt-payments/backend/payouts/payout.go
package payouts

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)


// PayoutStatus represents payout lifecycle states
type PayoutStatus string

const (
	PayoutStatusPending     PayoutStatus = "pending"
	PayoutStatusInTransit   PayoutStatus = "in_transit"
	PayoutStatusPaid        PayoutStatus = "paid"
	PayoutStatusFailed      PayoutStatus = "failed"
	PayoutStatusCanceled    PayoutStatus = "canceled"
)

// PayoutMethod represents payout destination type
type PayoutMethod string

const (
	PayoutMethodACH     PayoutMethod = "ach"
	PayoutMethodSEPA    PayoutMethod = "sepa"
	PayoutMethodFaster  PayoutMethod = "faster_payments"
	PayoutMethodCard    PayoutMethod = "card"
)

// Payout represents a disbursement to a connected account
type Payout struct {
	ID              string          `json:"id"`
	MerchantID      string          `json:"merchant_id"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	Status          PayoutStatus    `json:"status"`
	Method          PayoutMethod    `json:"method"`
	DestinationID   string          `json:"destination_id"`
	ArrivalDate     int64           `json:"arrival_date"`
	PaidAt          int64           `json:"paid_at,omitempty"`
	FailureCode     string          `json:"failure_code,omitempty"`
	FailureMessage  string          `json:"failure_message,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       int64           `json:"created_at"`
	UpdatedAt       int64           `json:"updated_at"`
}

// NewPayout creates a new pending payout
func NewPayout(merchantID string, amount decimal.Decimal, currency string, method PayoutMethod, destinationID string) *Payout {
	now := time.Now().UTC().Unix()
	return &Payout{
		ID:            "po_" + uuid.NewString(),
		MerchantID:    merchantID,
		Amount:        amount,
		Currency:      currency,
		Status:        PayoutStatusPending,
		Method:        method,
		DestinationID: destinationID,
		CreatedAt:     now,
		UpdatedAt:     now,
		Metadata:      make(map[string]string),
	}
}

// MarkInTransit marks payout as being processed
func (p *Payout) MarkInTransit(arrivalDate int64) {
	p.Status = PayoutStatusInTransit
	p.ArrivalDate = arrivalDate
	p.UpdatedAt = time.Now().UTC().Unix()
}

// MarkPaid marks payout as successfully completed
func (p *Payout) MarkPaid() {
	p.Status = PayoutStatusPaid
	p.PaidAt = time.Now().UTC().Unix()
	p.UpdatedAt = time.Now().UTC().Unix()
}

// MarkFailed marks payout as failed with error details
func (p *Payout) MarkFailed(code string, message string) {
	p.Status = PayoutStatusFailed
	p.FailureCode = code
	p.FailureMessage = message
	p.UpdatedAt = time.Now().UTC().Unix()
}

// Cancel cancels a pending payout
func (p *Payout) Cancel() error {
	if p.Status != PayoutStatusPending {
		return nil
	}
	p.Status = PayoutStatusCanceled
	p.UpdatedAt = time.Now().UTC().Unix()
	return nil
}
