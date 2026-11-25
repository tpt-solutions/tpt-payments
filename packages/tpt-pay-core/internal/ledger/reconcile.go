package ledger

import (
	"context"
	"fmt"
	"time"
)

// Discrepancy kinds reported by Reconcile.
const (
	DiscrepancyUnbalancedTxn        = "unbalanced_transaction"
	DiscrepancyIntentNotPosted      = "succeeded_intent_not_posted"
	DiscrepancyIntentAmountMismatch = "intent_amount_mismatch"
	DiscrepancyIntentTxnMismatch    = "intent_ledger_txn_mismatch"
	DiscrepancyRefundTxnMismatch    = "refund_ledger_txn_mismatch"
	DiscrepancyRefundNotPosted      = "succeeded_refund_not_posted"
	DiscrepancyRefundAmountMismatch = "refund_amount_mismatch"
	DiscrepancyOrphanPayment        = "payment_txn_without_intent"
	DiscrepancyOrphanRefund         = "refund_txn_without_refund"
)

// Discrepancy is one place where the ledger and the payment records disagree.
type Discrepancy struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"` // payment intent, refund, or transaction ID
	Detail  string `json:"detail"`
}

// ReconcileReport is the result of one reconciliation run. A run with no discrepancies is
// healthy; any discrepancy is an incident to investigate, not something to fix automatically.
type ReconcileReport struct {
	RanAt         time.Time     `json:"ran_at"`
	Discrepancies []Discrepancy `json:"discrepancies"`
}

// Healthy reports whether the run found no discrepancies.
func (r ReconcileReport) Healthy() bool { return len(r.Discrepancies) == 0 }

// Reconcile compares the ledger with the payment records. It is read-only and checks that:
//
//   - every succeeded payment intent is linked to a payment_intent posting for its ID, and the
//     merchant's credit equals amount_captured;
//   - every succeeded refund is linked to a refund posting for its ID, and the merchant's
//     debit equals the refund amount;
//   - every payment or refund posting refers to a record that exists;
//   - every transaction is balanced (the same check as VerifyIntegrity).
//
// Each query runs on its own statement, so a run can see postings committed during it. A
// discrepancy found while writes are in flight can be a false positive, so re-run before
// acting on one.
func (l *Ledger) Reconcile(ctx context.Context) (ReconcileReport, error) {
	report := ReconcileReport{RanAt: time.Now().UTC()}
	add := func(kind, subject, detail string) {
		report.Discrepancies = append(report.Discrepancies, Discrepancy{Kind: kind, Subject: subject, Detail: detail})
	}

	unbalanced, err := l.VerifyIntegrity(ctx)
	if err != nil {
		return ReconcileReport{}, err
	}
	if unbalanced > 0 {
		add(DiscrepancyUnbalancedTxn, "", fmt.Sprintf("%d unbalanced transactions", unbalanced))
	}

	if err := l.reconcileQuery(ctx, add, `
		SELECT pi.id FROM payment_intents pi
		WHERE pi.status = 'succeeded' AND pi.ledger_txn_id IS NULL
	`, DiscrepancyIntentNotPosted, "succeeded but has no ledger transaction"); err != nil {
		return ReconcileReport{}, err
	}

	// The merchant's own account is the one owned by the intent's merchant in its currency.
	// The clearing account is excluded by the FILTER, so the sum is the merchant credit only.
	if err := l.reconcileAmounts(ctx, add, `
		SELECT pi.id, pi.amount_captured,
		       COALESCE(SUM(le.amount) FILTER (WHERE a.merchant_id = pi.merchant_id), 0),
		       (lt.type = 'payment_intent' AND lt.reference_id = pi.id)
		FROM payment_intents pi
		JOIN ledger_transactions lt ON lt.id = pi.ledger_txn_id
		LEFT JOIN ledger_entries le ON le.transaction_id = lt.id
		LEFT JOIN accounts a ON a.id = le.account_id
		WHERE pi.status = 'succeeded'
		GROUP BY pi.id, pi.amount_captured, pi.merchant_id, lt.type, lt.reference_id
	`, DiscrepancyIntentAmountMismatch, DiscrepancyIntentTxnMismatch); err != nil {
		return ReconcileReport{}, err
	}

	if err := l.reconcileQuery(ctx, add, `
		SELECT r.id FROM refunds r
		WHERE r.status = 'succeeded' AND r.ledger_txn_id IS NULL
	`, DiscrepancyRefundNotPosted, "succeeded but has no ledger transaction"); err != nil {
		return ReconcileReport{}, err
	}

	if err := l.reconcileAmounts(ctx, add, `
		SELECT r.id, -r.amount,
		       COALESCE(SUM(le.amount) FILTER (WHERE a.merchant_id = r.merchant_id), 0),
		       (lt.type = 'refund' AND lt.reference_id = r.id)
		FROM refunds r
		JOIN ledger_transactions lt ON lt.id = r.ledger_txn_id
		LEFT JOIN ledger_entries le ON le.transaction_id = lt.id
		LEFT JOIN accounts a ON a.id = le.account_id
		WHERE r.status = 'succeeded'
		GROUP BY r.id, r.amount, r.merchant_id, lt.type, lt.reference_id
	`, DiscrepancyRefundAmountMismatch, DiscrepancyRefundTxnMismatch); err != nil {
		return ReconcileReport{}, err
	}

	if err := l.reconcileQuery(ctx, add, `
		SELECT COALESCE(lt.reference_id, lt.id::text) FROM ledger_transactions lt
		WHERE lt.type = 'payment_intent'
		  AND NOT EXISTS (SELECT 1 FROM payment_intents pi WHERE pi.id = lt.reference_id)
	`, DiscrepancyOrphanPayment, "payment_intent posting refers to no payment intent"); err != nil {
		return ReconcileReport{}, err
	}

	if err := l.reconcileQuery(ctx, add, `
		SELECT COALESCE(lt.reference_id, lt.id::text) FROM ledger_transactions lt
		WHERE lt.type = 'refund'
		  AND NOT EXISTS (SELECT 1 FROM refunds r WHERE r.id = lt.reference_id)
	`, DiscrepancyOrphanRefund, "refund posting refers to no refund"); err != nil {
		return ReconcileReport{}, err
	}

	return report, nil
}

