// tpt-payments/backend/internal/payouts/ach.go
package payouts

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ACHProcessor handles ACH payment processing
type ACHProcessor struct {
	sponsorBankURL string
	apiKey         string
}

// NewACHProcessor creates a new ACH processor
func NewACHProcessor(sponsorBankURL, apiKey string) *ACHProcessor {
	return &ACHProcessor{
		sponsorBankURL: sponsorBankURL,
		apiKey:         apiKey,
	}
}

// ACHPayoutRequest represents an ACH payout request
type ACHPayoutRequest struct {
	ID              string    `json:"id"`
	MerchantID      string    `json:"merchant_id"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	AccountNumber   string    `json:"account_number"`
	RoutingNumber   string    `json:"routing_number"`
	AccountType     string    `json:"account_type"` // checking, savings
	IndividualName  string    `json:"individual_name"`
	CompanyName     string    `json:"company_name,omitempty"`
	Description     string    `json:"description,omitempty"`
	IDempotencyKey string    `json:"idempotency_key"`
}

// ACHPayoutResponse represents the response from ACH payout processing
type ACHPayoutResponse struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"` // pending, processing, succeeded, failed
	ACHFileID     string    `json:"ach_file_id,omitempty"`
	TraceNumber   string    `json:"trace_number,omitempty"`
	EstimatedArrival time.Time `json:"estimated_arrival"`
	CreatedAt     time.Time `json:"created_at"`
	FailureReason string    `json:"failure_reason,omitempty"`
}

// NACHAFile represents a NACHA file for ACH transfers
type NACHAFile struct {
	ID          string      `json:"id"`
	FileID      string      `json:"file_id"`
	BatchNumber int        `json:"batch_number"`
	EntryCount  int        `json:"entry_count"`
	TotalAmount int64      `json:"total_amount"`
	Entries     []NACHAEntry `json:"entries"`
	CreatedAt   time.Time  `json:"created_at"`
	Status      string     `json:"status"` // generated, submitted, acknowledged, rejected
}

// NACHAEntry represents a single entry in a NACHA file
type NACHAEntry struct {
	TransactionCode  int    `json:"transaction_code"`   // 22=checking credit, 32=savings credit
	ReceivingDFI     string `json:"receiving_dfi"`      // First 8 digits of routing number
	AccountNumber   string `json:"account_number"`
	Amount          int64  `json:"amount"`
	IndividualID    string `json:"individual_id"`
	IndividualName  string `json:"individual_name"`
	TraceNumber     string `json:"trace_number"`
	AddendaRecord  bool   `json:"addenda_record"`
}

// CreateACHFile generates a NACHA file for a batch of payouts
func (p *ACHProcessor) CreateACHFile(ctx context.Context, payouts []*Payout) (*NACHAFile, error) {
	if len(payouts) == 0 {
		return nil, fmt.Errorf("no payouts to process")
	}

	file := &NACHAFile{
		ID:         uuid.New().String(),
		FileID:     fmt.Sprintf("ACH%s", time.Now().Format("20060102")),
		BatchNumber: 1,
		CreatedAt:   time.Now().UTC(),
		Status:      "generated",
	}

	for _, payout := range payouts {
		if payout.Method != "ach" {
			continue
		}

		entry := NACHAEntry{
			TransactionCode: 22, // Checking credit
			ReceivingDFI:   payout.RoutingNumber[:8],
			AccountNumber:   payout.AccountNumber,
			Amount:          payout.Amount,
			IndividualID:    payout.MerchantID,
			IndividualName:  payout.AccountHolderName,
			TraceNumber:     generateTraceNumber(payout.RoutingNumber, file.EntryCount+1),
			AddendaRecord:  false,
		}

		file.Entries = append(file.Entries, entry)
		file.TotalAmount += payout.Amount
		file.EntryCount++
	}

	file.BatchNumber = len(file.Entries)

	return file, nil
}

// SubmitACHFile submits a NACHA file to the sponsor bank
func (p *ACHProcessor) SubmitACHFile(ctx context.Context, file *NACHAFile) error {
	// In production, this would:
	// 1. Serialize the NACHA file to proper NACHA format
	// 2. Submit via sponsor bank API or direct SFTP
	// 3. Get acknowledgment with file ID

	// For now, simulate submission
	file.Status = "submitted"

	// Log the file (in production, save to database)
	fileJSON, _ := json.MarshalIndent(file, "", "  ")
	fmt.Printf("NACHA File Submitted:\n%s\n", string(fileJSON))

	return nil
}

