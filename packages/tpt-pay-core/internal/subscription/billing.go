package subscription

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// RunDue bills every subscription whose charge is due at the service's clock time, at most limit
// of them, and returns how many it attempted. A subscription that fails is not an error here; it is
// recorded as past_due and retried later.
func (s *Service) RunDue(ctx context.Context, limit int) (int, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id FROM subscriptions
		WHERE status <> 'canceled' AND next_attempt_at <= $1
		ORDER BY next_attempt_at
		LIMIT $2
	`, s.clk.Now(), limit)
	if err != nil {
		return 0, fmt.Errorf("find due subscriptions: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan due subscription: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	attempted := 0
	for _, id := range ids {
		if _, err := s.Bill(ctx, id); err != nil {
			slog.Error("subscription billing failed", "subscription_id", id, "err", err)
			continue
		}
		attempted++
	}
	return attempted, nil
}

// Bill runs the billing step for one subscription if it is due. It returns the subscription as it
// is afterwards. A subscription that is not due, or is leased by another run, is returned unchanged.
func (s *Service) Bill(ctx context.Context, id string) (*Subscription, error) {
	now := s.clk.Now()
	// Claim: hide the subscription for the lease duration. Only one run can win the claim.
	sub, err := scanSubscription(s.pool.QueryRow(ctx, `
		UPDATE subscriptions SET next_attempt_at = $2
		WHERE id = $1 AND status <> 'canceled' AND next_attempt_at <= $3
		RETURNING `+columns, id, now.Add(leaseDuration), now))
	if errors.Is(err, pgx.ErrNoRows) {
		return s.load(ctx, id)
	}
	if err != nil {
		return nil, err
	}

	// Cancel at period end takes effect when the period that was paid ends, which is when the next
	// charge would be due. Nothing is charged.
	if sub.CancelAtPeriodEnd {
		sub.Status = StatusCanceled
		sub.CancelReason = CancelReasonAtPeriodEnd
		sub.CanceledAt = &now
		return sub, s.commit(ctx, sub, webhook.EventSubscriptionCanceled)
	}

	charged, chargeErr := s.charge(ctx, sub)
	if charged {
		return sub, s.succeed(ctx, sub)
	}
	if chargeErr != nil {
		slog.Info("subscription charge did not succeed", "subscription_id", sub.ID, "err", chargeErr)
	}
	return sub, s.fail(ctx, sub, now, chargeErr)
}

// succeed moves the subscription to the next period. The charge it just paid was for the period
// that started at PeriodStart; the next one starts where this one ends.
func (s *Service) succeed(ctx context.Context, sub *Subscription) error {
	sub.PeriodStart = sub.PeriodEnd
	sub.PeriodEnd = AddInterval(sub.PeriodStart, sub.Interval)
	sub.NextAttemptAt = sub.PeriodStart
	sub.RetryCount = 0
	sub.Status = StatusActive
	return s.commit(ctx, sub, "")
}

// fail records a failed charge. It schedules a retry, or cancels the subscription once the
// retries are used up. An unknown outcome is different: the money may already have moved, so it
// is not retried or counted. The subscription is re-checked daily until the intent is reconciled.
func (s *Service) fail(ctx context.Context, sub *Subscription, now time.Time, cause error) error {
	if errors.Is(cause, errUnknownOutcome) {
		var ev webhook.EventType
		if sub.Status == StatusActive {
			ev = webhook.EventSubscriptionPastDue
		}
		sub.Status = StatusPastDue
		sub.NextAttemptAt = now.Add(unknownOutcomeRecheck)
		return s.commit(ctx, sub, ev)
	}
	sub.RetryCount++
	if sub.RetryCount > len(RetrySchedule) {
		sub.Status = StatusCanceled
		sub.CancelReason = CancelReasonPaymentFailed
		sub.CanceledAt = &now
		return s.commit(ctx, sub, webhook.EventSubscriptionCanceled)
	}
	var ev webhook.EventType
	if sub.Status == StatusActive {
		ev = webhook.EventSubscriptionPastDue
	}
	sub.Status = StatusPastDue
	sub.NextAttemptAt = now.Add(RetrySchedule[sub.RetryCount-1])
	return s.commit(ctx, sub, ev)
}

// errUnknownOutcome means the card network did not answer, so the period's intent may or may not
// have been charged. Only reconciliation can settle it, so the billing run must not try again.
var errUnknownOutcome = errors.New("subscription: the charge outcome is unknown and awaits reconciliation")

// unknownOutcomeRecheck is how long to wait before looking at an unresolved charge again.
const unknownOutcomeRecheck = 24 * time.Hour

// charge pays for the subscription's current period. It is safe to call again for the same period:
// the intent created for the period is found and confirmed again, not duplicated.
func (s *Service) charge(ctx context.Context, sub *Subscription) (bool, error) {
	scope := Scope{MerchantID: sub.MerchantID, Livemode: sub.Livemode}
	periodKey := sub.PeriodStart.UTC().Format(time.RFC3339Nano)

	intentID, status, err := s.periodIntent(ctx, sub, periodKey)
	if err != nil {
		return false, err
	}
	if intentID == "" {
		intent, err := s.payments.Create(ctx, scope, payment.CreateParams{
			Amount:        sub.Amount,
			Currency:      sub.Currency,
			CaptureMethod: payment.CaptureAutomatic,
			Metadata:      map[string]string{"subscription_id": sub.ID, "period_start": periodKey},
		})
		if err != nil {
			return false, err
		}
		intentID, status = intent.ID, intent.Status
	}
	if status == payment.StatusSucceeded {
		return true, nil
	}
	if status == payment.StatusProcessing {
		return false, errUnknownOutcome
	}

	confirmed, err := s.payments.Confirm(ctx, intentID, scope, payment.ConfirmParams{PaymentMethodID: sub.PaymentMethodID})
	if err != nil {
		var apiErr *apierrors.APIError
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusBadGateway {
			// The payment service reports this when the network gave no answer.
			return false, errUnknownOutcome
		}
		return false, err
	}
	return confirmed.Status == payment.StatusSucceeded, nil
}

// periodIntent finds the payment intent already created for this subscription and period.
func (s *Service) periodIntent(ctx context.Context, sub *Subscription, periodKey string) (string, payment.Status, error) {
	var id string
	var status string
	err := s.pool.QueryRow(ctx, `
		SELECT id, status FROM payment_intents
		WHERE merchant_id = $1 AND livemode = $2
		  AND metadata->>'subscription_id' = $3
		  AND metadata->>'period_start' = $4
		ORDER BY created_at
		LIMIT 1
	`, sub.MerchantID, sub.Livemode, sub.ID, periodKey).Scan(&id, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("find period payment: %w", err)
	}
	return id, payment.Status(status), nil
}

// commit writes the subscription's new state and, if ev is set, the event about it, in one
// transaction. The row is locked first. If the subscription was canceled while this run was
// charging, the cancellation stands and nothing is written. A charge that was already in flight
// is not reversed; cancelling stops later charges, not one that has started.
func (s *Service) commit(ctx context.Context, sub *Subscription, ev webhook.EventType) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var current string
		if err := tx.QueryRow(ctx, `SELECT status FROM subscriptions WHERE id = $1 FOR UPDATE`, sub.ID).Scan(&current); err != nil {
			return fmt.Errorf("lock subscription: %w", err)
		}
		if Status(current) == StatusCanceled {
			return nil
		}
		if err := write(ctx, tx, sub); err != nil {
			return err
		}
		if ev != "" {
			return enqueue(ctx, tx, sub, ev)
		}
		return nil
	})
}

// load reads a subscription by ID without scope. It is used only by the billing run.
func (s *Service) load(ctx context.Context, id string) (*Subscription, error) {
	sub, err := scanSubscription(s.pool.QueryRow(ctx, `SELECT `+columns+` FROM subscriptions WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierrors.NotFound("subscription", id)
	}
	return sub, err
}
