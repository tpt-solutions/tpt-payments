package subscription_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/subscription"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

// env is a subscription service on a sandbox clock, with one merchant in test mode.
type env struct {
	pool  *pgxpool.Pool
	clk   *clock.Test
	sb    *sandbox.Sandbox
	subs  *subscription.Service
	scope subscription.Scope
}

func newEnv(t *testing.T) *env {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	var merchant string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Billing Merchant', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid").Scan(&merchant))
	// An endpoint that receives every event, so the outbox rows can be checked.
	_, err := pool.Exec(ctx, `
		INSERT INTO webhook_endpoints (id, merchant_id, url, enabled_events, secret_ciphertext, secret_prefix, livemode)
		VALUES ($1, $2, 'https://example.com/hooks', '{*}', 'v1:test', 'whsec_test', FALSE)
	`, "we_"+uuid.New().String(), merchant)
	require.NoError(t, err)

	clk := clock.NewTest(time.Date(2030, 5, 1, 12, 0, 0, 0, time.UTC))
	sb := sandbox.New(sandbox.Config{Clock: clk})
	pay := payment.NewService(pool, sb, ledger.New(pool))
	return &env{
		pool:  pool,
		clk:   clk,
		sb:    sb,
		subs:  subscription.NewService(pool, pay, clk),
		scope: subscription.Scope{MerchantID: merchant},
	}
}

func (e *env) create(t *testing.T, pm string, interval subscription.Interval) *subscription.Subscription {
	t.Helper()
	sub, err := e.subs.Create(context.Background(), e.scope, subscription.CreateParams{
		Amount: 1000, Currency: "usd", Interval: interval, PaymentMethodID: pm,
	})
	require.NoError(t, err)
	return sub
}

// intentsFor counts the payment intents created for a subscription, and how many succeeded.
func (e *env) intentsFor(t *testing.T, subID string) (total, succeeded int) {
	t.Helper()
	require.NoError(t, e.pool.QueryRow(context.Background(), `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE status = 'succeeded')
		FROM payment_intents WHERE metadata->>'subscription_id' = $1
	`, subID).Scan(&total, &succeeded))
	return total, succeeded
}

