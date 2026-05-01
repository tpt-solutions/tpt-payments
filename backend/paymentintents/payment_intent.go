// tpt-payments/backend/paymentintents/payment_intent.go
package paymentintents

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PaymentIntent states
const (
	StateRequiresPaymentMethod = "requires_payment_method"
	StateRequiresConfirmation  = "requires_confirmation"
	StateRequiresAction        = "requires_action"
	StateProcessing            = "processing"
	StateSucceeded             = "succeeded"
	StateCanceled              = "canceled"
)

// PaymentIntent represents a payment attempt lifecycle
type PaymentIntent struct {
	ID                    string            `db:"id" json:"id"`
	MerchantID            string            `db:"merchant_id" json:"merchant_id"`
	Amount                decimal.Decimal   `db:"amount" json:"amount"`
	Currency              string            `db:"currency" json:"currency"`
	Status                string            `db:"status" json:"status"`
	Description           string            `db:"description" json:"description,omitempty"`
	StatementDescriptor   string            `db:"statement_descriptor" json:"statement_descriptor,omitempty"`
	CaptureMethod         string            `db:"capture_method" json:"capture_method"` // automatic, manual
	ConfirmationMethod    string            `db:"confirmation_method" json:"confirmation_method"` // automatic, manual
	ClientSecret          string            `db:"client_secret" json:"client_secret"`
	PaymentMethodID       string            `db:"payment_method_id" json:"payment_method_id,omitempty"`
	AuthorizationID       string            `db:"authorization_id" json:"authorization_id,omitempty"`
	CaptureID             string            `db:"capture_id" json:"capture_id,omitempty"`
	AmountCapturable      decimal.Decimal   `db:"amount_capturable" json:"amount_capturable"`
	AmountReceived        decimal.Decimal   `db:"amount_received" json:"amount_received"`
	LastPaymentError      map[string]string `db:"last_payment_error" json:"last_payment_error,omitempty"`
	IdempotencyKey        string            `db:"idempotency_key" json:"idempotency_key"`
	Metadata              map[string]string `db:"metadata" json:"metadata"`
	CreatedAt             time.Time         `db:"created_at" json:"created_at"`
	ExpiresAt             time.Time         `db:"expires_at" json:"expires_at"`
	ConfirmedAt           *time.Time        `db:"confirmed_at" json:"confirmed_at,omitempty"`
	CanceledAt            *time.Time        `db:"canceled_at" json:"canceled_at,omitempty"`
	CancellationReason    string            `db:"cancellation_reason" json:"cancellation_reason,omitempty"`
}

// Service handles payment intent business logic
type Service struct {
	db *sql.DB
}

// NewService creates a new PaymentIntent service
func NewService(db *sql.DB) *Service {
	return &Service{
		db: db,
	}
}

// Create creates a new PaymentIntent
func (s *Service) Create(ctx context.Context, intent *PaymentIntent) error {
	intent.ID = uuid.NewString()
	intent.CreatedAt = time.Now().UTC()
	intent.ExpiresAt = intent.CreatedAt.Add(24 * 7 * time.Hour)
	intent.Status = StateRequiresPaymentMethod
	intent.ClientSecret = "pi_" + uuid.NewString() + "_secret_" + randomHex(16)
	intent.AmountCapturable = intent.Amount
	intent.AmountReceived = decimal.Zero

	if intent.CaptureMethod == "" {
		intent.CaptureMethod = "automatic"
	}

	if intent.ConfirmationMethod == "" {
		intent.ConfirmationMethod = "automatic"
	}

	query := `
		INSERT INTO payment_intents (
			id, merchant_id, amount, currency, status, description, statement_descriptor,
			capture_method, confirmation_method, client_secret, amount_capturable, 
			amount_received, idempotency_key, created_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
	`

	_, err := s.db.ExecContext(ctx, query,
		intent.ID, intent.MerchantID, intent.Amount, intent.Currency, intent.Status,
		intent.Description, intent.StatementDescriptor, intent.CaptureMethod,
		intent.ConfirmationMethod, intent.ClientSecret, intent.AmountCapturable,
		intent.AmountReceived, intent.IdempotencyKey, intent.CreatedAt, intent.ExpiresAt)

	return err
}

