// Package subscription bills a fixed amount to a payment method at the start of each period.
//
// Each charge is a payment intent tagged with the subscription and the period it pays for, so a
// retry after a crash finds the intent it already created and never charges the same period twice.
// A failed charge moves the subscription to past_due and is retried on RetrySchedule. After the
// last retry the subscription is canceled. Subscriptions are sandbox-only until a live billing
// policy is decided.
package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// Status is the state of a subscription.
type Status string

const (
	StatusActive   Status = "active"
	StatusPastDue  Status = "past_due"
	StatusCanceled Status = "canceled"
)

// Interval is the length of one billing period.
type Interval string

const (
	IntervalWeek  Interval = "week"
	IntervalMonth Interval = "month"
	IntervalYear  Interval = "year"
)

// RetrySchedule is how long to wait before each retry of a failed charge. The first failure
// waits RetrySchedule[0]; after the last retry fails, the subscription is canceled. The total
// window is five days.
var RetrySchedule = []time.Duration{24 * time.Hour, 48 * time.Hour, 48 * time.Hour}

// leaseDuration hides a subscription that a billing run has claimed, so two runs never charge the
// same period at once. If a run dies, the lease expires and the next run picks the subscription up.
const leaseDuration = 2 * time.Minute

// Cancel reasons recorded on a subscription.
const (
	CancelReasonPaymentFailed = "payment_failed"
	CancelReasonRequested     = "requested"
	CancelReasonAtPeriodEnd   = "canceled_at_period_end"
)

// Subscription is a recurring charge. PeriodStart and PeriodEnd are the period the next charge
// pays for, and that charge is due at NextAttemptAt (the period start, or a retry time).
type Subscription struct {
	ID                string
	MerchantID        string
	Status            Status
	Amount            int64
	Currency          string
	Interval          Interval
	PaymentMethodID   string
	PeriodStart       time.Time
	PeriodEnd         time.Time
	NextAttemptAt     time.Time
	RetryCount        int
	CancelAtPeriodEnd bool
	CancelReason      string
	CanceledAt        *time.Time
	Livemode          bool
	Metadata          map[string]string
	CreatedAt         time.Time
}

// Scope is the caller's merchant and mode, the same as for payment intents.
type Scope = payment.Scope

// Service creates, reads, cancels and bills subscriptions.
type Service struct {
	pool     *pgxpool.Pool
	payments *payment.Service
	clk      clock.Clock
}

// NewService returns a subscription service that charges through payments and reads time from clk.
func NewService(pool *pgxpool.Pool, payments *payment.Service, clk clock.Clock) *Service {
	return &Service{pool: pool, payments: payments, clk: clk}
}

// CreateParams are the parameters for a new subscription.
type CreateParams struct {
	Amount          int64
	Currency        string
	Interval        Interval
	PaymentMethodID string
	Metadata        map[string]string
}