// ProcessACHReturn handles ACH return entries (returns, corrections)
func (p *ACHProcessor) ProcessACHReturn(ctx context.Context, returnCode string, traceNumber string) error {
	// Look up the original payout by trace number
	// Update payout status to failed
	// Create ledger entry for the return
	// Notify merchant via webhook

	returnCodes := map[string]string{
		"R01": "Insufficient Funds",
		"R02": "Account Closed",
		"R03": "No Account/Unable to Locate Account",
		"R04": "Invalid Account Number",
		"R05": "Unauthorized Debit to Consumer Account",
		"R06": "Returned per ODFI Request",
		"R07": "Authorization Revoked by Customer",
		"R08": "Payment Stopped",
		"R09": "Uncollected Funds",
		"R10": "Customer Advises Not Authorized",
		"R11": "Check Truncation Entry Returned",
		"R12": "Branch Sold to Another DFI",
		"R13": "RDFI Not Qualified to Participate",
		"R14": "Representative Payee Deceased",
		"R15": "Beneficiary or Account Holder Deceased",
		"R16": "Bank Account Frozen",
		"R17": "File Record Edit Criteria",
		"R18": "Improper Effective Entry Date",
		"R19": "Amount Field Error",
		"R20": "Non-Transaction Account",
		"R21": "Invalid Company Identification",
		"R22": "Invalid Individual ID Number",
		"R23": "Credit Entry Refused by Receiver",
		"R24": "Duplicate Entry",
		"R25": "Addenda Error",
		"R26": "Mandatory Field Error",
		"R27": "Trace Number Error",
		"R28": "Transit Routing Number Check Digit Error",
		"R29": "Corporate Customer Advises Not Authorized",
		"R30": "RDFI Not Participating",
		"R31": "Permitted to Debit",
		"R32": "RDFI Non-Settlement",
		"R33": "Return of XCK Entry",
		"R34": "Limited Participation DFI",
		"R35": "Return of Improper Debit Entry",
		"R36": "Return of Improper Credit Entry",
		"R37": "Source Document Presented for Payment",
		"R38": "Stop Payment on Source Document",
		"R39": "Improper Source Document",
		"R40": "Return of ENR Entry by RDFI",
		"R41": "Invalid Transaction Code",
		"R42": "Routing Number/Check Digit Error",
		"R43": "Invalid DFI Account Number",
		"R44": "Invalid Individual ID Number",
		"R45": "Invalid Individual Name/Company Name",
		"R46": "Invalid Representative Payee Indicator",
		"R47": "Duplicate Enrollment",
		"R50": "State Law Affecting RCK Acceptance",
		"R51": "Item Related to RCK Entry is Ineligible",
		"R52": "Stop Payment on Item Related to RCK Entry",
		"R53": "Item and RCK Entry Presented for Payment",
		"R60": "Misrouted Notification of Change",
		"R61": "Misrouted Return",
		"R67": "Duplicate Return",
		"R68": "Untimely Return",
		"R69": "Field Error(s)",
		"R70": "Permissible Return Entry",
		"R71": "Misrouted Dishonored Return",
		"R72": "Untimely Dishonored Return",
		"R73": "Untimely Notification of Change",
		"R74": "Corrected Return",
		"R75": "Return Not in Original Entry",
		"R76": "Incorrect Data",
		"R80": "IAT Entry Coding Error",
		"R81": "Non-Participant in IAT Program",
		"R82": "Invalid Foreign Transaction Code",
		"R83": "Foreign Receiving DFI Unqualified",
		"R84": "Entry Not Processed by Gateway",
		"R85": "Incorrectly Coded Outbound International Payment",
		"R86": "Incorrect DFI Account Number",
		"R87": "Invalid Foreign Exchange Rate",
		"R88": "Invalid Foreign Trace Number",
		"R89": "Invalid Foreign Amount",
		"R90": "Foreign Item Rejected by Gateway",
		"R91": "Deposit Account Not Found",
		"R92": "Invalid Foreign Receiving DFI Identification",
		"R93": "Invalid Foreign Terminal City",
		"R94": "Invalid Individual Card Account Number",
		"R95": "Duplicate Notification of Change",
		"R96": "Incorrectly Coded Return",
		"R97": "Incorrectly Coded Notification of Change",
		"R98": "Invalid Foreign Transaction Code",
		"R99": "Notification of Change Not Supported",
	}

	reason, exists := returnCodes[returnCode]
	if !exists {
		return fmt.Errorf("unknown return code: %s", returnCode)
	}

	// Log the return
	fmt.Printf("ACH Return: Code=%s, Reason=%s, TraceNumber=%s\n", returnCode, reason, traceNumber)

	return nil
}

// generateTraceNumber generates a unique trace number for ACH entries
func generateTraceNumber(routingNumber string, sequence int) string {
	return fmt.Sprintf("%s%08d", routingNumber[:8], sequence)
}

// ValidateRoutingNumber validates a US routing number using the check digit algorithm
func ValidateRoutingNumber(routingNumber string) bool {
	if len(routingNumber) != 9 {
		return false
	}

	// Routing number check digit algorithm
	// d1 + 3*d2 + 7*d3 + d4 + 3*d5 + 7*d6 + d7 + 3*d8 + 7*d9 ≡ 0 (mod 10)
	sum := 0
	weights := []int{3, 7, 1, 3, 7, 1, 3, 7, 1}

	for i, ch := range routingNumber {
		digit := int(ch - '0')
		if digit < 0 || digit > 9 {
			return false
		}
		sum += digit * weights[i]
	}

	return sum%10 == 0
}