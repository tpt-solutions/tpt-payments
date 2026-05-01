// tpt-payments/backend/ledger/ledger_integration_test.go
package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testPostgresURL = "postgres://postgres:postgres@localhost:5432/tpt_test?sslmode=disable"

func getTestDB(t *testing.T) *sql.DB {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = testPostgresURL
	}

	db, err := sql.Open("postgres", dbURL)
	require.NoError(t, err)

	// Ping to ensure connection
	err = db.Ping()
	require.NoError(t, err)

	// Setup test schema
	setupTestSchema(t, db)

	return db
}

func setupTestSchema(t *testing.T, db *sql.DB) {
	// Create tables for test
	schema := `
	CREATE TABLE IF NOT EXISTS accounts (
		id TEXT PRIMARY KEY,
		merchant_id TEXT NOT NULL,
		type TEXT NOT NULL,
		name TEXT NOT NULL,
		balance DECIMAL(20,6) NOT NULL DEFAULT 0,
		normal_balance TEXT NOT NULL,
		currency TEXT NOT NULL DEFAULT 'USD',
		created_at TIMESTAMPTZ NOT NULL,
		last_modified_at TIMESTAMPTZ NOT NULL,
		version BIGINT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS transactions (
		id TEXT PRIMARY KEY,
		merchant_id TEXT NOT NULL,
		state TEXT NOT NULL,
		description TEXT,
		idempotency_key TEXT UNIQUE,
		metadata BYTEA,
		created_at TIMESTAMPTZ NOT NULL,
		committed_at TIMESTAMPTZ
	);

	CREATE TABLE IF NOT EXISTS entries (
		id TEXT PRIMARY KEY,
		transaction_id TEXT NOT NULL REFERENCES transactions(id) ON DELETE CASCADE,
		account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
		amount DECIMAL(20,6) NOT NULL,
		entry_type TEXT NOT NULL,
		sequence INT NOT NULL,
		description TEXT,
		idempotency_key TEXT,
		created_at TIMESTAMPTZ NOT NULL
	);
	`

	_, err := db.Exec(schema)
	require.NoError(t, err)
}

func cleanupTestDB(t *testing.T, db *sql.DB) {
	_, err := db.Exec(`
	DROP TABLE IF EXISTS entries;
	DROP TABLE IF EXISTS transactions;
	DROP TABLE IF EXISTS accounts;
	`)
	require.NoError(t, err)
}

func TestLedgerIntegration_PostgreSQL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := getTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	ctx := context.Background()
	l := NewLedger(db)

	merchantID := "test-merchant-123"

	// Test 1: Create accounts
	cashAccount := &Account{
		MerchantID: merchantID,
		Type:       AccountTypeAsset,
		Name:       "Cash",
		Balance:    decimal.NewFromInt(0),
		Currency:   "USD",
	}

	revenueAccount := &Account{
		MerchantID: merchantID,
		Type:       AccountTypeRevenue,
		Name:       "Revenue",
		Balance:    decimal.NewFromInt(0),
		Currency:   "USD",
	}

	err := l.CreateAccount(ctx, cashAccount)
	require.NoError(t, err)
	assert.NotEmpty(t, cashAccount.ID)

	err = l.CreateAccount(ctx, revenueAccount)
	require.NoError(t, err)
	assert.NotEmpty(t, revenueAccount.ID)

	// Test 2: Create valid transaction
	amount := decimal.NewFromInt(100)
	tx := &Transaction{
		MerchantID:  merchantID,
		Description: "Test transaction",
		Entries: []Entry{
			{
				AccountID: cashAccount.ID,
				Amount:    amount,
				EntryType: "debit",
			},
			{
				AccountID: revenueAccount.ID,
				Amount:    amount,
				EntryType: "credit",
			},
		},
	}

	err = l.CreateTransaction(ctx, tx)
	require.NoError(t, err)
	assert.NotEmpty(t, tx.ID)
	assert.Equal(t, TxStateCommitted, tx.State)

	// Test 3: Verify integrity globally
	valid, err := l.VerifyIntegrity(ctx)
	require.NoError(t, err)
	assert.True(t, valid, "Ledger should be balanced")

	// Test 4: Verify merchant integrity
	valid, err = l.VerifyMerchantIntegrity(ctx, merchantID)
	require.NoError(t, err)
	assert.True(t, valid, "Merchant ledger should be balanced")

	// Test 5: Full reconciliation
	result, err := l.Reconcile(ctx)
	require.NoError(t, err)
	assert.True(t, result.GlobalValid)
	assert.Equal(t, 0, result.MismatchCount)
	assert.Empty(t, result.InvalidMerchants)
	assert.Greater(t, result.Duration, time.Duration(0))
	assert.NotEmpty(t, result.RunID)
}

func TestReconciliationJobIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := getTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	l := NewLedger(db)

	alertReceived := make(chan *ReconciliationResult, 1)
	alertFn := func(result *ReconciliationResult) {
		alertReceived <- result
	}

	// Create job with very short interval for test
	job := NewReconciliationJob(l, alertFn, 50*time.Millisecond)
	job.Start(ctx)
	defer job.Stop()

	// Create accounts and transaction
	merchantID := "test-merchant-456"
	cashAccount := &Account{
		MerchantID: merchantID,
		Type:       AccountTypeAsset,
		Name:       "Cash",
		Balance:    decimal.NewFromInt(0),
		Currency:   "USD",
	}
	revenueAccount := &Account{
		MerchantID: merchantID,
		Type:       AccountTypeRevenue,
		Name:       "Revenue",
		Balance:    decimal.NewFromInt(0),
		Currency:   "USD",
	}

	err := l.CreateAccount(ctx, cashAccount)
	require.NoError(t, err)
	err = l.CreateAccount(ctx, revenueAccount)
	require.NoError(t, err)

	// Create imbalance manually (simulate corruption)
	_, err = db.ExecContext(ctx, `
	INSERT INTO entries (id, transaction_id, account_id, amount, entry_type, sequence, created_at)
	VALUES ($1, $2, $3, $4, 'debit', 1, NOW())
	`, "corrupt-entry-id", "corrupt-tx-id", cashAccount.ID, decimal.NewFromInt(500))
	require.NoError(t, err)

	// Wait for alert
	select {
	case result := <-alertReceived:
		assert.False(t, result.GlobalValid)
		assert.Greater(t, result.MismatchCount, 0)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Timeout waiting for reconciliation alert")
	}
}

func TestConcurrentTransactionsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	db := getTestDB(t)
	defer db.Close()
	defer cleanupTestDB(t, db)

	ctx := context.Background()
	l := NewLedger(db)

	merchantID := "test-merchant-concurrent"

	cashAccount := &Account{
		MerchantID: merchantID,
		Type:       AccountTypeAsset,
		Name:       "Cash",
		Balance:    decimal.NewFromInt(0),
		Currency:   "USD",
	}
	revenueAccount := &Account{
		MerchantID: merchantID,
		Type:       AccountTypeRevenue,
		Name:       "Revenue",
		Balance:    decimal.NewFromInt(0),
		Currency:   "USD",
	}

	err := l.CreateAccount(ctx, cashAccount)
	require.NoError(t, err)
	err = l.CreateAccount(ctx, revenueAccount)
	require.NoError(t, err)

	// Run 10 concurrent transactions
	const concurrency = 10
	errChan := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			amount := decimal.NewFromInt(int64(idx + 1) * 100)
			tx := &Transaction{
				MerchantID:     merchantID,
				Description:    fmt.Sprintf("Concurrent transaction %d", idx),
				IdempotencyKey: fmt.Sprintf("concurrent-tx-%d", idx),
				Entries: []Entry{
					{
						AccountID: cashAccount.ID,
						Amount:    amount,
						EntryType: "debit",
					},
					{
						AccountID: revenueAccount.ID,
						Amount:    amount,
						EntryType: "credit",
					},
				},
			}

			errChan <- l.CreateTransaction(ctx, tx)
		}(i)
	}

	// Check all transactions succeeded
	for i := 0; i < concurrency; i++ {
		err := <-errChan
		assert.NoError(t, err)
	}

	// Verify ledger is still balanced
	valid, err := l.VerifyIntegrity(ctx)
	require.NoError(t, err)
	assert.True(t, valid, "Ledger should remain balanced after concurrent transactions")
}