// reconcileQuery adds one discrepancy per row returned by query. The query selects one text
// column, the subject ID.
func (l *Ledger) reconcileQuery(ctx context.Context, add func(kind, subject, detail string), query, kind, detail string) error {
	rows, err := l.pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("reconcile %s: %w", kind, err)
	}
	defer rows.Close()
	for rows.Next() {
		var subject string
		if err := rows.Scan(&subject); err != nil {
			return fmt.Errorf("reconcile %s: scan: %w", kind, err)
		}
		add(kind, subject, detail)
	}
	return rows.Err()
}

// reconcileAmounts runs a query returning (subject, expected, actual, linkOK). It adds an
// amount discrepancy when the actual merchant posting differs from the expected amount, and a
// txn discrepancy when the linked transaction is not the expected kind for that record.
func (l *Ledger) reconcileAmounts(ctx context.Context, add func(kind, subject, detail string), query, amountKind, linkKind string) error {
	rows, err := l.pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("reconcile %s: %w", amountKind, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			subject          string
			expected, actual int64
			linkOK           bool
		)
		if err := rows.Scan(&subject, &expected, &actual, &linkOK); err != nil {
			return fmt.Errorf("reconcile %s: scan: %w", amountKind, err)
		}
		if !linkOK {
			add(linkKind, subject, "linked ledger transaction is not the posting for this record")
		}
		if actual != expected {
			add(amountKind, subject, fmt.Sprintf("merchant posting %d, expected %d", actual, expected))
		}
	}
	return rows.Err()
}

// RunReconcileJob runs Reconcile every interval until ctx is cancelled, reporting each run.
func (l *Ledger) RunReconcileJob(ctx context.Context, interval time.Duration, report func(ReconcileReport, error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r, err := l.Reconcile(ctx)
			if ctx.Err() != nil {
				return // stopped mid-run: not a failed reconciliation
			}
			report(r, err)
		}
	}
}
