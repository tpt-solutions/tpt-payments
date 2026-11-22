// tpt-payments/backend/connect/account.go
package connect

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// AccountStatus represents connected account lifecycle states
type AccountStatus string

const (
	AccountStatusPending      AccountStatus = "pending"
	AccountStatusRestricted   AccountStatus = "restricted"
	AccountStatusActive       AccountStatus = "active"
	AccountStatusSuspended    AccountStatus = "suspended"
)

// Capability represents available payment capabilities
type Capability string

const (
	CapabilityCardPayments    Capability = "card_payments"
	CapabilityTransfers       Capability = "transfers"
	CapabilityLegacyPayments  Capability = "legacy_payments"
	CapabilityPlatformPay     Capability = "platform_pay"
)

// Account represents a connected merchant account
type Account struct {
	ID                  string                 `json:"id"`
	PlatformID          string                 `json:"platform_id"`
	Email               string                 `json:"email"`
	DisplayName         string                 `json:"display_name"`
	Status              AccountStatus          `json:"status"`
	Capabilities        map[Capability]bool   `json:"capabilities"`
	DefaultCurrency     string                 `json:"default_currency"`
	ApplicationFeeRate  decimal.Decimal        `json:"application_fee_rate"`
	Metadata            map[string]string      `json:"metadata,omitempty"`
	CreatedAt           int64                  `json:"created_at"`
	UpdatedAt           int64                  `json:"updated_at"`
}

// NewAccount creates a new connected account
func NewAccount(platformID string, email string, displayName string) *Account {
	now := time.Now().UTC().Unix()
	return &Account{
		ID:              "acct_" + uuid.NewString(),
		PlatformID:      platformID,
		Email:           email,
		DisplayName:     displayName,
		Status:          AccountStatusPending,
		Capabilities:    make(map[Capability]bool),
		DefaultCurrency: "usd",
		Metadata:        make(map[string]string),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// Activate marks the account as active with full capabilities
func (a *Account) Activate() {
	a.Status = AccountStatusActive
	a.Capabilities[CapabilityCardPayments] = true
	a.Capabilities[CapabilityTransfers] = true
	a.UpdatedAt = time.Now().UTC().Unix()
}

// Suspend suspends the account temporarily
func (a *Account) Suspend() {
	a.Status = AccountStatusSuspended
	a.UpdatedAt = time.Now().UTC().Unix()
}

// Restrict restricts account capabilities
func (a *Account) Restrict() {
	a.Status = AccountStatusRestricted
	a.Capabilities[CapabilityCardPayments] = false
	a.UpdatedAt = time.Now().UTC().Unix()
}

// SetCapability enables/disables a specific capability
func (a *Account) SetCapability(capability Capability, enabled bool) {
	a.Capabilities[capability] = enabled
	a.UpdatedAt = time.Now().UTC().Unix()
}

// CalculateApplicationFee calculates fee for a transaction amount
func (a *Account) CalculateApplicationFee(amount decimal.Decimal) decimal.Decimal {
	return amount.Mul(a.ApplicationFeeRate).RoundBank(2)
}
