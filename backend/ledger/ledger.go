// tpt-payments/backend/ledger/ledger.go
package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Standard account types
const (
	AccountTypeAsset     = "asset"
	AccountTypeLiability = "liability"
	AccountTypeEquity    = "equity"
	AccountTypeRevenue   = "revenue"
	AccountTypeExpense   = "expense"
)

// Standard transaction states
const (
	TxStatePending   = "pending"
	TxStateCommitted = "committed"
	TxStateVoided    = "voided"
)

// Account represents a ledger account
type Account struct {
	ID             string          `db:"id" json:"id"`
	MerchantID     string          `db:"merchant_id" json:"merchant_id"`
	Type           string          `db:"type" json:"type"`
	Name           string          `db:"name" json:"name"`
	Balance        decimal.Decimal `db:"balance" json:"balance"`
	NormalBalance  string          `db:"normal_balance" json:"normal_balance"` // debit, credit
	Currency       string          `db:"currency" json:"currency"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	LastModifiedAt time.Time       `db:"last_modified_at" json:"last_modified_at"`
	Version        int64           `db:"version" json:"version"`
}

// Entry represents a single ledger entry (debit or credit)
// This table is APPEND-ONLY: no UPDATE or DELETE operations permitted
type Entry struct {
	ID             string          `db:"id" json:"id"`
	TransactionID  string          `db:"transaction_id" json:"transaction_id"`
	AccountID      string          `db:"account_id" json:"account_id"`
	Amount         decimal.Decimal `db:"amount" json:"amount"`
	EntryType      string          `db:"entry_type" json:"entry_type"` // debit, credit
	Sequence       int             `db:"sequence" json:"sequence"`
	Description    string          `db:"description" json:"description"`
	IdempotencyKey string          `db:"idempotency_key" json:"idempotency_key"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
}

// Transaction represents a balanced set of debit and credit entries
// Debits MUST equal Credits for every transaction
type Transaction struct {
	ID             string          `db:"id" json:"id"`
	MerchantID     string          `db:"merchant_id" json:"merchant_id"`
	State          string          `db:"state" json:"state"`
	Description    string          `db:"description" json:"description"`
	IdempotencyKey string          `db:"idempotency_key" json:"idempotency_key"`
	Metadata       []byte          `db:"metadata" json:"metadata"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	CommittedAt    *time.Time      `db:"committed_at" json:"committed_at,omitempty"`
	Entries        []Entry         `json:"entries,omitempty"`
}

// Ledger is the core double-entry accounting system
type Ledger struct {
	db *sql.DB
}

// NewLedger creates a new ledger instance
func NewLedger(db *sql.DB) *Ledger {
	return &Ledger{
		db: db,
	}
}

// CreateAccount creates a new ledger account
func (l *Ledger) CreateAccount(ctx context.Context, account *Account) error {
	account.ID = uuid.NewString()
	account.CreatedAt = time.Now().UTC()
	account.LastModifiedAt = account.CreatedAt
	account.Version = 1

	switch account.Type {
	case AccountTypeAsset, AccountTypeExpense:
		account.NormalBalance = "debit"
	case AccountTypeLiability, AccountTypeEquity, AccountTypeRevenue:
		account.NormalBalance = "credit"
	default:
		return fmt.Errorf("invalid account type: %s", account.Type)
	}

	query := `
		INSERT INTO accounts (id, merchant_id, type, name, balance, normal_balance, currency, created_at, last_modified_at, version)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := l.db.ExecContext(ctx, query,
		account.ID, account.MerchantID, account.Type, account.Name, account.Balance,
		account.NormalBalance, account.Currency, account.CreatedAt, account.LastModifiedAt, account.Version)

	return err
}

// getAdvisoryLockKey generates a deterministic advisory lock key from idempotency key
func getAdvisoryLockKey(idempotencyKey string) int64 {
	h := fnv.New64a()
	h.Write([]byte(idempotencyKey))
	return int64(h.Sum64())
}

