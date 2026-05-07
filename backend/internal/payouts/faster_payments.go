// tpt-payments/backend/internal/payouts/faster_payments.go
package payouts

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// FasterPaymentsProcessor handles UK Faster Payments
type FasterPaymentsProcessor struct {
	sponsorBankURL string
	apiKey         string
}

// NewFasterPaymentsProcessor creates a new Faster Payments processor
func NewFasterPaymentsProcessor(sponsorBankURL, apiKey string) *FasterPaymentsProcessor {
	return &FasterPaymentsProcessor{
		sponsorBankURL: sponsorBankURL,
		apiKey:         apiKey,
	}
}

// FasterPaymentsRequest represents a Faster Payments transfer request
type FasterPaymentsRequest struct {
	ID            string `json:"id"`
	MerchantID    string `json:"merchant_id"`
	Amount        int64  `json:"amount"`         // in pence (1/100 GBP)
	Currency      string `json:"currency"`       // GBP required
	SortCode      string `json:"sort_code"`      // 6-digit UK sort code
	AccountNumber string `json:"account_number"` // 8-digit UK account number
	AccountName   string `json:"account_name"`
	Reference     string `json:"reference,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// FasterPaymentsResponse represents the response from Faster Payments processing
type FasterPaymentsResponse struct {
	ID              string    `json:"id"`
	Status          string    `json:"status"`           // pending, processing, succeeded, failed
	TransactionID   string    `json:"transaction_id"`
	ConfirmationID  string    `json:"confirmation_id"`
	EstimatedArrival time.Time `json:"estimated_arrival"`
	CreatedAt       time.Time `json:"created_at"`
	FailureReason   string    `json:"failure_reason,omitempty"`
}

// SortCodeAccount represents a UK bank account with sort code and account number
type SortCodeAccount struct {
	SortCode      string `json:"sort_code"`
	AccountNumber string `json:"account_number"`
	AccountName   string `json:"account_name"`
	BankName      string `json:"bank_name,omitempty"`
}

// ValidateSortCode validates a UK sort code (6 digits)
func ValidateSortCode(sortCode string) bool {
	if len(sortCode) != 6 {
		return false
	}
	for _, ch := range sortCode {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	// Basic check: first two digits represent bank (valid range)
	// In production, you'd validate against a list of valid sort codes
	return true
}

// ValidateUKAccountNumber validates a UK account number (8 digits)
func ValidateUKAccountNumber(accountNumber string) bool {
	if len(accountNumber) != 8 {
		return false
	}
	for _, ch := range accountNumber {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// CreateTransfer creates a Faster Payments transfer
func (p *FasterPaymentsProcessor) CreateTransfer(ctx context.Context, req *FasterPaymentsRequest) (*FasterPaymentsResponse, error) {
	// Validate currency is GBP
	if req.Currency != "GBP" {
		return nil, fmt.Errorf("faster payments require GBP currency")
	}

	// Validate sort code
	if !ValidateSortCode(req.SortCode) {
		return nil, fmt.Errorf("invalid sort code: %s", req.SortCode)
	}

	// Validate account number
	if !ValidateUKAccountNumber(req.AccountNumber) {
		return nil, fmt.Errorf("invalid account number: %s", req.AccountNumber)
	}

	// Generate transaction ID if not provided
	if req.ID == "" {
		req.ID = uuid.New().String()
	}

	// In production, this would call the sponsor bank API or Faster Payments scheme
	// For now, simulate the response
	response := &FasterPaymentsResponse{
		ID:              req.ID,
		Status:          "pending",
		TransactionID:   fmt.Sprintf("FP%s", uuid.New().String()[:8]),
		ConfirmationID:  fmt.Sprintf("CONF%s", uuid.New().String()[:8]),
		EstimatedArrival: time.Now().Add(2 * time.Hour), // Faster Payments typically within 2 hours
		CreatedAt:       time.Now().UTC(),
	}

	return response, nil
}

// GetTransferStatus retrieves the status of a Faster Payments transfer
func (p *FasterPaymentsProcessor) GetTransferStatus(ctx context.Context, transactionID string) (*FasterPaymentsResponse, error) {
	// In production, query the sponsor bank API
	// For now, return a mock response
	return &FasterPaymentsResponse{
		ID:              transactionID,
		Status:          "processing",
		TransactionID:   transactionID,
		EstimatedArrival: time.Now().Add(1 * time.Hour),
	}, nil
}

// HandleReturn handles Faster Payments return codes
func (p *FasterPaymentsProcessor) HandleReturn(ctx context.Context, returnCode string, transactionID string) error {
	returnCodes := map[string]string{
		"AC01": "Invalid Account Number",
		"AC04": "Closed Account",
		"AC06": "Blocked Account",
		"AG01": "Transaction Forbidden",
		"AM04": "Invalid Amount",
		"AM05": "Transaction Amount Exceeds Limit",
		"RC01": "Insufficient Funds",
		"RC04": "Account Closed",
		"RC07": "Account Closed",
		"BE01": "Invalid Bank Code",
		"BE04": "Invalid Account Name",
		"BE07": "Invalid Branch Code",
		"BE13": "Invalid BIC",
		"MS02": "No Mandate",
		"MS03": "No Mandate",
		"FF01": "Invalid BIC",
		"FF05": "Missing Creditor Address",
		"TM01": "Transaction Timed Out",
		"TM02": "Transaction Timed Out",
	}

	reason, exists := returnCodes[returnCode]
	if !exists {
		return fmt.Errorf("unknown Faster Payments return code: %s", returnCode)
	}

	// Log the return
	fmt.Printf("Faster Payments Return: Code=%s, Reason=%s, TransactionID=%s\n", returnCode, reason, transactionID)

	// In production:
	// 1. Look up the original payout by transaction ID
	// 2. Update payout status to failed
	// 3. Create ledger entry for the return
	// 4. Notify merchant via webhook

	return nil
}

// GetBankName returns the bank name for a given sort code (simplified example)
func GetBankName(sortCode string) string {
	bankSortCodes := map[string]string{
		"010203": "Bank of England",
		"040600": "HSBC UK",
		"090126": "Santander UK",
		"160048": "Lloyds Bank",
		"200000": "Barclays",
		"230000": "Lloyds Bank",
		"400000": "HSBC UK",
		"500000": "National Westminster Bank",
		"600000": "Barclays",
		"700000": "Santander UK",
		"800000": "Royal Bank of Scotland",
	}

	if name, ok := bankSortCodes[sortCode]; ok {
		return name
	}
	return "Unknown Bank"
}

// FasterPaymentsBatch represents a batch of Faster Payments transfers
type FasterPaymentsBatch struct {
	ID            string                    `json:"id"`
	BatchID        string                    `json:"batch_id"`
	TransferCount  int                      `json:"transfer_count"`
	TotalAmount    int64                    `json:"total_amount"`
	Currency       string                   `json:"currency"`
	Transfers      []FasterPaymentsRequest  `json:"transfers"`
	Status         string                   `json:"status"` // draft, submitted, acknowledged, rejected
	CreatedAt      time.Time                `json:"created_at"`
}

// CreateBatch creates a batch of Faster Payments transfers
func (p *FasterPaymentsProcessor) CreateBatch(ctx context.Context, transfers []FasterPaymentsRequest) (*FasterPaymentsBatch, error) {
	if len(transfers) == 0 {
		return nil, fmt.Errorf("no transfers to process")
	}

	batch := &FasterPaymentsBatch{
		ID:           uuid.New().String(),
		BatchID:       fmt.Sprintf("FPBATCH%s", time.Now().Format("20060102")),
		TransferCount: len(transfers),
		Currency:      "GBP",
		Transfers:     transfers,
		Status:        "draft",
		CreatedAt:     time.Now().UTC(),
	}

	for _, transfer := range transfers {
		batch.TotalAmount += transfer.Amount
	}

	// In production, submit batch to sponsor bank
	batch.Status = "submitted"

	return batch, nil
}