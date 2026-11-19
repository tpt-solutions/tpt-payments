package payment_test

// Database integration tests for the payment lifecycle.
//
// These run against a real PostgreSQL server and are skipped unless TEST_DATABASE_URL
// is set (see internal/testdb).

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/testdb"
)

type env struct {
	pool       *pgxpool.Pool
	svc        *payment.Service
	lgr        *ledger.Ledger
	merchantID string
}

// setup returns an isolated migrated database with a test merchant.
func setup(t *testing.T) *env {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	var merchantID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Test Merchant', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid",
	).Scan(&merchantID))

	lgr := ledger.New(pool)
	return &env{
		pool:       pool,
		svc:        payment.NewService(pool, sandbox.Default(), lgr),
		lgr:        lgr,
		merchantID: merchantID.String(),
	}
}

// scope is the test merchant in test mode.
func (e *env) scope() payment.Scope {
	return payment.Scope{MerchantID: e.merchantID}
}

// merchantBalance sums every ledger entry posted to the merchant's accounts.
func (e *env) merchantBalance(t *testing.T) int64 {
	t.Helper()
	var sum int64
	require.NoError(t, e.pool.QueryRow(context.Background(), `
		SELECT COALESCE(SUM(le.amount), 0)
		FROM ledger_entries le JOIN accounts a ON a.id = le.account_id
		WHERE a.merchant_id = $1
	`, e.merchantID).Scan(&sum))
	return sum
}

