package payment

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// PlatformMerchantID owns the platform clearing account. It is seeded by migration
// 20260101000009_seed_platform_merchant.sql so the ledger foreign key is satisfied.
var PlatformMerchantID = uuid.Nil

// Service orchestrates the PaymentIntent lifecycle.
//
// Concurrency model: every state change is a conditional UPDATE
// (`WHERE status = <expected>`), so only one of several concurrent requests can
// move an intent. The card network is called between state changes, never inside
// a database transaction, so no row lock is held across network I/O. Ledger
// postings for a capture commit in the same transaction as the status change.
//
// Failure model:
//   - Card declines roll the intent back to a retryable state.
//   - Any other adapter failure leaves the intent in `processing` (the outcome is
//     unknown and the charge may exist). It is not rolled back, so it cannot be
//     charged twice; a reconciliation job resolves it using adapter_charge_id.
type Service struct {
	pool    *pgxpool.Pool
	adapter adapter.CardNetworkAdapter
	ledger  *ledger.Ledger
}

// NewService creates a Service with the given dependencies.
func NewService(pool *pgxpool.Pool, adpt adapter.CardNetworkAdapter, lgr *ledger.Ledger) *Service {
	return &Service{pool: pool, adapter: adpt, ledger: lgr}
}

// execer is satisfied by both *pgxpool.Pool and pgx.Tx.
type execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Create creates a new PaymentIntent in status requires_payment_method and emits
// payment_intent.created in the same transaction.
func (s *Service) Create(ctx context.Context, scope Scope, params CreateParams) (*Intent, error) {
	if params.Currency == "" {
		return nil, apierrors.MissingParam("currency")
	}
	code, err := NormalizeCurrency(params.Currency)
	if err != nil {
		return nil, err
	}
	if err := ValidateAmount(params.Amount, code); err != nil {
		return nil, err
	}
	if err := ValidateMetadata(params.Metadata); err != nil {
		return nil, err
	}
	if err := ValidCaptureMethod(string(params.CaptureMethod)); err != nil {
		return nil, err
	}
	if err := ValidConfirmationMethod(params.ConfirmationMethod); err != nil {
		return nil, err
	}
	params.Currency = code
	if params.CaptureMethod == "" {
		params.CaptureMethod = CaptureAutomatic
	}
	if params.ConfirmationMethod == "" {
		params.ConfirmationMethod = "automatic"
	}

	id := "pi_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	var intent Intent
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO payment_intents
				(id, merchant_id, amount, currency, status, capture_method, confirmation_method, metadata, livemode)
			VALUES
				($1, $2, $3, $4, $5, $6, $7, $8, $9)
			RETURNING id, merchant_id, amount, currency, status, capture_method, confirmation_method,
			          metadata, livemode, created_at, updated_at
		`,
			id, scope.MerchantID, params.Amount, strings.ToUpper(params.Currency),
			StatusRequiresPaymentMethod, params.CaptureMethod, params.ConfirmationMethod,
			metadataToJSON(params.Metadata), scope.Livemode,
		).Scan(
			&intent.ID, &intent.MerchantID, &intent.Amount, &intent.Currency,
			&intent.Status, &intent.CaptureMethod, &intent.ConfirmationMethod,
			&intent.Metadata, &intent.Livemode, &intent.CreatedAt, &intent.UpdatedAt,
		); err != nil {
			return fmt.Errorf("create payment intent: %w", err)
		}
		return enqueueEvent(ctx, tx, &intent, webhook.EventPaymentIntentCreated)
	})
	if err != nil {
		return nil, err
	}
	return &intent, nil
}

// Retrieve fetches a PaymentIntent by ID. Intents outside the caller's merchant or
// mode are reported as not found, so their existence is not revealed.
func (s *Service) Retrieve(ctx context.Context, id string, scope Scope) (*Intent, error) {
	intent, err := s.get(ctx, id)
	if err != nil {
		return nil, err
	}
	if intent.MerchantID != scope.MerchantID || intent.Livemode != scope.Livemode {
		return nil, apierrors.NotFound("payment_intent", id)
	}
	return intent, nil
}

// Confirm attaches a payment method (if not yet attached) and authorizes the payment.
// For automatic capture, the authorization is captured immediately and the ledger is posted.
func (s *Service) Confirm(ctx context.Context, id string, scope Scope, params ConfirmParams) (*Intent, error) {
	intent, err := s.Retrieve(ctx, id, scope)
	if err != nil {
		return nil, err
	}

	// Step 1: attach the payment method (requires_payment_method → requires_confirmation).
	switch intent.Status {
	case StatusRequiresPaymentMethod:
		if params.PaymentMethodID == "" {
			return nil, apierrors.MissingParam("payment_method")
		}
		ok, err := s.attachPaymentMethod(ctx, id, params.PaymentMethodID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, s.conflict(ctx, id, "confirm")
		}
		intent.Status = StatusRequiresConfirmation
		intent.PaymentMethodID = params.PaymentMethodID

	case StatusRequiresConfirmation:
		if params.PaymentMethodID != "" && params.PaymentMethodID != intent.PaymentMethodID {
			return nil, apierrors.InvalidParam("payment_method",
				"the payment method cannot be changed once attached")
		}

	default:
		return nil, apierrors.IntentInvalidState(string(intent.Status), "confirm")
	}

	// Step 2: claim the intent for processing. Only one concurrent confirm can win.
	ok, err := casStatus(ctx, s.pool, id, []Status{StatusRequiresConfirmation}, StatusProcessing)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, s.conflict(ctx, id, "confirm")
	}

	// Step 3: authorize with the card network (outside any transaction).
	auth, err := s.adapter.Authorize(ctx, adapter.AuthRequest{
		Amount:          intent.Amount,
		Currency:        intent.Currency,
		PaymentMethodID: intent.PaymentMethodID,
		MerchantID:      scope.MerchantID,
	})
	if err != nil {
		var cardErr *adapter.CardError
		if errors.As(err, &cardErr) {
			// Definitive decline: the customer may retry with another payment method.
			s.failPayment(ctx, id)
			return nil, &apierrors.APIError{
				StatusCode: 402,
				Type:       apierrors.ErrorTypeCardError,
				Code:       apierrors.ErrorCode(cardErr.Code),
				Message:    cardErr.Message,
			}
		}
		return nil, outcomeUnknown("authorize", err)
	}

	// Step 4: persist the charge reference before going further, so a later failure
	// can still be reconciled against the network.
	if err := s.setChargeID(ctx, id, auth.ChargeID); err != nil {
		return nil, outcomeUnknown("persist charge reference", err)
	}
	intent.AdapterChargeID = auth.ChargeID

	if intent.CaptureMethod == CaptureManual {
		ok, err := casStatus(ctx, s.pool, id, []Status{StatusProcessing}, StatusRequiresCapture)
		if err != nil {
			return nil, outcomeUnknown("mark requires_capture", err)
		}
		if !ok {
			return nil, s.conflict(ctx, id, "confirm")
		}
		return s.get(ctx, id)
	}

	// Step 5 (automatic capture): capture the full authorized amount, then finalize.
	if _, err := s.adapter.Capture(ctx, adapter.CaptureRequest{
		ChargeID: auth.ChargeID,
		Amount:   intent.Amount,
	}); err != nil {
		return nil, outcomeUnknown("capture", err)
	}
	if err := s.finalizeCapture(ctx, intent, intent.Amount); err != nil {
		return nil, outcomeUnknown("finalize", err)
	}
	return s.get(ctx, id)
}

// Capture captures a previously authorized PaymentIntent, in full or in part.
// Only valid when the status is requires_capture (manual capture).
func (s *Service) Capture(ctx context.Context, id string, scope Scope, params CaptureParams) (*Intent, error) {
	intent, err := s.Retrieve(ctx, id, scope)
	if err != nil {
		return nil, err
	}
	if intent.Status != StatusRequiresCapture {
		return nil, apierrors.IntentInvalidState(string(intent.Status), "capture")
	}
	if intent.AdapterChargeID == "" {
		return nil, apierrors.Internal("payment intent has no charge reference; it cannot be captured")
	}

	amount := params.AmountToCapture
	switch {
	case amount == 0:
		amount = intent.Amount
	case amount < 0:
		return nil, apierrors.InvalidParam("amount_to_capture", "amount_to_capture must be greater than zero")
	case amount > intent.Amount:
		return nil, apierrors.InvalidParam("amount_to_capture",
			"amount_to_capture cannot exceed the authorized amount")
	}

	// Claim the intent: only one concurrent capture can proceed.
	ok, err := casStatus(ctx, s.pool, id, []Status{StatusRequiresCapture}, StatusProcessing)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, s.conflict(ctx, id, "capture")
	}

	if _, err := s.adapter.Capture(ctx, adapter.CaptureRequest{
		ChargeID: intent.AdapterChargeID,
		Amount:   amount,
	}); err != nil {
		var cardErr *adapter.CardError
		if errors.As(err, &cardErr) {
			s.revert(ctx, id, StatusRequiresCapture)
			return nil, &apierrors.APIError{
				StatusCode: 402,
				Type:       apierrors.ErrorTypeCardError,
				Code:       apierrors.ErrorCode(cardErr.Code),
				Message:    cardErr.Message,
			}
		}
		return nil, outcomeUnknown("capture", err)
	}

	if err := s.finalizeCapture(ctx, intent, amount); err != nil {
		return nil, outcomeUnknown("finalize", err)
	}
	return s.get(ctx, id)
}

// Cancel cancels a PaymentIntent that has not yet been captured. The state change and the
// payment_intent.canceled event commit together.
func (s *Service) Cancel(ctx context.Context, id string, scope Scope, params CancelParams) (*Intent, error) {
	intent, err := s.Retrieve(ctx, id, scope)
	if err != nil {
		return nil, err
	}
	if !CanTransition(intent.Status, StatusCanceled) {
		return nil, apierrors.IntentInvalidState(string(intent.Status), "cancel")
	}

	// Mark canceled first, guarded on the status we read. Once canceled, the intent
	// can never be captured, so a failed void below only leaves an authorization
	// hold that expires on its own; it cannot cause a charge.
	now := time.Now().UTC()
	var canceled *Intent
	err = pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE payment_intents
			SET status = $1, canceled_at = $2, cancellation_reason = $3
			WHERE id = $4 AND status = $5
		`, StatusCanceled, now, params.Reason, id, intent.Status)
		if err != nil {
			return fmt.Errorf("cancel payment intent: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return errConflict
		}
		canceled, err = getWith(ctx, tx, id)
		if err != nil {
			return err
		}
		return enqueueEvent(ctx, tx, canceled, webhook.EventPaymentIntentCanceled)
	})
	if errors.Is(err, errConflict) {
		return nil, s.conflict(ctx, id, "cancel")
	}
	if err != nil {
		return nil, err
	}

	if intent.Status == StatusRequiresCapture && intent.AdapterChargeID != "" {
		if _, err := s.adapter.Void(ctx, adapter.VoidRequest{
			ChargeID: intent.AdapterChargeID,
			Reason:   params.Reason,
		}); err != nil {
			slog.Error("void after cancel failed; authorization will expire at the network",
				"payment_intent", id, "charge_id", intent.AdapterChargeID, "err", err)
		}
	}
	return canceled, nil
}

