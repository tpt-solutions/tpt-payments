// tpt-payments/backend/internal/disputes/webhooks.go
package disputes

import (
	"context"
	"fmt"
	"time"
)

// DisputeWebhookHandler handles dispute-related webhooks
type DisputeWebhookHandler struct {
	webhookService DisputeWebhookService
}

// DisputeWebhookService interface for sending webhooks
type DisputeWebhookService interface {
	SendEvent(ctx context.Context, merchantID, eventType string, data interface{}) error
}

// NewDisputeWebhookHandler creates a new dispute webhook handler
func NewDisputeWebhookHandler(service DisputeWebhookService) *DisputeWebhookHandler {
	return &DisputeWebhookHandler{webhookService: service}
}

// DisputeWebhookData represents the data sent with dispute webhooks
type DisputeWebhookData struct {
	ID                string                 `json:"id"`
	Object            string                 `json:"object"` // dispute
	PaymentIntentID   string                 `json:"payment_intent"`
	Amount            int64                  `json:"amount"`
	Currency          string                 `json:"currency"`
	Status            string                 `json:"status"`
	Reason            string                 `json:"reason"`
	EvidenceDueBy     time.Time              `json:"evidence_due_by"`
	EvidenceSubmitted bool                   `json:"evidence_submitted"`
	Resolution        string                 `json:"resolution,omitempty"`
	NetworkReasonCode string                 `json:"network_reason_code"`
	CreatedAt         time.Time              `json:"created"`
	UpdatedAt         time.Time              `json:"updated"`
	Evidence          *DisputeEvidenceData `json:"evidence,omitempty"`
}

// DisputeEvidenceData represents evidence in webhook payload
type DisputeEvidenceData struct {
	ProductDescription string `json:"product_description,omitempty"`
	CustomerName     string `json:"customer_name,omitempty"`
	CustomerEmail    string `json:"customer_email,omitempty"`
	HasReceipt       bool   `json:"has_receipt"`
	HasShipping      bool   `json:"has_shipping_document"`
	HasSignature     bool   `json:"has_customer_signature"`
	HasServiceDoc    bool   `json:"has_service_documentation"`
	HasAccessLog     bool   `json:"has_access_activity_log"`
	HasRefundPolicy  bool   `json:"has_refund_policy"`
	HasCancelPolicy  bool   `json:"has_cancel_policy"`
	HasCommunication bool   `json:"has_customer_communication"`
}

// OnDisputeCreated fires when a new dispute is created
func (h *DisputeWebhookHandler) OnDisputeCreated(ctx context.Context, dispute *Dispute) error {
	data := h.buildWebhookData(dispute)
	return h.webhookService.SendEvent(ctx, dispute.MerchantID, "chargeback.created", data)
}

// OnDisputeUpdated fires when a dispute is updated
func (h *DisputeWebhookHandler) OnDisputeUpdated(ctx context.Context, dispute *Dispute) error {
	data := h.buildWebhookData(dispute)
	return h.webhookService.SendEvent(ctx, dispute.MerchantID, "chargeback.updated", data)
}

// OnDisputeEvidenceSubmitted fires when evidence is submitted
func (h *DisputeWebhookHandler) OnDisputeEvidenceSubmitted(ctx context.Context, dispute *Dispute) error {
	data := h.buildWebhookData(dispute)
	return h.webhookService.SendEvent(ctx, dispute.MerchantID, "chargeback.evidence_submitted", data)
}

// OnDisputeWarning fires when a dispute warning is issued
func (h *DisputeWebhookHandler) OnDisputeWarning(ctx context.Context, dispute *Dispute) error {
	data := h.buildWebhookData(dispute)
	return h.webhookService.SendEvent(ctx, dispute.MerchantID, "chargeback.warning.issued", data)
}

// OnDisputeClosed fires when a dispute is closed (won or lost)
func (h *DisputeWebhookHandler) OnDisputeClosed(ctx context.Context, dispute *Dispute) error {
	data := h.buildWebhookData(dispute)
	eventType := "chargeback.closed"
	return h.webhookService.SendEvent(ctx, dispute.MerchantID, eventType, data)
}

