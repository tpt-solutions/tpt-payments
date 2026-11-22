package connect

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// =============================================================================
// Platform Payout Types
// =============================================================================

// PlatformPayout represents a payout from the platform to a connected account
// Used for paying out marketplace sellers, commissions, etc.
type PlatformPayout struct {
	ID                 string          `json:"id"`
	PlatformID         string          `json:"platform_id"`
	ConnectedAccountID string          `json:"connected_account_id"`
	Amount             decimal.Decimal `json:"amount"`
	Currency           string          `json:"currency"`
	Status             string          `json:"status"`
	Method             string          `json:"method"` // bank_transfer, instant, etc.
	DestinationID      string          `json:"destination_id"` // bank account ID
	Description        string          `json:"description,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	ArrivalDate        int64           `json:"arrival_date,omitempty"`
	TransferredAt      int64           `json:"transferred_at,omitempty"`
	FailureCode        string          `json:"failure_code,omitempty"`
	FailureMessage     string          `json:"failure_message,omitempty"`
	CreatedAt          int64           `json:"created_at"`
	UpdatedAt          int64           `json:"updated_at"`
}

// PlatformPayoutStatus represents the lifecycle states of a platform payout
type PlatformPayoutStatus string

const (
	PlatformPayoutStatusPending     PlatformPayoutStatus = "pending"
	PlatformPayoutStatusInTransit   PlatformPayoutStatus = "in_transit"
	PlatformPayoutStatusPaid        PlatformPayoutStatus = "paid"
	PlatformPayoutStatusFailed      PlatformPayoutStatus = "failed"
	PlatformPayoutStatusCanceled    PlatformPayoutStatus = "canceled"
)

// PlatformPayoutMethod represents available payout methods
type PlatformPayoutMethod string

const (
	PlatformPayoutMethodBankTransfer PlatformPayoutMethod = "bank_transfer"
	PlatformPayoutMethodInstant      PlatformPayoutMethod = "instant"
	PlatformPayoutMethodCard         PlatformPayoutMethod = "card"
)

// =============================================================================
// Platform Balance Service
// =============================================================================

// PlatformBalance tracks the platform's balance available for payouts
type PlatformBalance struct {
	PlatformID      string          `json:"platform_id"`
	Available       decimal.Decimal `json:"available"`
	Pending         decimal.Decimal `json:"pending"`
	Currency        string          `json:"currency"`
	UpdatedAt       int64           `json:"updated_at"`
}

// PlatformPayoutService handles platform payouts to connected accounts
type PlatformPayoutService struct {
	db *sql.DB
}

// NewPlatformPayoutService creates a new platform payout service
func NewPlatformPayoutService(db *sql.DB) *PlatformPayoutService {
	return &PlatformPayoutService{db: db}
}

// CreatePlatformPayout creates a new payout from platform to connected account
func (s *PlatformPayoutService) CreatePlatformPayout(ctx context.Context, platformID string, connectedAccountID string, amount decimal.Decimal, currency string, method string, destinationID string, description string, metadata map[string]string) (*PlatformPayout, error) {
	now := time.Now().UTC().Unix()
	
	// Verify platform has sufficient balance
	balance, err := s.GetPlatformBalance(ctx, platformID, currency)
	if err != nil {
		return nil, err
	}
	
	if balance.Available.LessThan(amount) {
		return nil, sql.ErrNoRows // insufficient balance
	}
	
	payout := &PlatformPayout{
		ID:                 "po_" + uuid.NewString(),
		PlatformID:         platformID,
		ConnectedAccountID: connectedAccountID,
		Amount:             amount,
		Currency:           currency,
		Status:             string(PlatformPayoutStatusPending),
		Method:             method,
		DestinationID:      destinationID,
		Description:        description,
		Metadata:           metadata,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	
	metadataJSON, _ := json.Marshal(payout.Metadata)
	
	_, err = tx.ExecContext(ctx, `
		INSERT INTO platform_payouts (
			id, platform_id, connected_account_id, amount, currency,
			status, method, destination_id, description, metadata,
			arrival_date, transferred_at, failure_code, failure_message,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		payout.ID, payout.PlatformID, payout.ConnectedAccountID, payout.Amount, payout.Currency,
		payout.Status, payout.Method, payout.DestinationID, payout.Description, metadataJSON,
		nil, nil, nil, nil,
		payout.CreatedAt, payout.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	// Deduct from platform balance
	_, err = tx.ExecContext(ctx, `
		UPDATE platform_balances 
		SET available = available - $1, updated_at = $2
		WHERE platform_id = $3 AND currency = $4`,
		amount, now, platformID, currency,
	)
	if err != nil {
		return nil, err
	}
	
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	
	return payout, nil
}

// MarkPlatformPayoutInTransit marks a payout as in transit
func (s *PlatformPayoutService) MarkPlatformPayoutInTransit(ctx context.Context, payoutID string) (*PlatformPayout, error) {
	payout, err := s.GetPlatformPayout(ctx, payoutID)
	if err != nil {
		return nil, err
	}
	
	if payout.Status != string(PlatformPayoutStatusPending) {
		return nil, sql.ErrNoRows
	}
	
	now := time.Now().UTC().Unix()
	payout.Status = string(PlatformPayoutStatusInTransit)
	payout.UpdatedAt = now
	
	_, err = s.db.ExecContext(ctx, `
		UPDATE platform_payouts SET status = $1, updated_at = $2 WHERE id = $3`,
		payout.Status, payout.UpdatedAt, payout.ID,
	)
	if err != nil {
		return nil, err
	}
	
	return payout, nil
}

// MarkPlatformPayoutPaid marks a payout as paid
func (s *PlatformPayoutService) MarkPlatformPayoutPaid(ctx context.Context, payoutID string, transferredAt int64) (*PlatformPayout, error) {
	payout, err := s.GetPlatformPayout(ctx, payoutID)
	if err != nil {
		return nil, err
	}
	
	if payout.Status != string(PlatformPayoutStatusInTransit) {
		return nil, sql.ErrNoRows
	}
	
	now := time.Now().UTC().Unix()
	payout.Status = string(PlatformPayoutStatusPaid)
	payout.TransferredAt = transferredAt
	payout.UpdatedAt = now
	
	_, err = s.db.ExecContext(ctx, `
		UPDATE platform_payouts SET status = $1, transferred_at = $2, updated_at = $3 WHERE id = $4`,
		payout.Status, payout.TransferredAt, payout.UpdatedAt, payout.ID,
	)
	if err != nil {
		return nil, err
	}
	
	return payout, nil
}

// MarkPlatformPayoutFailed marks a payout as failed and returns funds to platform balance
func (s *PlatformPayoutService) MarkPlatformPayoutFailed(ctx context.Context, payoutID string, failureCode string, failureMessage string) (*PlatformPayout, error) {
	payout, err := s.GetPlatformPayout(ctx, payoutID)
	if err != nil {
		return nil, err
	}
	
	now := time.Now().UTC().Unix()
	payout.Status = string(PlatformPayoutStatusFailed)
	payout.FailureCode = failureCode
	payout.FailureMessage = failureMessage
	payout.UpdatedAt = now
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	
	_, err = tx.ExecContext(ctx, `
		UPDATE platform_payouts SET status = $1, failure_code = $2, failure_message = $3, updated_at = $4 WHERE id = $5`,
		payout.Status, payout.FailureCode, payout.FailureMessage, payout.UpdatedAt, payout.ID,
	)
	if err != nil {
		return nil, err
	}
	
	// Return funds to platform balance
	_, err = tx.ExecContext(ctx, `
		UPDATE platform_balances 
		SET available = available + $1, updated_at = $2
		WHERE platform_id = $3 AND currency = $4`,
		payout.Amount, now, payout.PlatformID, payout.Currency,
	)
	if err != nil {
		return nil, err
	}
	
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	
	return payout, nil
}

// GetPlatformPayout retrieves a platform payout by ID
func (s *PlatformPayoutService) GetPlatformPayout(ctx context.Context, payoutID string) (*PlatformPayout, error) {
	var payout PlatformPayout
	var metadataJSON []byte
	var arrivalDate, transferredAt sql.NullInt64
	var failureCode, failureMessage sql.NullString
	
	err := s.db.QueryRowContext(ctx, `
		SELECT id, platform_id, connected_account_id, amount, currency,
		       status, method, destination_id, description, metadata,
		       arrival_date, transferred_at, failure_code, failure_message,
		       created_at, updated_at
		FROM platform_payouts WHERE id = $1`,
		payoutID,
	).Scan(
		&payout.ID, &payout.PlatformID, &payout.ConnectedAccountID, &payout.Amount, &payout.Currency,
		&payout.Status, &payout.Method, &payout.DestinationID, &payout.Description, &metadataJSON,
		&arrivalDate, &transferredAt, &failureCode, &failureMessage,
		&payout.CreatedAt, &payout.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	json.Unmarshal(metadataJSON, &payout.Metadata)
	
	if arrivalDate.Valid {
		payout.ArrivalDate = arrivalDate.Int64
	}
	if transferredAt.Valid {
		payout.TransferredAt = transferredAt.Int64
	}
	if failureCode.Valid {
		payout.FailureCode = failureCode.String
	}
	if failureMessage.Valid {
		payout.FailureMessage = failureMessage.String
	}
	
	return &payout, nil
}

// ListPlatformPayouts lists platform payouts for a connected account
func (s *PlatformPayoutService) ListPlatformPayouts(ctx context.Context, connectedAccountID string, limit int, offset int) ([]*PlatformPayout, error) {
	if limit <= 0 {
		limit = 10
	}
	
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, platform_id, connected_account_id, amount, currency,
		       status, method, destination_id, description, metadata,
		       arrival_date, transferred_at, failure_code, failure_message,
		       created_at, updated_at
		FROM platform_payouts WHERE connected_account_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		connectedAccountID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var payouts []*PlatformPayout
	for rows.Next() {
		var payout PlatformPayout
		var metadataJSON []byte
		var arrivalDate, transferredAt sql.NullInt64
		var failureCode, failureMessage sql.NullString
		
		err := rows.Scan(
			&payout.ID, &payout.PlatformID, &payout.ConnectedAccountID, &payout.Amount, &payout.Currency,
			&payout.Status, &payout.Method, &payout.DestinationID, &payout.Description, &metadataJSON,
			&arrivalDate, &transferredAt, &failureCode, &failureMessage,
			&payout.CreatedAt, &payout.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		
		json.Unmarshal(metadataJSON, &payout.Metadata)
		
		if arrivalDate.Valid {
			payout.ArrivalDate = arrivalDate.Int64
		}
		if transferredAt.Valid {
			payout.TransferredAt = transferredAt.Int64
		}
		if failureCode.Valid {
			payout.FailureCode = failureCode.String
		}
		if failureMessage.Valid {
			payout.FailureMessage = failureMessage.String
		}
		
		payouts = append(payouts, &payout)
	}
	
	return payouts, nil
}

// GetPlatformBalance retrieves the platform's available balance for a currency
func (s *PlatformPayoutService) GetPlatformBalance(ctx context.Context, platformID string, currency string) (*PlatformBalance, error) {
	var balance PlatformBalance
	
	err := s.db.QueryRowContext(ctx, `
		SELECT platform_id, available, pending, currency, updated_at
		FROM platform_balances WHERE platform_id = $1 AND currency = $2`,
		platformID, currency,
	).Scan(
		&balance.PlatformID, &balance.Available, &balance.Pending,
		&balance.Currency, &balance.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		// Return zero balance if not found
		return &PlatformBalance{
			PlatformID: platformID,
			Available:  decimal.Zero,
			Pending:    decimal.Zero,
			Currency:   currency,
			UpdatedAt:  time.Now().UTC().Unix(),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	
	return &balance, nil
}

// CreditPlatformBalance adds funds to the platform balance
// Typically called when application fees are collected from destination charges
func (s *PlatformPayoutService) CreditPlatformBalance(ctx context.Context, platformID string, amount decimal.Decimal, currency string) error {
	now := time.Now().UTC().Unix()
	
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO platform_balances (platform_id, available, pending, currency, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (platform_id, currency) DO UPDATE SET
			available = platform_balances.available + $2, updated_at = $5`,
		platformID, amount, decimal.Zero, currency, now,
	)
	
	return err
}

// DebitPlatformBalance deducts funds from the platform balance
func (s *PlatformPayoutService) DebitPlatformBalance(ctx context.Context, platformID string, amount decimal.Decimal, currency string) error {
	now := time.Now().UTC().Unix()
	
	_, err := s.db.ExecContext(ctx, `
		UPDATE platform_balances 
		SET available = available - $1, updated_at = $2
		WHERE platform_id = $3 AND currency = $4 AND available >= $1`,
		amount, now, platformID, currency,
	)
	
	return err
}
