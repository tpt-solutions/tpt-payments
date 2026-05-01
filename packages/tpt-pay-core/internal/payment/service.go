package payment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/adapter"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/ledger"
)

// Service orchestrates the PaymentIntent lifecycle.
type Service struct {
	pool    *pgxpool.Pool
	adapter adapter.CardNetworkAdapter
	ledger  *ledger.Ledger
}

// NewService creates a Service with the given dependencies.
func NewService(pool *pgxpool.Pool, adpt adapter.CardNetworkAdapter, lgr *ledger.Ledger) *Service {
	return &Service{pool: pool, adapter: adpt, ledger: lgr}
}

// Create creates a new PaymentIntent in status requires_payment_method.
func (s *Service) Create(ctx context.Context, merchantID string, params CreateParams) (*Intent, error) {
	if params.Amount <= 0 {
		return nil, apierrors.InvalidParam("amount", "amount must be greater than zero")
	}
	if params.Currency == "" {
		return nil, apierrors.MissingParam("currency")
	}
	if params.CaptureMethod == "" {
		params.CaptureMethod = CaptureAutomatic
	}
	if params.ConfirmationMethod == "" {
		params.ConfirmationMethod = "automatic"
	}

	id := "pi_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	var intent Intent
	err := s.pool.QueryRow(ctx, `
		INSERT INTO payment_intents
			(id, merchant_id, amount, currency, status, capture_method, confirmation_method, metadata)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, merchant_id, amount, currency, status, capture_method, confirmation_method,
		          metadata, created_at, updated_at
	`, id, merchantID, params.Amount, strings.ToUpper(params.Currency),
		StatusRequiresPaymentMethod, params.CaptureMethod, params.ConfirmationMethod,
		metadataToJSON(params.Metadata),
	).Scan(
		&intent.ID, &intent.MerchantID, &intent.Amount, &intent.Currency,
		&intent.Status, &intent.CaptureMethod, &intent.ConfirmationMethod,
		&intent.Metadata, &intent.CreatedAt, &intent.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create payment intent: %w", err)
	}
	return &intent, nil
}

// Retrieve fetches a PaymentIntent by ID, scoped to the merchant.
func (s *Service) Retrieve(ctx context.Context, id, merchantID string) (*Intent, error) {
	intent, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if intent.MerchantID != merchantID {
		return nil, apierrors.NotFound("payment_intent", id)
	}
	return intent, nil
}

// Confirm attaches a payment method and triggers the authorization.
// For automatic capture, this also captures the charge.
func (s *Service) Confirm(ctx context.Context, id, merchantID string, params ConfirmParams) (*Intent, error) {
	intent, err := s.Retrieve(ctx, id, merchantID)
	if err != nil {
		return nil, err
	}

	if apiErr := AssertTransition(intent.Status, StatusProcessing, "confirm"); apiErr != nil {
		return nil, apiErr
	}

	if params.PaymentMethodID == "" && intent.PaymentMethodID == "" {
		return nil, apierrors.MissingParam("payment_method")
	}
	if params.PaymentMethodID != "" {
		intent.PaymentMethodID = params.PaymentMethodID
	}

	// Move to processing.
	if err := s.updateStatus(ctx, intent.ID, StatusProcessing); err != nil {
		return nil, err
	}
	intent.Status = StatusProcessing

	// Authorize via adapter.
	authResp, err := s.adapter.Authorize(ctx, adapter.AuthRequest{
		Amount:          intent.Amount,
		Currency:        intent.Currency,
		PaymentMethodID: intent.PaymentMethodID,
		MerchantID:      merchantID,
	})
	if err != nil {
		var cardErr *adapter.CardError
		if errors.As(err, &cardErr) {
			// Move back to requires_payment_method on card decline.
			_ = s.updateStatus(ctx, intent.ID, StatusRequiresPaymentMethod)
			return nil, &apierrors.APIError{
				StatusCode: 402,
				Type:       apierrors.ErrorTypeCardError,
				Code:       apierrors.ErrorCode(cardErr.Code),
				Message:    cardErr.Message,
			}
		}
		_ = s.updateStatus(ctx, intent.ID, StatusRequiresPaymentMethod)
		return nil, fmt.Errorf("adapter authorize: %w", err)
	}

	intent.AdapterChargeID = authResp.ChargeID

	if intent.CaptureMethod == CaptureAutomatic {
		// Capture immediately.
		_, err = s.adapter.Capture(ctx, adapter.CaptureRequest{
			ChargeID: authResp.ChargeID,
			Amount:   intent.Amount,
		})
		if err != nil {
			_ = s.updateStatus(ctx, intent.ID, StatusRequiresPaymentMethod)
			return nil, fmt.Errorf("adapter capture: %w", err)
		}

		// Post to ledger.
		if lerr := s.postCaptureLedger(ctx, intent); lerr != nil {
			return nil, lerr
		}

		if err := s.updateStatus(ctx, intent.ID, StatusSucceeded); err != nil {
			return nil, err
		}
		intent.Status = StatusSucceeded
	} else {
		// Manual capture — move to requires_capture.
		if err := s.updateStatus(ctx, intent.ID, StatusRequiresCapture); err != nil {
			return nil, err
		}
		intent.Status = StatusRequiresCapture
	}

	// Persist adapter charge ID.
	_, err = s.pool.Exec(ctx,
		`UPDATE payment_intents SET adapter_charge_id = $1 WHERE id = $2`,
		authResp.ChargeID, intent.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("persist charge id: %w", err)
	}

	return s.get(ctx, id)
}

