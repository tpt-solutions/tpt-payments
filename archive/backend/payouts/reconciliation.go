// tpt-payments/backend/payouts/reconciliation.go
package payouts

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// =============================================================================
// Payout Reconciliation Types
// =============================================================================

// PayoutReconciliationStatus represents the match status of a payout reconciliation
type PayoutReconciliationStatus string

const (
	PayoutReconciliationMatched     PayoutReconciliationStatus = "matched"
	PayoutReconciliationUnmatched   PayoutReconciliationStatus = "unmatched"
	PayoutReconciliationDiscrepancy PayoutReconciliationStatus = "discrepancy"
)

// PayoutReconciliationEntry represents a single payout reconciliation record
type PayoutReconciliationEntry struct {
	ID                 string                     `json:"id"`
	MerchantID         string                     `json:"merchant_id"`
	PayoutID           string                     `json:"payout_id,omitempty"`
	BankTransactionID  string                     `json:"bank_transaction_id"`
	Amount             decimal.Decimal            `json:"amount"`
	Currency           string                     `json:"currency"`
	Status             PayoutReconciliationStatus `json:"status"`
	Type               string                     `json:"type"`
	MatchedAt          *time.Time                 `json:"matched_at,omitempty"`
	DiscrepancyReason  string                     `json:"discrepancy_reason,omitempty"`
	BankPostedAt       int64                      `json:"bank_posted_at"`
	CreatedAt          int64                      `json:"created_at"`
}

// PayoutReconciliationReport provides a summary of payout reconciliation for a period
type PayoutReconciliationReport struct {
	Date              string            `json:"date"`
	TotalPayouts      int               `json:"total_payouts"`
	MatchedCount      int               `json:"matched_count"`
	UnmatchedCount    int               `json:"unmatched_count"`
	DiscrepancyCount  int               `json:"discrepancy_count"`
	TotalAmount       decimal.Decimal   `json:"total_amount"`
	UnmatchedAmount   decimal.Decimal   `json:"unmatched_amount"`
	DiscrepancyAmount decimal.Decimal   `json:"discrepancy_amount"`
	Currency          string            `json:"currency"`
	GeneratedAt       int64             `json:"generated_at"`
}

// BankStatementLine represents a single line from a bank statement
type BankStatementLine struct {
	TransactionID string          `json:"transaction_id"`
	Amount        decimal.Decimal `json:"amount"`
	Currency      string          `json:"currency"`
	PostedAt      int64           `json:"posted_at"`
	Description   string          `json:"description"`
	Reference     string          `json:"reference"`
}

// =============================================================================
// Payout Reconciler
// =============================================================================

// PayoutReconciler handles matching internal payouts to bank statement lines
type PayoutReconciler struct {
	db *sql.DB
}

// NewPayoutReconciler creates a new payout reconciler
func NewPayoutReconciler(db *sql.DB) *PayoutReconciler {
	return &PayoutReconciler{db: db}
}

