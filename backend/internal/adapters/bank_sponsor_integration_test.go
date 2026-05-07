// tpt-payments/backend/internal/adapters/bank_sponsor_integration_test.go
package adapters

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBankSponsorAdapterIntegration runs against the sponsor bank sandbox environment.
// These tests are skipped unless BANK_SPONSOR_SANDBOX_URL, BANK_SPONSOR_CLIENT_ID,
// and BANK_SPONSOR_CLIENT_SECRET environment variables are set.
func TestBankSponsorAdapterIntegration(t *testing.T) {
	sandboxURL := os.Getenv("BANK_SPONSOR_SANDBOX_URL")
	clientID := os.Getenv("BANK_SPONSOR_CLIENT_ID")
	clientSecret := os.Getenv("BANK_SPONSOR_CLIENT_SECRET")

	if sandboxURL == "" || clientID == "" || clientSecret == "" {
		t.Skip("skipping bank sponsor integration tests: set BANK_SPONSOR_SANDBOX_URL, BANK_SPONSOR_CLIENT_ID, BANK_SPONSOR_CLIENT_SECRET")
	}

	cfg := BankSponsorConfig{
		SandboxURL:   sandboxURL,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Timeout:      30 * time.Second,
	}

	adapter, err := NewBankSponsorAdapter(cfg)
	require.NoError(t, err, "failed to initialize bank sponsor adapter")
	defer adapter.Close()

	ctx := context.Background()

	t.Run("Authorize", func(t *testing.T) {
		req := AuthRequest{
			Amount:      1000, // $10.00
			Currency:    "USD",
			CardToken:   "tok_visa_sandbox", // Use sponsor bank's test token
			Idempotency: "test-auth-" + time.Now().Format("20060102150405"),
		}

		resp, err := adapter.Authorize(ctx, req)
		require.NoError(t, err, "authorize request failed")
		assert.NotEmpty(t, resp.TransactionID, "transaction ID should not be empty")
		assert.True(t, resp.Approved, "authorization should be approved in sandbox")
	})

	t.Run("Capture", func(t *testing.T) {
		// First authorize a payment
		authReq := AuthRequest{
			Amount:      2000, // $20.00
			Currency:    "USD",
			CardToken:   "tok_visa_sandbox",
			Idempotency: "test-capture-auth-" + time.Now().Format("20060102150405"),
		}

		authResp, err := adapter.Authorize(ctx, authReq)
		require.NoError(t, err, "authorize request failed")

		// Capture the authorized amount
		captureReq := CaptureRequest{
			TransactionID: authResp.TransactionID,
			Amount:        2000,
			Idempotency:   "test-capture-" + time.Now().Format("20060102150405"),
		}

		captureResp, err := adapter.Capture(ctx, captureReq)
		require.NoError(t, err, "capture request failed")
		assert.NotEmpty(t, captureResp.CaptureID, "capture ID should not be empty")
		assert.Equal(t, "succeeded", captureResp.Status, "capture should succeed")
	})

	t.Run("Refund", func(t *testing.T) {
		// Authorize and capture first
		authReq := AuthRequest{
			Amount:      1500, // $15.00
			Currency:    "USD",
			CardToken:   "tok_visa_sandbox",
			Idempotency: "test-refund-auth-" + time.Now().Format("20060102150405"),
		}

		authResp, err := adapter.Authorize(ctx, authReq)
		require.NoError(t, err)

		captureReq := CaptureRequest{
			TransactionID: authResp.TransactionID,
			Amount:        1500,
			Idempotency:   "test-refund-capture-" + time.Now().Format("20060102150405"),
		}

		captureResp, err := adapter.Capture(ctx, captureReq)
		require.NoError(t, err)

		// Refund the captured amount
		refundReq := RefundRequest{
			CaptureID:   captureResp.CaptureID,
			Amount:      1500,
			Idempotency: "test-refund-" + time.Now().Format("20060102150405"),
		}

		refundResp, err := adapter.Refund(ctx, refundReq)
		require.NoError(t, err, "refund request failed")
		assert.NotEmpty(t, refundResp.RefundID, "refund ID should not be empty")
		assert.Equal(t, "succeeded", refundResp.Status, "refund should succeed")
	})

	t.Run("Void", func(t *testing.T) {
		// Authorize a payment
		authReq := AuthRequest{
			Amount:      3000, // $30.00
			Currency:    "USD",
			CardToken:   "tok_visa_sandbox",
			Idempotency: "test-void-auth-" + time.Now().Format("20060102150405"),
		}

		authResp, err := adapter.Authorize(ctx, authReq)
		require.NoError(t, err)

		// Void the authorization
		voidReq := VoidRequest{
			TransactionID: authResp.TransactionID,
			Idempotency:   "test-void-" + time.Now().Format("20060102150405"),
		}

		voidResp, err := adapter.Void(ctx, voidReq)
		require.NoError(t, err, "void request failed")
		assert.NotEmpty(t, voidResp.VoidID, "void ID should not be empty")
		assert.Equal(t, "succeeded", voidResp.Status, "void should succeed")
	})

	t.Run("Tokenize", func(t *testing.T) {
		tokenReq := TokenizeRequest{
			CardNumber: "4111111111111111", // Visa test card
			ExpiryMonth: "12",
			ExpiryYear:  "2030",
			CVV:         "123",
			Idempotency: "test-tokenize-" + time.Now().Format("20060102150405"),
		}

		tokenResp, err := adapter.Tokenize(ctx, tokenReq)
		require.NoError(t, err, "tokenize request failed")
		assert.NotEmpty(t, tokenResp.Token, "token should not be empty")
		assert.Equal(t, "visa", tokenResp.CardBrand, "card brand should be visa")
	})
}