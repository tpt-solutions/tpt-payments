package ledger_test

// Database-level ledger invariants (migration 16). These write raw SQL on purpose: the point
// is that the database refuses the write even when the application's checks are bypassed.
// Skipped unless TEST_DATABASE_URL is set.

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

// seedAccounts creates a merchant with a USD account and an EUR account.
func seedAccounts(t *testing.T, pool *pgxpool.Pool) (usd, eur uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	var merchant uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Ledger Test', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid",
	).Scan(&merchant))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO accounts (merchant_id, currency) VALUES ($1, 'USD') RETURNING id`, merchant,
	).Scan(&usd))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO accounts (merchant_id, currency) VALUES ($1, 'EUR') RETURNING id`, merchant,
	).Scan(&eur))
	return usd, eur
}

// insertTxn creates a transaction row and returns its ID, inside tx.
func insertTxn(t *testing.T, ctx context.Context, tx pgx.Tx, ref string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	require.NoError(t, tx.QueryRow(ctx,
		`INSERT INTO ledger_transactions (type, status, reference_id) VALUES ('payment_intent', 'posted', $1) RETURNING id`,
		ref,
	).Scan(&id))
	return id
}

func TestLedgerInvariant_UnbalancedTransactionFailsAtCommit(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	usd, _ := seedAccounts(t, pool)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	txn := insertTxn(t, ctx, tx, "unbalanced-1")
	_, err = tx.Exec(ctx,
		`INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1, $2, 1000, 'USD')`,
		txn, usd)
	require.NoError(t, err, "the check is deferred, so a single entry is accepted mid-transaction")
	_, err = tx.Exec(ctx,
		`INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1, $2, -900, 'USD')`,
		txn, usd)
	require.NoError(t, err)

	err = tx.Commit(ctx)
	require.Error(t, err, "a transaction whose entries do not sum to zero must not commit")
	assert.Contains(t, err.Error(), "not balanced")
}

func TestLedgerInvariant_BalancedTransactionCommits(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	usd, _ := seedAccounts(t, pool)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	txn := insertTxn(t, ctx, tx, "balanced-1")
	for _, amt := range []int64{1000, -1000} {
		_, err := tx.Exec(ctx,
			`INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1, $2, $3, 'USD')`,
			txn, usd, amt)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(ctx))
}

func TestLedgerInvariant_EntryCurrencyMustMatchAccount(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	usd, _ := seedAccounts(t, pool)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()

	txn := insertTxn(t, ctx, tx, "currency-1")
	_, err = tx.Exec(ctx,
		`INSERT INTO ledger_entries (transaction_id, account_id, amount, currency) VALUES ($1, $2, 1000, 'EUR')`,
		txn, usd)
	require.Error(t, err, "an EUR entry must not post to a USD account")
	assert.Contains(t, err.Error(), "does not match account currency")
}

func TestLedgerInvariant_TruncateIsForbidden(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()

	// ledger_entries has no inbound foreign keys, so the append-only trigger is what refuses.
	_, err := pool.Exec(ctx, "TRUNCATE ledger_entries")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "append-only")

	// ledger_transactions is referenced by ledger_entries and refunds, so PostgreSQL refuses
	// the TRUNCATE on the foreign key before the trigger runs. Either way it must fail.
	_, err = pool.Exec(ctx, "TRUNCATE ledger_transactions")
	require.Error(t, err, "TRUNCATE on ledger_transactions must be refused")
}

// The service-level posting path still works with the database invariants in place.
func TestLedgerInvariant_PostTransactionStillWorks(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	usd, _ := seedAccounts(t, pool)

	lgr := ledger.New(pool)
	// The platform merchant (uuid.Nil) is seeded by migration 9 and owns the clearing account.
	clearing, err := lgr.EnsureAccount(ctx, uuid.Nil, "USD")
	require.NoError(t, err)

	_, err = lgr.PostTransaction(ctx, ledger.TxnTypePaymentIntent, "pi_post_ok", []ledger.Entry{
		{AccountID: usd, Amount: 500, Currency: "USD"},
		{AccountID: clearing, Amount: -500, Currency: "USD"},
	})
	require.NoError(t, err)
}

func TestLedger_BalancesArePerCurrency(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	usd, eur := seedAccounts(t, pool)
	lgr := ledger.New(pool)
	clearing, err := lgr.EnsureAccount(ctx, uuid.Nil, "USD")
	require.NoError(t, err)

	_, err = lgr.PostTransaction(ctx, ledger.TxnTypePaymentIntent, "pi_balance_usd", []ledger.Entry{
		{AccountID: usd, Amount: 700, Currency: "USD"},
		{AccountID: clearing, Amount: -700, Currency: "USD"},
	})
	require.NoError(t, err)

	b, err := lgr.Balance(ctx, usd)
	require.NoError(t, err)
	assert.Equal(t, "USD", b.Currency)
	assert.Equal(t, int64(700), b.Amount)

	var merchant uuid.UUID
	require.NoError(t, pool.QueryRow(ctx, `SELECT merchant_id FROM accounts WHERE id = $1`, usd).Scan(&merchant))
	all, err := lgr.MerchantBalances(ctx, merchant)
	require.NoError(t, err)
	require.Len(t, all, 2, "one balance per currency account, never summed across currencies")
	assert.Equal(t, "EUR", all[0].Currency)
	assert.Equal(t, int64(0), all[0].Amount, "an empty account reports zero")
	assert.Equal(t, eur, all[0].AccountID)
	assert.Equal(t, "USD", all[1].Currency)
	assert.Equal(t, int64(700), all[1].Amount)

	_, err = lgr.Balance(ctx, uuid.New())
	require.Error(t, err, "an unknown account is an error, not a zero balance")
}

func TestLedger_IntegrityJobReportsAndStops(t *testing.T) {
	pool := testdb.Open(t)
	lgr := ledger.New(pool)

	ctx, cancel := context.WithCancel(context.Background())
	reports := make(chan int, 10)
	done := make(chan struct{})
	go func() {
		lgr.RunIntegrityJob(ctx, 10*time.Millisecond, func(unbalanced int, err error) {
			require.NoError(t, err)
			reports <- unbalanced
		})
		close(done)
	}()

	select {
	case n := <-reports:
		assert.Equal(t, 0, n, "a freshly migrated ledger is balanced")
	case <-time.After(5 * time.Second):
		t.Fatal("integrity job never ran")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("integrity job did not stop on cancel")
	}
}
