package ledger_test

// Reconciliation against real payment records. Skipped unless TEST_DATABASE_URL is set.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

// seedMerchantAndClearing creates a merchant with a USD account and a USD clearing account
// owned by a separate merchant, mirroring the capture posting.
func seedMerchantAndClearing(t *testing.T, pool *pgxpool.Pool) (merchant uuid.UUID, merchantAcct, clearingAcct uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Recon', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid",
	).Scan(&merchant))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO accounts (merchant_id, currency) VALUES ($1, 'USD') RETURNING id`, merchant,
	).Scan(&merchantAcct))
	var clearingOwner uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Clearing', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid",
	).Scan(&clearingOwner))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO accounts (merchant_id, currency) VALUES ($1, 'USD') RETURNING id`, clearingOwner,
	).Scan(&clearingAcct))
	return merchant, merchantAcct, clearingAcct
}

// insertSucceededIntent creates a succeeded intent whose amount_captured is recorded value.
// If postAmount is non-zero it posts a matching payment_intent transaction and links it.
func insertSucceededIntent(t *testing.T, lgr *ledger.Ledger, pool *pgxpool.Pool,
	merchant, merchantAcct, clearingAcct uuid.UUID, recorded, postAmount int64, link bool) string {
	t.Helper()
	ctx := context.Background()
	id := "pi_" + uuid.New().String()
	_, err := pool.Exec(ctx, `
		INSERT INTO payment_intents (id, merchant_id, amount, currency, status, amount_captured)
		VALUES ($1, $2, $3, 'USD', 'succeeded', $4)
	`, id, merchant, recorded, recorded)
	require.NoError(t, err)

	if postAmount == 0 {
		return id
	}
	txn, err := lgr.PostTransaction(ctx, ledger.TxnTypePaymentIntent, id, []ledger.Entry{
		{AccountID: merchantAcct, Amount: postAmount, Currency: "USD"},
		{AccountID: clearingAcct, Amount: -postAmount, Currency: "USD"},
	})
	require.NoError(t, err)
	if link {
		_, err = pool.Exec(ctx, `UPDATE payment_intents SET ledger_txn_id = $1 WHERE id = $2`, txn.ID, id)
		require.NoError(t, err)
	}
	return id
}

// kindsBySubject maps each subject to the discrepancy kinds reported for it.
func kindsBySubject(r ledger.ReconcileReport) map[string][]string {
	out := map[string][]string{}
	for _, d := range r.Discrepancies {
		out[d.Subject] = append(out[d.Subject], d.Kind)
	}
	return out
}

func TestReconcile_HealthyLedgerReportsNothing(t *testing.T) {
	pool := testdb.Open(t)
	lgr := ledger.New(pool)
	_, merchantAcct, clearingAcct := seedMerchantAndClearing(t, pool)
	merchant := uuid.Nil
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT merchant_id FROM accounts WHERE id = $1`, merchantAcct).Scan(&merchant))

	insertSucceededIntent(t, lgr, pool, merchant, merchantAcct, clearingAcct, 1000, 1000, true)

	report, err := lgr.Reconcile(context.Background())
	require.NoError(t, err)
	assert.True(t, report.Healthy(), "unexpected discrepancies: %+v", report.Discrepancies)
}

func TestReconcile_FlagsEachKindOfDivergence(t *testing.T) {
	pool := testdb.Open(t)
	lgr := ledger.New(pool)
	ctx := context.Background()
	merchant, merchantAcct, clearingAcct := seedMerchantAndClearing(t, pool)

	// Succeeded, but never posted.
	notPosted := insertSucceededIntent(t, lgr, pool, merchant, merchantAcct, clearingAcct, 500, 0, false)
	// Posted for 1000 but the intent records 999 captured.
	mismatched := insertSucceededIntent(t, lgr, pool, merchant, merchantAcct, clearingAcct, 999, 1000, true)
	// Posted but the link was never written back to the intent.
	unlinked := insertSucceededIntent(t, lgr, pool, merchant, merchantAcct, clearingAcct, 700, 700, false)

	// A payment posting that refers to no intent.
	_, err := lgr.PostTransaction(ctx, ledger.TxnTypePaymentIntent, "pi_missing", []ledger.Entry{
		{AccountID: merchantAcct, Amount: 300, Currency: "USD"},
		{AccountID: clearingAcct, Amount: -300, Currency: "USD"},
	})
	require.NoError(t, err)

	report, err := lgr.Reconcile(ctx)
	require.NoError(t, err)
	found := kindsBySubject(report)

	assert.Contains(t, found[notPosted], ledger.DiscrepancyIntentNotPosted)
	assert.Contains(t, found[mismatched], ledger.DiscrepancyIntentAmountMismatch)
	assert.Contains(t, found[unlinked], ledger.DiscrepancyIntentNotPosted)
	assert.Contains(t, found["pi_missing"], ledger.DiscrepancyOrphanPayment)
	assert.False(t, report.Healthy())
}

func TestReconcile_RefundPostingMustMatch(t *testing.T) {
	pool := testdb.Open(t)
	lgr := ledger.New(pool)
	ctx := context.Background()
	merchant, merchantAcct, clearingAcct := seedMerchantAndClearing(t, pool)

	pi := insertSucceededIntent(t, lgr, pool, merchant, merchantAcct, clearingAcct, 1000, 1000, true)

	refundID := "re_" + uuid.New().String()
	_, err := pool.Exec(ctx, `
		INSERT INTO refunds (id, payment_intent_id, merchant_id, amount, currency, status)
		VALUES ($1, $2, $3, 400, 'USD', 'succeeded')
	`, refundID, pi, merchant)
	require.NoError(t, err)

	// Refund posting says 300, the refund record says 400.
	txn, err := lgr.PostTransaction(ctx, ledger.TxnTypeRefund, refundID, []ledger.Entry{
		{AccountID: merchantAcct, Amount: -300, Currency: "USD"},
		{AccountID: clearingAcct, Amount: 300, Currency: "USD"},
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE refunds SET ledger_txn_id = $1 WHERE id = $2`, txn.ID, refundID)
	require.NoError(t, err)

	report, err := lgr.Reconcile(ctx)
	require.NoError(t, err)
	assert.Contains(t, kindsBySubject(report)[refundID], ledger.DiscrepancyRefundAmountMismatch)
}
