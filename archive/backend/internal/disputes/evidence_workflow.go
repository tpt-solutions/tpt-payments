// tpt-payments/backend/internal/disputes/evidence_workflow.go
package disputes

import (
	"context"
	"fmt"
	"mime/multipart"
	"time"

	"github.com/google/uuid"
)

// EvidenceSubmitter handles evidence submission workflow
type EvidenceSubmitter struct {
	store      DisputeStore
	uploadLimit int64 // Max file size in bytes
	allowedTypes []string
}

// NewEvidenceSubmitter creates a new evidence submitter
func NewEvidenceSubmitter(store DisputeStore) *EvidenceSubmitter {
	return &EvidenceSubmitter{
		store:       store,
		uploadLimit: 10 * 1024 * 1024, // 10MB default
		allowedTypes: []string{
			"image/jpeg",
			"image/png",
			"application/pdf",
			"text/plain",
			"text/csv",
		},
	}
}

// SubmitEvidenceFromForm handles evidence submission from multipart form
func (es *EvidenceSubmitter) SubmitEvidenceFromForm(ctx context.Context, disputeID string, form *multipart.Form) (*DisputeEvidence, error) {
	dispute, err := es.store.GetDispute(ctx, disputeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get dispute: %w", err)
	}

	// Check if dispute accepts evidence
	if dispute.Status != "needs_response" && dispute.Status != "under_review" {
		return nil, fmt.Errorf("dispute in status %s does not accept evidence", dispute.Status)
	}

	// Check deadline
	if time.Now().After(dispute.EvidenceDueBy) {
		return nil, fmt.Errorf("evidence submission deadline has passed")
	}

	evidence := &DisputeEvidence{
		DisputeID:        disputeID,
		ProductDescription: es.getFormValue(form, "product_description"),
		CustomerName:     es.getFormValue(form, "customer_name"),
		CustomerEmail:    es.getFormValue(form, "customer_email"),
		CustomerIP:       es.getFormValue(form, "customer_ip"),
		UncategorizedText: es.getFormValue(form, "uncategorized_text"),
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	// Process file uploads
	if files, ok := form.File["receipt"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process receipt: %w", err)
		}
		evidence.Receipt = file
	}

	if files, ok := form.File["shipping_document"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process shipping document: %w", err)
		}
		evidence.ShippingDocument = file
	}

	if files, ok := form.File["customer_signature"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process customer signature: %w", err)
		}
		evidence.CustomerSignature = file
	}

	if files, ok := form.File["service_documentation"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process service documentation: %w", err)
		}
		evidence.ServiceDocumentation = file
	}

	if files, ok := form.File["access_activity_log"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process access activity log: %w", err)
		}
		evidence.AccessActivityLog = file
	}

	if files, ok := form.File["refund_policy"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process refund policy: %w", err)
		}
		evidence.RefundPolicy = file
	}

	if files, ok := form.File["cancel_policy"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process cancel policy: %w", err)
		}
		evidence.CancelPolicy = file
	}

	if files, ok := form.File["customer_communication"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process customer communication: %w", err)
		}
		evidence.CustomerCommunication = file
	}

	if files, ok := form.File["uncategorized_file"]; ok && len(files) > 0 {
		file, err := es.processUploadedFile(ctx, files[0])
		if err != nil {
			return nil, fmt.Errorf("failed to process uncategorized file: %w", err)
		}
		evidence.UncategorizedFile = file
	}

	// Save evidence
	if err := es.store.SaveEvidence(ctx, evidence); err != nil {
		return nil, fmt.Errorf("failed to save evidence: %w", err)
	}

	// Update dispute
	now := time.Now().UTC()
	dispute.Evidence = evidence
	dispute.EvidenceSubmitted = true
	dispute.EvidenceDueBy = now
	if dispute.Status == "needs_response" {
		dispute.Status = "under_review"
	}
	dispute.UpdatedAt = now

	if err := es.store.UpdateDispute(ctx, dispute); err != nil {
		return nil, fmt.Errorf("failed to update dispute: %w", err)
	}

	// TODO: Fire webhook: chargeback.evidence_submitted
	// TODO: Notify merchant

	return evidence, nil
}

