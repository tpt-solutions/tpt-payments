// tpt-payments/backend/subscriptions/service.go
package subscriptions

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
)

// Service handles subscription business logic
type Service struct {
	db          *sql.DB
	payments    PaymentProcessor
	webhook     WebhookEmitter
	invoicing   InvoiceService
}

// InvoiceService interface for generating invoices
type InvoiceService interface {
	CreateForSubscription(ctx context.Context, subscriptionID string, customerID string, amount decimal.Decimal, currency string) (string, error)
}

// PaymentProcessor interface for charging subscriptions
type PaymentProcessor interface {
	ChargeCustomer(ctx context.Context, customerID string, amount decimal.Decimal, currency string, description string) (string, error)
}

// WebhookEmitter interface for sending events
type WebhookEmitter interface {
	Emit(ctx context.Context, eventType string, data interface{}) error
}

// NewService creates a new subscription service
func NewService(db *sql.DB, payments PaymentProcessor, webhook WebhookEmitter, invoicing InvoiceService) *Service {
	return &Service{
		db:       db,
		payments: payments,
		webhook:  webhook,
		invoicing: invoicing,
	}
}

// CreateSubscriptionRequest is the request to create a subscription
type CreateSubscriptionRequest struct {
	CustomerID   string            `json:"customer_id"`
	PriceID      string            `json:"price_id"`
	TrialDays    int               `json:"trial_days,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	Prorate      bool              `json:"prorate,omitempty"`
}

// Create creates a new subscription
func (s *Service) Create(ctx context.Context, req CreateSubscriptionRequest) (*Subscription, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Get price
	var price Price
	err = tx.QueryRowContext(ctx, `
		SELECT id, product_id, amount, currency, active, interval, interval_count, trial_period_days
		FROM prices WHERE id = $1`,
		req.PriceID,
	).Scan(
		&price.ID, &price.ProductID, &price.Amount, &price.Currency,
		&price.Active, &price.Interval, &price.IntervalCount, &price.TrialPeriodDays,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("price not found")
		}
		return nil, err
	}

	if !price.Active {
		return nil, errors.New("price is not active")
	}

	trialDays := req.TrialDays
	if trialDays == 0 {
		trialDays = price.TrialPeriodDays
	}

	sub := NewSubscription(req.CustomerID, req.PriceID, trialDays)
	sub.Metadata = req.Metadata

	// Store subscription
	_, err = tx.ExecContext(ctx, `
		INSERT INTO subscriptions (
			id, customer_id, price_id, status, current_period_start, current_period_end,
			cancel_at_period_end, canceled_at, ended_at, trial_start, trial_end, metadata,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		sub.ID, sub.CustomerID, sub.PriceID, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd, sub.CanceledAt, sub.EndedAt, sub.TrialStart, sub.TrialEnd, sub.Metadata,
		sub.CreatedAt, sub.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	s.webhook.Emit(ctx, "subscription.created", sub)
	return sub, nil
}

// Cancel cancels a subscription
func (s *Service) Cancel(ctx context.Context, subscriptionID string, atPeriodEnd bool) (*Subscription, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var sub Subscription
	err = tx.QueryRowContext(ctx, `
		SELECT id, customer_id, price_id, status, current_period_start, current_period_end,
		       cancel_at_period_end, canceled_at, ended_at, trial_start, trial_end, metadata
		FROM subscriptions WHERE id = $1 FOR UPDATE`,
		subscriptionID,
	).Scan(
		&sub.ID, &sub.CustomerID, &sub.PriceID, &sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.CancelAtPeriodEnd, &sub.CanceledAt, &sub.EndedAt, &sub.TrialStart, &sub.TrialEnd, &sub.Metadata,
	)
	if err == sql.ErrNoRows {
		return nil, errors.New("subscription not found")
	}

	if !sub.IsActive() {
		return nil, errors.New("cannot cancel inactive subscription")
	}

	sub.Cancel(atPeriodEnd)

	_, err = tx.ExecContext(ctx, `
		UPDATE subscriptions SET status = $1, cancel_at_period_end = $2, canceled_at = $3, ended_at = $4, updated_at = $5 WHERE id = $6`,
		sub.Status, sub.CancelAtPeriodEnd, sub.CanceledAt, sub.EndedAt, sub.UpdatedAt, sub.ID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	s.webhook.Emit(ctx, "subscription.updated", sub)
	if !atPeriodEnd {
		s.webhook.Emit(ctx, "subscription.canceled", sub)
	}

	return &sub, nil
}

// ProcessRenewalsJob is the scheduled renewal job
type ProcessRenewalsJob struct {
	service *Service
}

// NewProcessRenewalsJob creates the renewal job
func NewProcessRenewalsJob(service *Service) *ProcessRenewalsJob {
	return &ProcessRenewalsJob{service: service}
}

// Run processes all subscriptions due for renewal
func (j *ProcessRenewalsJob) Run(ctx context.Context) error {
	// Find subscriptions due for renewal
	rows, err := j.service.db.QueryContext(ctx, `
		SELECT id FROM subscriptions 
		 WHERE current_period_end < $1 
		   AND status IN ('active', 'trialing', 'past_due')
		   AND cancel_at_period_end = false`,
		time.Now().Unix(),
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var subscriptionIDs []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		subscriptionIDs = append(subscriptionIDs, id)
	}

	for _, id := range subscriptionIDs {
		if err := j.service.ProcessRenewal(ctx, id); err != nil {
			// Log error and continue
		}
	}

	return nil
}

// ProcessRenewal processes a single subscription renewal
func (s *Service) ProcessRenewal(ctx context.Context, subscriptionID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var sub Subscription
	var price Price

	err = tx.QueryRowContext(ctx, `
		SELECT s.id, s.customer_id, s.price_id, s.status, s.current_period_start, s.current_period_end,
		       s.cancel_at_period_end, s.attempt_count,
		       p.amount, p.currency
		FROM subscriptions s
		JOIN prices p ON s.price_id = p.id
		WHERE s.id = $1 FOR UPDATE`,
		subscriptionID,
	).Scan(
		&sub.ID, &sub.CustomerID, &sub.PriceID, &sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.CancelAtPeriodEnd, &sub.AttemptCount,
		&price.Amount, &price.Currency,
	)
	if err != nil {
		return err
	}

	// Process payment
	_, err = s.payments.ChargeCustomer(ctx, sub.CustomerID, price.Amount, price.Currency, "Subscription renewal")
	if err != nil {
		// Handle dunning
		sub.AttemptCount++
		nextAttempt := calculateNextAttempt(sub.AttemptCount)

		if sub.AttemptCount >= 3 {
			sub.Status = SubscriptionStatusUnpaid
			sub.EndedAt = time.Now().Unix()
			s.webhook.Emit(ctx, "subscription.payment_failed", sub)
		} else {
			sub.Status = SubscriptionStatusPastDue
			s.webhook.Emit(ctx, "subscription.payment_will_be_retried", sub)
		}

		_, err = tx.ExecContext(ctx, `
			UPDATE subscriptions SET status = $1, attempt_count = $2, next_attempt_at = $3, updated_at = $4 WHERE id = $5`,
			sub.Status, sub.AttemptCount, nextAttempt, time.Now().Unix(), sub.ID,
		)
		return tx.Commit()
	}

	// Payment succeeded
	sub.Renew()
	sub.AttemptCount = 0
	sub.Status = SubscriptionStatusActive

	_, err = tx.ExecContext(ctx, `
		UPDATE subscriptions SET 
			status = $1, current_period_start = $2, current_period_end = $3, 
			attempt_count = $4, updated_at = $5
		WHERE id = $6`,
		sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd, sub.AttemptCount, sub.UpdatedAt, sub.ID,
	)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	
	// Auto-generate invoice for successful renewal
	if s.invoicing != nil {
		go func() {
			// Use background context for non-blocking invoice creation
			_, err := s.invoicing.CreateForSubscription(context.Background(), sub.ID, sub.CustomerID, price.Amount, price.Currency)
			if err != nil {
				// Log error but don't fail renewal - invoice can be regenerated manually
			}
		}()
	}

	s.webhook.Emit(ctx, "subscription.renewed", sub)
	s.webhook.Emit(ctx, "subscription.updated", sub)

	return nil
}

// CalculateProration calculates proration amount for plan change
func (s *Service) CalculateProration(ctx context.Context, subscriptionID string, newPriceID string) (decimal.Decimal, error) {
	var sub Subscription
	var oldPrice Price
	var newPrice Price

	err := s.db.QueryRowContext(ctx, `
		SELECT s.current_period_start, s.current_period_end,
		       p.amount, p.currency
		FROM subscriptions s
		JOIN prices p ON s.price_id = p.id
		WHERE s.id = $1`,
		subscriptionID,
	).Scan(&sub.CurrentPeriodStart, &sub.CurrentPeriodEnd, &oldPrice.Amount, &oldPrice.Currency)
	if err != nil {
		return decimal.Zero, err
	}

	err = s.db.QueryRowContext(ctx, `SELECT amount, currency FROM prices WHERE id = $1`, newPriceID).Scan(&newPrice.Amount, &newPrice.Currency)
	if err != nil {
		return decimal.Zero, err
	}

	now := time.Now().Unix()
	totalPeriod := sub.CurrentPeriodEnd - sub.CurrentPeriodStart
	remaining := sub.CurrentPeriodEnd - now
	usageRatio := decimal.NewFromInt(totalPeriod - remaining).Div(decimal.NewFromInt(totalPeriod))

	oldUsed := oldPrice.Amount.Mul(usageRatio)
	proration := newPrice.Amount.Sub(oldPrice.Amount).Mul(decimal.NewFromInt(1).Sub(usageRatio))

	return proration.Round(0), nil
}

// HTTPHandler returns http handler for subscription endpoints
func HTTPHandler(service *Service) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var req CreateSubscriptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		sub, err := service.Create(ctx, req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sub)
	})

	r.Post("/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := chi.URLParam(r, "id")

		var body struct {
			AtPeriodEnd bool `json:"at_period_end"`
		}
		json.NewDecoder(r.Body).Decode(&body)

		sub, err := service.Cancel(ctx, id, body.AtPeriodEnd)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sub)
	})

	return r
}

// calculateNextAttempt returns next attempt timestamp for dunning
func calculateNextAttempt(attempt int) int64 {
	delays := []time.Duration{
		3 * 24 * time.Hour,   // 3 days
		7 * 24 * time.Hour,   // 7 days
		14 * 24 * time.Hour,  // 14 days
	}

	if attempt-1 >= len(delays) {
		attempt = len(delays)
	}

	return time.Now().Add(delays[attempt-1]).Unix()
}