// Create starts a subscription. The first period is charged before Create returns. If that charge
// fails, the subscription is returned as past_due and retried on the schedule.
func (s *Service) Create(ctx context.Context, scope Scope, p CreateParams) (*Subscription, error) {
	if scope.Livemode {
		// Recurring charges to real cards need a billing policy (retry schedule, dunning, customer
		// notices) that has not been decided. Until it is, only test mode may create subscriptions.
		return nil, apierrors.PermissionDenied("Subscriptions are not available in live mode yet. Use a test key (sk_test_...).")
	}
	if p.Currency == "" {
		return nil, apierrors.MissingParam("currency")
	}
	code, err := payment.NormalizeCurrency(p.Currency)
	if err != nil {
		return nil, err
	}
	if err := payment.ValidateAmount(p.Amount, code); err != nil {
		return nil, err
	}
	if err := ValidInterval(p.Interval); err != nil {
		return nil, err
	}
	if p.PaymentMethodID == "" {
		return nil, apierrors.MissingParam("payment_method")
	}
	if err := payment.ValidateMetadata(p.Metadata); err != nil {
		return nil, err
	}

	now := s.clk.Now()
	id := "sub_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	sub := &Subscription{
		ID: id, MerchantID: scope.MerchantID, Status: StatusActive,
		Amount: p.Amount, Currency: code, Interval: p.Interval, PaymentMethodID: p.PaymentMethodID,
		PeriodStart: now, PeriodEnd: AddInterval(now, p.Interval), NextAttemptAt: now,
		Livemode: scope.Livemode, Metadata: p.Metadata, CreatedAt: now,
	}
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO subscriptions
				(id, merchant_id, status, amount, currency, billing_interval, payment_method_id,
				 current_period_start, current_period_end, next_attempt_at, livemode, metadata, created_at, updated_at)
			VALUES ($1, $2, 'active', $3, $4, $5, $6, $7, $8, $9, $10, $11, $7, $7)
		`, id, scope.MerchantID, p.Amount, code, string(p.Interval), p.PaymentMethodID,
			sub.PeriodStart, sub.PeriodEnd, sub.NextAttemptAt, scope.Livemode, metadataJSON(p.Metadata)); err != nil {
			return fmt.Errorf("insert subscription: %w", err)
		}
		return enqueue(ctx, tx, sub, webhook.EventSubscriptionCreated)
	})
	if err != nil {
		return nil, err
	}
	return s.Bill(ctx, id)
}

// Retrieve returns a subscription in the caller's merchant and mode.
func (s *Service) Retrieve(ctx context.Context, scope Scope, id string) (*Subscription, error) {
	sub, err := scanSubscription(s.pool.QueryRow(ctx, selectColumns+`
		WHERE id = $1 AND merchant_id = $2 AND livemode = $3
	`, id, scope.MerchantID, scope.Livemode))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierrors.NotFound("subscription", id)
	}
	return sub, err
}

// ListParams page through the caller's subscriptions, newest first.
type ListParams struct {
	Scope         Scope
	Limit         int
	StartingAfter string
}

// List returns one page of subscriptions and whether another page exists.
func (s *Service) List(ctx context.Context, p ListParams) ([]*Subscription, bool, error) {
	if p.Limit < 1 || p.Limit > 100 {
		return nil, false, apierrors.InvalidParam("limit", "limit must be between 1 and 100")
	}
	var cursorAt, cursorID any
	if p.StartingAfter != "" {
		sub, err := s.Retrieve(ctx, p.Scope, p.StartingAfter)
		if err != nil {
			return nil, false, apierrors.InvalidParam("starting_after", "no subscription with that id")
		}
		cursorAt, cursorID = sub.CreatedAt, sub.ID
	}
	rows, err := s.pool.Query(ctx, selectColumns+`
		WHERE merchant_id = $1 AND livemode = $2
		  AND ($3::timestamptz IS NULL OR (created_at, id) < ($3::timestamptz, $4::text))
		ORDER BY created_at DESC, id DESC
		LIMIT $5
	`, p.Scope.MerchantID, p.Scope.Livemode, cursorAt, cursorID, p.Limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()
	var out []*Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > p.Limit
	if hasMore {
		out = out[:p.Limit]
	}
	return out, hasMore, nil
}

// Cancel ends a subscription now, or at the end of the current period when atPeriodEnd is set.
// A canceled subscription cannot be canceled again.
func (s *Service) Cancel(ctx context.Context, scope Scope, id string, atPeriodEnd bool) (*Subscription, error) {
	var out *Subscription
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		sub, err := scanSubscription(tx.QueryRow(ctx, selectColumns+`
			WHERE id = $1 AND merchant_id = $2 AND livemode = $3 FOR UPDATE
		`, id, scope.MerchantID, scope.Livemode))
		if errors.Is(err, pgx.ErrNoRows) {
			return apierrors.NotFound("subscription", id)
		}
		if err != nil {
			return err
		}
		if sub.Status == StatusCanceled {
			return apierrors.InvalidParam("id", "the subscription is already canceled")
		}
		if atPeriodEnd {
			sub.CancelAtPeriodEnd = true
		} else {
			now := s.clk.Now()
			sub.Status = StatusCanceled
			sub.CanceledAt = &now
			sub.CancelReason = CancelReasonRequested
			if err := enqueue(ctx, tx, sub, webhook.EventSubscriptionCanceled); err != nil {
				return err
			}
		}
		if err := write(ctx, tx, sub); err != nil {
			return err
		}
		out = sub
		return nil
	})
	return out, err
}

// ValidInterval reports whether an interval is one of week, month or year.
func ValidInterval(i Interval) error {
	switch i {
	case IntervalWeek, IntervalMonth, IntervalYear:
		return nil
	}
	return apierrors.InvalidParam("interval", "must be one of week, month or year")
}

// AddInterval returns the end of a period that starts at start. Month and year periods follow the
// calendar, so a period starting on 31 January ends on 28 February (or 29 in a leap year).
func AddInterval(start time.Time, i Interval) time.Time {
	switch i {
	case IntervalYear:
		return start.AddDate(1, 0, 0)
	case IntervalMonth:
		return start.AddDate(0, 1, 0)
	default:
		return start.AddDate(0, 0, 7)
	}
}

// columns is the projection every subscription read and claim returns, in scanSubscription's order.
const columns = `id, merchant_id, status, amount, currency, billing_interval, payment_method_id,
	current_period_start, current_period_end, next_attempt_at, retry_count,
	cancel_at_period_end, COALESCE(cancel_reason, ''), canceled_at, livemode, metadata, created_at`

const selectColumns = "SELECT " + columns + " FROM subscriptions "

type scanner interface {
	Scan(dest ...any) error
}

func scanSubscription(row scanner) (*Subscription, error) {
	var sub Subscription
	var status, interval string
	var metadata map[string]string
	err := row.Scan(&sub.ID, &sub.MerchantID, &status, &sub.Amount, &sub.Currency, &interval,
		&sub.PaymentMethodID, &sub.PeriodStart, &sub.PeriodEnd, &sub.NextAttemptAt, &sub.RetryCount,
		&sub.CancelAtPeriodEnd, &sub.CancelReason, &sub.CanceledAt, &sub.Livemode, &metadata, &sub.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("scan subscription: %w", err)
	}
	sub.Status = Status(status)
	sub.Interval = Interval(interval)
	sub.Metadata = metadata
	return &sub, nil
}

// write stores every mutable field of sub.
func write(ctx context.Context, tx pgx.Tx, sub *Subscription) error {
	_, err := tx.Exec(ctx, `
		UPDATE subscriptions
		SET status = $2, current_period_start = $3, current_period_end = $4, next_attempt_at = $5,
		    retry_count = $6, cancel_at_period_end = $7, cancel_reason = $8, canceled_at = $9,
		    updated_at = NOW()
		WHERE id = $1
	`, sub.ID, string(sub.Status), sub.PeriodStart, sub.PeriodEnd, sub.NextAttemptAt, sub.RetryCount,
		sub.CancelAtPeriodEnd, nullable(sub.CancelReason), sub.CanceledAt)
	if err != nil {
		return fmt.Errorf("update subscription: %w", err)
	}
	return nil
}

// enqueue queues a webhook event describing sub's current state, in the same transaction as the
// change that caused it.
func enqueue(ctx context.Context, tx pgx.Tx, sub *Subscription, ev webhook.EventType) error {
	_, err := webhook.Enqueue(ctx, tx, sub.MerchantID, sub.Livemode, ev, Wire(sub))
	return err
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func metadataJSON(m map[string]string) any {
	if m == nil {
		return nil
	}
	return m
}

// Wire is the JSON object for a subscription.
func Wire(sub *Subscription) map[string]any {
	out := map[string]any{
		"id":                   sub.ID,
		"object":               "subscription",
		"status":               string(sub.Status),
		"amount":               sub.Amount,
		"currency":             strings.ToLower(sub.Currency),
		"interval":             string(sub.Interval),
		"payment_method":       sub.PaymentMethodID,
		"current_period_start": sub.PeriodStart.Unix(),
		"current_period_end":   sub.PeriodEnd.Unix(),
		"cancel_at_period_end": sub.CancelAtPeriodEnd,
		"livemode":             sub.Livemode,
		"created":              sub.CreatedAt.Unix(),
	}
	if sub.CanceledAt != nil {
		out["canceled_at"] = sub.CanceledAt.Unix()
	}
	if sub.CancelReason != "" {
		out["cancel_reason"] = sub.CancelReason
	}
	if sub.Metadata != nil {
		out["metadata"] = sub.Metadata
	}
	return out
}
