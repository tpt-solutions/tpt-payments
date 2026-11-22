// tpt-payments/backend/internal/compliance/tax_1099k.go
package compliance

import (
	"context"
	"fmt"
	"time"
)

// Form1099KGenerator generates 1099-K tax forms for US merchants
type Form1099KGenerator struct {
	store          TaxStore
	merchantService MerchantService
}

// TaxStore interface for tax data persistence
type TaxStore interface {
	SaveTaxForm(ctx context.Context, form *TaxForm) error
	GetTaxForm(ctx context.Context, id string) (*TaxForm, error)
	ListTaxForms(ctx context.Context, filters TaxFormFilters) ([]*TaxForm, error)
}

// MerchantService interface for merchant data
type MerchantService interface {
	GetMerchant(ctx context.Context, id string) (*Merchant, error)
}

// Merchant represents merchant info for tax purposes
type Merchant struct {
	ID        string
	Name      string
	TaxID     string // EIN or SSN
	Address    Address
	Email     string
	IsBusiness bool
}

// Address represents a physical address
type Address struct {
	Line1   string
	Line2   string
	City    string
	State   string
	ZipCode string
	Country string
}

// TaxForm represents a tax form (1099-K, etc.)
type TaxForm struct {
	ID            string    `json:"id"`
	FormType      string    `json:"form_type"` // "1099-K", "1099-MISC", etc.
	TaxYear       int       `json:"tax_year"`
	MerchantID    string    `json:"merchant_id"`
	MerchantName  string    `json:"merchant_name"`
	MerchantTaxID string    `json:"merchant_tax_id"`
	MerchantAddress Address   `json:"merchant_address"`
	GrossAmount   int64     `json:"gross_amount"`
	TransactionCount int     `json:"transaction_count"`
	FederalTaxWithheld int64 `json:"federal_tax_withheld"`
	StateTaxWithheld    map[string]int64 `json:"state_tax_withheld"`
	PaymentSettlementThirdParty bool `json:"payment_settlement_third_party"`
	CreatedAt     time.Time `json:"created_at"`
	Status        string    `json:"status"` // draft, generated, filed, delivered
	FileURL       string    `json:"file_url,omitempty"`
}

// TaxFormFilters for querying tax forms
type TaxFormFilters struct {
	MerchantID string
	TaxYear    int
	FormType   string
	Status     string
}

// NewForm1099KGenerator creates a new 1099-K generator
func NewForm1099KGenerator(store TaxStore, merchantService MerchantService) *Form1099KGenerator {
	return &Form1099KGenerator{
		store:          store,
		merchantService: merchantService,
	}
}

// Generate1099K generates a 1099-K form for a merchant for a tax year
func (g *Form1099KGenerator) Generate1099K(ctx context.Context, merchantID string, taxYear int) (*TaxForm, error) {
	// Fetch merchant info
	merchant, err := g.merchantService.GetMerchant(ctx, merchantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get merchant: %w", err)
	}

	// Calculate gross amount and transaction count for the tax year
	// In production, this would query payment intents, payouts, etc.
	grossAmount, transactionCount, err := g.calculateGrossAmount(ctx, merchantID, taxYear)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate gross amount: %w", err)
	}

	// Check if 1099-K is required (threshold rules)
	// For 2024+, threshold is $5,000 in gross payments
	threshold := int64(500000) // $5,000 in cents
	if grossAmount < threshold {
		return nil, fmt.Errorf("gross amount %d is below 1099-K threshold %d", grossAmount, threshold)
	}

	// Create tax form
	form := &TaxForm{
		ID:            fmt.Sprintf("tax_%s_%d", merchantID, taxYear),
		FormType:      "1099-K",
		TaxYear:       taxYear,
		MerchantID:    merchantID,
		MerchantName:  merchant.Name,
		MerchantTaxID: merchant.TaxID,
		MerchantAddress: merchant.Address,
		GrossAmount:   grossAmount,
		TransactionCount: transactionCount,
		FederalTaxWithheld: 0, // TPT Pay doesn't withhold, but merchants might
		StateTaxWithheld: make(map[string]int64),
		PaymentSettlementThirdParty: true, // TPT Pay acts as third-party settlement entity
		CreatedAt:     time.Now().UTC(),
		Status:        "generated",
	}

	// Save form
	if err := g.store.SaveTaxForm(ctx, form); err != nil {
		return nil, fmt.Errorf("failed to save tax form: %w", err)
	}

	// Generate PDF (placeholder)
	// In production, use a PDF library to generate the actual 1099-K form
	form.FileURL = fmt.Sprintf("/api/v1/compliance/tax-forms/%s/download", form.ID)

	return form, nil
}

