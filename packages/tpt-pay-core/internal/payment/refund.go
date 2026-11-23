package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// RefundStatus is the lifecycle state of a refund.
type RefundStatus string

const (
	// RefundPending: reserved against the refundable balance; the network outcome is not
	// yet known. A pending refund counts against the balance, so concurrent refunds cannot
	// exceed what was captured.
	RefundPending   RefundStatus = "pending"
	RefundSucceeded RefundStatus = "succeeded"
	RefundFailed    RefundStatus = "failed"
)

// Refund reasons accepted from clients. They match the adapter's vocabulary.
var validRefundReasons = map[string]bool{
	"duplicate":             true,
	"fraudulent":            true,
	"requested_by_customer": true,
}

// Refund is a Refund resource.
type Refund struct {
	ID              string
	PaymentIntentID string
	MerchantID      string
	Livemode        bool
	Amount          int64
	Currency        string
	Status          RefundStatus
	Reason          string
	AdapterRefundID string
	LedgerTxnID     string
	FailureMessage  string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// CreateRefundParams are the inputs to Service.CreateRefund.
type CreateRefundParams struct {
	PaymentIntentID string
	// Amount in minor units. Zero refunds the whole remaining refundable balance.
	Amount int64
	Reason string
}

// refundInvalidState is reported when a refund targets an intent that is not succeeded.
func refundInvalidState(status Status) *apierrors.APIError {
	return &apierrors.APIError{
		StatusCode: 400,
		Type:       apierrors.ErrorTypeInvalidRequest,
		Code:       apierrors.CodeIntentInvalidState,
		Message: fmt.Sprintf(
			"This PaymentIntent's status (%s) does not support refund; only succeeded payments can be refunded",
			status,
		),
	}
}

func chargeAlreadyRefunded() *apierrors.APIError {
	return &apierrors.APIError{
		StatusCode: 400,
		Type:       apierrors.ErrorTypeCardError,
		Code:       "charge_already_refunded",
		Message:    "The charge has already been refunded in full.",
	}
}

// CreateRefund refunds all or part of a captured payment.
//
// Three phases keep the database out of network I/O while still preventing over-refunds:
//  1. In one transaction, lock the intent row, check the refundable balance, and insert a
//     pending refund. The row lock serializes concurrent refunds on the same intent.
//  2. Call the card network with no transaction open.
//  3. In one transaction, mark the refund succeeded (or failed) and post the reversal to
//     the ledger, which is idempotent per refund.
//
// A definitive decline marks the refund failed. Any other adapter failure leaves it pending
// and returns an error, so the caller must reconcile rather than retry blindly.
func (s *Service) CreateRefund(ctx context.Context, scope Scope, params CreateRefundParams) (*Refund, error) {
	if params.Amount < 0 {
		return nil, apierrors.InvalidParam("amount", "amount must be greater than zero")
	}
	if params.Reason != "" && !validRefundReasons[params.Reason] {
		return nil, apierrors.InvalidParam("reason",
			"reason must be 'duplicate', 'fraudulent' or 'requested_by_customer'")
	}

	refundID := "re_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	var (
		chargeID string
		amount   int64
	)

	// Phase 1: reserve.
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var (
			status   Status
			captured int64
			currency string
			merchant string
			livemode bool
		)
		err := tx.QueryRow(ctx, `
			SELECT status, COALESCE(amount_captured, 0), currency, merchant_id::text, livemode,
			       COALESCE(adapter_charge_id, '')
			FROM payment_intents
			WHERE id = $1
			FOR UPDATE
		`, params.PaymentIntentID).Scan(&status, &captured, &currency, &merchant, &livemode, &chargeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apierrors.NotFound("payment_intent", params.PaymentIntentID)
		}
		if err != nil {
			return fmt.Errorf("lock payment intent: %w", err)
		}
		if merchant != scope.MerchantID || livemode != scope.Livemode {
			return apierrors.NotFound("payment_intent", params.PaymentIntentID)
		}
		if status != StatusSucceeded || chargeID == "" {
			return refundInvalidState(status)
		}

		var reserved int64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(SUM(amount), 0) FROM refunds
			WHERE payment_intent_id = $1 AND status IN ('pending', 'succeeded')
		`, params.PaymentIntentID).Scan(&reserved); err != nil {
			return fmt.Errorf("sum refunds: %w", err)
		}
		refundable := captured - reserved

		amount = params.Amount
		if amount == 0 {
			amount = refundable
		}
		if refundable <= 0 {
			return chargeAlreadyRefunded()
		}
		if amount > refundable {
			return apierrors.InvalidParam("amount", fmt.Sprintf(
				"amount exceeds the refundable balance of %d", refundable))
		}

		if _, err := tx.Exec(ctx, `
			INSERT INTO refunds
				(id, payment_intent_id, merchant_id, livemode, amount, currency, status, reason)
			VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7)
		`, refundID, params.PaymentIntentID, merchant, livemode, amount, currency,
			nullString(params.Reason)); err != nil {
			return fmt.Errorf("insert refund: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Phase 2: network. No transaction is open here.
	resp, err := s.adapter.Refund(ctx, adapter.RefundRequest{
		ChargeID: chargeID,
		Amount:   amount,
		Reason:   params.Reason,
	})
	if err != nil {
		var cardErr *adapter.CardError
		if errors.As(err, &cardErr) {
			s.failRefund(ctx, refundID, cardErr.Message)
			return s.RetrieveRefund(ctx, refundID, scope)
		}
		slog.Error("refund outcome unknown", "refund", refundID, "err", err)
		return nil, refundOutcomeUnknown()
	}

	// Phase 3: settle.
	if err := s.settleRefund(ctx, refundID, resp.RefundID); err != nil {
		slog.Error("refund succeeded at the network but was not recorded", "refund", refundID,
			"adapter_refund_id", resp.RefundID, "err", err)
		return nil, refundOutcomeUnknown()
	}
	return s.RetrieveRefund(ctx, refundID, scope)
}

// RetrieveRefund fetches a refund. Refunds outside the caller's merchant or mode are not
// found.
func (s *Service) RetrieveRefund(ctx context.Context, id string, scope Scope) (*Refund, error) {
	r, err := getRefundWith(ctx, s.pool, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierrors.NotFound("refund", id)
	}
	if err != nil {
		return nil, err
	}
	if r.MerchantID != scope.MerchantID || r.Livemode != scope.Livemode {
		return nil, apierrors.NotFound("refund", id)
	}
	return r, nil
}

// settleRefund marks a pending refund succeeded, posts the reversal to the ledger, and
// emits charge.refunded, all in one transaction.
func (s *Service) settleRefund(ctx context.Context, refundID, adapterRefundID string) error {
	bg := context.WithoutCancel(ctx)
	return pgx.BeginFunc(bg, s.pool, func(tx pgx.Tx) error {
		var (
			r             Refund
			merchantIDStr string
		)
		err := tx.QueryRow(bg, `
			UPDATE refunds
			SET status = 'succeeded', adapter_refund_id = $2, updated_at = NOW()
			WHERE id = $1 AND status = 'pending'
			RETURNING payment_intent_id, merchant_id::text, livemode, amount, currency
		`, refundID, adapterRefundID).Scan(
			&r.PaymentIntentID, &merchantIDStr, &r.Livemode, &r.Amount, &r.Currency,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("refund %s was not pending", refundID)
		}
		if err != nil {
			return fmt.Errorf("mark refund succeeded: %w", err)
		}
		merchantID, err := uuid.Parse(merchantIDStr)
		if err != nil {
			return fmt.Errorf("invalid merchant id: %w", err)
		}

		// The merchant owes the refunded amount back, so their account is debited and the
		// clearing account is credited: the mirror image of the capture posting.
		merchantAcct, err := s.ledger.EnsureAccountTx(bg, tx, merchantID, r.Currency)
		if err != nil {
			return err
		}
		clearingAcct, err := s.ledger.EnsureAccountTx(bg, tx, PlatformMerchantID, r.Currency)
		if err != nil {
			return err
		}
		txn, _, err := s.ledger.PostTransactionTx(bg, tx, ledger.TxnTypeRefund, refundID, []ledger.Entry{
			{AccountID: merchantAcct, Amount: -r.Amount, Currency: r.Currency, Description: "refund: " + refundID},
			{AccountID: clearingAcct, Amount: r.Amount, Currency: r.Currency, Description: "clearing refund: " + refundID},
		})
		if err != nil {
			return fmt.Errorf("post refund ledger transaction: %w", err)
		}
		if _, err := tx.Exec(bg,
			`UPDATE refunds SET ledger_txn_id = $1 WHERE id = $2`, txn.ID, refundID); err != nil {
			return fmt.Errorf("link refund ledger transaction: %w", err)
		}

		refunded, err := getRefundWith(bg, tx, refundID)
		if err != nil {
			return err
		}
		_, err = webhook.Enqueue(bg, tx, refunded.MerchantID, refunded.Livemode,
			webhook.EventChargeRefunded, RefundWire(refunded))
		return err
	})
}

// failRefund records a definitive decline from the network. No ledger entry is posted.
func (s *Service) failRefund(ctx context.Context, refundID, message string) {
	bg := context.WithoutCancel(ctx)
	if _, err := s.pool.Exec(bg, `
		UPDATE refunds SET status = 'failed', failure_message = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
	`, refundID, message); err != nil {
		slog.Error("failed to record refund decline", "refund", refundID, "err", err)
	}
}

// getRefundWith loads a refund by ID. It returns pgx.ErrNoRows (wrapped) when absent.
func getRefundWith(ctx context.Context, q queryer, id string) (*Refund, error) {
	var r Refund
	err := q.QueryRow(ctx, `
		SELECT id, payment_intent_id, merchant_id::text, livemode, amount, currency, status,
		       COALESCE(reason, ''), COALESCE(adapter_refund_id, ''),
		       COALESCE(ledger_txn_id::text, ''), COALESCE(failure_message, ''),
		       created_at, updated_at
		FROM refunds WHERE id = $1
	`, id).Scan(
		&r.ID, &r.PaymentIntentID, &r.MerchantID, &r.Livemode, &r.Amount, &r.Currency, &r.Status,
		&r.Reason, &r.AdapterRefundID, &r.LedgerTxnID, &r.FailureMessage, &r.CreatedAt, &r.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get refund: %w", err)
	}
	return &r, nil
}

// RefundWire returns the Stripe-compatible representation of a refund. Currency uses the
// same upper-case form as the payment intent wire format.
func RefundWire(r *Refund) map[string]any {
	out := map[string]any{
		"id":             r.ID,
		"object":         "refund",
		"amount":         r.Amount,
		"currency":       r.Currency,
		"payment_intent": r.PaymentIntentID,
		"status":         string(r.Status),
		"created":        r.CreatedAt.Unix(),
		"livemode":       r.Livemode,
	}
	if r.Reason != "" {
		out["reason"] = r.Reason
	}
	if r.LedgerTxnID != "" {
		out["ledger_transaction_id"] = r.LedgerTxnID
	}
	if r.FailureMessage != "" {
		out["failure_message"] = r.FailureMessage
	}
	return out
}

func refundOutcomeUnknown() error {
	return &apierrors.APIError{
		StatusCode: 502,
		Type:       apierrors.ErrorTypeAPIError,
		Code:       apierrors.ErrorCode("processing_outcome_unknown"),
		Message: "The refund did not return a definitive result. The refund is left pending " +
			"and must be reconciled before retrying.",
	}
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
