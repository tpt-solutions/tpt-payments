// tpt-payments/backend/invoicing/service.go
package invoicing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"github.com/google/uuid"
)

// InvoiceStatus represents invoice lifecycle states
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
	Description string          `json:"description"`
	Amount      decimal.Decimal `json:"amount"`
	Quantity    int             `json:"quantity"`
	UnitAmount  decimal.Decimal `json:"unit_amount"`
	TaxRate     decimal.Decimal `json:"tax_rate,omitempty"`
}

// Invoice represents a customer invoice
type Invoice struct {
	ID              string            `json:"id"`
	MerchantID      string            `json:"merchant_id"`
	CustomerID      string            `json:"customer_id"`
	Number          string            `json:"number"`
	Status          InvoiceStatus     `json:"status"`
	LineItems       []InvoiceLineItem `json:"line_items"`
	Subtotal        decimal.Decimal   `json:"subtotal"`
	Tax             decimal.Decimal   `json:"tax"`
	Total           decimal.Decimal   `json:"total"`
	Currency        string            `json:"currency"`
	DueAt           int64             `json:"due_at"`
	PaidAt          int64             `json:"paid_at,omitempty"`
	VoidedAt        int64             `json:"voided_at,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	HostedURL       string            `json:"hosted_url,omitempty"`
	PDFURL          string            `json:"pdf_url,omitempty"`
	CreatedAt       int64             `json:"created_at"`
	UpdatedAt       int64             `json:"updated_at"`
}

// NewInvoice creates a new draft invoice
func NewInvoice(merchantID string, customerID string) *Invoice {
	now := time.Now().UTC()
	return &Invoice{
		ID:         "inv_" + uuid.NewString(),
		MerchantID: merchantID,
		CustomerID: customerID,
		Status:     InvoiceStatusDraft,
		CreatedAt:  now.Unix(),
		UpdatedAt:  now.Unix(),
		Metadata:   make(map[string]string),
	}
}

// AddLineItem adds an item to the invoice
func (i *Invoice) AddLineItem(description string, unitAmount decimal.Decimal, quantity int) {
	amount := unitAmount.Mul(decimal.NewFromInt(int64(quantity)))
	i.LineItems = append(i.LineItems, InvoiceLineItem{
		Description: description,
		Amount:      amount,
		Quantity:    quantity,
		UnitAmount:  unitAmount,
	})

	i.Subtotal = i.Subtotal.Add(amount)
	i.Total = i.Subtotal.Add(i.Tax)
}

// Finalize marks the invoice as open and assigns invoice number
func (i *Invoice) Finalize(sequenceNumber int) {
	i.Status = InvoiceStatusOpen
	i.Number = generateInvoiceNumber(sequenceNumber)
	i.UpdatedAt = time.Now().UTC().Unix()
}

// Service handles invoice business logic
type Service struct {
	db          *sql.DB
	payments    PaymentProcessor
	emailSender EmailSender
}

// PaymentProcessor interface
type PaymentProcessor interface {
	CreatePaymentIntent(ctx context.Context, customerID string, amount decimal.Decimal, currency string) (string, error)
}

// EmailSender interface
type EmailSender interface {
	SendInvoice(ctx context.Context, invoice *Invoice) error
}

// NewService creates invoice service
func NewService(db *sql.DB, payments PaymentProcessor, emailSender EmailSender) *Service {
	return &Service{
		db:          db,
		payments:    payments,
		emailSender: emailSender,
	}
}

// Create creates a new draft invoice
func (s *Service) Create(ctx context.Context, merchantID string, customerID string, lineItems []InvoiceLineItem) (*Invoice, error) {
	inv := NewInvoice(merchantID, customerID)

	for _, item := range lineItems {
		inv.AddLineItem(item.Description, item.UnitAmount, item.Quantity)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Insert invoice
	lineItemsJSON, _ := json.Marshal(inv.LineItems)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (
			id, merchant_id, customer_id, status, line_items, subtotal, tax, total, currency, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		inv.ID, inv.MerchantID, inv.CustomerID, inv.Status, lineItemsJSON,
		inv.Subtotal, inv.Tax, inv.Total, inv.Currency, inv.CreatedAt, inv.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return inv, nil
}

// Finalize finalizes an invoice for sending
func (s *Service) Finalize(ctx context.Context, invoiceID string) (*Invoice, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var inv Invoice
	err = tx.QueryRowContext(ctx, `
		SELECT id, merchant_id, customer_id, status, line_items, subtotal, tax, total, currency
		FROM invoices WHERE id = $1 FOR UPDATE`,
		invoiceID,
	).Scan(&inv.ID, &inv.MerchantID, &inv.CustomerID, &inv.Status, &inv.LineItems,
		&inv.Subtotal, &inv.Tax, &inv.Total, &inv.Currency)
	if err == sql.ErrNoRows {
		return nil, errors.New("invoice not found")
	}

	if inv.Status != InvoiceStatusDraft {
		return nil, errors.New("only draft invoices can be finalized")
	}

	// Get next invoice number sequence
	var sequence int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1 FROM invoices WHERE merchant_id = $1`,
		inv.MerchantID,
	).Scan(&sequence)
	if err != nil {
		return nil, err
	}

	inv.Finalize(sequence)
	inv.HostedURL = fmt.Sprintf("/invoices/%s", inv.ID)

	_, err = tx.ExecContext(ctx, `
		UPDATE invoices SET status = $1, number = $2, sequence = $3, hosted_url = $4, updated_at = $5, due_at = $6 WHERE id = $7`,
		inv.Status, inv.Number, sequence, inv.HostedURL, inv.UpdatedAt, inv.DueAt, inv.ID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Send invoice email
	go s.emailSender.SendInvoice(context.Background(), &inv)

	return &inv, nil
}

