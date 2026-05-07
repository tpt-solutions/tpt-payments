// tpt-payments/backend/internal/disputes/dispute.go
package disputes

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Dispute represents a chargeback/dispute
type Dispute struct {
	ID                string                 `json:"id"`
	PaymentIntentID   string                 `json:"payment_intent_id"`
	MerchantID        string                 `json:"merchant_id"`
	Amount            int64                  `json:"amount"`
	Currency          string                 `json:"currency"`
	Reason            string                 `json:"reason"`            // fraud, duplicate, product_not_received, etc.
	Status            string                 `json:"status"`            // warning_needs_response, needs_response, under_review, won, lost
	EvidenceDueBy     time.Time              `json:"evidence_due_by"`
	EvidenceSubmitted bool                   `json:"evidence_submitted"`
	Evidence          *DisputeEvidence       `json:"evidence,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	ResolvedAt        *time.Time             `json:"resolved_at,omitempty"`
	Resolution        string                 `json:"resolution,omitempty"`    // won, lost
	NetworkReasonCode string                 `json:"network_reason_code"` // Visa/MC reason code
	NetworkDisputeID string                 `json:"network_dispute_id"`  // ID from card network
	Metadata          map[string]string      `json:"metadata,omitempty"`
	IsWarning         bool                   `json:"is_warning"`
}

// DisputeEvidence represents evidence for a dispute
type DisputeEvidence struct {
	ID               string                 `json:"id"`
	DisputeID        string                 `json:"dispute_id"`
	ProductDescription string                 `json:"product_description,omitempty"`
	CustomerName     string                 `json:"customer_name,omitempty"`
	CustomerEmail    string                 `json:"customer_email,omitempty"`
	CustomerIP       string                 `json:"customer_ip,omitempty"`
	CustomerSignature *EvidenceFile          `json:"customer_signature,omitempty"`
	ShippingDocument *EvidenceFile          `json:"shipping_document,omitempty"`
	Receipt          *EvidenceFile          `json:"receipt,omitempty"`
	DuplicateCharge  *DuplicateChargeEvidence `json:"duplicate_charge,omitempty"`
	ServiceDocumentation *EvidenceFile      `json:"service_documentation,omitempty"`
	AccessActivityLog  *EvidenceFile        `json:"access_activity_log,omitempty"`
	RefundPolicy     *EvidenceFile          `json:"refund_policy,omitempty"`
	CancelPolicy     *EvidenceFile          `json:"cancel_policy,omitempty"`
	CustomerCommunication *EvidenceFile      `json:"customer_communication,omitempty"`
	UncategorizedFile   *EvidenceFile      `json:"uncategorized_file,omitempty"`
	UncategorizedText   string             `json:"uncategorized_text,omitempty"`
	SubmittedAt        *time.Time           `json:"submitted_at,omitempty"`
	CreatedAt          time.Time            `json:"created_at"`
	UpdatedAt          time.Time            `json:"updated_at"`
}

// EvidenceFile represents an uploaded file for evidence
type EvidenceFile struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	FileURL   string `json:"file_url"`
	MimeType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// DuplicateChargeEvidence represents evidence for duplicate charge disputes
type DuplicateChargeEvidence struct {
	ID               string `json:"id"`
	DisputeID        string `json:"dispute_id"`
	OriginalPaymentIntentID string `json:"original_payment_intent_id"`
	Explanation      string `json:"explanation,omitempty"`
}

// DisputeStateMachine handles dispute state transitions
type DisputeStateMachine struct{}

// ValidTransitions defines allowed state transitions
var ValidTransitions = map[string][]string{
	"warning_needs_response": {"needs_response", "won"}, // warning can go to needs_response or won (if warning resolved)
	"needs_response":        {"under_review", "lost", "won"},
	"under_review":         {"won", "lost"},
	"won":                  {}, // terminal state
	"lost":                 {}, // terminal state
}

// IsValidTransition checks if a state transition is valid
func (sm *DisputeStateMachine) IsValidTransition(from, to string) bool {
	allowed, exists := ValidTransitions[from]
	if !exists {
		return false
	}
	for _, state := range allowed {
		if state == to {
			return true
		}
	}
	return false
}

// DisputeStore interface for persistence
type DisputeStore interface {
	CreateDispute(ctx context.Context, dispute *Dispute) error
	GetDispute(ctx context.Context, id string) (*Dispute, error)
	UpdateDispute(ctx context.Context, dispute *Dispute) error
	ListDisputes(ctx context.Context, filters DisputeFilters) ([]*Dispute, error)
	SaveEvidence(ctx context.Context, evidence *DisputeEvidence) error
	UpdateEvidence(ctx context.Context, evidence *DisputeEvidence) error
	SaveEvidenceFile(ctx context.Context, file *EvidenceFile) error
}

// DisputeFilters for querying disputes
type DisputeFilters struct {
	MerchantID string
	Status     string
	PaymentIntentID string
	StartDate  *time.Time
	EndDate    *time.Time
	Limit      int
	Offset     int
}

// DisputeService handles dispute business logic
type DisputeService struct {
	store DisputeStore
}

// NewDisputeService creates a new dispute service
func NewDisputeService(store DisputeStore) *DisputeService {
	return &DisputeService{store: store}
}

// CreateDispute creates a new dispute (usually from network webhook)
func (s *DisputeService) CreateDispute(ctx context.Context, paymentIntentID, merchantID string, amount int64, currency, reason, networkReasonCode, networkDisputeID string) (*Dispute, error) {
	now := time.Now().UTC()
	dispute := &Dispute{
		ID:                uuid.New().String(),
		PaymentIntentID:   paymentIntentID,
		MerchantID:        merchantID,
		Amount:            amount,
		Currency:          currency,
		Reason:            reason,
		Status:            "warning_needs_response", // Start with warning
		EvidenceDueBy:     now.Add(7 * 24 * time.Hour), // 7 days to respond
		EvidenceSubmitted: false,
		CreatedAt:         now,
		UpdatedAt:         now,
		NetworkReasonCode: networkReasonCode,
		NetworkDisputeID: networkDisputeID,
		IsWarning:         true, // Initial warning
	}

	if err := s.store.CreateDispute(ctx, dispute); err != nil {
		return nil, fmt.Errorf("failed to create dispute: %w", err)
	}

	// TODO: Fire webhook: chargeback.created or chargeback.warning
	// TODO: Send notification to merchant

	return dispute, nil
}

// UpdateDisputeStatus updates the dispute status with validation
func (s *DisputeService) UpdateDisputeStatus(ctx context.Context, disputeID, newStatus string) error {
	dispute, err := s.store.GetDispute(ctx, disputeID)
	if err != nil {
		return fmt.Errorf("failed to get dispute: %w", err)
	}

	sm := &DisputeStateMachine{}
	if !sm.IsValidTransition(dispute.Status, newStatus) {
		return fmt.Errorf("invalid status transition from %s to %s", dispute.Status, newStatus)
	}

	dispute.Status = newStatus
	dispute.UpdatedAt = time.Now().UTC()

	if newStatus == "won" || newStatus == "lost" {
		now := time.Now().UTC()
		dispute.ResolvedAt = &now
		dispute.Resolution = newStatus
		dispute.IsWarning = false
	}

	if err := s.store.UpdateDispute(ctx, dispute); err != nil {
		return fmt.Errorf("failed to update dispute: %w", err)
	}

	// TODO: Fire webhook: chargeback.updated, chargeback.closed (if won/lost)
	return nil
}

// SubmitEvidence submits evidence for a dispute
func (s *DisputeService) SubmitEvidence(ctx context.Context, disputeID string, evidence *DisputeEvidence) error {
	dispute, err := s.store.GetDispute(ctx, disputeID)
	if err != nil {
		return fmt.Errorf("failed to get dispute: %w", err)
	}

	// Check if evidence is due
	if time.Now().After(dispute.EvidenceDueBy) {
		return fmt.Errorf("evidence submission deadline has passed")
	}

	// Check if dispute is in a state that accepts evidence
	if dispute.Status != "needs_response" && dispute.Status != "under_review" {
		return fmt.Errorf("cannot submit evidence in status: %s", dispute.Status)
	}

	now := time.Now().UTC()
	evidence.ID = uuid.New().String()
	evidence.DisputeID = disputeID
	evidence.CreatedAt = now
	evidence.UpdatedAt = now
	evidence.SubmittedAt = &now

	if err := s.store.SaveEvidence(ctx, evidence); err != nil {
		return fmt.Errorf("failed to save evidence: %w", err)
	}

	// Update dispute
	dispute.EvidenceSubmitted = true
	dispute.UpdatedAt = now
	if dispute.Status == "needs_response" {
		dispute.Status = "under_review"
	}
	if err := s.store.UpdateDispute(ctx, dispute); err != nil {
		return fmt.Errorf("failed to update dispute: %w", err)
	}

	// TODO: Fire webhook: chargeback.evidence_submitted
	return nil
}

// AutoPopulateEvidence automatically populates evidence from transaction metadata
func (s *DisputeService) AutoPopulateEvidence(ctx context.Context, disputeID string) (*DisputeEvidence, error) {
	dispute, err := s.store.GetDispute(ctx, disputeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dispute: %w", err)
	}

	// In production, fetch payment intent details, shipping info, customer info, etc.
	// For now, create a skeleton
	evidence := &DisputeEvidence{
		DisputeID:        disputeID,
		ProductDescription: "Auto-populated from transaction metadata",
		CustomerName:     "Customer Name", // Fetch from payment intent
		CustomerEmail:    "customer@example.com",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	// TODO: Actually fetch from payment intent and related objects
	// - Payment intent metadata
	// - Shipping address
	// - Customer details
	// - Order details
	// - Previous communication

	return evidence, nil
}

// GetDisputeSummary returns a summary of disputes for reporting
func (s *DisputeService) GetDisputeSummary(ctx context.Context, merchantID string) (map[string]interface{}, error) {
	filters := DisputeFilters{
		MerchantID: merchantID,
		Limit:     1000, // Get all for summary
	}
	disputes, err := s.store.ListDisputes(ctx, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to list disputes: %w", err)
	}

	summary := map[string]interface{}{
		"total_disputes": len(disputes),
		"by_status":       make(map[string]int),
		"total_amount":    int64(0),
		"won_count":        0,
		"lost_count":       0,
		"warning_count":    0,
	}

	byStatus := summary["by_status"].(map[string]int)
	for _, d := range disputes {
		byStatus[d.Status]++
		summary["total_amount"] = summary["total_amount"].(int64) + d.Amount
		switch d.Status {
		case "won":
			summary["won_count"] = summary["won_count"].(int) + 1
		case "lost":
			summary["lost_count"] = summary["lost_count"].(int) + 1
		case "warning_needs_response":
			summary["warning_count"] = summary["warning_count"].(int) + 1
		}
	}

	return summary, nil
}