// Get retrieves a PaymentIntent by ID
func (s *Service) Get(ctx context.Context, id string) (*PaymentIntent, error) {
	intent := &PaymentIntent{}
	query := `
		SELECT id, merchant_id, amount, currency, status, description, statement_descriptor,
			capture_method, confirmation_method, client_secret, payment_method_id,
			authorization_id, capture_id, amount_capturable, amount_received,
			idempotency_key, created_at, expires_at, confirmed_at, canceled_at, cancellation_reason
		FROM payment_intents
		WHERE id = $1
	`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&intent.ID, &intent.MerchantID, &intent.Amount, &intent.Currency, &intent.Status,
		&intent.Description, &intent.StatementDescriptor, &intent.CaptureMethod,
		&intent.ConfirmationMethod, &intent.ClientSecret, &intent.PaymentMethodID,
		&intent.AuthorizationID, &intent.CaptureID, &intent.AmountCapturable,
		&intent.AmountReceived, &intent.IdempotencyKey, &intent.CreatedAt,
		&intent.ExpiresAt, &intent.ConfirmedAt, &intent.CanceledAt, &intent.CancellationReason)

	if err != nil {
		return nil, err
	}

	return intent, nil
}

// Confirm attempts to confirm and process a PaymentIntent
func (s *Service) Confirm(ctx context.Context, id string) (*PaymentIntent, error) {
	intent, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if intent.Status != StateRequiresPaymentMethod && intent.Status != StateRequiresConfirmation {
		return nil, nil
	}

	now := time.Now().UTC()
	intent.Status = StateProcessing
	intent.ConfirmedAt = &now

	updateQuery := `
		UPDATE payment_intents
		SET status = $1, confirmed_at = $2
		WHERE id = $3
	`

	_, err = s.db.ExecContext(ctx, updateQuery, intent.Status, intent.ConfirmedAt, id)
	if err != nil {
		return nil, err
	}

	return intent, nil
}

// Cancel cancels an existing PaymentIntent
func (s *Service) Cancel(ctx context.Context, id string, reason string) (*PaymentIntent, error) {
	intent, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if intent.Status == StateSucceeded || intent.Status == StateCanceled {
		return intent, nil
	}

	now := time.Now().UTC()
	intent.Status = StateCanceled
	intent.CanceledAt = &now
	intent.CancellationReason = reason

	updateQuery := `
		UPDATE payment_intents
		SET status = $1, canceled_at = $2, cancellation_reason = $3
		WHERE id = $4
	`

	_, err = s.db.ExecContext(ctx, updateQuery, intent.Status, intent.CanceledAt, intent.CancellationReason, id)
	if err != nil {
		return nil, err
	}

	return intent, nil
}

// Capture captures funds for an authorized PaymentIntent
func (s *Service) Capture(ctx context.Context, id string, amountToCapture decimal.Decimal) (*PaymentIntent, error) {
	intent, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	if intent.Status != StateProcessing {
		return nil, nil
	}

	if amountToCapture.GreaterThan(intent.AmountCapturable) {
		return nil, nil
	}

	intent.AmountReceived = amountToCapture
	intent.AmountCapturable = intent.AmountCapturable.Sub(amountToCapture)
	intent.Status = StateSucceeded

	updateQuery := `
		UPDATE payment_intents
		SET status = $1, amount_received = $2, amount_capturable = $3
		WHERE id = $4
	`

	_, err = s.db.ExecContext(ctx, updateQuery, intent.Status, intent.AmountReceived, intent.AmountCapturable, id)
	if err != nil {
		return nil, err
	}

	return intent, nil
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
