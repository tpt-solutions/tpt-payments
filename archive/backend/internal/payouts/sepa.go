// tpt-payments/backend/internal/payouts/sepa.go
package payouts

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SEPACreditTransfer handles SEPA Credit Transfer processing
type SEPACreditTransfer struct {
	sponsorBankURL string
	apiKey         string
}

// NewSEPACreditTransfer creates a new SEPA transfer processor
func NewSEPACreditTransfer(sponsorBankURL, apiKey string) *SEPACreditTransfer {
	return &SEPACreditTransfer{
		sponsorBankURL: sponsorBankURL,
		apiKey:         apiKey,
	}
}

// SEPATransferRequest represents a SEPA Credit Transfer request
type SEPATransferRequest struct {
	ID              string    `json:"id"`
	MerchantID      string    `json:"merchant_id"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`         // EUR required for SEPA
	IBAN            string    `json:"iban"`
	BIC             string    `json:"bic,omitempty"`
	AccountName     string    `json:"account_name"`
	Description     string    `json:"description,omitempty"`
	EndToEndID      string    `json:"end_to_end_id"`     // Unique identifier for the transaction
	IdempotencyKey string    `json:"idempotency_key"`
}

// SEPATransferResponse represents the response from SEPA transfer processing
type SEPATransferResponse struct {
	ID              string    `json:"id"`
	Status          string    `json:"status"`           // pending, processing, succeeded, failed
	TransactionID   string    `json:"transaction_id"`
	EndToEndID      string    `json:"end_to_end_id"`
	EstimatedArrival time.Time `json:"estimated_arrival"`
	CreatedAt       time.Time `json:"created_at"`
	FailureReason   string    `json:"failure_reason,omitempty"`
}

// SEPABatch represents a batch of SEPA transfers
type SEPABatch struct {
	ID           string             `json:"id"`
	BatchID       string             `json:"batch_id"`
	TransferCount int                `json:"transfer_count"`
	TotalAmount   int64              `json:"total_amount"`
	Currency      string             `json:"currency"`
	Transfers     []SEPATransferRequest `json:"transfers"`
	Status        string             `json:"status"` // draft, submitted, acknowledged, rejected
	CreatedAt     time.Time          `json:"created_at"`
}

// ValidateIBAN validates a SEPA IBAN number
func ValidateIBAN(iban string) bool {
	// Remove spaces
	cleanIBAN := ""
	for _, ch := range iban {
		if ch != ' ' && ch != '\t' {
			cleanIBAN += string(ch)
		}
	}

	// IBAN must be at least 15 characters and at most 34
	if len(cleanIBAN) < 15 || len(cleanIBAN) > 34 {
		return false
	}

	// Move first 4 characters to the end
	rearranged := cleanIBAN[4:] + cleanIBAN[:4]

	// Convert letters to numbers (A=10, B=11, etc.)
	numStr := ""
	for _, ch := range rearranged {
		if ch >= '0' && ch <= '9' {
			numStr += string(ch)
		} else if ch >= 'A' && ch <= 'Z' {
			numStr += fmt.Sprintf("%d", int(ch-'A')+10)
		} else {
			return false // Invalid character
		}
	}

	// Check if the number is valid using mod 97
	return mod97(numStr) == 1
}

// mod97 calculates the IBAN checksum using mod 97 algorithm
func mod97(s string) int {
	const maxLen = 9
	remainder := 0
	for i := 0; i < len(s); i += maxLen {
		end := i + maxLen
		if end > len(s) {
			end = len(s)
		}
		segment := fmt.Sprintf("%d%s", remainder, s[i:end])
		fmt.Sscanf(segment, "%d", &remainder)
		remainder %= 97
	}
	return remainder
}

