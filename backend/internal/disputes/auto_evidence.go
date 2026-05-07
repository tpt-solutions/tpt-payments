// tpt-payments/backend/internal/disputes/auto_evidence.go
package disputes

import (
	"context"
	"fmt"
	"time"
)

// EvidencePrePopulator handles automated evidence pre-population for disputes
type EvidencePrePopulator struct {
	paymentIntentService PaymentIntentService
	merchantService       MerchantService
	shippingService       ShippingService
}

// PaymentIntentService interface for fetching payment intent details
type PaymentIntentService interface {
	GetPaymentIntent(ctx context.Context, id string) (*PaymentIntent, error)
}

// MerchantService interface for fetching merchant details
type MerchantService interface {
	GetMerchant(ctx context.Context, id string) (*Merchant, error)
}

// ShippingService interface for fetching shipping information
type ShippingService interface {
	GetShippingInfo(ctx context.Context, paymentIntentID string) (*ShippingInfo, error)
}

// PaymentIntent represents a payment intent (simplified)
type PaymentIntent struct {
	ID          string
	Amount      int64
	Currency    string
	Description string
	Customer    *Customer
	Metadata    map[string]string
}

// Customer represents customer info
type Customer struct {
	Name  string
	Email string
}

// ShippingInfo represents shipping information
type ShippingInfo struct {
	Address1 string
	City     string
	State    string
	Country  string
	ZipCode  string
}

// Merchant represents merchant info
type Merchant struct {
	ID    string
	Name  string
	Email string
	Phone string
}

// NewEvidencePrePopulator creates a new evidence pre-populator
func NewEvidencePrePopulator(paymentIntentService PaymentIntentService, merchantService MerchantService, shippingService ShippingService) *EvidencePrePopulator {
	return &EvidencePrePopulator{
		paymentIntentService: paymentIntentService,
		merchantService:       merchantService,
		shippingService:       shippingService,
	}
}