// ReconcilePayouts performs reconciliation for payouts on a given date
func (r *PayoutReconciler) ReconcilePayouts(ctx context.Context, date time.Time) (*PayoutReconciliationReport, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// 1. Fetch all payouts that were expected to settle on this date
	payouts, err := r.fetchPayoutsForDate(ctx, tx, start.Unix(), end.Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch payouts: %w", err)
	}

	// 2. Fetch bank statement lines for the same period
	statementLines, err := r.fetchBankStatementLines(ctx, tx, start.Unix(), end.Unix())
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bank statement lines: %w", err)
	}

	// 3. Match payouts to statement lines
	report := &PayoutReconciliationReport{
		Date:        start.Format("2006-01-02"),
		TotalPayouts: len(payouts),
		GeneratedAt: time.Now().Unix(),
	}

	for _, payout := range payouts {
		report.TotalAmount = report.TotalAmount.Add(payout.Amount)

		match, status, reason := r.findMatch(payout, statementLines)

		entry := PayoutReconciliationEntry{
			ID:                fmt.Sprintf("prec_%d", time.Now().UnixNano()),
			MerchantID:        payout.MerchantID,
			PayoutID:          payout.ID,
			BankTransactionID: match,
			Amount:            payout.Amount,
			Currency:          payout.Currency,
			Status:            status,
			Type:              "payout",
			DiscrepancyReason: reason,
			BankPostedAt:      start.Unix(),
			CreatedAt:         time.Now().Unix(),
		}

		if status == PayoutReconciliationMatched {
			now := time.Now().UTC()
			entry.MatchedAt = &now
			report.MatchedCount++
		} else if status == PayoutReconciliationUnmatched {
			report.UnmatchedCount++
			report.UnmatchedAmount = report.UnmatchedAmount.Add(payout.Amount)
		} else {
			report.DiscrepancyCount++
			report.DiscrepancyAmount = report.DiscrepancyAmount.Add(payout.Amount)
		}

		if err := r.saveReconciliationEntry(ctx, tx, &entry); err != nil {
			return nil, fmt.Errorf("failed to save reconciliation entry: %w", err)
		}
	}

	// 4. Check for unmatched bank statement lines (potential unknown payouts)
	for _, line := range statementLines {
		if !line.matched {
			entry := PayoutReconciliationEntry{
				ID:                fmt.Sprintf("prec_%d", time.Now().UnixNano()),
				MerchantID:        "",
				PayoutID:          "",
				BankTransactionID: line.TransactionID,
				Amount:            line.Amount,
				Currency:          line.Currency,
				Status:            PayoutReconciliationUnmatched,
				Type:              "bank_statement",
				DiscrepancyReason: "bank transaction not matched to any internal payout",
				BankPostedAt:      line.PostedAt,
				CreatedAt:         time.Now().Unix(),
			}
			report.UnmatchedCount++
			report.UnmatchedAmount = report.UnmatchedAmount.Add(line.Amount)

			if err := r.saveReconciliationEntry(ctx, tx, &entry); err != nil {
				return nil, fmt.Errorf("failed to save unmatched entry: %w", err)
			}
		}
	}

	// 5. Save report
	if err := r.saveReport(ctx, tx, report); err != nil {
		return nil, fmt.Errorf("failed to save report: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return report, nil
}

// fetchPayoutsForDate retrieves payouts expected to settle in the given range
func (r *PayoutReconciler) fetchPayoutsForDate(ctx context.Context, tx *sql.Tx, start, end int64) ([]*Payout, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, merchant_id, amount, currency, status, method, destination_id, arrival_date,
		       paid_at, failure_code, failure_message, metadata, created_at, updated_at
		FROM payouts 
		WHERE arrival_date >= $1 AND arrival_date < $2
		OR (status = 'paid' AND paid_at >= $1 AND paid_at < $2)`,
		start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payouts []*Payout
	for rows.Next() {
		var p Payout
		var metadataJSON []byte
		err := rows.Scan(
			&p.ID, &p.MerchantID, &p.Amount, &p.Currency, &p.Status,
			&p.Method, &p.DestinationID, &p.ArrivalDate, &p.PaidAt,
			&p.FailureCode, &p.FailureMessage, &metadataJSON, &p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		json.Unmarshal(metadataJSON, &p.Metadata)
		payouts = append(payouts, &p)
	}

	return payouts, nil
}

// fetchBankStatementLines retrieves bank statement lines for the period
func (r *PayoutReconciler) fetchBankStatementLines(ctx context.Context, tx *sql.Tx, start, end int64) ([]*bankStatementLineInternal, error) {
	// In production, this would query a bank_statement_lines table
	// For now, query the reconciliation_entries table for bank transactions
	rows, err := tx.QueryContext(ctx, `
		SELECT bank_transaction_id, amount, currency, bank_posted_at, type
		FROM payout_reconciliation_entries 
		WHERE bank_posted_at >= $1 AND bank_posted_at < $2 AND type = 'bank_statement'`,
		start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []*bankStatementLineInternal
	for rows.Next() {
		var line bankStatementLineInternal
		err := rows.Scan(
			&line.TransactionID, &line.Amount, &line.Currency,
			&line.PostedAt, &line.Description,
		)
		if err != nil {
			return nil, err
		}
		lines = append(lines, &line)
	}

	return lines, nil
}

// bankStatementLineInternal extends BankStatementLine with matched tracking
type bankStatementLineInternal struct {
	BankStatementLine
	matched bool
}

// findMatch attempts to match a payout to a bank statement line
func (r *PayoutReconciler) findMatch(payout *Payout, lines []*bankStatementLineInternal) (string, PayoutReconciliationStatus, string) {
	// Try to match by external_id first
	if extID, ok := payout.Metadata["external_id"]; ok && extID != "" {
		for _, line := range lines {
			if line.TransactionID == extID {
				line.matched = true
				if !line.Amount.Equal(payout.Amount) {
					return line.TransactionID, PayoutReconciliationDiscrepancy,
						fmt.Sprintf("amount mismatch: internal=%s bank=%s", payout.Amount, line.Amount)
				}
				return line.TransactionID, PayoutReconciliationMatched, ""
			}
		}
	}

	// Try to match by payout ID in reference
	for _, line := range lines {
		if line.matched {
			continue
		}
		if line.Reference == payout.ID || line.Description == payout.ID {
			line.matched = true
			if !line.Amount.Equal(payout.Amount) {
				return line.TransactionID, PayoutReconciliationDiscrepancy,
					fmt.Sprintf("amount mismatch: internal=%s bank=%s", payout.Amount, line.Amount)
			}
			return line.TransactionID, PayoutReconciliationMatched, ""
		}
	}

	// No match found
	return "", PayoutReconciliationUnmatched, "no matching bank transaction found"
}

// saveReconciliationEntry persists a reconciliation entry
func (r *PayoutReconciler) saveReconciliationEntry(ctx context.Context, tx *sql.Tx, entry *PayoutReconciliationEntry) error {
	var matchedAt interface{}
	if entry.MatchedAt != nil {
		matchedAt = *entry.MatchedAt
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO payout_reconciliation_entries (
			id, merchant_id, payout_id, bank_transaction_id, amount, currency, status, type,
			matched_at, discrepancy_reason, bank_posted_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			status = $7, matched_at = $9, discrepancy_reason = $10`,
		entry.ID, entry.MerchantID, entry.PayoutID, entry.BankTransactionID,
		entry.Amount, entry.Currency, entry.Status, entry.Type,
		matchedAt, entry.DiscrepancyReason, entry.BankPostedAt, entry.CreatedAt,
	)

	return err
}

