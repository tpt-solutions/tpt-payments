// tpt-payments/backend/internal/settlement/reconciliation.go
package settlement

import (
	"context"
	"fmt"
	"time"
)

// ReconciliationEngine handles matching settlement files to internal transactions
type ReconciliationEngine struct {
	store SettlementStore
}

// NewReconciliationEngine creates a new reconciliation engine
func NewReconciliationEngine(store SettlementStore) *ReconciliationEngine {
	return &ReconciliationEngine{store: store}
}

// ReconcileSettlement matches a settlement file against internal transactions
func (r *ReconciliationEngine) ReconcileSettlement(ctx context.Context, batchID string, networkTransactions []SettlementTransaction) (*ReconciliationReport, error) {
	batch, err := r.store.GetBatch(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get batch: %w", err)
	}

	// Build lookup maps
	internalTxs := make(map[string]SettlementTransaction)
	for _, tx := range batch.Transactions {
		key := r.getTransactionKey(tx)
		internalTxs[key] = tx
	}

	networkTxs := make(map[string]SettlementTransaction)
	for _, tx := range networkTransactions {
		key := r.getTransactionKey(tx)
		networkTxs[key] = tx
	}

	report := &ReconciliationReport{
		ID:            uuid.New().String(),
		BatchID:       batchID,
		ReconciliationDate: time.Now().UTC(),
		Matches:       []ReconciliationMatch{},
		Mismatches:    []ReconciliationMismatch{},
		Missing:       []SettlementTransaction{},
		Extra:         []SettlementTransaction{},
	}

	// Check for matches and mismatches
	for key, internalTx := range internalTxs {
		networkTx, exists := networkTxs[key]
		if !exists {
			report.Missing = append(report.Missing, internalTx)
			continue
		}

		// Check for mismatches
		mismatch := r.checkMismatch(internalTx, networkTx)
		if mismatch != nil {
			report.Mismatches = append(report.Mismatches, *mismatch)
		} else {
			report.Matches = append(report.Matches, ReconciliationMatch{
				InternalTx: internalTx,
				NetworkTx:  networkTx,
				Matched:    true,
			})
		}
	}

	// Check for extra transactions (in network but not internal)
	for key, networkTx := range networkTxs {
		if _, exists := internalTxs[key]; !exists {
			report.Extra = append(report.Extra, networkTx)
		}
	}

	// Update batch status
	if len(report.Mismatches) == 0 && len(report.Missing) == 0 && len(report.Extra) == 0 {
		now := time.Now().UTC()
		batch.Status = "reconciled"
		batch.ReconciledAt = &now
	} else {
		batch.Status = "exception"
	}

	if err := r.store.UpdateBatch(ctx, batch); err != nil {
		return nil, fmt.Errorf("failed to update batch: %w", err)
	}

	// Save report
	if err := r.store.SaveReconciliationReport(ctx, report); err != nil {
		return nil, fmt.Errorf("failed to save reconciliation report: %w", err)
	}

	return report, nil
}

// getTransactionKey creates a unique key for matching transactions
func (r *ReconciliationEngine) getTransactionKey(tx SettlementTransaction) string {
	// Use retrieval reference as primary key (should be unique per transaction)
	if tx.RetrievalRef != "" {
		return tx.RetrievalRef
	}
	// Fallback to STAN + date
	return fmt.Sprintf("%s-%s", tx.STAN, tx.TransactionTime.Format("2006-01-02"))
}

// checkMismatch checks for discrepancies between internal and network transactions
func (r *ReconciliationEngine) checkMismatch(internal, network SettlementTransaction) *ReconciliationMismatch {
	mismatch := &ReconciliationMismatch{
		InternalTx: internal,
		NetworkTx:  network,
		Fields:     []MismatchField{},
	}

	// Check amount
	if internal.Amount != network.Amount {
		mismatch.Fields = append(mismatch.Fields, MismatchField{
			Field:    "amount",
			Internal: fmt.Sprintf("%d", internal.Amount),
			Network:  fmt.Sprintf("%d", network.Amount),
		})
	}

	// Check currency
	if internal.Currency != network.Currency {
		mismatch.Fields = append(mismatch.Fields, MismatchField{
			Field:    "currency",
			Internal: internal.Currency,
			Network:  network.Currency,
		})
	}

	// Check card type
	if internal.CardType != network.CardType {
		mismatch.Fields = append(mismatch.Fields, MismatchField{
			Field:    "card_type",
			Internal: internal.CardType,
			Network:  network.CardType,
		})
	}

	if len(mismatch.Fields) == 0 {
		return nil
	}

	return mismatch
}

// ReconciliationReport represents the result of a reconciliation
type ReconciliationReport struct {
	ID                 string                     `json:"id"`
	BatchID            string                     `json:"batch_id"`
	ReconciliationDate time.Time                `json:"reconciliation_date"`
	Matches            []ReconciliationMatch      `json:"matches"`
	Mismatches         []ReconciliationMismatch   `json:"mismatches"`
	Missing            []SettlementTransaction `json:"missing"`
	Extra              []SettlementTransaction `json:"extra"`
	TotalMatched       int                       `json:"total_matched"`
	TotalMismatched    int                       `json:"total_mismatched"`
	TotalMissing       int                       `json:"total_missing"`
	TotalExtra         int                       `json:"total_extra"`
}