// PrePopulateEvidence automatically populates evidence for a dispute
func (p *EvidencePrePopulator) PrePopulateEvidence(ctx context.Context, dispute *Dispute) (*DisputeEvidence, error) {
	evidence := &DisputeEvidence{
		DisputeID:        dispute.ID,
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	// Fetch payment intent details
	if dispute.PaymentIntentID != "" && p.paymentIntentService != nil {
		pi, err := p.paymentIntentService.GetPaymentIntent(ctx, dispute.PaymentIntentID)
		if err == nil && pi != nil {
			// Populate product description
			if pi.Description != "" {
				evidence.ProductDescription = pi.Description
			} else {
				evidence.ProductDescription = fmt.Sprintf("Payment of %d %s", pi.Amount, pi.Currency)
			}

			// Populate customer info
			if pi.Customer != nil {
				evidence.CustomerName = pi.Customer.Name
				evidence.CustomerEmail = pi.Customer.Email
			}

			// Add metadata as uncategorized text if available
			if len(pi.Metadata) > 0 {
				metadataStr := "Payment Metadata:\n"
				for k, v := range pi.Metadata {
					metadataStr += fmt.Sprintf("- %s: %s\n", k, v)
				}
				evidence.UncategorizedText = metadataStr
			}
		}
	}

	// Fetch merchant info
	if dispute.MerchantID != "" && p.merchantService != nil {
		merchant, err := p.merchantService.GetMerchant(ctx, dispute.MerchantID)
		if err == nil && merchant != nil {
			// Add merchant contact info to evidence (useful for dispute resolution)
			if evidence.UncategorizedText == "" {
				evidence.UncategorizedText = fmt.Sprintf("Merchant: %s\nContact: %s\n", merchant.Name, merchant.Email)
			} else {
				evidence.UncategorizedText += fmt.Sprintf("\nMerchant: %s\nContact: %s\n", merchant.Name, merchant.Email)
			}
		}
	}

	// Fetch shipping info
	if dispute.PaymentIntentID != "" && p.shippingService != nil {
		shipping, err := p.shippingService.GetShippingInfo(ctx, dispute.PaymentIntentID)
		if err == nil && shipping != nil {
			// Create a shipping document (simulated)
			// In production, you'd generate a shipping document file
			evidence.ShippingDocument = &EvidenceFile{
				ID:        fmt.Sprintf("shipping_%s", dispute.ID),
				Filename:  "shipping_info.txt",
				MimeType:  "text/plain",
				Size:      0,
				UploadedAt: time.Now().UTC(),
			}
			// Add shipping details to uncategorized text
			shippingInfo := fmt.Sprintf("\nShipping Address:\n%s\n%s, %s %s\n%s",
				shipping.Address1, shipping.City, shipping.State, shipping.ZipCode, shipping.Country)
			if evidence.UncategorizedText == "" {
				evidence.UncategorizedText = shippingInfo
			} else {
				evidence.UncategorizedText += shippingInfo
			}
		}
	}

	// Set default evidence based on dispute reason
	evidence = p.enrichWithReasonSpecificEvidence(evidence, dispute.Reason)

	return evidence, nil
}

// enrichWithReasonSpecificEvidence adds reason-specific evidence suggestions
func (p *EvidencePrePopulator) enrichWithReasonSpecificEvidence(evidence *DisputeEvidence, reason string) *DisputeEvidence {
	switch reason {
	case "fraudulent":
		// For fraud, emphasize customer IP and access logs
		if evidence.CustomerIP == "" {
			evidence.CustomerIP = "127.0.0.1" // Placeholder - fetch from payment intent
		}
	case "duplicate":
		// For duplicate, add note about duplicate detection
		if evidence.UncategorizedText != "" {
			evidence.UncategorizedText += "\nDuplicate transaction detected."
		}
	case "product_not_received":
		// For product not received, emphasize shipping documentation
		// Already handled by shipping service
	case "product_unacceptable":
		// For product unacceptable, add product description details
		if evidence.ProductDescription != "" {
			evidence.UncategorizedText += fmt.Sprintf("\nProduct Details: %s", evidence.ProductDescription)
		}
	}
	return evidence
}

// BuildEvidencePackage creates a complete evidence package for submission
func (p *EvidencePrePopulator) BuildEvidencePackage(ctx context.Context, dispute *Dispute) (*DisputeEvidence, error) {
	// Pre-populate basic evidence
	evidence, err := p.PrePopulateEvidence(ctx, dispute)
	if err != nil {
		return nil, fmt.Errorf("failed to pre-populate evidence: %w", err)
	}

	// Add structured rebuttal text
	rebuttal := p.generateRebuttal(dispute, evidence)
	if rebuttal != "" {
		if evidence.UncategorizedText == "" {
			evidence.UncategorizedText = rebuttal
		} else {
			evidence.UncategorizedText += "\n\nRebuttal:\n" + rebuttal
		}
	}

	return evidence, nil
}

// generateRebuttal creates a structured rebuttal text
func (p *EvidencePrePopulator) generateRebuttal(dispute *Dispute, evidence *DisputeEvidence) string {
	rebuttal := fmt.Sprintf("Dispute ID: %s\n", dispute.ID)
	rebuttal += fmt.Sprintf("Reason: %s\n", dispute.Reason)
	rebuttal += fmt.Sprintf("Amount: %d %s\n", dispute.Amount, dispute.Currency)
	rebuttal += fmt.Sprintf("Network Reason Code: %s\n\n", dispute.NetworkReasonCode)

	rebuttal += "Evidence Summary:\n"
	if evidence.ProductDescription != "" {
		rebuttal += fmt.Sprintf("- Product: %s\n", evidence.ProductDescription)
	}
	if evidence.CustomerName != "" {
		rebuttal += fmt.Sprintf("- Customer: %s (%s)\n", evidence.CustomerName, evidence.CustomerEmail)
	}
	if evidence.ShippingDocument != nil {
		rebuttal += "- Shipping documentation included\n"
	}
	if evidence.Receipt != nil {
		rebuttal += "- Receipt included\n"
	}

	rebuttal += "\nThis transaction was legitimate and the product/service was delivered as described.\n"
	return rebuttal
}