// buildWebhookData builds the webhook payload from a dispute
func (h *DisputeWebhookHandler) buildWebhookData(dispute *Dispute) DisputeWebhookData {
	data := DisputeWebhookData{
		ID:                dispute.ID,
		Object:            "dispute",
		PaymentIntentID:   dispute.PaymentIntentID,
		Amount:            dispute.Amount,
		Currency:          dispute.Currency,
		Status:            dispute.Status,
		Reason:            dispute.Reason,
		EvidenceDueBy:     dispute.EvidenceDueBy,
		EvidenceSubmitted: dispute.EvidenceSubmitted,
		Resolution:        dispute.Resolution,
		NetworkReasonCode: dispute.NetworkReasonCode,
		CreatedAt:         dispute.CreatedAt,
		UpdatedAt:         dispute.UpdatedAt,
	}

	// Include evidence summary if available
	if dispute.Evidence != nil {
		data.Evidence = &DisputeEvidenceData{
			ProductDescription: dispute.Evidence.ProductDescription,
			CustomerName:     dispute.Evidence.CustomerName,
			CustomerEmail:    dispute.Evidence.CustomerEmail,
			HasReceipt:       dispute.Evidence.Receipt != nil,
			HasShipping:      dispute.Evidence.ShippingDocument != nil,
			HasSignature:     dispute.Evidence.CustomerSignature != nil,
			HasServiceDoc:    dispute.Evidence.ServiceDocumentation != nil,
			HasAccessLog:     dispute.Evidence.AccessActivityLog != nil,
			HasRefundPolicy:  dispute.Evidence.RefundPolicy != nil,
			HasCancelPolicy:  dispute.Evidence.CancelPolicy != nil,
			HasCommunication: dispute.Evidence.CustomerCommunication != nil,
		}
	}

	return data
}

// DisputeWarningHandler handles dispute warnings (pre-dispute notifications)
type DisputeWarningHandler struct {
	service *DisputeService
}

// NewDisputeWarningHandler creates a new dispute warning handler
func NewDisputeWarningHandler(service *DisputeService) *DisputeWarningHandler {
	return &DisputeWarningHandler{service: service}
}

// ProcessWarning processes a dispute warning from the network
func (h *DisputeWarningHandler) ProcessWarning(ctx context.Context, paymentIntentID, merchantID string, amount int64, currency, reason, networkReasonCode string) (*Dispute, error) {
	// Create dispute in warning state
	dispute, err := h.service.CreateDispute(ctx, paymentIntentID, merchantID, amount, currency, reason, networkReasonCode, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create dispute from warning: %w", err)
	}

	// Fire warning webhook
	handler := &DisputeWebhookHandler{webhookService: h.service}
	if err := handler.OnDisputeWarning(ctx, dispute); err != nil {
		// Log but don't fail - warning was created
		fmt.Printf("Failed to send warning webhook: %v\n", err)
	}

	return dispute, nil
}

// DisputeNotificationSender sends email notifications to merchants
type DisputeNotificationSender struct {
	emailService DisputeEmailService
}

// DisputeEmailService interface for sending emails
type DisputeEmailService interface {
	SendDisputeNotification(ctx context.Context, merchantID, subject string, data interface{}) error
	SendEvidenceReminder(ctx context.Context, merchantID, disputeID string, dueBy time.Time) error
	SendDisputeResolved(ctx context.Context, merchantID, disputeID, resolution string) error
}

// NewDisputeNotificationSender creates a new notification sender
func NewDisputeNotificationSender(emailService DisputeEmailService) *DisputeNotificationSender {
	return &DisputeNotificationSender{emailService: emailService}
}

// SendDisputeCreatedNotification sends notification for new dispute
func (s *DisputeNotificationSender) SendDisputeCreatedNotification(ctx context.Context, dispute *Dispute) error {
	subject := fmt.Sprintf("Action Required: Dispute for $%.2f", float64(dispute.Amount)/100)
	return s.emailService.SendDisputeNotification(ctx, dispute.MerchantID, subject, dispute)
}

// SendEvidenceReminder sends reminder before evidence deadline
func (s *DisputeNotificationSender) SendEvidenceReminder(ctx context.Context, dispute *Dispute) error {
	return s.emailService.SendEvidenceReminder(ctx, dispute.MerchantID, dispute.ID, dispute.EvidenceDueBy)
}

// SendDisputeResolvedNotification sends notification when dispute is resolved
func (s *DisputeNotificationSender) SendDisputeResolvedNotification(ctx context.Context, dispute *Dispute) error {
	subject := fmt.Sprintf("Dispute %s: %s", dispute.ID, dispute.Resolution)
	return s.emailService.SendDisputeResolved(ctx, dispute.MerchantID, dispute.ID, dispute.Resolution)
}

// WebhookEventType returns the webhook event type for a dispute status
func WebhookEventType(status string) string {
	switch status {
	case "warning_needs_response":
		return "chargeback.warning.issued"
	case "needs_response":
		return "chargeback.created"
	case "under_review":
		return "chargeback.updated"
	case "won":
		return "chargeback.closed"
	case "lost":
		return "chargeback.closed"
	default:
		return "chargeback.updated"
	}
}