// tpt-payments/backend/adapters/bank_reconciliation.go
package adapters

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ReconciliationEntry represents a bank transaction matching record
type ReconciliationEntry struct {
	ID                  string         `db:"id"`
	MerchantID          string         `db:"merchant_id"`
	TransactionID       sql.NullString `db:"transaction_id"`
	BankTransactionID   string         `db:"bank_transaction_id"`
	Amount              int64          `db:"amount"`
	Currency            string         `db:"currency"`
	Status              string         `db:"status"`
	Type                string         `db:"type"`
	MatchedAt           sql.NullTime   `db:"matched_at"`
	DiscrepancyReason   sql.NullString `db:"discrepancy_reason"`
	BankPostedAt        int64          `db:"bank_posted_at"`
	CreatedAt           int64          `db:"created_at"`
}

// ReconciliationMatcher handles matching internal transactions to bank transactions
type ReconciliationMatcher struct {
	db *sql.DB
}

// NewReconciliationMatcher creates a new reconciliation matcher
func NewReconciliationMatcher(db *sql.DB) *ReconciliationMatcher {
	return &ReconciliationMatcher{db: db}
}

// MatchTransaction matches a bank webhook event to an internal transaction
func (r *ReconciliationMatcher) MatchTransaction(ctx context.Context, event BankWebhookEvent) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if already matched
	var exists bool
	err = tx.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM reconciliation_entries 
		 WHERE bank_transaction_id = $1)`,
		event.BankReference,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if exists {
		// Already processed
		return nil
	}

	// Find matching internal transaction
	var transaction struct {
		ID     string `db:"id"`
		Amount int64  `db:"amount"`
		Status string `db:"status"`
	}

	err = tx.QueryRowContext(ctx, `
		SELECT id, amount, status FROM transactions
		 WHERE external_reference = $1 OR id = $2
		 LIMIT 1`,
		event.BankReference,
		event.ObjectID,
	).Scan(&transaction.ID, &transaction.Amount, &transaction.Status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Create unmatched entry for later reconciliation
			_, err = tx.ExecContext(ctx, `
				INSERT INTO reconciliation_entries (
					id, merchant_id, bank_transaction_id, amount, currency, status, type, 
					bank_posted_at, created_at, discrepancy_reason
				) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
				fmt.Sprintf("rec_%d", time.Now().UnixNano()),
				event.MerchantID,
				event.BankReference,
				0, // Will be updated when we get bank report
				"",
				"unmatched",
				event.ObjectType,
				event.CreatedAt,
				time.Now().Unix(),
				"transaction not found in internal ledger",
			)
			return err
		}
		return err
	}

	// Verify amount matches
	bankAmount, ok := event.RawData["amount"].(float64)
	if ok && int64(bankAmount) != transaction.Amount {
		// Amount discrepancy
		_, err = tx.ExecContext(ctx, `
			INSERT INTO reconciliation_entries (
				id, merchant_id, transaction_id, bank_transaction_id, amount, currency, status, type, 
				bank_posted_at, created_at, discrepancy_reason
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			fmt.Sprintf("rec_%d", time.Now().UnixNano()),
			event.MerchantID,
			transaction.ID,
			event.BankReference,
			int64(bankAmount),
			event.RawData["currency"].(string),
			"discrepancy",
			event.ObjectType,
			event.CreatedAt,
			time.Now().Unix(),
			fmt.Sprintf("amount mismatch: internal=%d bank=%d", transaction.Amount, int64(bankAmount)),
		)
		return err
	}

	// Perfect match
	now := time.Now()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO reconciliation_entries (
			id, merchant_id, transaction_id, bank_transaction_id, amount, currency, status, type, 
			matched_at, bank_posted_at, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		fmt.Sprintf("rec_%d", time.Now().UnixNano()),
		event.MerchantID,
		transaction.ID,
		event.BankReference,
		transaction.Amount,
		event.RawData["currency"].(string),
		"matched",
		event.ObjectType,
		now,
		event.CreatedAt,
		time.Now().Unix(),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// ReconciliationReport represents a daily reconciliation report
type ReconciliationReport struct {
	Date               string `json:"date"`
	TotalTransactions  int    `json:"total_transactions"`
	MatchedCount       int    `json:"matched_count"`
	UnmatchedCount     int    `json:"unmatched_count"`
	DiscrepancyCount   int    `json:"discrepancy_count"`
	TotalAmount        int64  `json:"total_amount"`
	DiscrepancyAmount  int64  `json:"discrepancy_amount"`
}

// GenerateDailyReport generates reconciliation report for a given date
func (r *ReconciliationMatcher) GenerateDailyReport(ctx context.Context, date time.Time) (*ReconciliationReport, error) {
	start := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	var report ReconciliationReport
	report.Date = start.Format("2006-01-02")

	err := r.db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*),
			COUNT(CASE WHEN status = 'matched' THEN 1 END),
			COUNT(CASE WHEN status = 'unmatched' THEN 1 END),
			COUNT(CASE WHEN status = 'discrepancy' THEN 1 END),
			SUM(amount)
		FROM reconciliation_entries
		 WHERE bank_posted_at >= $1 AND bank_posted_at < $2`,
		start.Unix(),
		end.Unix(),
	).Scan(
		&report.TotalTransactions,
		&report.MatchedCount,
		&report.UnmatchedCount,
		&report.DiscrepancyCount,
		&report.TotalAmount,
	)
	if err != nil {
		return nil, err
	}

	return &report, nil
}

// ReconciliationJob is the scheduled batch reconciliation job
type ReconciliationJob struct {
	matcher  *ReconciliationMatcher
	adapter  CardNetworkAdapter
	alertFn  func(ctx context.Context, report ReconciliationReport) error
}

// NewReconciliationJob creates the daily reconciliation job
func NewReconciliationJob(matcher *ReconciliationMatcher, adapter CardNetworkAdapter, alertFn func(ctx context.Context, report ReconciliationReport) error) *ReconciliationJob {
	return &ReconciliationJob{
		matcher: matcher,
		adapter: adapter,
		alertFn: alertFn,
	}
}

// Run executes the daily reconciliation process
func (j *ReconciliationJob) Run(ctx context.Context, date time.Time) error {
	// Pull all transactions from bank for date
	// Compare against internal ledger
	// Generate report
	report, err := j.matcher.GenerateDailyReport(ctx, date)
	if err != nil {
		return err
	}

	// If discrepancies exist, trigger alert
	if report.DiscrepancyCount > 0 || report.UnmatchedCount > 10 {
		if err := j.alertFn(ctx, *report); err != nil {
			return err
		}
	}

	// Store report
	reportJSON, err := json.Marshal(report)
	if err != nil {
		return err
	}

	_, err = j.matcher.db.ExecContext(ctx, `
		INSERT INTO reconciliation_reports (date, report, created_at)
		VALUES ($1, $2, $3)
		ON CONFLICT (date) DO UPDATE SET report = $2`,
		report.Date,
		reportJSON,
		time.Now().Unix(),
	)

	return err
}