package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
)

// RefundListParams selects one page of a merchant's refunds.
type RefundListParams struct {
	Scope Scope
	// Limit is the page size, between 1 and MaxListLimit. Zero means DefaultListLimit.
	Limit int
	// StartingAfter is the ID of the last refund on the previous page. Empty for the first page.
	StartingAfter string
	// Status filters by refund status. Empty means every status.
	Status RefundStatus
	// PaymentIntentID limits the list to the refunds of one payment intent. Empty means all.
	PaymentIntentID string
}

// RefundListPage is one page of refunds, newest first.
type RefundListPage struct {
	Refunds []*Refund
	HasMore bool
}

var knownRefundStatuses = map[RefundStatus]bool{
	RefundPending:   true,
	RefundSucceeded: true,
	RefundFailed:    true,
}

// ValidateRefundListParams normalizes and checks refund list parameters. Anything out of range is
// a client error (400).
func ValidateRefundListParams(p RefundListParams) (RefundListParams, error) {
	if p.Limit == 0 {
		p.Limit = DefaultListLimit
	}
	if p.Limit < 1 || p.Limit > MaxListLimit {
		return p, apierrors.InvalidParam("limit", fmt.Sprintf("limit must be between 1 and %d", MaxListLimit))
	}
	if p.Status != "" && !knownRefundStatuses[p.Status] {
		return p, apierrors.InvalidParam("status", "status is not a refund status")
	}
	return p, nil
}

// ListRefunds returns one page of the scope's refunds, newest first. Like ListIntents, pages are
// keyset based, so a refund created while paging does not shift the results.
func (s *Service) ListRefunds(ctx context.Context, p RefundListParams) (RefundListPage, error) {
	p, err := ValidateRefundListParams(p)
	if err != nil {
		return RefundListPage{}, err
	}

	// The cursor is resolved within the caller's scope. A refund from another merchant or mode is
	// reported as an invalid cursor, exactly as a direct lookup reports it missing.
	var cursorAt any
	var cursorID any
	if p.StartingAfter != "" {
		var createdAt time.Time
		var id string
		err := s.pool.QueryRow(ctx, `
			SELECT created_at, id FROM refunds
			WHERE id = $1 AND merchant_id = $2 AND livemode = $3
		`, p.StartingAfter, p.Scope.MerchantID, p.Scope.Livemode).Scan(&createdAt, &id)
		if err == pgx.ErrNoRows {
			return RefundListPage{}, apierrors.InvalidParam("starting_after", "no refund with that id")
		}
		if err != nil {
			return RefundListPage{}, fmt.Errorf("resolve refund cursor: %w", err)
		}
		cursorAt, cursorID = createdAt, id
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, payment_intent_id, merchant_id::text, livemode, amount, currency, status,
		       COALESCE(reason, ''), COALESCE(adapter_refund_id, ''),
		       COALESCE(ledger_txn_id::text, ''), COALESCE(failure_message, ''),
		       created_at, updated_at
		FROM refunds
		WHERE merchant_id = $1
		  AND livemode = $2
		  AND ($3::text = '' OR status = $3)
		  AND ($4::text = '' OR payment_intent_id = $4)
		  AND ($5::timestamptz IS NULL
		       OR (created_at, id) < ($5::timestamptz, $6::text))
		ORDER BY created_at DESC, id DESC
		LIMIT $7
	`, p.Scope.MerchantID, p.Scope.Livemode, string(p.Status), p.PaymentIntentID,
		cursorAt, cursorID, p.Limit+1)
	if err != nil {
		return RefundListPage{}, fmt.Errorf("list refunds: %w", err)
	}
	defer rows.Close()

	var page RefundListPage
	for rows.Next() {
		var r Refund
		if err := rows.Scan(
			&r.ID, &r.PaymentIntentID, &r.MerchantID, &r.Livemode, &r.Amount, &r.Currency, &r.Status,
			&r.Reason, &r.AdapterRefundID, &r.LedgerTxnID, &r.FailureMessage,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return RefundListPage{}, fmt.Errorf("scan refund: %w", err)
		}
		page.Refunds = append(page.Refunds, &r)
	}
	if err := rows.Err(); err != nil {
		return RefundListPage{}, err
	}
	if len(page.Refunds) > p.Limit {
		page.HasMore = true
		page.Refunds = page.Refunds[:p.Limit]
	}
	return page, nil
}
