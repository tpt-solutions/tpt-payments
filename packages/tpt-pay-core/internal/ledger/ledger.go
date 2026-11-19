// Package ledger implements TPT Pay's append-only double-entry financial ledger.
//
// Core invariants:
//   - For every transaction, the sum of its entry amounts is zero (debits = credits).
//   - Every entry in a transaction uses the same currency.
//   - A (type, reference_id) pair is posted at most once (database-enforced), which
//     makes posting idempotent for retried operations.
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
	"github.com/jackc/pgx/v5/pgconn"
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

// DBTX is the subset of pgx operations shared by *pgxpool.Pool and pgx.Tx, so
// callers can run ledger writes inside their own database transaction.
type DBTX interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Ledger provides write and read operations against the double-entry ledger.
type Ledger struct {
	pool *pgxpool.Pool
}

// New creates a Ledger backed by the given connection pool.
func New(pool *pgxpool.Pool) *Ledger {
	return &Ledger{pool: pool}
}

// ErrUnbalanced is returned when a transaction's entries do not sum to zero.
var ErrUnbalanced = errors.New("ledger entries are not balanced: sum must equal zero")

// ErrCurrencyMismatch is returned when entries in one transaction use different currencies.
var ErrCurrencyMismatch = errors.New("ledger entries must all use the same currency")

// ValidateEntries checks the invariants a transaction must satisfy before it is written:
// at least two entries, a zero sum, and a single currency. It touches no database.
func ValidateEntries(entries []Entry) error {
	if len(entries) < 2 {
		return errors.New("a transaction requires at least two entries")
	}
	var sum int64
	currency := entries[0].Currency
	for _, e := range entries {
		if e.Amount == 0 {
			return errors.New("ledger entry amount must be non-zero")
		}
		if e.Currency != currency {
			return ErrCurrencyMismatch
		}
		sum += e.Amount
	}
	if sum != 0 {
		return fmt.Errorf("%w (got %d)", ErrUnbalanced, sum)
	}
	return nil
}

// EnsureAccount returns the account ID for (merchantID, currency), creating it
// if it does not exist. Safe to call concurrently.
func (l *Ledger) EnsureAccount(ctx context.Context, merchantID uuid.UUID, currency string) (uuid.UUID, error) {
	return ensureAccount(ctx, l.pool, merchantID, currency)
}

// EnsureAccountTx is EnsureAccount inside the caller's transaction.
func (l *Ledger) EnsureAccountTx(ctx context.Context, tx pgx.Tx, merchantID uuid.UUID, currency string) (uuid.UUID, error) {
	return ensureAccount(ctx, tx, merchantID, currency)
}

func ensureAccount(ctx context.Context, db DBTX, merchantID uuid.UUID, currency string) (uuid.UUID, error) {
	// DO NOTHING avoids rewriting an existing row on every call.
	var id uuid.UUID
	err := db.QueryRow(ctx, `
		INSERT INTO accounts (merchant_id, currency)
		VALUES ($1, $2)
		ON CONFLICT (merchant_id, currency) DO NOTHING
		RETURNING id
	`, merchantID, currency).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		// Row already existed; fetch it.
		err = db.QueryRow(ctx, `
			SELECT id FROM accounts WHERE merchant_id = $1 AND currency = $2
		`, merchantID, currency).Scan(&id)
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("ensure account: %w", err)
	}
	return id, nil
}

// PostTransaction writes a balanced set of entries in its own database transaction.
func (l *Ledger) PostTransaction(ctx context.Context, txnType TransactionType, referenceID string, entries []Entry) (*Transaction, error) {
	var txn *Transaction
	err := pgx.BeginTxFunc(ctx, l.pool, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, func(tx pgx.Tx) error {
		var perr error
		txn, _, perr = l.PostTransactionTx(ctx, tx, txnType, referenceID, entries)
		return perr
	})
	if err != nil {
		return nil, err
	}
	return txn, nil
}

// PostTransactionTx writes a balanced set of entries inside the caller's transaction,
// so the ledger write commits or rolls back together with the caller's other changes.
//
// Posting is idempotent per (txnType, referenceID): if the transaction already exists,
// the existing record is returned with created=false and no entries are written again.
func (l *Ledger) PostTransactionTx(ctx context.Context, tx pgx.Tx, txnType TransactionType, referenceID string, entries []Entry) (*Transaction, bool, error) {
	if err := ValidateEntries(entries); err != nil {
		return nil, false, err
	}

	var txn Transaction
	err := tx.QueryRow(ctx, `
		INSERT INTO ledger_transactions (type, status, reference_id)
		VALUES ($1, 'posted', $2)
		ON CONFLICT (type, reference_id) DO NOTHING
		RETURNING id, created_at
	`, txnType, referenceID).Scan(&txn.ID, &txn.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// Already posted: return the existing transaction without writing entries.
		var existing uuid.UUID
		if err := tx.QueryRow(ctx, `
			SELECT id, created_at FROM ledger_transactions WHERE type = $1 AND reference_id = $2
		`, txnType, referenceID).Scan(&existing, &txn.CreatedAt); err != nil {
			return nil, false, fmt.Errorf("lookup existing transaction: %w", err)
		}
		txn.ID = existing
		txn.Type = txnType
		txn.Status = "posted"
		txn.ReferenceID = referenceID
		txn.Entries = entries
		return &txn, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("insert transaction: %w", err)
	}

	for i, e := range entries {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ledger_entries (transaction_id, account_id, amount, currency, description)
			VALUES ($1, $2, $3, $4, $5)
		`, txn.ID, e.AccountID, e.Amount, e.Currency, e.Description); err != nil {
			return nil, false, fmt.Errorf("insert entry %d: %w", i, err)
		}
	}

	txn.Type = txnType
	txn.Status = "posted"
	txn.ReferenceID = referenceID
	txn.Entries = entries
	return &txn, true, nil
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