// ── Private helpers ───────────────────────────────────────────────────────────

// finalizeCapture marks the intent succeeded and posts the captured amount to the
// ledger, all in one database transaction. If the status is no longer processing
// (another request already finalized it), nothing is written.
func (s *Service) finalizeCapture(ctx context.Context, intent *Intent, captured int64) error {
	merchantUUID, err := uuid.Parse(intent.MerchantID)
	if err != nil {
		return fmt.Errorf("invalid merchant id: %w", err)
	}

	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE payment_intents
			SET status = $1, amount_captured = $2
			WHERE id = $3 AND status = $4
		`, StatusSucceeded, captured, intent.ID, StatusProcessing)
		if err != nil {
			return fmt.Errorf("mark succeeded: %w", err)
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("payment intent %s was not in processing state", intent.ID)
		}

		merchantAcct, err := s.ledger.EnsureAccountTx(ctx, tx, merchantUUID, intent.Currency)
		if err != nil {
			return err
		}
		clearingAcct, err := s.ledger.EnsureAccountTx(ctx, tx, PlatformMerchantID, intent.Currency)
		if err != nil {
			return err
		}

		entries := []ledger.Entry{
			{AccountID: merchantAcct, Amount: captured, Currency: intent.Currency,
				Description: "payment captured: " + intent.ID},
			{AccountID: clearingAcct, Amount: -captured, Currency: intent.Currency,
				Description: "clearing debit: " + intent.ID},
		}
		txn, _, err := s.ledger.PostTransactionTx(ctx, tx, ledger.TxnTypePaymentIntent, intent.ID, entries)
		if err != nil {
			return fmt.Errorf("post ledger transaction: %w", err)
		}

		if _, err := tx.Exec(ctx,
			`UPDATE payment_intents SET ledger_txn_id = $1 WHERE id = $2`,
			txn.ID, intent.ID,
		); err != nil {
			return fmt.Errorf("link ledger transaction: %w", err)
		}

		final, err := getWith(ctx, tx, intent.ID)
		if err != nil {
			return err
		}
		return enqueueEvent(ctx, tx, final, webhook.EventPaymentIntentSucceeded)
	})
}

// attachPaymentMethod moves requires_payment_method → requires_confirmation and
// stores the payment method, guarded on the current status.
func (s *Service) attachPaymentMethod(ctx context.Context, id, paymentMethodID string) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		UPDATE payment_intents
		SET status = $1, payment_method_id = $2
		WHERE id = $3 AND status = $4
	`, StatusRequiresConfirmation, paymentMethodID, id, StatusRequiresPaymentMethod)
	if err != nil {
		return false, fmt.Errorf("attach payment method: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// setChargeID stores the adapter charge reference while the intent is processing.
func (s *Service) setChargeID(ctx context.Context, id, chargeID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE payment_intents SET adapter_charge_id = $1
		WHERE id = $2 AND status = $3
	`, chargeID, id, StatusProcessing)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("payment intent %s is no longer processing", id)
	}
	return nil
}

// revert moves a processing intent back to a retryable state after a definitive failure.
func (s *Service) revert(ctx context.Context, id string, to Status) {
	// Detach from request cancellation: the request context may already be done, and
	// the status must not be left stuck in processing.
	if _, err := casStatus(context.WithoutCancel(ctx), s.pool, id, []Status{StatusProcessing}, to); err != nil {
		slog.Error("failed to revert payment intent", "payment_intent", id, "to", to, "err", err)
	}
}

// conflict reports an invalid-state error using the intent's current status.
func (s *Service) conflict(ctx context.Context, id, action string) error {
	cur, err := s.get(ctx, id)
	if err != nil {
		return err
	}
	return apierrors.IntentInvalidState(string(cur.Status), action)
}

// casStatus atomically moves an intent to `to` only if it is currently in one of `from`.
// It reports false (not an error) when another request has already changed the status.
func casStatus(ctx context.Context, db execer, id string, from []Status, to Status) (bool, error) {
	states := make([]string, len(from))
	for i, s := range from {
		states[i] = string(s)
	}
	tag, err := db.Exec(ctx, `
		UPDATE payment_intents SET status = $1
		WHERE id = $2 AND status = ANY($3)
	`, to, id, states)
	if err != nil {
		return false, fmt.Errorf("update status: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// outcomeUnknown is returned when the card network did not give a definitive answer.
// The intent is left in processing for reconciliation; the caller must not retry blindly.
func outcomeUnknown(stage string, cause error) error {
	slog.Error("card network outcome unknown", "stage", stage, "err", cause)
	return &apierrors.APIError{
		StatusCode: 502,
		Type:       apierrors.ErrorTypeAPIError,
		Code:       apierrors.ErrorCode("processing_outcome_unknown"),
		Message: fmt.Sprintf("The %s step did not return a definitive result. "+
			"The payment is left in processing and must be reconciled before retrying.", stage),
	}
}

func (s *Service) get(ctx context.Context, id string) (*Intent, error) {
	return getWith(ctx, s.pool, id)
}

// queryer is satisfied by *pgxpool.Pool and pgx.Tx.
type queryer interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func getWith(ctx context.Context, q queryer, id string) (*Intent, error) {
	var intent Intent
	err := q.QueryRow(ctx, `
		SELECT id, merchant_id, amount, currency, status, capture_method, confirmation_method,
		       COALESCE(payment_method_id, ''), COALESCE(adapter_charge_id, ''),
		       COALESCE(amount_captured, 0), canceled_at,
		       COALESCE(cancellation_reason, ''), COALESCE(ledger_txn_id::text, ''),
		       livemode, metadata, created_at, updated_at
		FROM payment_intents
		WHERE id = $1
	`, id).Scan(
		&intent.ID, &intent.MerchantID, &intent.Amount, &intent.Currency,
		&intent.Status, &intent.CaptureMethod, &intent.ConfirmationMethod,
		&intent.PaymentMethodID, &intent.AdapterChargeID,
		&intent.AmountCaptured, &intent.CanceledAt,
		&intent.CancellationReason, &intent.LedgerTxnID,
		&intent.Livemode, &intent.Metadata, &intent.CreatedAt, &intent.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierrors.NotFound("payment_intent", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get payment intent: %w", err)
	}
	return &intent, nil
}

// enqueueEvent records a webhook event for the intent inside the caller's transaction.
func enqueueEvent(ctx context.Context, tx pgx.Tx, intent *Intent, ev webhook.EventType) error {
	_, err := webhook.Enqueue(ctx, tx, intent.MerchantID, intent.Livemode, ev, Wire(intent))
	return err
}

// failPayment returns a processing intent to requires_payment_method after a card decline
// and emits payment_failed, in one transaction. It detaches from request cancellation so a
// client disconnect cannot leave the intent stuck in processing.
func (s *Service) failPayment(ctx context.Context, id string) {
	bg := context.WithoutCancel(ctx)
	err := pgx.BeginFunc(bg, s.pool, func(tx pgx.Tx) error {
		ok, err := casStatus(bg, tx, id, []Status{StatusProcessing}, StatusRequiresPaymentMethod)
		if err != nil {
			return err
		}
		if !ok {
			return errConflict
		}
		failed, err := getWith(bg, tx, id)
		if err != nil {
			return err
		}
		return enqueueEvent(bg, tx, failed, webhook.EventPaymentIntentFailed)
	})
	if err != nil {
		slog.Error("failed to record card decline", "payment_intent", id, "err", err)
	}
}

// errConflict signals that a compare-and-set found the intent in an unexpected state.
var errConflict = errors.New("payment intent state changed concurrently")

func metadataToJSON(m map[string]string) interface{} {
	if len(m) == 0 {
		return nil
	}
	return m
}