// calculateGrossAmount calculates the gross amount for a merchant in a tax year
func (g *Form1099KGenerator) calculateGrossAmount(ctx context.Context, merchantID string, taxYear int) (int64, int, error) {
	// In production, query payment intents that were successfully settled
	// For now, return placeholder values
	return 1000000, 100, nil // $10,000 gross, 100 transactions
}

// GenerateAll1099K generates 1099-K forms for all eligible merchants
func (g *Form1099KGenerator) GenerateAll1099K(ctx context.Context, taxYear int) ([]*TaxForm, error) {
	// In production, list all active merchants
	// For now, return empty slice
	return []*TaxForm{}, nil
}

// Validate1099K validates a 1099-K form for correctness
func (g *Form1099KGenerator) Validate1099K(form *TaxForm) []string {
	errors := []string{}

	// Check required fields
	if form.MerchantName == "" {
		errors = append(errors, "merchant name is required")
	}
	if form.MerchantTaxID == "" {
		errors = append(errors, "merchant tax ID is required")
	}
	if form.GrossAmount <= 0 {
		errors = append(errors, "gross amount must be positive")
	}
	if form.MerchantAddress.Line1 == "" {
		errors = append(errors, "merchant address is required")
	}

	// Validate tax ID format (EIN or SSN)
	if form.MerchantTaxID != "" {
		if !isValidTaxID(form.MerchantTaxID) {
			errors = append(errors, "invalid tax ID format (must be EIN XX-XXXXXXX or SSN XXX-XX-XXXX)")
		}
	}

	return errors
}

// isValidTaxID validates EIN or SSN format
func isValidTaxID(taxID string) bool {
	// Simple validation: EIN format XX-XXXXXXX or SSN format XXX-XX-XXXX
	// In production, use proper validation
	if len(taxID) == 10 && taxID[2] == '-' { // EIN
		return true
	}
	if len(taxID) == 11 && taxID[3] == '-' && taxID[6] == '-' { // SSN
		return true
	}
	return false
}

// Get1099KSummary returns a summary of 1099-K forms for a tax year
func (g *Form1099KGenerator) Get1099KSummary(ctx context.Context, taxYear int) (map[string]interface{}, error) {
	forms, err := g.store.ListTaxForms(ctx, TaxFormFilters{
		TaxYear:  taxYear,
		FormType: "1099-K",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list tax forms: %w", err)
	}

	summary := map[string]interface{}{
		"tax_year":         taxYear,
		"total_forms":      len(forms),
		"total_gross":       int64(0),
		"total_transactions": 0,
		"by_state":          make(map[string]int64),
	}

	byState := summary["by_state"].(map[string]int64)

	for _, form := range forms {
		summary["total_gross"] = summary["total_gross"].(int64) + form.GrossAmount
		summary["total_transactions"] = summary["total_transactions"].(int) + form.TransactionCount

		// Group by state
		if form.MerchantAddress.State != "" {
			byState[form.MerchantAddress.State] += form.GrossAmount
		}
	}

	return summary, nil
}

// Export1099K exports a 1099-K form in the specified format
func (g *Form1099KGenerator) Export1099K(form *TaxForm, format string) ([]byte, error) {
	switch format {
	case "pdf":
		return g.exportPDF(form)
	case "csv":
		return g.exportCSV(form)
	case "json":
		return g.exportJSON(form)
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// exportPDF exports the 1099-K as PDF
func (g *Form1099KGenerator) exportPDF(form *TaxForm) ([]byte, error) {
	// In production, generate actual 1099-K PDF using a library like go-pdf
	return []byte("PDF content for 1099-K"), nil
}

// exportCSV exports the 1099-K as CSV
func (g *Form1099KGenerator) exportCSV(form *TaxForm) ([]byte, error) {
	csv := fmt.Sprintf("Form Type,Merchant Name,Tax ID,Gross Amount,Transaction Count\n")
	csv += fmt.Sprintf("1099-K,%s,%s,%d,%d\n",
		form.MerchantName, form.MerchantTaxID, form.GrossAmount, form.TransactionCount)
	return []byte(csv), nil
}

// exportJSON exports the 1099-K as JSON
func (g *Form1099KGenerator) exportJSON(form *TaxForm) ([]byte, error) {
	// In production, use encoding/json
	return []byte("{}"), nil
}