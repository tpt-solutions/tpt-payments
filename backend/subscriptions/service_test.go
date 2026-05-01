// tpt-payments/backend/subscriptions/service_test.go
package subscriptions

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

type mockPaymentProcessor struct{}

func (m *mockPaymentProcessor) ChargeCustomer(ctx context.Context, customerID string, amount decimal.Decimal, currency string, description string) (string, error) {
	return "pi_123", nil
}

type mockWebhookEmitter struct{}

func (m *mockWebhookEmitter) Emit(ctx context.Context, eventType string, data interface{}) error {
	return nil
}

type mockInvoiceService struct{}

func (m *mockInvoiceService) CreateForSubscription(ctx context.Context, subscriptionID string, customerID string, amount decimal.Decimal, currency string) (string, error) {
	return "inv_123", nil
}

func TestSubscriptionLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock db: %v", err)
	}
	defer db.Close()

	payments := &mockPaymentProcessor{}
	webhook := &mockWebhookEmitter{}
	invoicing := &mockInvoiceService{}

	service := NewService(db, payments, webhook, invoicing)

	t.Run("Create subscription", func(t *testing.T) {
		req := CreateSubscriptionRequest{
			CustomerID: "cus_123",
			PriceID:    "price_123",
			TrialDays:  7,
		}

		mock.ExpectBegin()

		priceRows := sqlmock.NewRows([]string{"id", "product_id", "amount", "currency", "active", "interval", "interval_count", "trial_period_days"}).
			AddRow("price_123", "prod_123", decimal.NewFromInt(9900), "usd", true, BillingIntervalMonth, 1, 7)

		mock.ExpectQuery("SELECT id, product_id, amount, currency, active, interval, interval_count, trial_period_days FROM prices WHERE id = \\$1").
			WithArgs("price_123").
			WillReturnRows(priceRows)

		mock.ExpectExec("INSERT INTO subscriptions").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectCommit()

		sub, err := service.Create(context.Background(), req)

		assert.NoError(t, err)
		assert.NotNil(t, sub)
		assert.Equal(t, "cus_123", sub.CustomerID)
		assert.Equal(t, SubscriptionStatusTrialing, sub.Status)
	})

	t.Run("Cancel subscription immediately", func(t *testing.T) {
		subID := "sub_test_123"

		mock.ExpectBegin()

		subRows := sqlmock.NewRows([]string{"id", "customer_id", "price_id", "status", "current_period_start", "current_period_end", "cancel_at_period_end", "canceled_at", "ended_at", "trial_start", "trial_end", "metadata"}).
			AddRow(subID, "cus_123", "price_123", SubscriptionStatusActive, time.Now().Unix(), time.Now().AddDate(0, 1, 0).Unix(), false, 0, 0, 0, 0, []byte("{}"))

		mock.ExpectQuery("SELECT id, customer_id, price_id, status, current_period_start, current_period_end, cancel_at_period_end, canceled_at, ended_at, trial_start, trial_end, metadata FROM subscriptions WHERE id = \\$1 FOR UPDATE").
			WithArgs(subID).
			WillReturnRows(subRows)

		mock.ExpectExec("UPDATE subscriptions SET status = \\$1, cancel_at_period_end = \\$2, canceled_at = \\$3, ended_at = \\$4, updated_at = \\$5 WHERE id = \\$6").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectCommit()

		sub, err := service.Cancel(context.Background(), subID, false)

		assert.NoError(t, err)
		assert.Equal(t, SubscriptionStatusCanceled, sub.Status)
		assert.NotZero(t, sub.EndedAt)
	})

	t.Run("Cancel subscription at period end", func(t *testing.T) {
		subID := "sub_test_456"

		mock.ExpectBegin()

		subRows := sqlmock.NewRows([]string{"id", "customer_id", "price_id", "status", "current_period_start", "current_period_end", "cancel_at_period_end", "canceled_at", "ended_at", "trial_start", "trial_end", "metadata"}).
			AddRow(subID, "cus_123", "price_123", SubscriptionStatusActive, time.Now().Unix(), time.Now().AddDate(0, 1, 0).Unix(), false, 0, 0, 0, 0, []byte("{}"))

		mock.ExpectQuery("SELECT id, customer_id, price_id, status, current_period_start, current_period_end, cancel_at_period_end, canceled_at, ended_at, trial_start, trial_end, metadata FROM subscriptions WHERE id = \\$1 FOR UPDATE").
			WithArgs(subID).
			WillReturnRows(subRows)

		mock.ExpectExec("UPDATE subscriptions SET status = \\$1, cancel_at_period_end = \\$2, canceled_at = \\$3, ended_at = \\$4, updated_at = \\$5 WHERE id = \\$6").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectCommit()

		sub, err := service.Cancel(context.Background(), subID, true)

		assert.NoError(t, err)
		assert.Equal(t, SubscriptionStatusActive, sub.Status)
		assert.True(t, sub.CancelAtPeriodEnd)
		assert.Zero(t, sub.EndedAt)
	})

	t.Run("Process renewal success", func(t *testing.T) {
		subID := "sub_test_789"
		now := time.Now()

		mock.ExpectBegin()

		subRows := sqlmock.NewRows([]string{"s.id", "s.customer_id", "s.price_id", "s.status", "s.current_period_start", "s.current_period_end", "s.cancel_at_period_end", "s.attempt_count", "p.amount", "p.currency"}).
			AddRow(subID, "cus_123", "price_123", SubscriptionStatusActive, now.AddDate(0, -1, 0).Unix(), now.Unix(), false, 0, decimal.NewFromInt(9900), "usd")

		mock.ExpectQuery("SELECT s.id, s.customer_id, s.price_id, s.status, s.current_period_start, s.current_period_end, s.cancel_at_period_end, s.attempt_count, p.amount, p.currency FROM subscriptions s JOIN prices p ON s.price_id = p.id WHERE s.id = \\$1 FOR UPDATE").
			WithArgs(subID).
			WillReturnRows(subRows)

		mock.ExpectExec("UPDATE subscriptions SET status = \\$1, current_period_start = \\$2, current_period_end = \\$3, attempt_count = \\$4, updated_at = \\$5 WHERE id = \\$6").
			WillReturnResult(sqlmock.NewResult(1, 1))

		mock.ExpectCommit()

		err := service.ProcessRenewal(context.Background(), subID)

		assert.NoError(t, err)
	})

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestCalculateProration(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create mock db: %v", err)
	}
	defer db.Close()

	service := NewService(db, &mockPaymentProcessor{}, &mockWebhookEmitter{}, &mockInvoiceService{})

	subID := "sub_prorate_123"
	now := time.Now()
	periodStart := now.AddDate(0, 0, -15).Unix()
	periodEnd := now.AddDate(0, 0, 15).Unix()

	subRows := sqlmock.NewRows([]string{"s.current_period_start", "s.current_period_end", "p.amount", "p.currency"}).
		AddRow(periodStart, periodEnd, decimal.NewFromInt(9900), "usd")

	mock.ExpectQuery("SELECT s.current_period_start, s.current_period_end, p.amount, p.currency FROM subscriptions s JOIN prices p ON s.price_id = p.id WHERE s.id = \\$1").
		WithArgs(subID).
		WillReturnRows(subRows)

	newPriceRows := sqlmock.NewRows([]string{"amount", "currency"}).
		AddRow(decimal.NewFromInt(19900), "usd")

	mock.ExpectQuery("SELECT amount, currency FROM prices WHERE id = \\$1").
		WithArgs("price_new").
		WillReturnRows(newPriceRows)

	proration, err := service.CalculateProration(context.Background(), subID, "price_new")

	assert.NoError(t, err)
	assert.True(t, proration.GreaterThan(decimal.Zero))
}