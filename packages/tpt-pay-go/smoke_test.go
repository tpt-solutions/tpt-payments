package tptpay

import (
	"context"
	"os"
	"testing"
)

// TestAgainstARunningServer exercises the SDK against a real TPT Pay server. It is skipped unless
// TPT_PAY_TEST_URL and TPT_PAY_TEST_KEY (a test secret key) are set, so it never runs by accident.
func TestAgainstARunningServer(t *testing.T) {
	url, key := os.Getenv("TPT_PAY_TEST_URL"), os.Getenv("TPT_PAY_TEST_KEY")
	if url == "" || key == "" {
		t.Skip("set TPT_PAY_TEST_URL and TPT_PAY_TEST_KEY to run against a server")
	}
	ctx := context.Background()
	c, err := New(key, WithBaseURL(url))
	if err != nil {
		t.Fatal(err)
	}

	pi, err := c.PaymentIntents.Create(ctx, CreatePaymentIntentParams{Amount: 2500, Currency: "usd", CaptureMethod: "automatic"}, WithIdempotencyKey(newIdempotencyKey()))
	if err != nil {
		t.Fatalf("create intent: %v", err)
	}
	pi, err = c.PaymentIntents.Confirm(ctx, pi.ID, ConfirmParams{PaymentMethod: "pm_card_visa"})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if pi.Status != "succeeded" {
		t.Fatalf("status = %s, want succeeded", pi.Status)
	}

	refund, err := c.Refunds.Create(ctx, CreateRefundParams{PaymentIntent: pi.ID, Amount: 500, Reason: "requested_by_customer"})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if got, err := c.Refunds.Retrieve(ctx, refund.ID); err != nil || got.Amount != 500 {
		t.Fatalf("retrieve refund: %v %+v", err, got)
	}

	sub, err := c.Subscriptions.Create(ctx, CreateSubscriptionParams{Amount: 1500, Currency: "usd", Interval: "month", PaymentMethod: "pm_card_visa"})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if sub.Status != "active" {
		t.Fatalf("subscription status = %s, want active after its first charge", sub.Status)
	}
	canceled, err := c.Subscriptions.Cancel(ctx, sub.ID, false)
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if canceled.Status != "canceled" {
		t.Fatalf("status after cancel = %s", canceled.Status)
	}

	_, err = c.PaymentIntents.Retrieve(ctx, "pi_does_not_exist")
	if !IsNotFound(err) {
		t.Fatalf("a missing intent must be not found, got %v", err)
	}
}
