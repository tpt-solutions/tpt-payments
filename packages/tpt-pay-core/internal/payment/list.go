package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
)

// Pagination limits for list endpoints.
const (
	DefaultListLimit = 10
	MaxListLimit     = 100
)

// ListParams selects one page of a merchant's payment intents.
type ListParams struct {
	Scope Scope
	// Limit is the page size, between 1 and MaxListLimit. Zero means DefaultListLimit.
	Limit int
	// StartingAfter is the ID of the last intent on the previous page. Empty for the first page.
	StartingAfter string
	// Status filters by status. Empty means every status.
	Status Status
}

// ListPage is one page of results, newest first.
type ListPage struct {
	Intents []*Intent
	HasMore bool
}

// knownStatuses is the set of values a status filter may take.
var knownStatuses = map[Status]bool{
	StatusRequiresPaymentMethod: true,
	StatusRequiresConfirmation:  true,
	StatusRequiresAction:        true,
	StatusProcessing:            true,
	StatusRequiresCapture:       true,
	StatusSucceeded:             true,
	StatusCanceled:              true,
}

// ValidateListParams normalizes and checks list parameters. It returns a client error (400) for
// anything out of range, so the database is never asked for a page it cannot serve.
func ValidateListParams(p ListParams) (ListParams, error) {
	if p.Limit == 0 {
		p.Limit = DefaultListLimit
	}
	if p.Limit < 1 || p.Limit > MaxListLimit {
		return p, apierrors.InvalidParam("limit", fmt.Sprintf("limit must be between 1 and %d", MaxListLimit))
	}
	if p.Status != "" && !knownStatuses[p.Status] {
		return p, apierrors.InvalidParam("status", "status is not a payment intent status")
	}
	return p, nil
}

// ListIntents returns one page of the scope's payment intents, newest first. Pages are keyset
// based: the cursor is a row position, so an intent created during paging does not shift results.
func (s *Service) ListIntents(ctx context.Context, p ListParams) (ListPage, error) {
	p, err := ValidateListParams(p)
	if err != nil {
		return ListPage{}, err
	}

	// The cursor is resolved within the caller's scope. An intent from another merchant or mode
	// is reported as missing, exactly as a direct lookup would.
	var cursorAt any
	var cursorID any
	if p.StartingAfter != "" {
		var createdAt time.Time
		var id string
		err := s.pool.QueryRow(ctx, `
			SELECT created_at, id FROM payment_intents
			WHERE id = $1 AND merchant_id = $2 AND livemode = $3
		`, p.StartingAfter, p.Scope.MerchantID, p.Scope.Livemode).Scan(&createdAt, &id)
		if err == pgx.ErrNoRows {
			return ListPage{}, apierrors.InvalidParam("starting_after", "no payment intent with that id")
		}
		if err != nil {
			return ListPage{}, fmt.Errorf("resolve cursor: %w", err)
		}
		cursorAt, cursorID = createdAt, id
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, merchant_id, amount, currency, status, capture_method, confirmation_method,
		       COALESCE(payment_method_id, ''), COALESCE(adapter_charge_id, ''),
		       COALESCE(amount_captured, 0), canceled_at,
		       COALESCE(cancellation_reason, ''), COALESCE(ledger_txn_id::text, ''),
		       livemode, metadata, created_at, updated_at
		FROM payment_intents
		WHERE merchant_id = $1
		  AND livemode = $2
		  AND ($3::text = '' OR status = $3)
		  AND ($4::timestamptz IS NULL
		       OR (created_at, id) < ($4::timestamptz, $5::text))
		ORDER BY created_at DESC, id DESC
		LIMIT $6
	`, p.Scope.MerchantID, p.Scope.Livemode, string(p.Status), cursorAt, cursorID, p.Limit+1)
	if err != nil {
		return ListPage{}, fmt.Errorf("list payment intents: %w", err)
	}
	defer rows.Close()

	var page ListPage
	for rows.Next() {
		var intent Intent
		if err := rows.Scan(
			&intent.ID, &intent.MerchantID, &intent.Amount, &intent.Currency,
			&intent.Status, &intent.CaptureMethod, &intent.ConfirmationMethod,
			&intent.PaymentMethodID, &intent.AdapterChargeID,
			&intent.AmountCaptured, &intent.CanceledAt,
			&intent.CancellationReason, &intent.LedgerTxnID,
			&intent.Livemode, &intent.Metadata, &intent.CreatedAt, &intent.UpdatedAt,
		); err != nil {
			return ListPage{}, fmt.Errorf("scan payment intent: %w", err)
		}
		page.Intents = append(page.Intents, &intent)
	}
	if err := rows.Err(); err != nil {
		return ListPage{}, err
	}
	if len(page.Intents) > p.Limit {
		page.HasMore = true
		page.Intents = page.Intents[:p.Limit]
	}
	return page, nil
}