// Capture manually captures a previously authorized PaymentIntent.
// Only valid when capture_method is "manual" and status is requires_capture.
func (s *Service) Capture(ctx context.Context, id, merchantID string, params CaptureParams) (*Intent, error) {
	intent, err := s.Retrieve(ctx, id, merchantID)
	if err != nil {
		return nil, err
	}

	if apiErr := AssertTransition(intent.Status, StatusSucceeded, "capture"); apiErr != nil {
		return nil, apiErr
	}
	if intent.Status != StatusRequiresCapture {
		return nil, apierrors.IntentInvalidState(string(intent.Status), "capture")
	}

	captureAmount := params.AmountToCapture
	if captureAmount == 0 {
		captureAmount = intent.Amount
	}

	_, err = s.adapter.Capture(ctx, adapter.CaptureRequest{
		ChargeID: intent.AdapterChargeID,
		Amount:   captureAmount,
	})
	if err != nil {
		return nil, fmt.Errorf("adapter capture: %w", err)
	}

	if lerr := s.postCaptureLedger(ctx, intent); lerr != nil {
		return nil, lerr
	}

	if err := s.updateStatus(ctx, id, StatusSucceeded); err != nil {
		return nil, err
	}
	return s.get(ctx, id)
}

// Cancel cancels a PaymentIntent that has not yet been captured.
func (s *Service) Cancel(ctx context.Context, id, merchantID string, params CancelParams) (*Intent, error) {
	intent, err := s.Retrieve(ctx, id, merchantID)
	if err != nil {
		return nil, err
	}

	if apiErr := AssertTransition(intent.Status, StatusCanceled, "cancel"); apiErr != nil {
		return nil, apiErr
	}

	if intent.AdapterChargeID != "" {
		_, err = s.adapter.Void(ctx, adapter.VoidRequest{
			ChargeID: intent.AdapterChargeID,
			Reason:   params.Reason,
		})
		if err != nil {
			return nil, fmt.Errorf("adapter void: %w", err)
		}
	}

	now := time.Now().UTC()
	_, err = s.pool.Exec(ctx, `
		UPDATE payment_intents
		SET status = $1, canceled_at = $2, cancellation_reason = $3
		WHERE id = $4
	`, StatusCanceled, now, params.Reason, id)
	if err != nil {
		return nil, fmt.Errorf("cancel payment intent: %w", err)
	}

	return s.get(ctx, id)
}

// ── Private helpers ───────────────────────────────────────────────────────────

func (s *Service) get(ctx context.Context, id string) (*Intent, error) {
	var intent Intent
	err := s.pool.QueryRow(ctx, `
		SELECT id, merchant_id, amount, currency, status, capture_method, confirmation_method,
		       COALESCE(payment_method_id, ''), COALESCE(adapter_charge_id, ''),
		       COALESCE(cancellation_reason, ''), metadata, created_at, updated_at
		FROM payment_intents
		WHERE id = $1
	`, id).Scan(
		&intent.ID, &intent.MerchantID, &intent.Amount, &intent.Currency,
		&intent.Status, &intent.CaptureMethod, &intent.ConfirmationMethod,
		&intent.PaymentMethodID, &intent.AdapterChargeID,
		&intent.CancellationReason, &intent.Metadata,
		&intent.CreatedAt, &intent.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierrors.NotFound("payment_intent", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get payment intent: %w", err)
	}
	return &intent, nil
}

func (s *Service) updateStatus(ctx context.Context, id string, status Status) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE payment_intents SET status = $1 WHERE id = $2`,
		status, id,
	)
	return err
}

func (s *Service) postCaptureLedger(ctx context.Context, intent *Intent) error {
	merchantUUID, err := uuid.Parse(intent.MerchantID)
	if err != nil {
		return fmt.Errorf("invalid merchant id: %w", err)
	}

	merchantAcctID, err := s.ledger.EnsureAccount(ctx, merchantUUID, intent.Currency)
	if err != nil {
		return err
	}

	// The clearing account is a platform-level account for the pending settlement float.
	// In a real deployment this would be a separate account per currency.
	clearingAcctID, err := s.ledger.EnsureAccount(ctx, uuid.MustParse("00000000-0000-0000-0000-000000000000"), intent.Currency)
	if err != nil {
		return err
	}

	entries := []ledger.Entry{
		{
			AccountID:   merchantAcctID,
			Amount:      intent.Amount,
			Currency:    intent.Currency,
			Description: "payment captured: " + intent.ID,
		},
		{
			AccountID:   clearingAcctID,
			Amount:      -intent.Amount,
			Currency:    intent.Currency,
			Description: "clearing debit: " + intent.ID,
		},
	}

	txn, err := s.ledger.PostTransaction(ctx, ledger.TxnTypePaymentIntent, intent.ID, entries)
	if err != nil {
		return fmt.Errorf("post ledger transaction: %w", err)
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE payment_intents SET ledger_txn_id = $1 WHERE id = $2`,
		txn.ID, intent.ID,
	)
	return err
}

func metadataToJSON(m map[string]string) interface{} {
	if len(m) == 0 {
		return nil
	}
	return m
}
