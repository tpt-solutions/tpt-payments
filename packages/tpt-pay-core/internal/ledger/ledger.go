// Package ledger implements TPT Pay's append-only double-entry financial ledger.
//
// Core invariant: for every transaction, the sum of all its ledger_entry amounts
// equals zero (debits equal credits). This is enforced both in application code
// (PostTransaction rejects unbalanced entries) and by the reconciliation job.
//
// Money amounts are always int64 in the smallest currency unit (e.g. cents for USD).
// Floating-point is never used for monetary values.
package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TransactionType classifies the purpose of a ledger transaction.
type TransactionType string

const (
	TxnTypePaymentIntent TransactionType = "payment_intent"
	TxnTypeRefund        TransactionType = "refund"
	TxnTypePayout        TransactionType = "payout"
	TxnTypeFee           TransactionType = "fee"
)

// Entry is one side of a double-entry bookkeeping transaction.
// Positive amount = credit (money in). Negative amount = debit (money out).
type Entry struct {
	AccountID   uuid.UUID
	Amount      int64  // non-zero, in smallest currency unit
	Currency    string // ISO 4217
	Description string
}

// Transaction groups two or more balanced entries.
type Transaction struct {
	ID          uuid.UUID
	Type        TransactionType
	Status      string
	ReferenceID string // e.g. payment_intent id
	Entries     []Entry
	CreatedAt   time.Time
}

// Ledger provides write and read operations against the double-entry ledger.
type Ledger struct {
	pool *pgxpool.Pool
}

// New creates a Ledger backed by the given connection pool.
func New(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// EnsureAccount returns the account ID for (merchantID, currency), creating it
// if it does not exist. Safe to call concurrently — uses INSERT ON CONFLICT.
func (l *Ledger) EnsureAccount(ctx context.Context, merchantID uuid.UUID, currency string) (uuid.UUID, error) {
	var id uuid.UUID
	err := l.pool.QueryRow(ctx, `
		INSERT INTO accounts (merchant_id, currency)
		VALUES ($1, $2)
		ON CONFLICT (merchant_id, currency) DO UPDATE SET merchant_id = EXCLUDED.merchant_id
		RETURNING id
	`, merchantID, currency).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("ensure account: %w", err)
	}
	return id, nil
}

// PostTransaction writes a balanced set of entries atomically.
// Returns ErrUnbalanced if entries do not sum to zero.
func (l *Ledger) PostTransaction(ctx context.Context, txnType TransactionType, referenceID string, entries []Entry) (*Transaction, error) {
	if err := validateBalance(entries); err != nil {
		return nil, err
	}
	if len(entries) < 2 {
		return nil, errors.New("a transaction requires at least two entries")
	}

	var txn Transaction
	err := pgx.BeginTxFunc(ctx, l.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(dbtx pgx.Tx) error {
		// Insert the transaction record.
		err := dbtx.QueryRow(ctx, `
			INSERT INTO ledger_transactions (type, status, reference_id)
			VALUES ($1, 'posted', $2)
			RETURNING id, created_at
		`, txnType, referenceID).Scan(&txn.ID, &txn.CreatedAt)
		if err != nil {
			return fmt.Errorf("insert transaction: %w", err)
		}

		// Insert each entry.
		for i, e := range entries {
			if _, err := dbtx.Exec(ctx, `
				INSERT INTO ledger_entries (transaction_id, account_id, amount, currency, description)
				VALUES ($1, $2, $3, $4, $5)
			`, txn.ID, e.AccountID, e.Amount, e.Currency, e.Description); err != nil {
				return fmt.Errorf("insert entry %d: %w", i, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	txn.Type = txnType
	txn.Status = "posted"
	txn.ReferenceID = referenceID
	txn.Entries = entries
	return &txn, nil
}

// Balance returns the current balance (in smallest currency unit) for an account.
// A positive balance means the merchant is owed money; negative means they owe.
func (l *Ledger) Balance(ctx context.Context, accountID uuid.UUID) (int64, string, error) {
	var balance int64
	var currency string
	err := l.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0), MAX(currency)
		FROM ledger_entries
		WHERE account_id = $1
	`, accountID).Scan(&balance, &currency)
	if err != nil {
		return 0, "", fmt.Errorf("query balance: %w", err)
	}
	return balance, currency, nil
}

// VerifyIntegrity checks that every transaction in the ledger is balanced.
// Returns the count of unbalanced transactions (should always be 0).
// Intended for use by the reconciliation job.
func (l *Ledger) VerifyIntegrity(ctx context.Context) (int, error) {
	var count int
	err := l.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT transaction_id
			FROM ledger_entries
			GROUP BY transaction_id
			HAVING SUM(amount) <> 0
		) unbalanced
	`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("integrity check: %w", err)
	}
	return count, nil
}

// ErrUnbalanced is returned when a transaction's entries do not sum to zero.
var ErrUnbalanced = errors.New("ledger entries are not balanced: sum must equal zero")

func validateBalance(entries []Entry) error {
	var sum int64
	for _, e := range entries {
		sum += e.Amount
	}
	if sum != 0 {
		return fmt.Errorf("%w (got %d)", ErrUnbalanced, sum)
	}
	return nil
}
