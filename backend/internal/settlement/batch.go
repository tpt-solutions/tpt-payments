// tpt-payments/backend/internal/settlement/batch.go
package settlement

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// SettlementBatch represents an end-of-day settlement batch
type SettlementBatch struct {
	ID              string    `json:"id"`
	BatchNumber     string    `json:"batch_number"`
	BatchDate       time.Time `json:"batch_date"`
	Status          string    `json:"status"` // draft, generated, submitted, acknowledged, reconciled, exception
	TotalAmount     int64     `json:"total_amount"`
	TransactionCount int       `json:"transaction_count"`
	Currency        string    `json:"currency"`
	Transactions    []SettlementTransaction `json:"transactions"`
	GeneratedAt     time.Time `json:"generated_at"`
	SubmittedAt     *time.Time `json:"submitted_at,omitempty"`
	AcknowledgedAt  *time.Time `json:"acknowledged_at,omitempty"`
	ReconciledAt   *time.Time `json:"reconciled_at,omitempty"`
	FileReference   string    `json:"file_reference,omitempty"` // Network-specific reference
	Exceptions      []SettlementException `json:"exceptions,omitempty"`
}

// SettlementTransaction represents a single transaction in a settlement batch
type SettlementTransaction struct {
	ID              string    `json:"id"`
	PaymentIntentID string    `json:"payment_intent_id"`
	Amount          int64     `json:"amount"`
	Currency        string    `json:"currency"`
	CardType        string    `json:"card_type"`        // visa, mastercard, amex
	MerchantID      string    `json:"merchant_id"`
	TransactionTime time.Time `json:"transaction_time"`
	CaptureTime     time.Time `json:"capture_time"`
	MTI             string    `json:"mti"`               // ISO 8583 MTI
	ProcessingCode  string    `json:"processing_code"`    // ISO 8583 field 3
	STAN            string    `json:"stan"`              // System Trace Audit Number
	RetrievalRef    string    `json:"retrieval_reference"` // Retrieval reference number
	NetworkID       string    `json:"network_id"`        // Visa, MC, etc.
}

// SettlementException represents a problem with a settlement transaction
type SettlementException struct {
	ID          string `json:"id"`
	BatchID     string `json:"batch_id"`
	TransactionID string `json:"transaction_id"`
	ExceptionType string `json:"exception_type"` // mismatch, missing, duplicate, amount_difference
	Description string `json:"description"`
	Status      string `json:"status"` // open, investigating, resolved
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	Resolution  string `json:"resolution,omitempty"` // adjusted, waived, escalated
}

// BatchGenerator generates end-of-day settlement batches
type BatchGenerator struct {
	store       SettlementStore
	acquiringAdapter AcquiringAdapter
}

// SettlementStore interface for persistence
type SettlementStore interface {
	SaveBatch(ctx context.Context, batch *SettlementBatch) error
	GetBatch(ctx context.Context, batchID string) (*SettlementBatch, error)
	UpdateBatch(ctx context.Context, batch *SettlementBatch) error
	ListBatches(ctx context.Context, filters BatchFilters) ([]*SettlementBatch, error)
	SaveTransaction(ctx context.Context, tx *SettlementTransaction) error
	SaveException(ctx context.Context, exc *SettlementException) error
}

// BatchFilters for querying batches
type BatchFilters struct {
	StartDate  *time.Time
	EndDate    *time.Time
	Status     string
	Currency   string
	Limit      int
	Offset     int
}

// AcquiringAdapter interface for fetching transactions
type AcquiringAdapter interface {
	GetCapturedTransactions(ctx context.Context, date time.Time) ([]SettlementTransaction, error)
}

// NewBatchGenerator creates a new settlement batch generator
func NewBatchGenerator(store SettlementStore, adapter AcquiringAdapter) *BatchGenerator {
	return &BatchGenerator{
		store:       store,
		acquiringAdapter: adapter,
	}
}

// GenerateEndOfDayBatch generates the settlement batch for a specific date
func (g *BatchGenerator) GenerateEndOfDayBatch(ctx context.Context, date time.Time) (*SettlementBatch, error) {
	// Fetch all captured transactions for the date
	transactions, err := g.acquiringAdapter.GetCapturedTransactions(ctx, date)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch transactions: %w", err)
	}

	if len(transactions) == 0 {
		return nil, fmt.Errorf("no transactions to settle for %s", date.Format("2006-01-02"))
	}

	// Create batch
	batch := &SettlementBatch{
		ID:              uuid.New().String(),
		BatchNumber:     fmt.Sprintf("SET%s%03d", date.Format("20060102"), 1), // Simple sequential
		BatchDate:       date,
		Status:          "generated",
		TransactionCount: len(transactions),
		GeneratedAt:     time.Now().UTC(),
		Transactions:    transactions,
	}

	// Calculate totals
	for _, tx := range transactions {
		batch.TotalAmount += tx.Amount
		if batch.Currency == "" {
			batch.Currency = tx.Currency
		}
	}

	// Save batch
	if err := g.store.SaveBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to save batch: %w", err)
	}

	// Save individual transactions
	for i := range transactions {
		transactions[i].ID = uuid.New().String()
		if err := g.store.SaveTransaction(ctx, &transactions[i]); err != nil {
			return nil, fmt.Errorf("failed to save transaction: %w", err)
		}
	}

	return batch, nil
}

