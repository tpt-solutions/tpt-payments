// tpt-payments/backend/identity/kyc.go
package identity

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// KYCStatus represents the current KYC verification status of a merchant
type KYCStatus string

const (
	KYCStatusPending      KYCStatus = "pending"
	KYCStatusUnderReview  KYCStatus = "under_review"
	KYCStatusApproved     KYCStatus = "approved"
	KYCStatusRejected     KYCStatus = "rejected"
	KYCStatusSuspended    KYCStatus = "suspended"
)

// Merchant represents a merchant entity in the system
type Merchant struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Status     string    `json:"status"`
	KYCStatus  KYCStatus `json:"kyc_status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// KYCProvider defines the interface for KYC verification providers
type KYCProvider interface {
	// InitiateVerification starts a new KYC verification for a merchant
	InitiateVerification(ctx context.Context, merchant *Merchant) (string, error)
	
	// GetVerificationStatus retrieves the current status of a verification
	GetVerificationStatus(ctx context.Context, verificationID string) (*KYCVerification, error)
	
	// CancelVerification cancels an ongoing verification
	CancelVerification(ctx context.Context, verificationID string) error
}

// KYCVerification represents a KYC verification attempt
type KYCVerification struct {
	ID             string    `json:"id"`
	MerchantID     string    `json:"merchant_id"`
	Provider       string    `json:"provider"`
	ProviderRef    string    `json:"provider_ref"`
	Status         KYCStatus `json:"status"`
	RejectReason   string    `json:"reject_reason,omitempty"`
	VerificationURL string   `json:"verification_url,omitempty"`
	ExpiresAt      time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// KYCWebhookEvent represents an incoming webhook event from a KYC provider
type KYCWebhookEvent struct {
	Provider      string            `json:"provider"`
	EventType     string            `json:"event_type"`
	VerificationID string           `json:"verification_id"`
	Status        KYCStatus         `json:"status"`
	Reason        string            `json:"reason,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	RawPayload    []byte            `json:"-"`
}

// NewMerchant creates a new merchant entity
func NewMerchant(name string) *Merchant {
	return &Merchant{
		ID:         uuid.NewString(),
		Name:       name,
		Status:     "pending",
		KYCStatus:  KYCStatusPending,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
		Metadata:   make(map[string]string),
	}
}

// UpdateKYCStatus updates the merchant's KYC status and manages state transitions
func (m *Merchant) UpdateKYCStatus(newStatus KYCStatus, reason ...string) {
	oldStatus := m.KYCStatus
	m.KYCStatus = newStatus
	m.UpdatedAt = time.Now().UTC()

	// Update merchant status based on KYC status
	switch newStatus {
	case KYCStatusApproved:
		m.Status = "active"
	case KYCStatusRejected, KYCStatusSuspended:
		m.Status = "suspended"
	}

	if len(reason) > 0 {
		m.Metadata["kyc_reject_reason"] = reason[0]
	}
}

// KYCWebhookHandler processes incoming KYC provider webhooks
type KYCWebhookHandler struct {
	providers map[string]KYCProvider
}

// NewKYCWebhookHandler creates a new KYC webhook handler
func NewKYCWebhookHandler() *KYCWebhookHandler {
	return &KYCWebhookHandler{
		providers: make(map[string]KYCProvider),
	}
}

// RegisterProvider registers a KYC provider with the handler
func (h *KYCWebhookHandler) RegisterProvider(name string, provider KYCProvider) {
	h.providers[name] = provider
}

// ProcessEvent processes an incoming KYC webhook event
func (h *KYCWebhookHandler) ProcessEvent(ctx context.Context, event *KYCWebhookEvent) error {
	// Validate event
	if event.VerificationID == "" {
		return nil
	}

	// Update merchant status based on event
	// This would typically involve:
	// 1. Loading merchant from database
	// 2. Updating KYC status
	// 3. Triggering internal events/webhooks
	// 4. Logging the status change

	return nil
}