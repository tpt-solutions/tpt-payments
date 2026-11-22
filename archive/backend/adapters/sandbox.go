// tpt-payments/backend/adapters/sandbox.go
package adapters

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SandboxAdapter is a test implementation of CardNetworkAdapter
// Provides configurable success/failure scenarios and realistic testing behaviour
type SandboxAdapter struct {
	ArtificialLatency time.Duration
	TestCards         map[string]SandboxCardConfig
	WebhookURL        string
	WebhookSecret     string
	WebhookEnabled    bool
}

// SandboxCardConfig defines behaviour for specific test card numbers
type SandboxCardConfig struct {
	AlwaysApprove    bool
	AlwaysDecline    bool
	DeclineReason    string
	ApprovalCode     string
	NetworkID        string
}

// NewSandboxAdapter creates a new SandboxAdapter with default test cards
func NewSandboxAdapter() *SandboxAdapter {
	return &SandboxAdapter{
		ArtificialLatency: 150 * time.Millisecond,
		WebhookEnabled:    false,
		TestCards: map[string]SandboxCardConfig{
			"4242424242424242": {AlwaysApprove: true, ApprovalCode: "TEST_OK"},
			"4000000000000002": {AlwaysDecline: true, DeclineReason: ErrCardDeclined},
			"4000000000009995": {AlwaysDecline: true, DeclineReason: ErrInsufficientFunds},
			"4000000000000069": {AlwaysDecline: true, DeclineReason: ErrExpiredCard},
			"4000000000000127": {AlwaysDecline: true, DeclineReason: ErrInvalidCVV},
		},
	}
}

// Authorize implements CardNetworkAdapter
func (s *SandboxAdapter) Authorize(ctx context.Context, req AuthRequest) (*AuthResponse, error) {
	s.injectLatency()

	resp := &AuthResponse{
		AuthorizationID: uuid.NewString(),
		CreatedAt:       time.Now().UTC().Unix(),
		ExpiresAt:       time.Now().UTC().Add(7 * 24 * time.Hour).Unix(),
	}

	if config, ok := s.TestCards[req.CardToken]; ok {
		if config.AlwaysDecline {
			resp.Status = "declined"
			resp.ErrorCode = config.DeclineReason
			resp.ErrorMessage = s.getErrorMessage(config.DeclineReason)
			return resp, nil
		}

		if config.AlwaysApprove {
			resp.Status = "authorized"
			resp.ApprovalCode = config.ApprovalCode
			resp.NetworkID = "sandbox_" + randomHex(12)
			return resp, nil
		}
	}

	if req.Amount > 1000000 { // > $10,000
		resp.Status = "declined"
		resp.ErrorCode = ErrCardDeclined
		resp.ErrorMessage = "Amount exceeds sandbox transaction limit"
		return resp, nil
	}

	resp.Status = "authorized"
	resp.ApprovalCode = "SANDBOX_OK"
	resp.NetworkID = "sandbox_" + randomHex(12)
	return resp, nil
}

// Capture implements CardNetworkAdapter
func (s *SandboxAdapter) Capture(ctx context.Context, req CaptureRequest) (*CaptureResponse, error) {
	s.injectLatency()

	return &CaptureResponse{
		CaptureID: uuid.NewString(),
		Status:    "succeeded",
		NetworkID: "sandbox_cap_" + randomHex(10),
		CreatedAt: time.Now().UTC().Unix(),
		SettledAt: time.Now().UTC().Unix(),
	}, nil
}

// Void implements CardNetworkAdapter
func (s *SandboxAdapter) Void(ctx context.Context, req VoidRequest) (*VoidResponse, error) {
	s.injectLatency()

	return &VoidResponse{
		VoidID:    uuid.NewString(),
		Status:    "succeeded",
		CreatedAt: time.Now().UTC().Unix(),
	}, nil
}

// Refund implements CardNetworkAdapter
func (s *SandboxAdapter) Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error) {
	s.injectLatency()

	return &RefundResponse{
		RefundID:  uuid.NewString(),
		Status:    "succeeded",
		NetworkID: "sandbox_ref_" + randomHex(10),
		CreatedAt: time.Now().UTC().Unix(),
	}, nil
}

// Tokenize implements CardNetworkAdapter
func (s *SandboxAdapter) Tokenize(ctx context.Context, req TokenizeRequest) (*TokenResponse, error) {
	s.injectLatency()

	cardNumber := strings.ReplaceAll(req.CardNumber, " ", "")
	last4 := cardNumber[len(cardNumber)-4:]

	var brand string
	switch cardNumber[0] {
	case '4':
		brand = "visa"
	case '5':
		brand = "mastercard"
	case '3':
		brand = "amex"
	case '6':
		brand = "discover"
	default:
		brand = "unknown"
	}

	return &TokenResponse{
		Token:           "tok_" + randomHex(24),
		CardFingerprint: "fp_" + randomHex(32),
		Last4:           last4,
		ExpMonth:        req.ExpMonth,
		ExpYear:         req.ExpYear,
		CardBrand:       brand,
		CreatedAt:       time.Now().UTC().Unix(),
	}, nil
}

func (s *SandboxAdapter) injectLatency() {
	if s.ArtificialLatency > 0 {
		time.Sleep(s.ArtificialLatency)
	}
}

func (s *SandboxAdapter) getErrorMessage(code string) string {
	messages := map[string]string{
		ErrCardDeclined:        "The card was declined",
		ErrExpiredCard:         "The card has expired",
		ErrInsufficientFunds:   "Insufficient funds available",
		ErrInvalidCardNumber:   "Invalid card number",
		ErrInvalidExpiry:       "Invalid expiration date",
		ErrInvalidCVV:          "Invalid CVV code",
	}

	if msg, ok := messages[code]; ok {
		return msg
	}
	return "Transaction failed"
}

// SetWebhookConfig configures webhook simulation for the sandbox
func (s *SandboxAdapter) SetWebhookConfig(url string, secret string) {
	s.WebhookURL = url
	s.WebhookSecret = secret
	s.WebhookEnabled = true
}

// SendTestWebhook sends a test webhook event to configured endpoint
func (s *SandboxAdapter) SendTestWebhook(ctx context.Context) error {
	if !s.WebhookEnabled {
		return fmt.Errorf("webhooks not enabled")
	}

	event := map[string]interface{}{
		"id":      "evt_sandbox_test_" + randomHex(12),
		"type":    "sandbox.test",
		"created": time.Now().UTC().Unix(),
		"data": map[string]interface{}{
			"message": "Sandbox test webhook event",
		},
		"livemode": false,
	}

	// Webhook would be delivered asynchronously in production
	// This simulates the event being generated
	_ = event
	return nil
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	if _, err := rand.Read(bytes); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}
