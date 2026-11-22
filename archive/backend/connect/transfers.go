// tpt-payments/backend/connect/transfers.go
package connect

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// TransferStatus represents transfer lifecycle states
type TransferStatus string

const (
	TransferStatusPending     TransferStatus = "pending"
	TransferStatusInTransit   TransferStatus = "in_transit"
	TransferStatusPaid        TransferStatus = "paid"
	TransferStatusFailed      TransferStatus = "failed"
	TransferStatusCanceled    TransferStatus = "canceled"
)

// Transfer represents a destination charge transfer to connected account
type Transfer struct {
	ID                 string          `json:"id"`
	PaymentIntentID    string          `json:"payment_intent_id"`
	DestinationAccount string          `json:"destination_account"`
	Amount             decimal.Decimal `json:"amount"`
	ApplicationFee     decimal.Decimal `json:"application_fee"`
	PlatformAmount     decimal.Decimal `json:"platform_amount"`
	Status             TransferStatus  `json:"status"`
	Description        string          `json:"description,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	CreatedAt          int64           `json:"created_at"`
	TransferredAt      int64           `json:"transferred_at,omitempty"`
	UpdatedAt          int64           `json:"updated_at"`
}

// NewTransfer creates a new pending transfer
func NewTransfer(paymentIntentID string, destinationAccount string, amount decimal.Decimal, applicationFee decimal.Decimal) *Transfer {
	now := time.Now().UTC().Unix()
	return &Transfer{
		ID:              "tr_" + uuid.NewString(),
		PaymentIntentID: paymentIntentID,
		DestinationAccount: destinationAccount,
		Amount:          amount,
		ApplicationFee:  applicationFee,
		PlatformAmount:  amount.Sub(applicationFee),
		Status:          TransferStatusPending,
		Metadata:        make(map[string]string),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// TransferService handles marketplace transfers
type TransferService struct {
	db *sql.DB
}

// NewTransferService creates transfer service
func NewTransferService(db *sql.DB) *TransferService {
	return &TransferService{db: db}
}

// CreateDestinationTransfer creates a transfer for a destination charge
func (s *TransferService) CreateDestinationTransfer(ctx context.Context, paymentIntentID string, destinationAccount string, totalAmount decimal.Decimal, feeRate decimal.Decimal) (*Transfer, error) {
	applicationFee := totalAmount.Mul(feeRate).RoundBank(2)
	
	transfer := NewTransfer(paymentIntentID, destinationAccount, totalAmount, applicationFee)
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	
	metadataJSON, _ := Marshal(transfer.Metadata)
	
	_, err = tx.ExecContext(ctx, `
		INSERT INTO transfers (
			id, payment_intent_id, destination_account, amount, application_fee, 
			platform_amount, status, description, metadata, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		transfer.ID, transfer.PaymentIntentID, transfer.DestinationAccount, transfer.Amount,
		transfer.ApplicationFee, transfer.PlatformAmount, transfer.Status, transfer.Description,
		metadataJSON, transfer.CreatedAt, transfer.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	
	return transfer, nil
}

// Marshal helper function for JSON
func Marshal(v interface{}) []byte {
	data, _ := json.Marshal(v)
	return data
}