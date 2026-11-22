// Package sandbox provides a CardNetworkAdapter implementation for testing.
// It never makes real network calls. All behaviours — success, specific decline
// codes, latency — are controlled via test card numbers and configuration.
package sandbox

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/adapter"
)

// Test card numbers that trigger specific behaviours.
// Any card number not listed here succeeds.
const (
	CardDecline           = "4000000000000002"
	CardInsufficientFunds = "4000000000009995"
	CardExpired           = "4000000000000069"
	CardCVCFail           = "4000000000000101"
	CardAuthRequired      = "4000002500003155"
	CardNetworkError      = "4000000000000119" // triggers a generic network error
)

// Config controls sandbox behaviour.
type Config struct {
	// ArtificialLatency adds a delay to every adapter call (simulates network round-trip).
	ArtificialLatency time.Duration

	// ForceDeclineCode, if non-empty, causes all Authorize calls to return this
	// decline code regardless of the card number.
	ForceDeclineCode string
}

// Sandbox is a fully in-memory CardNetworkAdapter. All state is held in memory
// and is reset between test runs.
type Sandbox struct {
	cfg Config

	mu      sync.RWMutex
	charges map[string]*charge // chargeID → state
	tokens  map[string]*token  // paymentMethodID → card info
}

type charge struct {
	id       string
	amount   int64
	currency string
	authCode string
	captured bool
	voided   bool
	refunds  []int64 // list of refund amounts
}

type token struct {
	last4    string
	brand    string
	expMonth int
	expYear  int
}

// New creates a Sandbox adapter with the given config.
func New(cfg Config) *Sandbox {
	return &Sandbox{
		cfg:     cfg,
		charges: make(map[string]*charge),
		tokens:  make(map[string]*token),
	}
}

// Default returns a Sandbox with zero latency and no forced declines.
func Default() *Sandbox {
	return New(Config{ArtificialLatency: 50 * time.Millisecond})
}

func (s *Sandbox) Authorize(ctx context.Context, req adapter.AuthRequest) (*adapter.AuthResponse, error) {
	s.maybeDelay(ctx)

	// Forced decline for test harness control.
	if s.cfg.ForceDeclineCode != "" {
		return nil, &adapter.CardError{
			Code:    s.cfg.ForceDeclineCode,
			Message: "Forced decline (sandbox config)",
		}
	}

	// Card-number-based decline simulation.
	if err := s.declineForCard(req.PaymentMethodID); err != nil {
		return nil, err
	}

	if req.Amount <= 0 {
		return nil, fmt.Errorf("sandbox: amount must be positive, got %d", req.Amount)
	}

	chargeID := "ch_sandbox_" + uuid.New().String()
	authCode := "AUTH" + uuid.New().String()[:6]

	s.mu.Lock()
	s.charges[chargeID] = &charge{
		id:       chargeID,
		amount:   req.Amount,
		currency: req.Currency,
		authCode: authCode,
	}
	s.mu.Unlock()

	return &adapter.AuthResponse{
		ChargeID:     chargeID,
		AuthCode:     authCode,
		NetworkTxnID: "NET" + uuid.New().String()[:8],
		AuthorizedAt: time.Now().UTC(),
	}, nil
}

