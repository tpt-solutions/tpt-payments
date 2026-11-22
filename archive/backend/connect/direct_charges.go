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
// Direct Charge Types
// =============================================================================

// DirectCharge represents a charge made directly by a connected account
// Unlike destination charges, the connected account is the merchant of record
type DirectCharge struct {
	ID                  string          `json:"id"`
	PlatformID          string          `json:"platform_id"`
	ConnectedAccountID  string          `json:"connected_account_id"`
	CustomerID          string          `json:"customer_id"`
	Amount              decimal.Decimal `json:"amount"`
	Currency            string          `json:"currency"`
	ApplicationFeeAmount decimal.Decimal `json:"application_fee_amount"`
	PlatformFee         decimal.Decimal `json:"platform_fee"`
	Status              string          `json:"status"`
	Description         string          `json:"description,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	Captured            bool            `json:"captured"`
	CapturedAt          int64           `json:"captured_at,omitempty"`
	CreatedAt           int64           `json:"created_at"`
	UpdatedAt           int64           `json:"updated_at"`
}

// DirectChargeStatus represents the lifecycle states of a direct charge
type DirectChargeStatus string

const (
	DirectChargeStatusPending     DirectChargeStatus = "pending"
	DirectChargeStatusSucceeded   DirectChargeStatus = "succeeded"
	DirectChargeStatusFailed      DirectChargeStatus = "failed"
	DirectChargeStatusCanceled    DirectChargeStatus = "canceled"
	DirectChargeStatusDisputed    DirectChargeStatus = "disputed"
)

// =============================================================================
// Direct Charge Service
// =============================================================================

// DirectChargeService handles direct charges for connected accounts
type DirectChargeService struct {
	db *sql.DB
}

// NewDirectChargeService creates a new direct charge service
func NewDirectChargeService(db *sql.DB) *DirectChargeService {
	return &DirectChargeService{db: db}
}

// CreateDirectCharge creates a new direct charge on behalf of a connected account
func (s *DirectChargeService) CreateDirectCharge(ctx context.Context, platformID string, connectedAccountID string, amount decimal.Decimal, currency string, applicationFeeAmount decimal.Decimal, description string, metadata map[string]string) (*DirectCharge, error) {
	now := time.Now().UTC().Unix()
	
	// Calculate platform fee (can include additional fees beyond application_fee)
	platformFee := applicationFeeAmount
	
	charge := &DirectCharge{
		ID:                   "dc_" + uuid.NewString(),
		PlatformID:           platformID,
		ConnectedAccountID:   connectedAccountID,
		Amount:               amount,
		Currency:             currency,
		ApplicationFeeAmount: applicationFeeAmount,
		PlatformFee:          platformFee,
		Status:               string(DirectChargeStatusPending),
		Description:          description,
		Metadata:             metadata,
		Captured:             false,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	
	metadataJSON, _ := json.Marshal(charge.Metadata)
	
	_, err = tx.ExecContext(ctx, `
		INSERT INTO direct_charges (
			id, platform_id, connected_account_id, customer_id, amount, currency,
			application_fee_amount, platform_fee, status, description, metadata,
			captured, captured_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		charge.ID, charge.PlatformID, charge.ConnectedAccountID, charge.CustomerID,
		charge.Amount, charge.Currency, charge.ApplicationFeeAmount, charge.PlatformFee,
		charge.Status, charge.Description, metadataJSON, charge.Captured, nil,
		charge.CreatedAt, charge.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	// Record platform fee as a pending transfer to platform
	if platformFee.GreaterThan(decimal.Zero) {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO platform_fee_transfers (
				id, direct_charge_id, platform_id, connected_account_id, amount,
				currency, status, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
			"pft_"+uuid.NewString(), charge.ID, platformID, connectedAccountID,
			platformFee, currency, "pending", now,
		)
		if err != nil {
			return nil, err
		}
	}
	
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	
	return charge, nil
}

// CaptureDirectCharge captures a pending direct charge
func (s *DirectChargeService) CaptureDirectCharge(ctx context.Context, chargeID string) (*DirectCharge, error) {
	charge, err := s.GetDirectCharge(ctx, chargeID)
	if err != nil {
		return nil, err
	}
	
	if charge.Status != string(DirectChargeStatusPending) {
		return nil, sql.ErrNoRows
	}
	
	now := time.Now().UTC().Unix()
	charge.Status = string(DirectChargeStatusSucceeded)
	charge.Captured = true
	charge.CapturedAt = now
	charge.UpdatedAt = now
	
	_, err = s.db.ExecContext(ctx, `
		UPDATE direct_charges SET status = $1, captured = $2, captured_at = $3, updated_at = $4
		WHERE id = $5`,
		charge.Status, charge.Captured, charge.CapturedAt, charge.UpdatedAt, charge.ID,
	)
	if err != nil {
		return nil, err
	}
	
	// Update platform fee transfer status to completed
	_, err = s.db.ExecContext(ctx, `
		UPDATE platform_fee_transfers SET status = $1, updated_at = $2
		WHERE direct_charge_id = $3`,
		"completed", now, charge.ID,
	)
	if err != nil {
		// Log but don't fail - the charge succeeded
	}
	
	return charge, nil
}

// GetDirectCharge retrieves a direct charge by ID
func (s *DirectChargeService) GetDirectCharge(ctx context.Context, chargeID string) (*DirectCharge, error) {
	var charge DirectCharge
	var metadataJSON []byte
	
	err := s.db.QueryRowContext(ctx, `
		SELECT id, platform_id, connected_account_id, customer_id, amount, currency,
		       application_fee_amount, platform_fee, status, description, metadata,
		       captured, captured_at, created_at, updated_at
		FROM direct_charges WHERE id = $1`,
		chargeID,
	).Scan(
		&charge.ID, &charge.PlatformID, &charge.ConnectedAccountID, &charge.CustomerID,
		&charge.Amount, &charge.Currency, &charge.ApplicationFeeAmount, &charge.PlatformFee,
		&charge.Status, &charge.Description, &metadataJSON,
		&charge.Captured, &charge.CapturedAt, &charge.CreatedAt, &charge.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	json.Unmarshal(metadataJSON, &charge.Metadata)
	
	return &charge, nil
}

// ListDirectCharges lists direct charges for a connected account
func (s *DirectChargeService) ListDirectCharges(ctx context.Context, connectedAccountID string, limit int, offset int) ([]*DirectCharge, error) {
	if limit <= 0 {
		limit = 10
	}
	
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, platform_id, connected_account_id, customer_id, amount, currency,
		       application_fee_amount, platform_fee, status, description, metadata,
		       captured, captured_at, created_at, updated_at
		FROM direct_charges WHERE connected_account_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		connectedAccountID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var charges []*DirectCharge
	for rows.Next() {
		var charge DirectCharge
		var metadataJSON []byte
		
		err := rows.Scan(
			&charge.ID, &charge.PlatformID, &charge.ConnectedAccountID, &charge.CustomerID,
			&charge.Amount, &charge.Currency, &charge.ApplicationFeeAmount, &charge.PlatformFee,
			&charge.Status, &charge.Description, &metadataJSON,
			&charge.Captured, &charge.CapturedAt, &charge.CreatedAt, &charge.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		
		json.Unmarshal(metadataJSON, &charge.Metadata)
		charges = append(charges, &charge)
	}
	
	return charges, nil
}

// CancelDirectCharge cancels a pending direct charge
func (s *DirectChargeService) CancelDirectCharge(ctx context.Context, chargeID string) (*DirectCharge, error) {
	charge, err := s.GetDirectCharge(ctx, chargeID)
	if err != nil {
		return nil, err
	}
	
	if charge.Status != string(DirectChargeStatusPending) {
		return nil, sql.ErrNoRows
	}
	
	now := time.Now().UTC().Unix()
	charge.Status = string(DirectChargeStatusCanceled)
	charge.UpdatedAt = now
	
	_, err = s.db.ExecContext(ctx, `
		UPDATE direct_charges SET status = $1, updated_at = $2 WHERE id = $3`,
		charge.Status, charge.UpdatedAt, charge.ID,
	)
	if err != nil {
		return nil, err
	}
	
	// Cancel associated platform fee transfer
	_, err = s.db.ExecContext(ctx, `
		UPDATE platform_fee_transfers SET status = $1, updated_at = $2
		WHERE direct_charge_id = $3`,
		"canceled", now, charge.ID,
	)
	if err != nil {
		// Log but don't fail
	}
	
	return charge, nil
}
