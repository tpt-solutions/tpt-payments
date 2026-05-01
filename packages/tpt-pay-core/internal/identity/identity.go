// Package identity provides the merchant entity and the KYC provider interface.
// This is the Phase 0 skeleton — full KYC integration is Phase 1.
package identity

import (
	"context"
	"time"
)

// MerchantStatus is the operational status of a merchant account.
type MerchantStatus string

const (
	MerchantStatusPending     MerchantStatus = "pending"
	MerchantStatusUnderReview MerchantStatus = "under_review"
	MerchantStatusActive      MerchantStatus = "active"
	MerchantStatusSuspended   MerchantStatus = "suspended"
)

// KYCStatus is the KYC verification status of a merchant.
type KYCStatus string

const (
	KYCStatusNotStarted KYCStatus = "not_started"
	KYCStatusPending    KYCStatus = "pending"
	KYCStatusApproved   KYCStatus = "approved"
	KYCStatusRejected   KYCStatus = "rejected"
)

// Merchant represents a business using TPT Pay.
type Merchant struct {
	ID        string
	Name      string
	Email     string
	Status    MerchantStatus
	KYCStatus KYCStatus
	Metadata  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// KYCSession is initiated when a merchant starts the KYC process.
type KYCSession struct {
	SessionID   string
	MerchantID  string
	RedirectURL string // URL to send the merchant to complete verification
	ExpiresAt   time.Time
}

// VerificationResult is returned by the KYC provider after verification completes.
type VerificationResult struct {
	Unique   bool   // provider asserts this is a unique individual
	PersonID string // provider's internal stable person identifier
	Status   string // "approved" | "declined" | "review"
}

// KYCProvider is the interface that all KYC integrations must implement.
// Concrete implementations: PersonaProvider, OnfidoProvider, SandboxKYCProvider.
type KYCProvider interface {
	// InitiateVerification starts a KYC session for the given merchant.
	// Returns a session with a redirect URL for the merchant to complete verification.
	InitiateVerification(ctx context.Context, merchantID string) (*KYCSession, error)

	// GetVerificationResult polls or retrieves the result of a KYC session.
	GetVerificationResult(ctx context.Context, sessionID string) (*VerificationResult, error)

	// HandleWebhook processes an inbound webhook from the KYC provider.
	// Returns the merchant ID and updated result.
	HandleWebhook(ctx context.Context, payload []byte) (merchantID string, result *VerificationResult, err error)
}
