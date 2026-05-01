// tpt-payments/backend/refunds/api.go
package refunds

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
)

// Service handles refund business logic
type Service struct {
	db          *sql.DB
	ledger      LedgerWriter
	adapter     PaymentAdapter
	webhook     WebhookEmitter
}

// LedgerWriter interface for posting ledger entries
type LedgerWriter interface {
	PostRefundEntry(ctx context.Context, refund *Refund) error
	ReverseTransaction(ctx context.Context, paymentIntentID string, amount decimal.Decimal) error
}

// PaymentAdapter interface for processing refunds
type PaymentAdapter interface {
	Refund(ctx context.Context, captureID string, amount int64, idempotencyKey string) (string, error)
}

// WebhookEmitter interface for sending events
type WebhookEmitter interface {
	Emit(ctx context.Context, eventType string, data interface{}) error
}

// NewService creates a new refund service
func NewService(db *sql.DB, ledger LedgerWriter, adapter PaymentAdapter, webhook WebhookEmitter) *Service {
	return &Service{
		db:      db,
		ledger:  ledger,
		adapter: adapter,
		webhook: webhook,
	}
}

// CreateRefundRequest is the request body for creating a refund
type CreateRefundRequest struct {
	PaymentIntentID string            `json:"payment_intent_id"`
	Amount          decimal.Decimal   `json:"amount"`
	Reason          string            `json:"reason,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// Create creates a new refund
func (s *Service) Create(ctx context.Context, req CreateRefundRequest, idempotencyKey string) (*Refund, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Check idempotency
	var existingRefundID string
	err = tx.QueryRowContext(ctx, `
		SELECT refund_id FROM idempotency_keys WHERE key = $1`,
		idempotencyKey,
	).Scan(&existingRefundID)
	if err == nil {
		// Return existing refund
		return s.GetByID(ctx, existingRefundID)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Get payment intent
	var pi struct {
		ID             string
		Amount         decimal.Decimal
		Currency       string
		Status         string
		CapturedAmount decimal.Decimal
		RefundedAmount decimal.Decimal
		TimeWindowEnd  int64
		CaptureID      string
		MerchantID     string
	}

	err = tx.QueryRowContext(ctx, `
		SELECT id, amount, currency, status, captured_amount, refunded_amount, refund_window_end, capture_id, merchant_id
		FROM payment_intents WHERE id = $1`,
		req.PaymentIntentID,
	).Scan(
		&pi.ID, &pi.Amount, &pi.Currency, &pi.Status,
		&pi.CapturedAmount, &pi.RefundedAmount, &pi.TimeWindowEnd, &pi.CaptureID, &pi.MerchantID,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, errors.New("payment intent not found")
		}
		return nil, err
	}

	// Validate payment is captured
	if pi.Status != "succeeded" && pi.Status != "captured" {
		return nil, errors.New("cannot refund payment that has not been captured")
	}

	// Validate refund window
	if time.Now().Unix() > pi.TimeWindowEnd {
		return nil, errors.New("refund window has expired for this payment")
	}

	// Validate amount
	if req.Amount.LessThanOrEqual(decimal.Zero) {
		return nil, errors.New("refund amount must be greater than zero")
	}

	remaining := pi.CapturedAmount.Sub(pi.RefundedAmount)
	if req.Amount.GreaterThan(remaining) {
		return nil, fmt.Errorf("cannot refund more than remaining captured amount: %s", remaining)
	}

	// Create refund
	refund := NewRefund(req.PaymentIntentID, req.Amount, pi.Currency, req.Reason)
	refund.Metadata = req.Metadata

	// Store refund
	_, err = tx.ExecContext(ctx, `
		INSERT INTO refunds (
			id, payment_intent_id, amount, currency, status, reason, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		refund.ID, refund.PaymentIntentID, refund.Amount, refund.Currency,
		refund.Status, refund.Reason, refund.Metadata, refund.CreatedAt, refund.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	// Store idempotency key
	_, err = tx.ExecContext(ctx, `
		INSERT INTO idempotency_keys (key, refund_id, created_at) VALUES ($1, $2, $3)`,
		idempotencyKey, refund.ID, time.Now().Unix(),
	)
	if err != nil {
		return nil, err
	}

	// Update payment intent refunded amount
	_, err = tx.ExecContext(ctx, `
		UPDATE payment_intents SET refunded_amount = refunded_amount + $1, updated_at = $2 WHERE id = $3`,
		refund.Amount, time.Now().Unix(), req.PaymentIntentID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Process refund asynchronously
	go s.processRefund(context.Background(), refund, pi.CaptureID, idempotencyKey)

	// Emit webhook event
	s.webhook.Emit(ctx, "refund.created", refund)

	return refund, nil
}

// processRefund processes the refund with the payment adapter
func (s *Service) processRefund(ctx context.Context, refund *Refund, captureID string, idempotencyKey string) {
	// Call adapter to process refund
	networkID, err := s.adapter.Refund(ctx, captureID, refund.Amount.IntPart(), idempotencyKey)
	if err != nil {
		refund.MarkFailed(err.Error())
		s.updateRefundStatus(ctx, refund)
		s.webhook.Emit(ctx, "refund.failed", refund)
		return
	}

	// Post ledger entry
	if err := s.ledger.PostRefundEntry(ctx, refund); err != nil {
		refund.MarkFailed("ledger entry failed")
		s.updateRefundStatus(ctx, refund)
		return
	}

	refund.MarkSucceeded()
	s.updateRefundStatus(ctx, refund)

	s.webhook.Emit(ctx, "refund.succeeded", refund)
}

// updateRefundStatus updates refund status in database
func (s *Service) updateRefundStatus(ctx context.Context, refund *Refund) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE refunds SET status = $1, succeeded_at = $2, failure_reason = $3, updated_at = $4 WHERE id = $5`,
		refund.Status, refund.SucceededAt, refund.FailureReason, time.Now().Unix(), refund.ID,
	)
	return err
}

// GetByID retrieves a refund by ID
func (s *Service) GetByID(ctx context.Context, id string) (*Refund, error) {
	var refund Refund
	err := s.db.QueryRowContext(ctx, `
		SELECT id, payment_intent_id, amount, currency, status, reason, metadata, created_at, updated_at, succeeded_at, failure_reason
		FROM refunds WHERE id = $1`,
		id,
	).Scan(
		&refund.ID, &refund.PaymentIntentID, &refund.Amount, &refund.Currency,
		&refund.Status, &refund.Reason, &refund.Metadata, &refund.CreatedAt,
		&refund.UpdatedAt, &refund.SucceededAt, &refund.FailureReason,
	)
	if err == sql.ErrNoRows {
		return nil, errors.New("refund not found")
	}
	return &refund, err
}

// ListResponse is the paginated list response
type ListResponse struct {
	Data      []*Refund `json:"data"`
	HasMore   bool      `json:"has_more"`
	TotalCount int      `json:"total_count"`
}

// List refunds with filters
func (s *Service) List(ctx context.Context, merchantID string, limit int, startingAfter string, status string, paymentIntentID string) (*ListResponse, error) {
	args := []interface{}{merchantID}
	conditions := []string{"r.merchant_id = $1"}
	argIndex := 2

	if status != "" {
		conditions = append(conditions, fmt.Sprintf("r.status = $%d", argIndex))
		args = append(args, status)
		argIndex++
	}

	if paymentIntentID != "" {
		conditions = append(conditions, fmt.Sprintf("r.payment_intent_id = $%d", argIndex))
		args = append(args, paymentIntentID)
		argIndex++
	}

	if startingAfter != "" {
		conditions = append(conditions, fmt.Sprintf("r.created_at < (SELECT created_at FROM refunds WHERE id = $%d)", argIndex))
		args = append(args, startingAfter)
		argIndex++
	}

	whereClause := strings.Join(conditions, " AND ")

	query := fmt.Sprintf(`
		SELECT r.id, r.payment_intent_id, r.amount, r.currency, r.status, r.reason, r.metadata, r.created_at, r.updated_at
		FROM refunds r
		WHERE %s
		ORDER BY r.created_at DESC
		LIMIT $%d`,
		whereClause, argIndex,
	)
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refunds []*Refund
	for rows.Next() {
		var r Refund
		err := rows.Scan(
			&r.ID, &r.PaymentIntentID, &r.Amount, &r.Currency, &r.Status,
			&r.Reason, &r.Metadata, &r.CreatedAt, &r.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		refunds = append(refunds, &r)
	}

	hasMore := len(refunds) > limit
	if hasMore {
		refunds = refunds[:limit]
	}

	// Get total count
	var totalCount int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM refunds r WHERE %s`, whereClause)
	s.db.QueryRowContext(ctx, countQuery, args[:len(args)-1]...).Scan(&totalCount)

	return &ListResponse{
		Data:      refunds,
		HasMore:   hasMore,
		TotalCount: totalCount,
	}, nil
}

// HTTPHandler returns an http.Handler for refund endpoints
func HTTPHandler(service *Service) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		idempotencyKey := r.Header.Get("Idempotency-Key")
		if idempotencyKey == "" {
			http.Error(w, "Idempotency-Key header required", http.StatusBadRequest)
			return
		}

		var req CreateRefundRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		refund, err := service.Create(ctx, req, idempotencyKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(refund)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := chi.URLParam(r, "id")

		refund, err := service.GetByID(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(refund)
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		if limit <= 0 || limit > 100 {
			limit = 10
		}

		startingAfter := r.URL.Query().Get("starting_after")
		status := r.URL.Query().Get("status")
		paymentIntentID := r.URL.Query().Get("payment_intent")

		// Get merchant ID from auth context
		merchantID := ctx.Value("merchant_id").(string)

		response, err := service.List(ctx, merchantID, limit, startingAfter, status, paymentIntentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	return r
}