// CreateTransaction creates a new balanced transaction and commits it atomically
// Uses PostgreSQL advisory locks for transaction rollback safety
// This operation is ACID compliant and idempotent
func (l *Ledger) CreateTransaction(ctx context.Context, tx *Transaction) error {
	if len(tx.Entries) < 2 {
		return errors.New("transaction must have at least two entries")
	}

	var totalDebit, totalCredit decimal.Decimal
	for _, entry := range tx.Entries {
		if entry.EntryType == "debit" {
			totalDebit = totalDebit.Add(entry.Amount)
		} else if entry.EntryType == "credit" {
			totalCredit = totalCredit.Add(entry.Amount)
		}
	}

	if !totalDebit.Equal(totalCredit) {
		return fmt.Errorf("transaction not balanced: debits = %s, credits = %s", totalDebit, totalCredit)
	}

	tx.ID = uuid.NewString()
	tx.CreatedAt = time.Now().UTC()
	tx.State = TxStateCommitted
	committedAt := time.Now().UTC()
	tx.CommittedAt = &committedAt

	txOptions := &sql.TxOptions{
		Isolation: sql.LevelSerializable,
		ReadOnly:  false,
	}

	dbTx, err := l.db.BeginTx(ctx, txOptions)
	if err != nil {
		return err
	}
	defer dbTx.Rollback()

	// Acquire PostgreSQL advisory lock to prevent race conditions on concurrent transactions
	// Lock is automatically released when transaction commits/rollbacks
	if tx.IdempotencyKey != "" {
		lockKey := getAdvisoryLockKey(tx.IdempotencyKey)
		_, err = dbTx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lockKey)
		if err != nil {
			return fmt.Errorf("failed to acquire transaction lock: %w", err)
		}

		// Check for existing committed transaction with same idempotency key
		var existingTxID string
		err = dbTx.QueryRowContext(ctx,
			"SELECT id FROM transactions WHERE idempotency_key = $1 AND state = 'committed' LIMIT 1",
			tx.IdempotencyKey).Scan(&existingTxID)
		if err == nil {
			return fmt.Errorf("transaction already committed with id: %s", existingTxID)
		}
		if err != sql.ErrNoRows {
			return err
		}
	}

	txQuery := `
		INSERT INTO transactions (id, merchant_id, state, description, idempotency_key, metadata, created_at, committed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = dbTx.ExecContext(ctx, txQuery,
		tx.ID, tx.MerchantID, tx.State, tx.Description, tx.IdempotencyKey,
		tx.Metadata, tx.CreatedAt, tx.CommittedAt)
	if err != nil {
		return err
	}

	entryQuery := `
		INSERT INTO entries (id, transaction_id, account_id, amount, entry_type, sequence, description, idempotency_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	for i, entry := range tx.Entries {
		entry.ID = uuid.NewString()
		entry.TransactionID = tx.ID
		entry.Sequence = i + 1
		entry.CreatedAt = tx.CreatedAt

		_, err = dbTx.ExecContext(ctx, entryQuery,
			entry.ID, entry.TransactionID, entry.AccountID, entry.Amount,
			entry.EntryType, entry.Sequence, entry.Description, tx.IdempotencyKey, entry.CreatedAt)
		if err != nil {
			return err
		}

		var balanceMultiplier decimal.Decimal
		if entry.EntryType == "debit" {
			balanceMultiplier = decimal.NewFromInt(1)
		} else {
			balanceMultiplier = decimal.NewFromInt(-1)
		}

		updateQuery := `
			UPDATE accounts
			SET balance = balance + $1, last_modified_at = $2, version = version + 1
			WHERE id = $3
		`

		_, err = dbTx.ExecContext(ctx, updateQuery,
			entry.Amount.Mul(balanceMultiplier), time.Now().UTC(), entry.AccountID)
		if err != nil {
			return err
		}
	}

	return dbTx.Commit()
}