// saveReport persists the reconciliation report
func (r *PayoutReconciler) saveReport(ctx context.Context, tx *sql.Tx, report *PayoutReconciliationReport) error {
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO payout_reconciliation_reports (date, report, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (date) DO UPDATE SET report = $2, updated_at = $3`,
		report.Date, reportJSON, time.Now().Unix(),
	)

	return err
}

// GetReconciliationReport retrieves a previously generated report
func (r *PayoutReconciler) GetReconciliationReport(ctx context.Context, date time.Time) (*PayoutReconciliationReport, error) {
	var reportJSON []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT report FROM payout_reconciliation_reports WHERE date = $1`,
		time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC),
	).Scan(&reportJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no report found for date %s", date.Format("2006-01-02"))
		}
		return nil, err
	}

	var report PayoutReconciliationReport
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		return nil, err
	}

	return &report, nil
}

// ListUnmatchedPayouts returns all unmatched payouts for manual review
func (r *PayoutReconciler) ListUnmatchedPayouts(ctx context.Context, date time.Time) ([]PayoutReconciliationEntry, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, merchant_id, payout_id, bank_transaction_id, amount, currency, status, type,
		       discrepancy_reason, bank_posted_at, created_at
		FROM payout_reconciliation_entries
		WHERE status = 'unmatched' AND bank_posted_at >= $1 AND bank_posted_at < $2`,
		start.Unix(), end.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []PayoutReconciliationEntry
	for rows.Next() {
		var e PayoutReconciliationEntry
		err := rows.Scan(
			&e.ID, &e.MerchantID, &e.PayoutID, &e.BankTransactionID,
			&e.Amount, &e.Currency, &e.Status, &e.Type,
			&e.DiscrepancyReason, &e.BankPostedAt, &e.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// ManualMatch allows an operator to manually match an unmatched payout to a bank transaction
func (r *PayoutReconciler) ManualMatch(ctx context.Context, entryID string, bankTransactionID string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE payout_reconciliation_entries
		SET status = 'matched', bank_transaction_id = $2, matched_at = $3, discrepancy_reason = NULL
		WHERE id = $1`,
		entryID, bankTransactionID, time.Now().UTC(),
	)
	return err
}

// PayoutReconciliationJob is a scheduled job that runs payout reconciliation
type PayoutReconciliationJob struct {
	reconciler *PayoutReconciler
	alertFn    func(*PayoutReconciliationReport) error
	interval   time.Duration
	stopChan   chan struct{}
	running    bool
}

// NewPayoutReconciliationJob creates a new scheduled reconciliation job
func NewPayoutReconciliationJob(reconciler *PayoutReconciler, alertFn func(*PayoutReconciliationReport) error, interval time.Duration) *PayoutReconciliationJob {
	return &PayoutReconciliationJob{
		reconciler: reconciler,
		alertFn:    alertFn,
		interval:   interval,
		stopChan:   make(chan struct{}),
	}
}

// Start begins running the reconciliation job on schedule
func (j *PayoutReconciliationJob) Start(ctx context.Context) {
	if j.running {
		return
	}
	j.running = true

	go func() {
		ticker := time.NewTicker(j.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Run reconciliation for yesterday
				yesterday := time.Now().UTC().AddDate(0, 0, -1)
				report, err := j.reconciler.ReconcilePayouts(ctx, yesterday)
				if err != nil {
					// Log error
					continue
				}

				// Alert if there are unmatched or discrepancy entries
				if report.UnmatchedCount > 0 || report.DiscrepancyCount > 0 {
					if j.alertFn != nil {
						_ = j.alertFn(report)
					}
				}
			case <-j.stopChan:
				j.running = false
				return
			case <-ctx.Done():
				j.running = false
				return
			}
		}
	}()
}

// Stop halts the reconciliation job
func (j *PayoutReconciliationJob) Stop() {
	close(j.stopChan)
}
