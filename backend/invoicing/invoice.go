// tpt-payments/backend/invoicing/invoice.go
package invoicing

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// InvoiceStatus represents the current status of an invoice
type InvoiceStatus string

const (
	InvoiceStatusDraft        InvoiceStatus = "draft"
	InvoiceStatusOpen         InvoiceStatus = "open"
	InvoiceStatusPaid         InvoiceStatus = "paid"
	InvoiceStatusVoid         InvoiceStatus = "void"
	InvoiceStatusUncollectible InvoiceStatus = "uncollectible"
)

// InvoiceLineItem represents a single line item on an invoice
type InvoiceLineItem struct {
	ID          string          `json:"id"`
	Description string          `json:"description"`
	Amount      decimal.Decimal `json:"amount"`
	Quantity    int             `json:"quantity"`
	Currency    string          `json:"currency"`
	PriceID     string          `json:"price_id,omitempty"`
}

// Invoice represents a bill sent to a customer
type Invoice struct {
	ID               string            `json:"id"`
	MerchantID       string            `json:"merchant_id"`
	CustomerID       string            `json:"customer_id"`
	SubscriptionID   string            `json:"subscription_id,omitempty"`
	Status           InvoiceStatus     `json:"status"`
	Number           string            `json:"number,omitempty"`
	LineItems        []InvoiceLineItem `json:"line_items"`
	Subtotal         decimal.Decimal   `json:"subtotal"`
	Tax              decimal.Decimal   `json:"tax"`
	Total            decimal.Decimal   `json:"total"`
	Currency         string            `json:"currency"`
	DueDate          int64             `json:"due_date,omitempty"`
	PaidAt           int64             `json:"paid_at,omitempty"`
	VoidedAt         int64             `json:"voided_at,omitempty"`
	FinalizedAt      int64             `json:"finalized_at,omitempty"`
	HostedInvoiceURL string            `json:"hosted_invoice_url,omitempty"`
	PDFURL           string            `json:"pdf_url,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	CreatedAt        int64             `json:"created_at"`
	UpdatedAt        int64             `json:"updated_at"`
}

// NewInvoice creates a new draft invoice
func NewInvoice(customerID string, currency string) *Invoice {
	return &Invoice{
		ID:         "in_" + uuid.NewString(),
		CustomerID: customerID,
		Status:     InvoiceStatusDraft,
		Currency:   currency,
		CreatedAt:  time.Now().UTC().Unix(),
		UpdatedAt:  time.Now().UTC().Unix(),
		LineItems:  []InvoiceLineItem{},
		Metadata:   make(map[string]string),
	}
}

// AddLineItem adds a line item to the invoice
func (i *Invoice) AddLineItem(description string, amount decimal.Decimal, quantity int) error {
	if i.Status != InvoiceStatusDraft {
		return nil
	}

	item := InvoiceLineItem{
		ID:          "il_" + uuid.NewString(),
		Description: description,
		Amount:      amount,
		Quantity:    quantity,
		Currency:    i.Currency,
	}

	i.LineItems = append(i.LineItems, item)
	i.RecalculateTotals()
	i.UpdatedAt = time.Now().UTC().Unix()
	return nil
}

// RecalculateTotals recalculates subtotal, tax and total
func (i *Invoice) RecalculateTotals() {
	subtotal := decimal.Zero
	for _, item := range i.LineItems {
		subtotal = subtotal.Add(item.Amount.Mul(decimal.NewFromInt(int64(item.Quantity))))
	}
	i.Subtotal = subtotal
	i.Total = subtotal.Add(i.Tax)
}

// Finalize marks the invoice as open and generates invoice number
func (i *Invoice) Finalize(invoiceNumber string) error {
	if i.Status != InvoiceStatusDraft {
		return nil
	}
	i.Status = InvoiceStatusOpen
	i.Number = invoiceNumber
	i.FinalizedAt = time.Now().UTC().Unix()
	i.UpdatedAt = time.Now().UTC().Unix()
	return nil
}

// MarkPaid marks the invoice as paid
func (i *Invoice) MarkPaid() {
	i.Status = InvoiceStatusPaid
	i.PaidAt = time.Now().UTC().Unix()
	i.UpdatedAt = time.Now().UTC().Unix()
}

// Void cancels an issued invoice
func (i *Invoice) Void() error {
	if i.Status == InvoiceStatusPaid || i.Status == InvoiceStatusVoid {
		return nil
	}
	i.Status = InvoiceStatusVoid
	i.VoidedAt = time.Now().UTC().Unix()
	i.UpdatedAt = time.Now().UTC().Unix()
	return nil
}