func (e *env) ledgerTxnCount(t *testing.T, intentID string) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM ledger_transactions WHERE reference_id = $1`, intentID,
	).Scan(&n))
	return n
}

func (e *env) newIntent(t *testing.T, amount int64, capture payment.CaptureMethod) *payment.Intent {
	t.Helper()
	intent, err := e.svc.Create(context.Background(), e.scope(), payment.CreateParams{
		Amount:        amount,
		Currency:      "usd",
		CaptureMethod: capture,
	})
	require.NoError(t, err)
	return intent
}

// Any payment method that is not a sandbox test card is approved.
const approvedPM = "pm_card_visa"

func TestIntegration_AutomaticCapture_HappyPath(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.newIntent(t, 1000, payment.CaptureAutomatic)

	got, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: approvedPM})
	require.NoError(t, err)

	assert.Equal(t, payment.StatusSucceeded, got.Status)
	assert.Equal(t, int64(1000), got.AmountCaptured)
	assert.NotEmpty(t, got.LedgerTxnID, "capture must link the ledger transaction")
	assert.NotEmpty(t, got.AdapterChargeID, "charge reference must be persisted")

	assert.Equal(t, 1, e.ledgerTxnCount(t, intent.ID))
	assert.Equal(t, int64(1000), e.merchantBalance(t))
	n, err := e.lgr.VerifyIntegrity(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "no unbalanced ledger transactions")
}

func TestIntegration_CardDecline_RollsBackAndAllowsRetry(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.newIntent(t, 500, payment.CaptureAutomatic)

	_, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: sandbox.CardDecline})
	var apiErr *apierrors.APIError
	require.True(t, errors.As(err, &apiErr), "decline must return an APIError, got %v", err)
	assert.Equal(t, 402, apiErr.StatusCode)

	cur, err := e.svc.Retrieve(ctx, intent.ID, e.scope())
	require.NoError(t, err)
	assert.Equal(t, payment.StatusRequiresPaymentMethod, cur.Status, "decline must allow retry")
	assert.Equal(t, 0, e.ledgerTxnCount(t, intent.ID))

	got, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: approvedPM})
	require.NoError(t, err)
	assert.Equal(t, payment.StatusSucceeded, got.Status)
}

func TestIntegration_ManualCapture_PartialCapturePostsCapturedAmount(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.newIntent(t, 1000, payment.CaptureManual)

	got, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: approvedPM})
	require.NoError(t, err)
	require.Equal(t, payment.StatusRequiresCapture, got.Status)
	assert.Equal(t, 0, e.ledgerTxnCount(t, intent.ID), "authorization alone must not post to the ledger")

	got, err = e.svc.Capture(ctx, intent.ID, e.scope(), payment.CaptureParams{AmountToCapture: 400})
	require.NoError(t, err)
	assert.Equal(t, payment.StatusSucceeded, got.Status)
	assert.Equal(t, int64(400), got.AmountCaptured)
	assert.Equal(t, int64(400), e.merchantBalance(t), "ledger must record the captured amount, not the authorized amount")
}

func TestIntegration_CaptureAmountValidation(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.newIntent(t, 1000, payment.CaptureManual)
	_, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: approvedPM})
	require.NoError(t, err)

	for _, bad := range []int64{-1, 1001} {
		_, err := e.svc.Capture(ctx, intent.ID, e.scope(), payment.CaptureParams{AmountToCapture: bad})
		assert.Error(t, err, "amount_to_capture=%d must be rejected", bad)
	}

	cur, err := e.svc.Retrieve(ctx, intent.ID, e.scope())
	require.NoError(t, err)
	assert.Equal(t, payment.StatusRequiresCapture, cur.Status, "rejected captures must not change state")
}

func TestIntegration_ConcurrentCapture_ExactlyOneWins(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.newIntent(t, 1000, payment.CaptureManual)
	_, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: approvedPM})
	require.NoError(t, err)

	const workers = 20
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Capture(ctx, intent.ID, e.scope(), payment.CaptureParams{}); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent capture may succeed")
	assert.Equal(t, 1, e.ledgerTxnCount(t, intent.ID), "exactly one ledger transaction may be posted")
	assert.Equal(t, int64(1000), e.merchantBalance(t), "the amount must be posted once")
}

func TestIntegration_ConcurrentConfirm_ExactlyOneAuthorizes(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.newIntent(t, 700, payment.CaptureAutomatic)

	const workers = 20
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		successes int
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: approvedPM}); err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, successes, "exactly one concurrent confirm may succeed")
	assert.Equal(t, 1, e.ledgerTxnCount(t, intent.ID))
	assert.Equal(t, int64(700), e.merchantBalance(t))
}

func TestIntegration_CancelThenCapture_Rejected(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.newIntent(t, 900, payment.CaptureManual)
	_, err := e.svc.Confirm(ctx, intent.ID, e.scope(), payment.ConfirmParams{PaymentMethodID: approvedPM})
	require.NoError(t, err)

	canceled, err := e.svc.Cancel(ctx, intent.ID, e.scope(), payment.CancelParams{Reason: "requested_by_customer"})
	require.NoError(t, err)
	assert.Equal(t, payment.StatusCanceled, canceled.Status)
	assert.NotNil(t, canceled.CanceledAt)

	_, err = e.svc.Capture(ctx, intent.ID, e.scope(), payment.CaptureParams{})
	assert.Error(t, err, "a canceled intent must never capture")
	assert.Equal(t, 0, e.ledgerTxnCount(t, intent.ID))
}

func TestIntegration_ForeignMerchantCannotAccessIntent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	intent := e.newIntent(t, 100, payment.CaptureAutomatic)

	_, err := e.svc.Retrieve(ctx, intent.ID, payment.Scope{MerchantID: uuid.New().String()})
	var apiErr *apierrors.APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 404, apiErr.StatusCode)
}

func TestIntegration_LedgerPostingIsIdempotent(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	merchantUUID := uuid.MustParse(e.merchantID)
	a, err := e.lgr.EnsureAccount(ctx, merchantUUID, "USD")
	require.NoError(t, err)
	b, err := e.lgr.EnsureAccount(ctx, uuid.MustParse("00000000-0000-0000-0000-000000000000"), "USD")
	require.NoError(t, err)
	again, err := e.lgr.EnsureAccount(ctx, merchantUUID, "USD")
	require.NoError(t, err)
	assert.Equal(t, a, again, "EnsureAccount must return the same account")

	entries := []ledger.Entry{
		{AccountID: a, Amount: 250, Currency: "USD"},
		{AccountID: b, Amount: -250, Currency: "USD"},
	}
	post := func() (uuid.UUID, bool) {
		var id uuid.UUID
		var created bool
		require.NoError(t, pgx.BeginFunc(ctx, e.pool, func(tx pgx.Tx) error {
			txn, c, err := e.lgr.PostTransactionTx(ctx, tx, ledger.TxnTypePaymentIntent, "pi_idem", entries)
			if err != nil {
				return err
			}
			id, created = txn.ID, c
			return nil
		}))
		return id, created
	}

	id1, created1 := post()
	id2, created2 := post()
	assert.True(t, created1)
	assert.False(t, created2, "second post with the same reference must not create a new transaction")
	assert.Equal(t, id1, id2)
	assert.Equal(t, 1, e.ledgerTxnCount(t, "pi_idem"))
}