// CreateForSubscription creates an invoice for a successful subscription renewal
func (s *Service) CreateForSubscription(ctx context.Context, subscriptionID string, customerID string, amount decimal.Decimal, currency string) (string, error) {
	// Create invoice with subscription renewal line item
	inv := NewInvoice("", customerID) // Merchant ID will be populated from subscription lookup
	inv.AddLineItem("Subscription Renewal", amount, 1)
	inv.SubscriptionID = subscriptionID
	inv.Currency = currency
	inv.DueAt = time.Now().AddDate(0, 0, 14).Unix()
	
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	
	// Lookup merchant ID from subscription
	err = tx.QueryRowContext(ctx, `
		SELECT merchant_id FROM subscriptions WHERE id = $1`,
		subscriptionID,
	).Scan(&inv.MerchantID)
	if err != nil {
		return "", err
	}
	
	// Get next invoice number sequence
	var sequence int
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(sequence), 0) + 1 FROM invoices WHERE merchant_id = $1`,
		inv.MerchantID,
	).Scan(&sequence)
	if err != nil {
		return "", err
	}
	
	inv.Finalize(sequence)
	inv.HostedURL = fmt.Sprintf("/invoices/%s", inv.ID)
	inv.PDFURL = fmt.Sprintf("/invoices/%s/pdf", inv.ID)
	inv.Status = InvoiceStatusPaid
	inv.PaidAt = time.Now().Unix()
	
	// Insert invoice
	lineItemsJSON, _ := json.Marshal(inv.LineItems)
	
	_, err = tx.ExecContext(ctx, `
		INSERT INTO invoices (
			id, merchant_id, customer_id, subscription_id, status, line_items, subtotal, tax, total, 
			currency, number, sequence, hosted_url, pdf_url, due_at, paid_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`,
		inv.ID, inv.MerchantID, inv.CustomerID, inv.SubscriptionID, inv.Status, lineItemsJSON,
		inv.Subtotal, inv.Tax, inv.Total, inv.Currency, inv.Number, sequence, inv.HostedURL, inv.PDFURL,
		inv.DueAt, inv.PaidAt, inv.CreatedAt, inv.UpdatedAt,
	)
	if err != nil {
		return "", err
	}
	
	if err := tx.Commit(); err != nil {
		return "", err
	}
	
	// Send invoice email
	go s.emailSender.SendInvoice(context.Background(), inv)
	
	return inv.ID, nil
}

// HTTPHandler returns invoice API handler
func HTTPHandler(service *Service) http.Handler {
	r := chi.NewRouter()

	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		var body struct {
			CustomerID string          `json:"customer_id"`
			LineItems  []InvoiceLineItem `json:"line_items"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		merchantID := ctx.Value("merchant_id").(string)
		inv, err := service.Create(ctx, merchantID, body.CustomerID, body.LineItems)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(inv)
	})

	r.Post("/{id}/finalize", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		id := chi.URLParam(r, "id")

		inv, err := service.Finalize(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(inv)
	})

	r.Post("/{id}/credit_notes", func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		invoiceID := chi.URLParam(r, "id")

		var body struct {
			Amount decimal.Decimal `json:"amount"`
			Reason string          `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		creditNote, err := service.CreateCreditNote(ctx, invoiceID, body.Amount, body.Reason)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(creditNote)
	})

	return r
}

// CreditNote represents a credit note for partial or full invoice reversal
type CreditNote struct {
	ID              string            `json:"id"`
	InvoiceID       string            `json:"invoice_id"`
	MerchantID      string            `json:"merchant_id"`
	Amount          decimal.Decimal   `json:"amount"`
	Reason          string            `json:"reason"`
	Status          string            `json:"status"`
	Type            string            `json:"type"` // full, partial
	CreatedAt       int64             `json:"created_at"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// CreateCreditNote creates a credit note against an invoice
func (s *Service) CreateCreditNote(ctx context.Context, invoiceID string, amount decimal.Decimal, reason string) (*CreditNote, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var inv Invoice
	err = tx.QueryRowContext(ctx, `
		SELECT id, merchant_id, total, status FROM invoices WHERE id = $1 FOR UPDATE`,
		invoiceID,
	).Scan(&inv.ID, &inv.MerchantID, &inv.Total, &inv.Status)
	if err == sql.ErrNoRows {
		return nil, errors.New("invoice not found")
	}

	if inv.Status != InvoiceStatusPaid {
		return nil, errors.New("can only create credit notes against paid invoices")
	}

	if amount.GreaterThan(inv.Total) {
		return nil, errors.New("credit note amount cannot exceed invoice total")
	}

	noteType := "partial"
	if amount.Equal(inv.Total) {
		noteType = "full"
	}

	now := time.Now().UTC()
	creditNote := &CreditNote{
		ID:         "cn_" + uuid.NewString(),
		InvoiceID:  invoiceID,
		MerchantID: inv.MerchantID,
		Amount:     amount,
		Reason:     reason,
		Status:     "issued",
		Type:       noteType,
		CreatedAt:  now.Unix(),
		Metadata:   make(map[string]string),
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO credit_notes (id, invoice_id, merchant_id, amount, reason, status, type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		creditNote.ID, creditNote.InvoiceID, creditNote.MerchantID, creditNote.Amount,
		creditNote.Reason, creditNote.Status, creditNote.Type, creditNote.CreatedAt,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return creditNote, nil
}

// generateInvoiceNumber generates standard invoice number format
func generateInvoiceNumber(sequence int) string {
	return fmt.Sprintf("INV-%06d", sequence)
}
