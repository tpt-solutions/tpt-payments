// tpt-payments/backend/adapters/interface.go
package adapters

import "context"

// CardNetworkAdapter defines the standard interface for all payment processing adapters
// This interface is implemented by SandboxAdapter, BankSponsorAdapter, and DirectAcquiringAdapter
type CardNetworkAdapter interface {
	// Authorize requests authorization for a payment amount
	// Returns an authorization hold that can be later captured, voided, or refunded
	Authorize(ctx context.Context, req AuthRequest) (*AuthResponse, error)

	// Capture confirms and settles a previously authorized transaction
	// May be partial amount for partial captures
	Capture(ctx context.Context, req CaptureRequest) (*CaptureResponse, error)

	// Void cancels an existing authorization before it has been captured
	Void(ctx context.Context, req VoidRequest) (*VoidResponse, error)

	// Refund returns funds for an already captured transaction
	// May be partial amount for partial refunds
	Refund(ctx context.Context, req RefundRequest) (*RefundResponse, error)

	// Tokenize converts raw card data into a secure reusable token
	Tokenize(ctx context.Context, req TokenizeRequest) (*TokenResponse, error)
}

// AuthRequest represents an authorization request to the card network
type AuthRequest struct {
	Amount              int64             `json:"amount"`
	Currency            string            `json:"currency"`
	CardToken           string            `json:"card_token"`
	CardFingerprint     string            `json:"card_fingerprint"`
	MerchantID          string            `json:"merchant_id"`
	PaymentIntentID     string            `json:"payment_intent_id"`
	IdempotencyKey      string            `json:"idempotency_key"`
	ClientIP            string            `json:"client_ip"`
	UserAgent           string            `json:"user_agent"`
	Metadata            map[string]string `json:"metadata"`
	ThreeDSecureEnabled bool              `json:"three_d_secure_enabled"`
}

// AuthResponse is returned from a successful authorization request
type AuthResponse struct {
	AuthorizationID string `json:"authorization_id"`
	Status          string `json:"status"` // pending, authorized, declined, expired
	ApprovalCode    string `json:"approval_code,omitempty"`
	NetworkID       string `json:"network_id,omitempty"`
	ErrorCode       string `json:"error_code,omitempty"`
	ErrorMessage    string `json:"error_message,omitempty"`
	RawResponse     []byte `json:"raw_response,omitempty"`
	CreatedAt       int64  `json:"created_at"`
	ExpiresAt       int64  `json:"expires_at"`
}

// CaptureRequest represents a request to capture an existing authorization
type CaptureRequest struct {
	AuthorizationID string            `json:"authorization_id"`
	Amount          int64             `json:"amount"`
	MerchantID      string            `json:"merchant_id"`
	IdempotencyKey  string            `json:"idempotency_key"`
	Metadata        map[string]string `json:"metadata"`
}

// CaptureResponse is returned from a successful capture request
type CaptureResponse struct {
	CaptureID   string `json:"capture_id"`
	Status      string `json:"status"` // pending, succeeded, failed
	NetworkID   string `json:"network_id,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt   int64  `json:"created_at"`
	SettledAt   int64  `json:"settled_at,omitempty"`
}

// VoidRequest represents a request to void an authorization
type VoidRequest struct {
	AuthorizationID string `json:"authorization_id"`
	MerchantID      string `json:"merchant_id"`
	Reason          string `json:"reason,omitempty"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// VoidResponse is returned from a successful void request
type VoidResponse struct {
	VoidID     string `json:"void_id"`
	Status     string `json:"status"`
	ErrorCode  string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

// RefundRequest represents a request to refund a captured transaction
type RefundRequest struct {
	CaptureID      string            `json:"capture_id"`
	Amount         int64             `json:"amount"`
	MerchantID     string            `json:"merchant_id"`
	Reason         string            `json:"reason,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
	Metadata       map[string]string `json:"metadata"`
}

// RefundResponse is returned from a successful refund request
type RefundResponse struct {
	RefundID    string `json:"refund_id"`
	Status      string `json:"status"` // pending, succeeded, failed
	NetworkID   string `json:"network_id,omitempty"`
	ErrorCode   string `json:"error_code,omitempty"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

// TokenizeRequest represents a request to tokenize card data
type TokenizeRequest struct {
	CardNumber string `json:"card_number"`
	ExpMonth   int    `json:"exp_month"`
	ExpYear    int    `json:"exp_year"`
	CardHolder string `json:"card_holder"`
	CVV        string `json:"cvv,omitempty"`
	MerchantID string `json:"merchant_id"`
}

// TokenResponse is returned from a successful tokenization request
type TokenResponse struct {
	Token           string `json:"token"`
	CardFingerprint string `json:"card_fingerprint"`
	Last4           string `json:"last4"`
	ExpMonth        int    `json:"exp_month"`
	ExpYear         int    `json:"exp_year"`
	CardBrand       string `json:"card_brand"`
	CreatedAt       int64  `json:"created_at"`
}

// Standard error codes used across all adapters
const (
	ErrCardDeclined          = "card_declined"
	ErrExpiredCard           = "expired_card"
	ErrInsufficientFunds     = "insufficient_funds"
	ErrInvalidCardNumber     = "invalid_card_number"
	ErrInvalidExpiry         = "invalid_expiry"
	ErrInvalidCVV            = "invalid_cvv"
	ErrDuplicateTransaction  = "duplicate_transaction"
	ErrAuthorizationExpired  = "authorization_expired"
	ErrCaptureAlreadyDone    = "capture_already_done"
	ErrRefundNotAllowed      = "refund_not_allowed"
	ErrNetworkError          = "network_error"
	ErrRateLimitExceeded     = "rate_limit_exceeded"
	ErrCardVelocityExceeded  = "card_velocity_exceeded"
	ErrFraudDetected         = "fraud_detected"
)