// events returns how many webhook deliveries of the given type were queued.
func (e *env) events(t *testing.T, eventType string) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM webhook_deliveries WHERE event_type = $1`, eventType).Scan(&n))
	return n
}

// setTime moves the sandbox clock forward. The clock refuses to go back, which is a test error.
func (e *env) setTime(t *testing.T, to time.Time) {
	t.Helper()
	_, err := e.clk.Set(to)
	require.NoError(t, err)
}

func (e *env) reload(t *testing.T, id string) *subscription.Subscription {
	t.Helper()
	sub, err := e.subs.Retrieve(context.Background(), e.scope, id)
	require.NoError(t, err)
	return sub
}

func TestSubscription_FirstChargeThenMonthlyRenewal(t *testing.T) {
	e := newEnv(t)
	start := e.clk.Now()

	sub := e.create(t, "pm_card_visa", subscription.IntervalMonth)
	assert.Equal(t, subscription.StatusActive, sub.Status)
	// Create returns the subscription after its first charge, so the period it shows is the next one.
	assert.True(t, sub.PeriodStart.Equal(start.AddDate(0, 1, 0)), "the first charge covered the period that began at creation")
	assert.True(t, sub.PeriodEnd.Equal(start.AddDate(0, 2, 0)))
	assert.True(t, sub.NextAttemptAt.Equal(sub.PeriodStart), "the next charge is due when the next period starts")
	total, ok := e.intentsFor(t, sub.ID)
	assert.Equal(t, 1, total)
	assert.Equal(t, 1, ok, "the first period is charged on creation")
	assert.Equal(t, 1, e.events(t, "subscription.created"))

	// Nothing is due before the period ends.
	n, err := e.subs.RunDue(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// When the next period starts it is charged, and the period moves on.
	e.setTime(t, sub.PeriodStart)
	n, err = e.subs.RunDue(context.Background(), 10)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	renewed := e.reload(t, sub.ID)
	assert.Equal(t, subscription.StatusActive, renewed.Status)
	assert.True(t, renewed.PeriodStart.Equal(sub.PeriodEnd))
	assert.True(t, renewed.PeriodEnd.Equal(sub.PeriodEnd.AddDate(0, 1, 0)))
	total, ok = e.intentsFor(t, sub.ID)
	assert.Equal(t, 2, total)
	assert.Equal(t, 2, ok)
}

func TestSubscription_ConcurrentBillingChargesEachPeriodOnce(t *testing.T) {
	e := newEnv(t)
	sub := e.create(t, "pm_card_visa", subscription.IntervalWeek)
	e.setTime(t, sub.PeriodStart)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := e.subs.Bill(context.Background(), sub.ID)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	total, _ := e.intentsFor(t, sub.ID)
	assert.Equal(t, 2, total, "ten billing runs must create one renewal intent, not ten")
	renewed := e.reload(t, sub.ID)
	assert.True(t, renewed.PeriodStart.Equal(sub.PeriodEnd), "the period advances exactly once")
}

func TestSubscription_DeclinedCreditRetriesThenCancels(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()

	sub := e.create(t, "pm_card_declined", subscription.IntervalMonth)
	assert.Equal(t, subscription.StatusPastDue, sub.Status, "a declined first charge is past_due, not an error")
	assert.Equal(t, 1, sub.RetryCount)
	assert.Equal(t, e.clk.Now().Add(24*time.Hour), sub.NextAttemptAt)
	assert.Equal(t, 1, e.events(t, "subscription.past_due"))
	assert.Equal(t, 0, e.events(t, "subscription.canceled"))

	// Each retry waits the scheduled time; the declines keep coming back.
	for i, wait := range subscription.RetrySchedule {
		e.setTime(t, e.clk.Now().Add(wait))
		_, err := e.subs.RunDue(ctx, 10)
		require.NoError(t, err)
		got := e.reload(t, sub.ID)
		if i < len(subscription.RetrySchedule)-1 {
			assert.Equal(t, subscription.StatusPastDue, got.Status)
			assert.Equal(t, i+2, got.RetryCount)
		}
	}

	// The retries are used up: the next failure cancels.
	final := e.reload(t, sub.ID)
	assert.Equal(t, subscription.StatusCanceled, final.Status)
	assert.Equal(t, subscription.CancelReasonPaymentFailed, final.CancelReason)
	require.NotNil(t, final.CanceledAt)
	assert.Equal(t, 1, e.events(t, "subscription.canceled"))
	assert.Equal(t, 1, e.events(t, "subscription.past_due"), "past_due is sent once, on the first failure")

	// A canceled subscription is never billed again.
	e.setTime(t, e.clk.Now().Add(365*24*time.Hour))
	n, err := e.subs.RunDue(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestSubscription_UnknownOutcomeHoldsWithoutRetryingOrCanceling(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := e.create(t, "pm_card_visa", subscription.IntervalMonth)

	// The card network does not answer when the renewal is charged.
	outageStart := sub.PeriodEnd
	require.NoError(t, e.sb.AddOutage(sandbox.Outage{From: outageStart, To: outageStart.Add(time.Hour)}))
	e.setTime(t, outageStart)
	_, err := e.subs.RunDue(ctx, 10)
	require.NoError(t, err)
	held := e.reload(t, sub.ID)
	assert.Equal(t, subscription.StatusPastDue, held.Status)
	assert.Equal(t, 0, held.RetryCount, "an unknown outcome is not a failed retry")
	assert.Equal(t, outageStart.Add(24*time.Hour), held.NextAttemptAt)

	// Later runs do not charge again while the first intent is unresolved, and do not cancel.
	e.sb.ClearOutages()
	for i := 0; i < 5; i++ {
		e.setTime(t, e.clk.Now().Add(25*time.Hour))
		_, err := e.subs.RunDue(ctx, 10)
		require.NoError(t, err)
	}
	stillHeld := e.reload(t, sub.ID)
	assert.Equal(t, subscription.StatusPastDue, stillHeld.Status)
	assert.Equal(t, 0, stillHeld.RetryCount)
	total, _ := e.intentsFor(t, sub.ID)
	assert.Equal(t, 2, total, "no second intent is created for the same period")
}

func TestSubscription_CancelAtPeriodEndTakesEffectWhenThePeriodEnds(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := e.create(t, "pm_card_visa", subscription.IntervalMonth)

	scheduled, err := e.subs.Cancel(ctx, e.scope, sub.ID, true)
	require.NoError(t, err)
	assert.Equal(t, subscription.StatusActive, scheduled.Status, "still running until the period ends")
	assert.True(t, scheduled.CancelAtPeriodEnd)

	e.setTime(t, sub.PeriodEnd)
	_, err = e.subs.RunDue(ctx, 10)
	require.NoError(t, err)
	got := e.reload(t, sub.ID)
	assert.Equal(t, subscription.StatusCanceled, got.Status)
	assert.Equal(t, subscription.CancelReasonAtPeriodEnd, got.CancelReason)
	total, _ := e.intentsFor(t, sub.ID)
	assert.Equal(t, 1, total, "no renewal is charged after a cancellation at period end")
}

func TestSubscription_CancelNowStopsBillingAndCannotRepeat(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := e.create(t, "pm_card_visa", subscription.IntervalMonth)

	canceled, err := e.subs.Cancel(ctx, e.scope, sub.ID, false)
	require.NoError(t, err)
	assert.Equal(t, subscription.StatusCanceled, canceled.Status)
	assert.Equal(t, subscription.CancelReasonRequested, canceled.CancelReason)
	assert.Equal(t, 1, e.events(t, "subscription.canceled"))

	_, err = e.subs.Cancel(ctx, e.scope, sub.ID, false)
	assert.Error(t, err, "a canceled subscription cannot be canceled again")

	e.setTime(t, sub.PeriodEnd)
	n, err := e.subs.RunDue(ctx, 10)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestSubscription_ScopeAndValidation(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	sub := e.create(t, "pm_card_visa", subscription.IntervalYear)

	other := subscription.Scope{MerchantID: uuid.New().String()}
	_, err := e.subs.Retrieve(ctx, other, sub.ID)
	assert.Error(t, err, "another merchant cannot read the subscription")
	live := subscription.Scope{MerchantID: e.scope.MerchantID, Livemode: true}
	_, err = e.subs.Retrieve(ctx, live, sub.ID)
	assert.Error(t, err, "live mode cannot read test subscriptions")

	_, err = e.subs.Create(ctx, live, subscription.CreateParams{Amount: 1000, Currency: "usd", Interval: subscription.IntervalMonth, PaymentMethodID: "pm_card_visa"})
	assert.Error(t, err, "subscriptions are refused in live mode")

	_, err = e.subs.Create(ctx, e.scope, subscription.CreateParams{Amount: 1000, Currency: "usd", Interval: "day", PaymentMethodID: "pm_card_visa"})
	assert.Error(t, err, "daily intervals are not offered")
	_, err = e.subs.Create(ctx, e.scope, subscription.CreateParams{Amount: 0, Currency: "usd", Interval: subscription.IntervalMonth, PaymentMethodID: "pm_card_visa"})
	assert.Error(t, err, "a zero amount is refused")
	_, err = e.subs.Create(ctx, e.scope, subscription.CreateParams{Amount: 1000, Currency: "usd", Interval: subscription.IntervalMonth})
	assert.Error(t, err, "a payment method is required")
}

func TestSubscription_ListPagesNewestFirst(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, e.create(t, "pm_card_visa", subscription.IntervalMonth).ID)
		e.setTime(t, e.clk.Now().Add(time.Minute)) // distinct creation times give a stable order
	}

	first, more, err := e.subs.List(ctx, subscription.ListParams{Scope: e.scope, Limit: 2})
	require.NoError(t, err)
	require.Len(t, first, 2)
	assert.True(t, more)
	assert.Equal(t, ids[2], first[0].ID, "newest first")

	second, more, err := e.subs.List(ctx, subscription.ListParams{Scope: e.scope, Limit: 2, StartingAfter: first[1].ID})
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.False(t, more)
	assert.Equal(t, ids[0], second[0].ID)
}