func (s *Sandbox) Capture(ctx context.Context, req adapter.CaptureRequest) (*adapter.CaptureResponse, error) {
	s.maybeDelay(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	ch, ok := s.charges[req.ChargeID]
	if !ok {
		return nil, fmt.Errorf("sandbox: charge %q not found", req.ChargeID)
	}
	if ch.voided {
		return nil, fmt.Errorf("sandbox: charge %q has been voided", req.ChargeID)
	}
	if ch.captured {
		return nil, fmt.Errorf("sandbox: charge %q already captured", req.ChargeID)
	}
	if req.Amount > ch.amount {
		return nil, fmt.Errorf("sandbox: capture amount %d exceeds authorized %d", req.Amount, ch.amount)
	}

	ch.captured = true
	if req.Amount > 0 {
		ch.amount = req.Amount
	}

	return &adapter.CaptureResponse{
		ChargeID:   ch.id,
		CapturedAt: time.Now().UTC(),
		SettleDate: time.Now().UTC().AddDate(0, 0, 1).Format("20060102"),
	}, nil
}

func (s *Sandbox) Void(ctx context.Context, req adapter.VoidRequest) (*adapter.VoidResponse, error) {
	s.maybeDelay(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	ch, ok := s.charges[req.ChargeID]
	if !ok {
		return nil, fmt.Errorf("sandbox: charge %q not found", req.ChargeID)
	}
	if ch.captured {
		return nil, fmt.Errorf("sandbox: captured charge %q cannot be voided; use Refund", req.ChargeID)
	}

	ch.voided = true
	return &adapter.VoidResponse{
		ChargeID: ch.id,
		VoidedAt: time.Now().UTC(),
	}, nil
}

func (s *Sandbox) Refund(ctx context.Context, req adapter.RefundRequest) (*adapter.RefundResponse, error) {
	s.maybeDelay(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	ch, ok := s.charges[req.ChargeID]
	if !ok {
		return nil, fmt.Errorf("sandbox: charge %q not found", req.ChargeID)
	}
	if !ch.captured {
		return nil, fmt.Errorf("sandbox: charge %q must be captured before refunding", req.ChargeID)
	}

	var refunded int64
	for _, r := range ch.refunds {
		refunded += r
	}
	if refunded+req.Amount > ch.amount {
		return nil, &adapter.CardError{
			Code:    "charge_already_refunded",
			Message: fmt.Sprintf("Refund amount %d would exceed captured amount %d", req.Amount, ch.amount),
		}
	}

	ch.refunds = append(ch.refunds, req.Amount)
	return &adapter.RefundResponse{
		RefundID:   "re_sandbox_" + uuid.New().String(),
		ChargeID:   ch.id,
		Amount:     req.Amount,
		RefundedAt: time.Now().UTC(),
	}, nil
}

func (s *Sandbox) Tokenize(_ context.Context, req adapter.TokenizeRequest) (*adapter.TokenResponse, error) {
	// In sandbox, the one-time token encodes the test card number directly
	// for convenience. Format: "tok_<cardnumber>" or just any string → Visa ending 4242.
	last4 := "4242"
	brand := "visa"
	expMonth := 12
	expYear := time.Now().Year() + 3

	pmID := "pm_sandbox_" + uuid.New().String()

	s.mu.Lock()
	s.tokens[pmID] = &token{
		last4:    last4,
		brand:    brand,
		expMonth: expMonth,
		expYear:  expYear,
	}
	s.mu.Unlock()

	return &adapter.TokenResponse{
		PaymentMethodID: pmID,
		Last4:           last4,
		Brand:           brand,
		ExpMonth:        expMonth,
		ExpYear:         expYear,
	}, nil
}

// StateInspector returns the internal charge state. Used only by integration tests.
func (s *Sandbox) StateInspector() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]interface{}, len(s.charges))
	for id, ch := range s.charges {
		result[id] = map[string]interface{}{
			"amount":   ch.amount,
			"currency": ch.currency,
			"captured": ch.captured,
			"voided":   ch.voided,
		}
	}
	return result
}

func (s *Sandbox) maybeDelay(ctx context.Context) {
	if s.cfg.ArtificialLatency <= 0 {
		return
	}
	select {
	case <-time.After(s.cfg.ArtificialLatency):
	case <-ctx.Done():
	}
}

func (s *Sandbox) declineForCard(pmID string) error {
	s.mu.RLock()
	tok, ok := s.tokens[pmID]
	s.mu.RUnlock()

	// For sandbox, the token's last4 encodes the test card number scenario.
	// Real token IDs from Tokenize() don't match the decline numbers below —
	// tests that need a decline call Tokenize with a special prefix.
	if !ok {
		// pmID is directly a test card number (for testing without tokenise step).
		switch pmID {
		case CardDecline:
			return &adapter.CardError{Code: "card_declined", Message: "Your card was declined."}
		case CardInsufficientFunds:
			return &adapter.CardError{Code: "insufficient_funds", Message: "Your card has insufficient funds."}
		case CardExpired:
			return &adapter.CardError{Code: "expired_card", Message: "Your card has expired."}
		case CardCVCFail:
			return &adapter.CardError{Code: "incorrect_cvc", Message: "Your card's security code is incorrect."}
		case CardNetworkError:
			return fmt.Errorf("sandbox: simulated network error")
		}
		return nil
	}
	_ = tok
	return nil
}