// CreateTransfer creates a single SEPA Credit Transfer
func (s *SEPACreditTransfer) CreateTransfer(ctx context.Context, req *SEPATransferRequest) (*SEPATransferResponse, error) {
	// Validate currency is EUR
	if req.Currency != "EUR" {
		return nil, fmt.Errorf("SEPA transfers require EUR currency")
	}

	// Validate IBAN
	if !ValidateIBAN(req.IBAN) {
		return nil, fmt.Errorf("invalid IBAN: %s", req.IBAN)
	}

	// Generate end-to-end ID if not provided
	if req.EndToEndID == "" {
		req.EndToEndID = fmt.Sprintf("SEPA%s", uuid.New().String()[:16])
	}

	// In production, this would call the sponsor bank API
	// For now, simulate the response
	response := &SEPATransferResponse{
		ID:              uuid.New().String(),
		Status:          "pending",
		TransactionID:   fmt.Sprintf("TXN%s", uuid.New().String()[:8]),
		EndToEndID:      req.EndToEndID,
		EstimatedArrival: time.Now().Add(24 * time.Hour), // SEPA typically takes 1 business day
		CreatedAt:       time.Now().UTC(),
	}

	return response, nil
}

// CreateBatch creates a batch of SEPA transfers
func (s *SEPACreditTransfer) CreateBatch(ctx context.Context, transfers []SEPATransferRequest) (*SEPABatch, error) {
	if len(transfers) == 0 {
		return nil, fmt.Errorf("no transfers to process")
	}

	batch := &SEPABatch{
		ID:           uuid.New().String(),
		BatchID:       fmt.Sprintf("SEPABATCH%s", time.Now().Format("20060102")),
		TransferCount: len(transfers),
		Currency:      "EUR",
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

// GetTransferStatus retrieves the status of a SEPA transfer
func (s *SEPACreditTransfer) GetTransferStatus(ctx context.Context, transferID string) (*SEPATransferResponse, error) {
	// In production, query the sponsor bank API
	// For now, return a mock response
	return &SEPATransferResponse{
		ID:              transferID,
		Status:          "processing",
		EstimatedArrival: time.Now().Add(12 * time.Hour),
	}, nil
}

// HandleReturn handles SEPA return codes (R-transactions)
func (s *SEPACreditTransfer) HandleReturn(ctx context.Context, returnCode string, endToEndID string) error {
	returnCodes := map[string]string{
		"AC01": "Invalid IBAN",
		"AC04": "Closed Account Number",
		"AC06": "Blocked Account",
		"AG01": "Transaction Forbidden",
		"AM04": "Invalid Amount",
		"AM05": "Transaction Amount Exceeds Limit",
		"RC01": "Incorrect Account Number",
		"RC04": "Customer Deceased",
		"RC07": "Forgot Password/Account Number",
		"MD01": "No Mandate",
		"MD02": "Missing Mandate Details",
		"MD06": "Refund Request By End Customer",
		"MD07": "End Customer Deceased",
		"FF01": "Invalid BIC",
		"FF05": "Missing Creditor Address",
	}

	reason, exists := returnCodes[returnCode]
	if !exists {
		return fmt.Errorf("unknown SEPA return code: %s", returnCode)
	}

	// Log the return
	fmt.Printf("SEPA Return: Code=%s, Reason=%s, EndToEndID=%s\n", returnCode, reason, endToEndID)

	// In production:
	// 1. Look up the original transfer by endToEndID
	// 2. Update payout status to failed
	// 3. Create ledger entry for the return
	// 4. Notify merchant via webhook

	return nil
}

// GetSupportedCountries returns the list of SEPA zone countries
func GetSupportedCountries() []string {
	return []string{
		"AT", // Austria
		"BE", // Belgium
		"BG", // Bulgaria
		"HR", // Croatia
		"CY", // Cyprus
		"CZ", // Czech Republic
		"DK", // Denmark
		"EE", // Estonia
		"FI", // Finland
		"FR", // France
		"DE", // Germany
		"GR", // Greece
		"HU", // Hungary
		"IS", // Iceland
		"IE", // Ireland
		"IT", // Italy
		"LV", // Latvia
		"LT", // Lithuania
		"LU", // Luxembourg
		"MT", // Malta
		"NL", // Netherlands
		"NO", // Norway
		"PL", // Poland
		"PT", // Portugal
		"RO", // Romania
		"SK", // Slovakia
		"SI", // Slovenia
		"ES", // Spain
		"SE", // Sweden
		"CH", // Switzerland
		"GB", // United Kingdom (until fully exited)
		// Non-EEA SEPA members
		"MC", // Monaco
		"SM", // San Marino
		"VA", // Vatican City
	}
}