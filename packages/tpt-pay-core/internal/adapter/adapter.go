// Package adapter defines the CardNetworkAdapter interface that abstracts
// all card network interactions — whether via a bank sponsor API or a
// direct card network connection (ISO 8583).
//
// Every concrete adapter (SandboxAdapter, BankSponsorAdapter,
// DirectAcquiringAdapter) implements this interface. The payment service
// never imports a concrete adapter directly; it only knows this interface.
package adapter

import (
	"context"
	"time"
)

// AuthRequest is sent to the adapter to authorise a charge.
// Amount is always in the smallest currency unit (e.g. cents for USD).
type AuthRequest struct {
	Amount          int64
	Currency        string
	PaymentMethodID string // tokenized card reference
	MerchantID      string
	Descriptor      string // soft descriptor shown on customer's statement
	Metadata        map[string]string
}

// AuthResponse is returned by a successful authorization.
type AuthResponse struct {
	ChargeID     string // adapter-level charge identifier
	AuthCode     string // authorization code from card network
	NetworkTxnID string // card network transaction ID (for recurring use)
	AuthorizedAt time.Time
}

// CaptureRequest finalizes a previously authorized charge.
type CaptureRequest struct {
	ChargeID string
	Amount   int64 // may be <= the originally authorized amount
}

// CaptureResponse is returned by a successful capture.
type CaptureResponse struct {
	ChargeID   string
	CapturedAt time.Time
	SettleDate string // YYYYMMDD — expected settlement date
}

// VoidRequest cancels an authorization before capture.
type VoidRequest struct {
	ChargeID string
	Reason   string
}

// VoidResponse is returned by a successful void.
type VoidResponse struct {
	ChargeID string
	VoidedAt time.Time
}

// RefundRequest creates a refund against a captured charge.
type RefundRequest struct {
	ChargeID string
	Amount   int64  // partial refund if < original capture amount
	Reason   string // "duplicate" | "fraudulent" | "requested_by_customer"
}

// RefundResponse is returned by a successful refund.
type RefundResponse struct {
	RefundID   string
	ChargeID   string
	Amount     int64
	RefundedAt time.Time
}

// TokenizeRequest tokenizes card data. In production this is typically a
// one-time-use token from the card network's JS library (e.g. Stripe.js
// equivalent). The adapter converts it to a reusable payment method token.
type TokenizeRequest struct {
	// OneTimeToken is the short-lived token from the frontend card capture UI.
	// Raw PANs are never accepted here — card data must be captured client-side
	// via the card network's hosted field / JS library.
	OneTimeToken string
	MerchantID   string
}

// TokenResponse holds the reusable payment method token.
type TokenResponse struct {
	PaymentMethodID string // stable, reusable token stored server-side
	Last4           string
	Brand           string // "visa" | "mastercard" | "amex" | "discover" | "unknown"
	ExpMonth        int
	ExpYear         int
}

// CardError is returned by the adapter when the card network declines or
// otherwise rejects the request. It maps directly to a CardError in the API
// response.
type CardError struct {
	Code           string // e.g. "insufficient_funds", "expired_card"
	DeclineCode    string // network-level decline code
	Message        string
	NetworkMessage string // raw message from card network (never shown to end users)
}

func (e *CardError) Error() string { return e.Code + ": " + e.Message }

// CardNetworkAdapter is the interface that every card network integration must
// implement. Methods must be safe to call concurrently.
type CardNetworkAdapter interface {
	// Authorize sends an authorization request. Returns *CardError for card
	// declines; returns a generic error for network/system failures.
	Authorize(ctx context.Context, req AuthRequest) (*AuthResponse, error)

	// Capture finalizes a prior authorization. If the adapter uses
	// automatic capture, Authorize already captures and Capture is a no-op.
	Capture(ctx context.Context, req CaptureRequest) (*CaptureResponse, error)

	// Void cancels an authorization before capture.
	Void(ctx context.Context, req VoidRequest) (*VoidResponse, error)

	// Refund creates a refund against a captured charge.
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)

	// Tokenize converts a one-time frontend token into a reusable
	// payment method ID. Raw PANs must never be passed here.
	Tokenize(ctx context.Context, req TokenizeRequest) (*TokenResponse, error)
}
