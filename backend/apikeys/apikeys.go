// tpt-payments/backend/apikeys/apikeys.go
package apikeys

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// KeyScope defines API key permission levels
type KeyScope string

const (
	ScopeFullAccess KeyScope = "full_access"
	ScopeReadOnly   KeyScope = "read_only"
	ScopeWriteOnly  KeyScope = "write_only"
)

// APIKey represents an API key for merchant authentication
type APIKey struct {
	ID         string    `json:"id"`
	MerchantID string    `json:"merchant_id"`
	Prefix     string    `json:"prefix"`
	KeyHash    string    `json:"key_hash"`
	Mode       string    `json:"mode"` // test, live
	Type       string    `json:"type"` // publishable, secret
	Scope      KeyScope  `json:"scope"`
	Status     string    `json:"status"` // active, revoked
	CreatedAt  time.Time `json:"created_at"`
	RevokedAt  time.Time `json:"revoked_at,omitempty"`
}

// KeyPrefixes define standard key prefixes
const (
	PrefixPublishableTest = "pk_test_"
	PrefixPublishableLive = "pk_live_"
	PrefixSecretTest      = "sk_test_"
	PrefixSecretLive      = "sk_live_"
)

// GenerateKey creates a new API key with appropriate prefix
func GenerateKey(mode string, keyType string) (string, string, error) {
	var prefix string
	switch {
	case mode == "test" && keyType == "publishable":
		prefix = PrefixPublishableTest
	case mode == "live" && keyType == "publishable":
		prefix = PrefixPublishableLive
	case mode == "test" && keyType == "secret":
		prefix = PrefixSecretTest
	case mode == "live" && keyType == "secret":
		prefix = PrefixSecretLive
	default:
		return "", "", fmt.Errorf("invalid mode or key type")
	}

	randomBytes := make([]byte, 32)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", "", err
	}

	key := prefix + hex.EncodeToString(randomBytes)
	hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}

	return key, string(hash), nil
}

// VerifyKey checks if a provided API key matches the stored hash
func VerifyKey(providedKey string, storedHash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(providedKey))
	return err == nil
}

// NewAPIKey creates a new APIKey entity with optional scope (defaults to full_access)
func NewAPIKey(merchantID string, mode string, keyType string, scope ...KeyScope) (*APIKey, string, error) {
	key, hash, err := GenerateKey(mode, keyType)
	if err != nil {
		return nil, "", err
	}

	appliedScope := ScopeFullAccess
	if len(scope) > 0 {
		appliedScope = scope[0]
	}

	return &APIKey{
		ID:         uuid.NewString(),
		MerchantID: merchantID,
		Prefix:     key[:8],
		KeyHash:    hash,
		Mode:       mode,
		Type:       keyType,
		Scope:      appliedScope,
		Status:     "active",
		CreatedAt:  time.Now().UTC(),
	}, key, nil
}

// Revoke marks an API key as revoked
func (k *APIKey) Revoke() {
	k.Status = "revoked"
	k.RevokedAt = time.Now().UTC()
}

// GetModeFromKey extracts the mode from an API key string
func GetModeFromKey(key string) string {
	switch {
	case len(key) > 8 && key[:8] == PrefixPublishableTest, key[:8] == PrefixSecretTest:
		return "test"
	case len(key) > 8 && key[:8] == PrefixPublishableLive, key[:8] == PrefixSecretLive:
		return "live"
	default:
		return "unknown"
	}
}

// IsSecretKey returns true if the key is a secret key
func IsSecretKey(key string) bool {
	return len(key) > 5 && (key[:5] == "sk_te" || key[:5] == "sk_li")
}

// IsPublishableKey returns true if the key is a publishable key
func IsPublishableKey(key string) bool {
	return len(key) > 5 && (key[:5] == "pk_te" || key[:5] == "pk_li")
}

// CanRead checks if the API key has read permissions
func (k *APIKey) CanRead() bool {
	return k.Status == "active" && (k.Scope == ScopeFullAccess || k.Scope == ScopeReadOnly)
}

// CanWrite checks if the API key has write permissions
func (k *APIKey) CanWrite() bool {
	return k.Status == "active" && (k.Scope == ScopeFullAccess || k.Scope == ScopeWriteOnly)
}

// Rotate creates a replacement API key with same merchant, mode, type and scope
// Marks the current key for revocation after the grace period
func (k *APIKey) Rotate(gracePeriodHours ...int) (*APIKey, string, error) {
	if k.Status != "active" {
		return nil, "", fmt.Errorf("cannot rotate revoked key")
	}

	// Create new key with identical permissions
	newKey, newKeyRaw, err := NewAPIKey(k.MerchantID, k.Mode, k.Type, k.Scope)
	if err != nil {
		return nil, "", err
	}

	// Schedule old key revocation after grace period (default 24 hours)
	gracePeriod := 24
	if len(gracePeriodHours) > 0 {
		gracePeriod = gracePeriodHours[0]
	}

	// Mark this key as being rotated - will be automatically revoked after grace period
	// Note: Actual revocation would be handled by scheduled job
	k.RevokedAt = time.Now().UTC().Add(time.Duration(gracePeriod) * time.Hour)

	return newKey, newKeyRaw, nil
}