// VerifyIntegrity verifies that total debits equal total credits across the entire ledger
func (l *Ledger) VerifyIntegrity(ctx context.Context) (bool, error) {
	query := `
		SELECT
			SUM(CASE WHEN entry_type = 'debit' THEN amount ELSE 0 END) as total_debits,
			SUM(CASE WHEN entry_type = 'credit' THEN amount ELSE 0 END) as total_credits
		FROM entries
	`

	var totalDebits, totalCredits decimal.Decimal
	err := l.db.QueryRowContext(ctx, query).Scan(&totalDebits, &totalCredits)
	if err != nil {
		return false, err
	}

	return totalDebits.Equal(totalCredits), nil
}

// VerifyMerchantIntegrity verifies ledger integrity for a specific merchant
func (l *Ledger) VerifyMerchantIntegrity(ctx context.Context, merchantID string) (bool, error) {
	query := `
		SELECT
			SUM(CASE WHEN e.entry_type = 'debit' THEN e.amount ELSE 0 END) as total_debits,
			SUM(CASE WHEN e.entry_type = 'credit' THEN e.amount ELSE 0 END) as total_credits
		FROM entries e
		JOIN transactions t ON e.transaction_id = t.id
		WHERE t.merchant_id = $1
	`

	var totalDebits, totalCredits decimal.Decimal
	err := l.db.QueryRowContext(ctx, query, merchantID).Scan(&totalDebits, &totalCredits)
	if err != nil {
		return false, err
	}

	return totalDebits.Equal(totalCredits), nil
}

// ReconciliationResult contains results from a ledger reconciliation run
type ReconciliationResult struct {
	RunID         string            `json:"run_id"`
	RunAt         time.Time         `json:"run_at"`
	GlobalValid   bool              `json:"global_valid"`
	InvalidMerchants []string        `json:"invalid_merchants,omitempty"`
	MismatchCount int               `json:"mismatch_count"`
	Error         string            `json:"error,omitempty"`
	Duration      time.Duration     `json:"duration"`
}

// Reconcile performs a full ledger reconciliation check
func (l *Ledger) Reconcile(ctx context.Context) (*ReconciliationResult, error) {
	start := time.Now()
	result := &ReconciliationResult{
		RunID: uuid.NewString(),
		RunAt: time.Now().UTC(),
	}

	// First check global integrity
	globalValid, err := l.VerifyIntegrity(ctx)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	result.GlobalValid = globalValid

	// Check individual merchant integrity
	merchantsQuery := `SELECT DISTINCT merchant_id FROM transactions`
	rows, err := l.db.QueryContext(ctx, merchantsQuery)
	if err != nil {
		result.Error = err.Error()
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var merchantID string
		if err := rows.Scan(&merchantID); err != nil {
			continue
		}

		valid, err := l.VerifyMerchantIntegrity(ctx, merchantID)
		if err != nil {
			continue
		}

		if !valid {
			result.InvalidMerchants = append(result.InvalidMerchants, merchantID)
			result.MismatchCount++
		}
	}

	result.Duration = time.Since(start)
	return result, nil
}

// ReconciliationJob is a scheduled job that runs ledger reconciliation
type ReconciliationJob struct {
	ledger      *Ledger
	alertFn     func(*ReconciliationResult)
	interval    time.Duration
	stopChan    chan struct{}
	running     bool
}

// NewReconciliationJob creates a new scheduled reconciliation job
func NewReconciliationJob(ledger *Ledger, alertFn func(*ReconciliationResult), interval time.Duration) *ReconciliationJob {
	return &ReconciliationJob{
		ledger:   ledger,
		alertFn:  alertFn,
		interval: interval,
		stopChan: make(chan struct{}),
	}
}

// Start begins running the reconciliation job on schedule
func (j *ReconciliationJob) Start(ctx context.Context) {
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
				result, err := j.ledger.Reconcile(ctx)
				if err != nil {
					if j.alertFn != nil {
						j.alertFn(result)
					}
					continue
				}

				if result.MismatchCount > 0 || !result.GlobalValid {
					if j.alertFn != nil {
						j.alertFn(result)
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
func (j *ReconciliationJob) Stop() {
	close(j.stopChan)
}
