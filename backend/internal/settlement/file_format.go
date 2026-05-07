// tpt-payments/backend/internal/settlement/file_format.go
package settlement

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NetworkFormat represents the settlement file format type
type NetworkFormat string

const (
	FormatVisaBASEII   NetworkFormat = "visa_base_ii"
	FormatMastercardCIS NetworkFormat = "mastercard_cis"
	FormatStandardCSV  NetworkFormat = "standard_csv"
)

// SettlementFile represents a generated settlement file
type SettlementFile struct {
	FileName    string
	Format      NetworkFormat
	Content     []byte
	GeneratedAt time.Time
	BatchID     string
}

// FileGenerator generates network-specific settlement files
type FileGenerator struct {
	batch *SettlementBatch
}

// NewFileGenerator creates a new settlement file generator
func NewFileGenerator(batch *SettlementBatch) *FileGenerator {
	return &FileGenerator{batch: batch}
}

// GenerateFile generates a settlement file in the specified format
func (g *FileGenerator) GenerateFile(format NetworkFormat) (*SettlementFile, error) {
	switch format {
	case FormatVisaBASEII:
		return g.generateVisaBASEII()
	case FormatMastercardCIS:
		return g.generateMastercardCIS()
	case FormatStandardCSV:
		return g.generateStandardCSV()
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// generateVisaBASEII generates a Visa BASE II format settlement file
// Visa BASE II uses a fixed-width format with specific record types
func (g *FileGenerator) generateVisaBASEII() (*SettlementFile, error) {
	var buf bytes.Buffer

	// File Header Record (Type 1)
	header := g.buildVisaHeader()
	buf.WriteString(header)
	buf.WriteString("\n")

	// Transaction Records (Type 2)
	for i, tx := range g.batch.Transactions {
		record := g.buildVisaTransactionRecord(tx, i+1)
		buf.WriteString(record)
		buf.WriteString("\n")
	}

	// File Trailer Record (Type 9)
	trailer := g.buildVisaTrailer()
	buf.WriteString(trailer)

	return &SettlementFile{
		FileName:    fmt.Sprintf("VISA_SETTLEMENT_%s_%s.txt", g.batch.BatchNumber, time.Now().Format("20060102")),
		Format:      FormatVisaBASEII,
		Content:     buf.Bytes(),
		GeneratedAt: time.Now().UTC(),
		BatchID:     g.batch.ID,
	}, nil
}

// buildVisaHeader builds a Visa BASE II file header (simplified)
func (g *FileGenerator) buildVisaHeader() string {
	// Format: RecordType(1) + BatchNumber(10) + BatchDate(8) + Currency(3) + TotalAmount(15) + TransactionCount(8)
	header := fmt.Sprintf("1%-10s%-8s%-3s%015d%08d",
		g.batch.BatchNumber,
		g.batch.BatchDate.Format("20060102"),
		g.batch.Currency,
		g.batch.TotalAmount,
		g.batch.TransactionCount,
	)
	return header
}

// buildVisaTransactionRecord builds a Visa transaction record (simplified)
func (g *FileGenerator) buildVisaTransactionRecord(tx SettlementTransaction, sequence int) string {
	// Format: RecordType(2) + Sequence(6) + STAN(6) + RetrievalRef(12) + Amount(12) + CardType(2) + MTI(4)
	record := fmt.Sprintf("2%06d%-6s%-12s%012d%-2s%-4s",
		sequence,
		tx.STAN,
		tx.RetrievalRef,
		tx.Amount,
		getCardTypeCode(tx.CardType),
		tx.MTI,
	)
	return record
}

// buildVisaTrailer builds a Visa BASE II file trailer (simplified)
func (g *FileGenerator) buildVisaTrailer() string {
	// Format: RecordType(9) + TotalAmount(15) + TransactionCount(8) + Hash(32)
	trailer := fmt.Sprintf("9%015d%08d%-32s",
		g.batch.TotalAmount,
		g.batch.TransactionCount,
		"HASH_PLACEHOLDER", // In production, compute actual hash
	)
	return trailer
}

// generateMastercardCIS generates a Mastercard CIS format settlement file
// Mastercard CIS uses a CSV-like format with specific fields
func (g *FileGenerator) generateMastercardCIS() (*SettlementFile, error) {
	var buf bytes.Buffer

	// Write header line
	header := "RECORD_TYPE,BATCH_NUMBER,BATCH_DATE,CURRENCY,TOTAL_AMOUNT,TRANSACTION_COUNT"
	buf.WriteString(header)
	buf.WriteString("\n")

	// Write batch header
	batchHeader := fmt.Sprintf("BH,%s,%s,%s,%d,%d",
		g.batch.BatchNumber,
		g.batch.BatchDate.Format("2006-01-02"),
		g.batch.Currency,
		g.batch.TotalAmount,
		g.batch.TransactionCount,
	)
	buf.WriteString(batchHeader)
	buf.WriteString("\n")

	// Write transaction records
	for _, tx := range g.batch.Transactions {
		record := fmt.Sprintf("TX,%s,%s,%s,%d,%s,%s",
			tx.STAN,
			tx.RetrievalRef,
			tx.CardType,
			tx.Amount,
			tx.MTI,
			tx.NetworkID,
		)
		buf.WriteString(record)
		buf.WriteString("\n")
	}

	// Write trailer
	trailer := fmt.Sprintf("BT,%d,%d", g.batch.TotalAmount, g.batch.TransactionCount)
	buf.WriteString(trailer)

	return &SettlementFile{
		FileName:    fmt.Sprintf("MC_CIS_SETTLEMENT_%s_%s.csv", g.batch.BatchNumber, time.Now().Format("20060102")),
		Format:      FormatMastercardCIS,
		Content:     buf.Bytes(),
		GeneratedAt: time.Now().UTC(),
		BatchID:     g.batch.ID,
	}, nil
}

// generateStandardCSV generates a standard CSV settlement file
func (g *FileGenerator) generateStandardCSV() (*SettlementFile, error) {
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	// Write header
	header := []string{
		"transaction_id",
		"payment_intent_id",
		"amount",
		"currency",
		"card_type",
		"stan",
		"retrieval_reference",
		"mti",
		"transaction_time",
		"capture_time",
	}
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}

	// Write transaction rows
	for _, tx := range g.batch.Transactions {
		row := []string{
			tx.ID,
			tx.PaymentIntentID,
			strconv.FormatInt(tx.Amount, 10),
			tx.Currency,
			tx.CardType,
			tx.STAN,
			tx.RetrievalRef,
			tx.MTI,
			tx.TransactionTime.Format(time.RFC3339),
			tx.CaptureTime.Format(time.RFC3339),
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("failed to write CSV row: %w", err)
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("CSV writer error: %w", err)
	}

	return &SettlementFile{
		FileName:    fmt.Sprintf("SETTLEMENT_%s_%s.csv", g.batch.BatchNumber, time.Now().Format("20060102")),
		Format:      FormatStandardCSV,
		Content:     buf.Bytes(),
		GeneratedAt: time.Now().UTC(),
		BatchID:     g.batch.ID,
	}, nil
}

// getCardTypeCode returns the network-specific code for a card type
func getCardTypeCode(cardType string) string {
	cardType = strings.ToLower(cardType)
	switch cardType {
	case "visa":
		return "VI"
	case "mastercard":
		return "MC"
	case "amex":
		return "AX"
	case "discover":
		return "DS"
	case "jcb":
		return "JC"
	default:
		return "OT" // Other
	}
}

// ParseSettlementFile parses a network settlement file into transactions
func ParseSettlementFile(content []byte, format NetworkFormat) ([]SettlementTransaction, error) {
	switch format {
	case FormatVisaBASEII:
		return parseVisaBASEII(content)
	case FormatMastercardCIS:
		return parseMastercardCIS(content)
	case FormatStandardCSV:
		return parseStandardCSV(content)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// parseVisaBASEII parses a Visa BASE II format file (simplified)
func parseVisaBASEII(content []byte) ([]SettlementTransaction, error) {
	lines := strings.Split(string(content), "\n")
	transactions := []SettlementTransaction{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check record type
		if len(line) < 1 {
			continue
		}

		recordType := line[0]
		if recordType == '2' && len(line) >= 30 {
			// Parse transaction record (simplified)
			tx := SettlementTransaction{}
			// In production, parse according to Visa BASE II spec
			transactions = append(transactions, tx)
		}
	}

	return transactions, nil
}

// parseMastercardCIS parses a Mastercard CIS format file
func parseMastercardCIS(content []byte) ([]SettlementTransaction, error) {
	lines := strings.Split(string(content), "\n")
	transactions := []SettlementTransaction{}
	inTransactions := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Split(line, ",")
		if len(parts) < 1 {
			continue
		}

		recordType := parts[0]
		if recordType == "TX" && len(parts) >= 6 {
			// Parse transaction record
			tx := SettlementTransaction{
				STAN:        parts[1],
				RetrievalRef: parts[2],
				CardType:    parts[3],
			}
			if amount, err := strconv.ParseInt(parts[4], 10, 64); err == nil {
				tx.Amount = amount
			}
			if len(parts) > 4 {
				tx.MTI = parts[5]
			}
			if len(parts) > 5 {
				tx.NetworkID = parts[6]
			}
			transactions = append(transactions, tx)
		} else if recordType == "BH" {
			inTransactions = true
		}
	}

	return transactions, nil
}

// parseStandardCSV parses a standard CSV settlement file
func parseStandardCSV(content []byte) ([]SettlementTransaction, error) {
	reader := csv.NewReader(bytes.NewReader(content))
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to parse CSV: %w", err)
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("CSV file has no transaction records")
	}

	transactions := []SettlementTransaction{}
	// Skip header row
	for _, record := range records[1:] {
		if len(record) < 8 {
			continue
		}
		tx := SettlementTransaction{
			ID:           record[0],
			PaymentIntentID: record[1],
			Currency:     record[3],
			CardType:     record[4],
			STAN:         record[5],
			RetrievalRef: record[6],
			MTI:          record[7],
		}
		if amount, err := strconv.ParseInt(record[2], 10, 64); err == nil {
			tx.Amount = amount
		}
		transactions = append(transactions, tx)
	}

	return transactions, nil
}