// processUploadedFile processes an uploaded file
func (es *EvidenceSubmitter) processUploadedFile(ctx context.Context, file multipart.File) (*EvidenceFile, error) {
	defer file.Close()

	// Check file size
	// In production, you'd read the file and upload to S3 or similar
	// For now, create a placeholder
	evidenceFile := &EvidenceFile{
		ID:        uuid.New().String(),
		Filename:  file.Filename,
		MimeType:  file.Header.Get("Content-Type"),
		Size:      file.Size, // This is approximate
		UploadedAt: time.Now().UTC(),
	}

	// In production:
	// 1. Validate file type
	// 2. Scan for malware
	// 3. Upload to S3/storage
	// 4. Set FileURL to the storage URL

	evidenceFile.FileURL = fmt.Sprintf("/api/v1/disputes/evidence/%s/download", evidenceFile.ID)

	if err := es.store.SaveEvidenceFile(ctx, evidenceFile); err != nil {
		return nil, fmt.Errorf("failed to save evidence file: %w", err)
	}

	return evidenceFile, nil
}

// getFormValue safely gets a form value
func (es *EvidenceSubmitter) getFormValue(form *multipart.Form, key string) string {
	if values, ok := form.Value[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

// ValidateEvidence checks if evidence is complete and valid
func (es *EvidenceSubmitter) ValidateEvidence(evidence *DisputeEvidence) []string {
	errors := []string{}

	// Check required fields
	if evidence.ProductDescription == "" {
		errors = append(errors, "product_description is required")
	}

	// Check if at least one piece of evidence is provided
	hasEvidence := evidence.Receipt != nil ||
		evidence.ShippingDocument != nil ||
		evidence.CustomerSignature != nil ||
		evidence.ServiceDocumentation != nil ||
		evidence.AccessActivityLog != nil ||
		evidence.RefundPolicy != nil ||
		evidence.CancelPolicy != nil ||
		evidence.CustomerCommunication != nil ||
		evidence.UncategorizedFile != nil ||
		evidence.UncategorizedText != ""

	if !hasEvidence {
		errors = append(errors, "at least one piece of evidence must be provided")
	}

	return errors
}

// BuildStructuredRebuttal creates a structured rebuttal document
func (es *EvidenceSubmitter) BuildStructuredRebuttal(dispute *Dispute, evidence *DisputeEvidence) map[string]interface{} {
	rebuttal := map[string]interface{}{
		"dispute_id":        dispute.ID,
		"network_dispute_id": dispute.NetworkDisputeID,
		"reason":            dispute.Reason,
		"amount":            dispute.Amount,
		"currency":          dispute.Currency,
		"created_at":        dispute.CreatedAt,
		"evidence_due_by":   dispute.EvidenceDueBy,
	}

	// Add evidence summary
	if evidence != nil {
		evidenceSummary := map[string]interface{}{
			"product_description": evidence.ProductDescription,
			"customer_name":     evidence.CustomerName,
			"customer_email":    evidence.CustomerEmail,
			"has_receipt":       evidence.Receipt != nil,
			"has_shipping":      evidence.ShippingDocument != nil,
			"has_signature":     evidence.CustomerSignature != nil,
			"has_service_doc":   evidence.ServiceDocumentation != nil,
			"uncategorized_text": evidence.UncategorizedText,
		}
		rebuttal["evidence"] = evidenceSummary
	}

	// Add recommended actions
	rebuttal["recommended_actions"] = []string{
		"Review evidence for completeness",
		"Ensure all dates match transaction records",
		"Verify customer communication logs are included",
		"Check that refund policy was clearly communicated",
	}

	return rebuttal
}

// GetEvidenceChecklist returns a checklist of recommended evidence for a dispute reason
func (es *EvidenceSubmitter) GetEvidenceChecklist(reason string) map[string]bool {
	checklist := map[string]bool{
		"receipt":                false,
		"shipping_document":      false,
		"customer_signature":      false,
		"service_documentation":   false,
		"access_activity_log":     false,
		"refund_policy":          false,
		"cancel_policy":          false,
		"customer_communication": false,
	}

	switch reason {
	case "fraudulent":
		checklist["customer_signature"] = true
		checklist["access_activity_log"] = true
	case "duplicate":
		checklist["receipt"] = true
		checklist["customer_communication"] = true
	case "product_not_received":
		checklist["shipping_document"] = true
		checklist["service_documentation"] = true
	case "product_unacceptable":
		checklist["service_documentation"] = true
		checklist["customer_communication"] = true
		checklist["refund_policy"] = true
	case "credit_not_processed":
		checklist["receipt"] = true
		checklist["refund_policy"] = true
	case "general":
		// All evidence types recommended
		for k := range checklist {
			checklist[k] = true
		}
	}

	return checklist
}