// SubmitBatch submits the batch to the card network
func (g *BatchGenerator) SubmitBatch(ctx context.Context, batchID string) error {
	batch, err := g.store.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("failed to get batch: %w", err)
	}

	if batch.Status != "generated" {
		return fmt.Errorf("batch %s is not in generated state (current: %s)", batchID, batch.Status)
	}

	// In production, this would:
	// 1. Format the batch into network-specific format (Visa BASE II, Mastercard CIS)
	// 2. Submit via DirectAcquiringAdapter
	// 3. Get acknowledgment with file reference

	now := time.Now().UTC()
	batch.Status = "submitted"
	batch.SubmittedAt = &now
	batch.FileReference = fmt.Sprintf("FR%s", uuid.New().String()[:8])

	if err := g.store.UpdateBatch(ctx, batch); err != nil {
		return fmt.Errorf("failed to update batch: %w", err)
	}

	return nil
}

// AcknowledgeBatch marks a batch as acknowledged by the network
func (g *BatchGenerator) AcknowledgeBatch(ctx context.Context, batchID string, networkRef string) error {
	batch, err := g.store.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("failed to get batch: %w", err)
	}

	if batch.Status != "submitted" {
		return fmt.Errorf("batch %s is not in submitted state (current: %s)", batchID, batch.Status)
	}

	now := time.Now().UTC()
	batch.Status = "acknowledged"
	batch.AcknowledgedAt = &now
	batch.FileReference = networkRef

	if err := g.store.UpdateBatch(ctx, batch); err != nil {
		return fmt.Errorf("failed to update batch: %w", err)
	}

	return nil
}

// ReconcileBatch reconciles a batch against network report
func (g *BatchGenerator) ReconcileBatch(ctx context.Context, batchID string, networkReport []SettlementTransaction) error {
	batch, err := g.store.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("failed to get batch: %w", err)
	}

	if batch.Status != "acknowledged" {
		return fmt.Errorf("batch %s is not in acknowledged state (current: %s)", batchID, batch.Status)
	}

	// Build lookup maps
	internalTxs := make(map[string]SettlementTransaction)
	for _, tx := range batch.Transactions {
		internalTxs[tx.RetrievalRef] = tx
	}

	networkTxs := make(map[string]SettlementTransaction)
	for _, tx := range networkReport {
		networkTxs[tx.RetrievalRef] = tx
	}

	// Check for exceptions
	exceptions := []SettlementException{}

	// Check for missing transactions (in network but not internal)
	for ref, networkTx := range networkTxs {
		if _, exists := internalTxs[ref]; !exists {
			exceptions = append(exceptions, SettlementException{
				ID:          uuid.New().String(),
				BatchID:     batchID,
				TransactionID: networkTx.ID,
				ExceptionType: "missing",
				Description: fmt.Sprintf("Transaction %s in network report but not in internal batch", ref),
				Status:      "open",
			})
		}
	}

	// Check for extra transactions (internal but not in network)
	for ref, internalTx := range internalTxs {
		if _, exists := networkTxs[ref]; !exists {
			exceptions = append(exceptions, SettlementException{
				ID:          uuid.New().String(),
				BatchID:     batchID,
				TransactionID: internalTx.ID,
				ExceptionType: "extra",
				Description: fmt.Sprintf("Transaction %s in internal batch but not in network report", ref),
				Status:      "open",
			})
		}
	}

	// Check for amount mismatches
	for ref, internalTx := range internalTxs {
		if networkTx, exists := networkTxs[ref]; exists {
			if internalTx.Amount != networkTx.Amount {
				exceptions = append(exceptions, SettlementException{
					ID:          uuid.New().String(),
					BatchID:     batchID,
					TransactionID: internalTx.ID,
					ExceptionType: "amount_difference",
					Description: fmt.Sprintf("Amount mismatch for %s: internal=%d, network=%d", ref, internalTx.Amount, networkTx.Amount),
					Status:      "open",
				})
			}
		}
	}

	// Save exceptions
	for i := range exceptions {
		if err := g.store.SaveException(ctx, &exceptions[i]); err != nil {
			return fmt.Errorf("failed to save exception: %w", err)
		}
	}

	batch.Exceptions = exceptions

	// Update batch status
	if len(exceptions) == 0 {
		now := time.Now().UTC()
		batch.Status = "reconciled"
		batch.ReconciledAt = &now
	} else {
		batch.Status = "exception"
	}

	if err := g.store.UpdateBatch(ctx, batch); err != nil {
		return fmt.Errorf("failed to update batch: %w", err)
	}

	return nil
}

// GetBatchSummary returns a summary of a batch for reporting
func (g *BatchGenerator) GetBatchSummary(ctx context.Context, batchID string) (map[string]interface{}, error) {
	batch, err := g.store.GetBatch(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	summary := map[string]interface{}{
		"batch_id":         batch.ID,
		"batch_number":     batch.BatchNumber,
		"batch_date":       batch.BatchDate,
		"status":           batch.Status,
		"total_amount":     batch.TotalAmount,
		"transaction_count": batch.TransactionCount,
		"currency":         batch.Currency,
		"generated_at":     batch.GeneratedAt,
		"exception_count":  len(batch.Exceptions),
	}

	// Calculate by card type
	cardTypeTotals := make(map[string]int64)
	for _, tx := range batch.Transactions {
		cardTypeTotals[tx.CardType] += tx.Amount
	}
	summary["by_card_type"] = cardTypeTotals

	return summary, nil
}