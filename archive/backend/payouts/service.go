// tpt-payments/backend/payouts/service.go
package payouts

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
)


// PayoutProcessor interface for external payment rails
type PayoutProcessor interface {
	InitiatePayout(ctx context.Context, payout *Payout) (string, error)
	CancelPayout(ctx context.Context, payoutID string) error
	GetPayoutStatus(ctx context.Context, payoutID string) (PayoutStatus, error)
}

// Service handles payout business logic
type Service struct {
	db         *sql.DB
	processor  PayoutProcessor
	dispatcher *PayoutEventDispatcher
	handler    *FailureHandler
}

// NewService creates payout service
func NewService(db *sql.DB, processor PayoutProcessor, dispatcher *PayoutEventDispatcher) *Service {
	return &Service{
		db:         db,
		processor:  processor,
		dispatcher: dispatcher,
		handler:    NewFailureHandler(dispatcher, 3),
	}
}

// Create creates a new payout
func (s *Service) Create(ctx context.Context, merchantID string, amount decimal.Decimal, currency string, method PayoutMethod, destinationID string) (*Payout, error) {
	payout := NewPayout(merchantID, amount, currency, method, destinationID)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	metadataJSON, _ := json.Marshal(payout.Metadata)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO payouts (
			id, merchant_id, amount, currency, status, method, destination_id, arrival_date,
			metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		payout.ID, payout.MerchantID, payout.Amount, payout.Currency, payout.Status,
		payout.Method, payout.DestinationID, payout.ArrivalDate, metadataJSON,
		payout.CreatedAt, payout.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	if s.dispatcher != nil {
		_ = s.dispatcher.DispatchEvent(ctx, PayoutEventCreated, payout)
	}

	return payout, nil
}

// Get retrieves a payout by ID
func (s *Service) Get(ctx context.Context, payoutID string) (*Payout, error) {
	var payout Payout
	var metadataJSON []byte

	err := s.db.QueryRowContext(ctx, `
		SELECT id, merchant_id, amount, currency, status, method, destination_id, arrival_date,
		       paid_at, failure_code, failure_message, metadata, created_at, updated_at
		FROM payouts WHERE id = $1`,
		payoutID,
	).Scan(
		&payout.ID, &payout.MerchantID, &payout.Amount, &payout.Currency, &payout.Status,
		&payout.Method, &payout.DestinationID, &payout.ArrivalDate, &payout.PaidAt,
		&payout.FailureCode, &payout.FailureMessage, &metadataJSON, &payout.CreatedAt, &payout.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	json.Unmarshal(metadataJSON, &payout.Metadata)
	return &payout, nil
}

// List returns payouts for a merchant
func (s *Service) List(ctx context.Context, merchantID string, limit int, offset int) ([]Payout, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, amount, currency, status, method, arrival_date, created_at
		FROM payouts WHERE merchant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		merchantID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payouts []Payout
	for rows.Next() {
		var p Payout
		err := rows.Scan(
			&p.ID, &p.Amount, &p.Currency, &p.Status, &p.Method,
			&p.ArrivalDate, &p.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		payouts = append(payouts, p)
	}

	return payouts, nil
}

// Cancel cancels a pending payout
func (s *Service) Cancel(ctx context.Context, payoutID string) (*Payout, error) {
	payout, err := s.Get(ctx, payoutID)
	if err != nil {
		return nil, err
	}

	if err := payout.Cancel(); err != nil {
		return nil, err
	}

	_, err = s.db.ExecContext(ctx, `
		UPDATE payouts SET status = $1, updated_at = $2 WHERE id = $3`,
		payout.Status, payout.UpdatedAt, payout.ID,
	)
	if err != nil {
		return nil, err
	}

	if s.dispatcher != nil {
		_ = s.dispatcher.DispatchEvent(ctx, PayoutEventCanceled, payout)
	}

	return payout, nil
}

// RetryPayout manually retries a failed payout
func (s *Service) RetryPayout(ctx context.Context, payoutID string) (*Payout, error) {
	payout, err := s.Get(ctx, payoutID)
	if err != nil {
		return nil, err
	}

	if payout.Status != PayoutStatusFailed {
		return nil, fmt.Errorf("only failed payouts can be retried, current status: %s", payout.Status)
	}

	// Reset status to pending for retry
	payout.Status = PayoutStatusPending
	delete(payout.Metadata, "retry_at")
	delete(payout.Metadata, "last_failure_code")
	delete(payout.Metadata, "last_failure_message")
	payout.UpdatedAt = time.Now().Unix()

	metadataJSON, _ := json.Marshal(payout.Metadata)

	_, err = s.db.ExecContext(ctx, `
		UPDATE payouts SET status = $1, metadata = $2, updated_at = $3 WHERE id = $4`,
		payout.Status, metadataJSON, payout.UpdatedAt, payout.ID,
	)
	if err != nil {
		return nil, err
	}

	if s.dispatcher != nil {
		_ = s.dispatcher.DispatchEvent(ctx, PayoutEventRetried, payout)
	}

	return payout, nil
}

// ProcessPayoutsJob processes pending payouts
type ProcessPayoutsJob struct {
	service *Service
}

// NewProcessPayoutsJob creates payout processing job
func NewProcessPayoutsJob(service *Service) *ProcessPayoutsJob {
	return &ProcessPayoutsJob{service: service}
}

// Run processes all pending payouts (including retries)
func (j *ProcessPayoutsJob) Run(ctx context.Context) error {
	now := time.Now().Unix()
	rows, err := j.service.db.QueryContext(ctx, `
		SELECT id, metadata FROM payouts 
		WHERE status = $1 
		OR (status = $2 AND metadata->>'retry_at' IS NOT NULL AND (metadata->>'retry_at')::bigint <= $3)`,
		PayoutStatusPending,
		PayoutStatusFailed,
		now,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	var payoutIDs []string
	for rows.Next() {
		var id string
		var meta []byte
		rows.Scan(&id, &meta)
		payoutIDs = append(payoutIDs, id)
	}

	for _, id := range payoutIDs {
		if err := j.processPayout(ctx, id); err != nil {
			// Log error and continue
		}
	}

	return nil
}

func (j *ProcessPayoutsJob) processPayout(ctx context.Context, payoutID string) error {
	payout, err := j.service.Get(ctx, payoutID)
	if err != nil {
		return err
	}

	// Skip if payout is scheduled for retry in the future
	if retryAtStr, ok := payout.Metadata["retry_at"]; ok {
		retryAt, err := strconv.ParseInt(retryAtStr, 10, 64)
		if err == nil && retryAt > time.Now().Unix() {
			return nil // Skip, not ready for retry yet
		}
	}

	// Initiate payout with processor
	externalID, err := j.service.processor.InitiatePayout(ctx, payout)
	if err != nil {
		if j.service.handler != nil {
			if handleErr := j.service.handler.HandleFailure(ctx, payout, "processor_error", err.Error()); handleErr != nil {
				return handleErr
			}
		} else {
			payout.MarkFailed("processor_error", err.Error())
		}
	} else {
		payout.MarkInTransit(time.Now().AddDate(0, 0, 2).Unix())
		payout.Metadata["external_id"] = externalID
		delete(payout.Metadata, "retry_at")
		delete(payout.Metadata, "last_failure_code")
		delete(payout.Metadata, "last_failure_message")

		if j.service.dispatcher != nil {
			_ = j.service.dispatcher.DispatchEvent(ctx, PayoutEventInTransit, payout)
		}
	}

	retryCount, _ := parseRetryCount(payout.Metadata)
	metadataJSON, _ := json.Marshal(payout.Metadata)

	_, err = j.service.db.ExecContext(ctx, `
		UPDATE payouts SET status = $1, arrival_date = $2, paid_at = $3, failure_code = $4,
		       failure_message = $5, metadata = $6, updated_at = $7, retry_count = $8, external_id = $9 WHERE id = $10`,
		payout.Status, payout.ArrivalDate, payout.PaidAt, payout.FailureCode,
		payout.FailureMessage, metadataJSON, payout.UpdatedAt, retryCount, payout.Metadata["external_id"], payout.ID,
	)

	return err
}

// HTTPHandler returns payout API handler
func HTTPHandler(service *Service) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var body struct {
			Amount        decimal.Decimal `json:"amount"`
			Currency      string          `json:"currency"`
			Method        PayoutMethod    `json:"method"`
			DestinationID string          `json:"destination_id"`
		}

		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		merchantID := ctx.Value("merchant_id").(string)
		payout, err := service.Create(ctx, merchantID, body.Amount, body.Currency, body.Method, body.DestinationID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payout)
	})

	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		payoutID := chi.URLParam(r, "id")

		payout, err := service.Get(ctx, payoutID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payout)
	})

	r.Post("/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		payoutID := chi.URLParam(r, "id")

		payout, err := service.Cancel(ctx, payoutID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payout)
	})

	r.Post("/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		payoutID := chi.URLParam(r, "id")

		payout, err := service.RetryPayout(ctx, payoutID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payout)
	})

	return r
}