// ReconciliationMatch represents a matched transaction
type ReconciliationMatch struct {
	InternalTx SettlementTransaction `json:"internal_tx"`
	NetworkTx  SettlementTransaction `json:"network_tx"`
	Matched    bool                  `json:"matched"`
}

// ReconciliationMismatch represents a mismatched transaction
type ReconciliationMismatch struct {
	InternalTx SettlementTransaction `json:"internal_tx"`
	NetworkTx  SettlementTransaction `json:"network_tx"`
	Fields     []MismatchField     `json:"fields"`
}

// MismatchField represents a field that doesn't match
type MismatchField struct {
	Field    string `json:"field"`
	Internal string `json:"internal"`
	Network  string `json:"network"`
}

// HandleSettlementReject handles a settlement reject from the network
func (r *ReconciliationEngine) HandleSettlementReject(ctx context.Context, batchID, transactionRef, reason string) error {
	batch, err := r.store.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("failed to get batch: %w", err)
	}

	// Find the transaction
	var rejectedTx *SettlementTransaction
	for i := range batch.Transactions {
		if batch.Transactions[i].RetrievalRef == transactionRef {
			rejectedTx = &batch.Transactions[i]
			break
		}
	}

	if rejectedTx == nil {
		return fmt.Errorf("transaction %s not found in batch %s", transactionRef, batchID)
	}

	// Create exception
	exception := &SettlementException{
		ID:          uuid.New().String(),
		BatchID:     batchID,
		TransactionID: rejectedTx.PaymentIntentID,
		ExceptionType: "rejected",
		Description: fmt.Sprintf("Settlement rejected: %s", reason),
		Status:      "open",
	}

	if err := r.store.SaveException(ctx, exception); err != nil {
		return fmt.Errorf("failed to save exception: %w", err)
	}

	// Update batch
	batch.Exceptions = append(batch.Exceptions, *exception)
	batch.Status = "exception"

	if err := r.store.UpdateBatch(ctx, batch); err != nil {
		return fmt.Errorf("failed to update batch: %w", err)
	}

	// TODO: Fire webhook: settlement.rejected
	// TODO: Notify operator

	return nil
}

// HandlePartialSettlement handles a partial settlement
func (r *ReconciliationEngine) HandlePartialSettlement(ctx context.Context, batchID string, partialAmount int64, transactionRef string) error {
	batch, err := r.store.GetBatch(ctx, batchID)
	if err != nil {
		return fmt.Errorf("failed to get batch: %w", err)
	}

	// Find the transaction
	var partialTx *SettlementTransaction
	for i := range batch.Transactions {
		if batch.Transactions[i].RetrievalRef == transactionRef {
			partialTx = &batch.Transactions[i]
			break
		}
	}

	if partialTx == nil {
		return fmt.Errorf("transaction %s not found in batch %s", transactionRef, batchID)
	}

	// Create exception for partial settlement
	exception := &SettlementException{
		ID:          uuid.New().String(),
		BatchID:     batchID,
		TransactionID: partialTx.PaymentIntentID,
		ExceptionType: "partial_settlement",
		Description: fmt.Sprintf("Partial settlement: expected %d, received %d", partialTx.Amount, partialAmount),
		Status:      "open",
	}

	if err := r.store.SaveException(ctx, exception); err != nil {
		return fmt.Errorf("failed to save exception: %w", err)
	}

	// Update batch
	batch.Exceptions = append(batch.Exceptions, *exception)
	batch.Status = "exception"

	if err := r.store.UpdateBatch(ctx, batch); err != nil {
		return fmt.Errorf("failed to update batch: %w", err)
	}

	// TODO: Fire webhook: settlement.partial
	// TODO: Create ledger entry for the difference

	return nil
}

// GenerateReconciliationReport generates a summary report for operators
func (r *ReconciliationEngine) GenerateReconciliationReport(ctx context.Context, batchID string) (map[string]interface{}, error) {
	report, err := r.store.GetReconciliationReport(ctx, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reconciliation report: %w", err)
	}

	summary := map[string]interface{}{
		"batch_id":          report.BatchID,
		"reconciliation_date": report.ReconciliationDate,
		"total_matched":      len(report.Matches),
		"total_mismatched":   len(report.Mismatches),
		"total_missing":       len(report.Missing),
		"total_extra":         len(report.Extra),
		"match_percentage":    0.0,
	}

	if len(report.Matches)+len(report.Mismatches) > 0 {
		summary["match_percentage"] = float64(len(report.Matches)) / float64(len(report.Matches)+len(report.Mismatches)) * 100
	}

	// Add mismatch details
	if len(report.Mismatches) > 0 {
		mismatches := []map[string]interface{}{}
		for _, mm := range report.Mismatches {
			mismatches = append(mismatches, map[string]interface{}{
				"transaction_ref": mm.Internal.RetrievalRef,
				"internal_amount": mm.Internal.Amount,
				"network_amount":  mm.Network.Amount,
				"fields":         mm.Fields,
			})
		}
		summary["mismatches"] = mismatches
	}

	return summary, nil
}