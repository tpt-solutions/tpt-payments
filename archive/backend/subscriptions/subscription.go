// tpt-payments/backend/subscriptions/subscription.go
package subscriptions

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// SubscriptionStatus represents the current status of a subscription
type SubscriptionStatus string

const (
	SubscriptionStatusTrialing   SubscriptionStatus = "trialing"
	SubscriptionStatusActive     SubscriptionStatus = "active"
	SubscriptionStatusPastDue    SubscriptionStatus = "past_due"
	SubscriptionStatusCanceled   SubscriptionStatus = "canceled"
	SubscriptionStatusUnpaid     SubscriptionStatus = "unpaid"
)

// BillingInterval represents the billing frequency
type BillingInterval string

const (
	BillingIntervalDay   BillingInterval = "day"
	BillingIntervalWeek  BillingInterval = "week"
	BillingIntervalMonth BillingInterval = "month"
	BillingIntervalYear  BillingInterval = "year"
)

// Product represents a sellable item
type Product struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Active      bool              `json:"active"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   int64             `json:"created_at"`
	UpdatedAt   int64             `json:"updated_at"`
}

// Price represents a pricing structure for a product
type Price struct {
	ID               string            `json:"id"`
	ProductID        string            `json:"product_id"`
	Amount           decimal.Decimal   `json:"amount"`
	Currency         string            `json:"currency"`
	Active           bool              `json:"active"`
	Interval         BillingInterval   `json:"interval"`
	IntervalCount    int               `json:"interval_count"`
	TrialPeriodDays  int               `json:"trial_period_days,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        int64             `json:"created_at"`
	UpdatedAt        int64             `json:"updated_at"`
}

// Subscription represents a recurring payment subscription
type Subscription struct {
	ID                 string             `json:"id"`
	CustomerID         string             `json:"customer_id"`
	PriceID            string             `json:"price_id"`
	Status             SubscriptionStatus `json:"status"`
	CurrentPeriodStart int64              `json:"current_period_start"`
	CurrentPeriodEnd   int64              `json:"current_period_end"`
	CancelAtPeriodEnd  bool               `json:"cancel_at_period_end"`
	CanceledAt         int64              `json:"canceled_at,omitempty"`
	EndedAt            int64              `json:"ended_at,omitempty"`
	TrialStart         int64              `json:"trial_start,omitempty"`
	TrialEnd           int64              `json:"trial_end,omitempty"`
	Metadata           map[string]string  `json:"metadata,omitempty"`
	CreatedAt          int64              `json:"created_at"`
	UpdatedAt          int64              `json:"updated_at"`
}

// NewProduct creates a new product
func NewProduct(name string) *Product {
	return &Product{
		ID:        "prod_" + uuid.NewString(),
		Name:      name,
		Active:    true,
		CreatedAt: time.Now().UTC().Unix(),
		UpdatedAt: time.Now().UTC().Unix(),
		Metadata:  make(map[string]string),
	}
}

// NewPrice creates a new price for a product
func NewPrice(productID string, amount decimal.Decimal, currency string, interval BillingInterval, intervalCount int) *Price {
	return &Price{
		ID:            "price_" + uuid.NewString(),
		ProductID:     productID,
		Amount:        amount,
		Currency:      currency,
		Active:        true,
		Interval:      interval,
		IntervalCount: intervalCount,
		CreatedAt:     time.Now().UTC().Unix(),
		UpdatedAt:     time.Now().UTC().Unix(),
		Metadata:      make(map[string]string),
	}
}

// NewSubscription creates a new subscription
func NewSubscription(customerID string, priceID string, trialDays ...int) *Subscription {
	now := time.Now().UTC()
	sub := &Subscription{
		ID:                 "sub_" + uuid.NewString(),
		CustomerID:         customerID,
		PriceID:            priceID,
		Status:             SubscriptionStatusActive,
		CurrentPeriodStart: now.Unix(),
		CurrentPeriodEnd:   now.AddDate(0, 1, 0).Unix(),
		CreatedAt:          now.Unix(),
		UpdatedAt:          now.Unix(),
		Metadata:           make(map[string]string),
	}

	if len(trialDays) > 0 && trialDays[0] > 0 {
		sub.Status = SubscriptionStatusTrialing
		sub.TrialStart = now.Unix()
		sub.TrialEnd = now.AddDate(0, 0, trialDays[0]).Unix()
		sub.CurrentPeriodEnd = sub.TrialEnd
	}

	return sub
}

// Cancel cancels the subscription
func (s *Subscription) Cancel(atPeriodEnd ...bool) {
	s.CanceledAt = time.Now().UTC().Unix()
	s.UpdatedAt = time.Now().UTC().Unix()

	if len(atPeriodEnd) > 0 && atPeriodEnd[0] {
		s.CancelAtPeriodEnd = true
	} else {
		s.Status = SubscriptionStatusCanceled
		s.EndedAt = time.Now().UTC().Unix()
	}
}

// Renew renews the subscription for another billing period
func (s *Subscription) Renew() {
	now := time.Now().UTC()
	s.CurrentPeriodStart = now.Unix()
	
	// Calculate next period end
	switch s.Interval() {
	case BillingIntervalMonth:
		s.CurrentPeriodEnd = now.AddDate(0, 1, 0).Unix()
	case BillingIntervalYear:
		s.CurrentPeriodEnd = now.AddDate(1, 0, 0).Unix()
	case BillingIntervalWeek:
		s.CurrentPeriodEnd = now.AddDate(0, 0, 7).Unix()
	}
	
	s.UpdatedAt = now.Unix()
}

// Interval returns the billing interval for this subscription
func (s *Subscription) Interval() BillingInterval {
	return BillingIntervalMonth
}

// IsActive returns true if the subscription is active or trialing
func (s *Subscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive || s.Status == SubscriptionStatusTrialing
}