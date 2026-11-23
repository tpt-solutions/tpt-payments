// Package identity provides the identity bridge service for TPT Credit.
//
// The identity bridge is an off-chain service that sits between KYC providers
// and the TPT Credit chain. It:
//  1. Receives verified identity attestations from KYC providers
//  2. Validates the attestation format (signed JWT)
//  3. Computes the on-chain Verifiable Credential hash (SHA-256 of UUID)
//  4. Submits MsgRegisterHuman to the chain on behalf of the verified user
//
// Attestation format: signed JWT from KYC provider with claims:
//   - sub: unique person ID (UUID v4)
//   - address: Cosmos bech32 address
//   - tier: verification level (kyc | biometric | dna)
//   - iat: issued at timestamp
//   - exp: expiration timestamp
//
// The SHA-256 hash of the UUID is the IdentityUUID stored on-chain as the
// uniqueness constraint. This prevents the actual UUID from being stored
// on-chain while still allowing duplicate detection.
//
// Security model:
//   - The bridge holds a signing key that is authorized to submit registrations
//   - KYC providers sign JWTs that the bridge verifies
//   - The bridge never stores raw private keys in memory (uses KMS)
//   - Rate limiting prevents mass registration attacks
package identity

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// KYCProvider defines the supported KYC providers.
type KYCProvider string

const (
	ProviderForge   KYCProvider = "forgetpass"
	ProviderOnfido  KYCProvider = "onfido"
	ProviderJumio   KYCProvider = "jumio"
	ProviderPersona KYCProvider = "persona"
	ProviderCustom  KYCProvider = "custom"
)

// IdentityTier defines the verification level.
type IdentityTier string

const (
	TierKYC       IdentityTier = "kyc"
	TierBiometric IdentityTier = "biometric"
	TierDNA       IdentityTier = "dna"
)

// ValidTiers contains allowed identity tiers.
var ValidTiers = map[IdentityTier]bool{
	TierKYC:       true,
	TierBiometric: true,
	TierDNA:       true,
}

// AttestationClaims is the JWT claims format from KYC providers.
type AttestationClaims struct {
	jwt.RegisteredClaims
	// Address is the Cosmos bech32 address to register.
	Address string `json:"address"`
	// Tier is the verification level.
	Tier string `json:"tier"`
	// Provider is the KYC provider name.
	Provider string `json:"provider"`
}

// AttestationResult is the result of processing an attestation.
type AttestationResult struct {
	// Address is the Cosmos bech32 address.
	Address string `json:"address"`
	// IdentityUUID is the SHA-256 hash of the KYC provider's UUID.
	IdentityUUID string `json:"identity_uuid"`
	// Tier is the verification level.
	Tier string `json:"tier"`
	// Verified indicates whether the attestation was verified.
	Verified bool `json:"verified"`
	// Error contains any verification error message.
	Error string `json:"error,omitempty"`
	// Provider is the KYC provider name.
	Provider string `json:"provider"`
	// Timestamp is when the attestation was processed.
	Timestamp int64 `json:"timestamp"`
}

// BridgeConfig configures the identity bridge.
type BridgeConfig struct {
	// AllowedProviders is the list of KYC providers that this bridge accepts.
	// Empty means all providers are accepted.
	AllowedProviders []KYCProvider `json:"allowed_providers"`

	// JWTSecret is the shared secret for verifying KYC provider JWTs (HS256). Development only.
	JWTSecret string `json:"-"`

	// JWTPublicKeyPEM is the KYC provider's public key (PEM, SPKI form: Ed25519, RSA or ECDSA).
	// When set, only asymmetric signatures are accepted and JWTSecret must be empty. A verifier
	// that holds only a public key cannot forge attestations, so this is the production setting.
	JWTPublicKeyPEM string `json:"-"`

	// MaxAttestationsPerDay is the maximum number of attestations per day.
	// Set to 0 for unlimited.
	MaxAttestationsPerDay int `json:"max_attestations_per_day"`

	// RequireTier enables tier validation (only specified tiers accepted).
	RequireTier []IdentityTier `json:"require_tier"`
}

// DefaultBridgeConfig returns the default bridge configuration.
func DefaultBridgeConfig() BridgeConfig {
	return BridgeConfig{
		AllowedProviders:      []KYCProvider{ProviderForge, ProviderOnfido, ProviderJumio, ProviderPersona},
		MaxAttestationsPerDay: 1000,
		RequireTier:           []IdentityTier{TierKYC, TierBiometric, TierDNA},
	}
}

// BridgeClient provides KYC attestation utilities.
type BridgeClient struct {
	config BridgeConfig
	// publicKey is the parsed JWTPublicKeyPEM, or nil in secret mode. keyErr records a key that
	// did not parse; it is returned by every ProcessAttestation call.
	publicKey crypto.PublicKey
	keyErr    error

	mu             sync.Mutex
	dailyCount     int
	currentDay     int
	attestationLog []AttestationResult
	// usedJTI records attestation IDs already accepted, until their expiry, so a captured
	// token cannot be replayed.
	usedJTI map[string]time.Time
}

// ErrBothKeyKinds is returned when both an HMAC secret and a public key are configured.
var ErrBothKeyKinds = errors.New("identity bridge: set either JWTSecret or JWTPublicKeyPEM, not both")

// ErrNoJWTSecret is returned when attestations are processed without a configured secret.
// The bridge never falls back to a default key: an unconfigured bridge accepts nothing.
var ErrNoJWTSecret = errors.New("identity bridge has no JWT secret or public key configured; attestations are refused")

// NewBridgeClient creates a new identity bridge client.
func NewBridgeClient(config BridgeConfig) *BridgeClient {
	bc := &BridgeClient{
		config:         config,
		dailyCount:     0,
		currentDay:     time.Now().YearDay(),
		attestationLog: make([]AttestationResult, 0),
		usedJTI:        make(map[string]time.Time),
	}
	switch {
	case config.JWTSecret != "" && config.JWTPublicKeyPEM != "":
		bc.keyErr = ErrBothKeyKinds
	case config.JWTPublicKeyPEM != "":
		key, err := parsePublicKey(config.JWTPublicKeyPEM)
		if err != nil {
			bc.keyErr = fmt.Errorf("identity bridge: JWTPublicKeyPEM: %w", err)
		} else {
			bc.publicKey = key
		}
	}
	return bc
}

// parsePublicKey reads a PEM public key in SPKI form. Only key types the verifier can use are
// accepted, so a certificate or a private key is refused.
func parsePublicKey(pemData string) (crypto.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemData))
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("expected a PEM block of type PUBLIC KEY")
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	switch key.(type) {
	case ed25519.PublicKey, *rsa.PublicKey, *ecdsa.PublicKey:
		return key, nil
	}
	return nil, fmt.Errorf("unsupported key type %T", key)
}

// ProcessAttestation processes a JWT attestation from a KYC provider.
// Returns the attestation result with the IdentityUUID hash.
func (bc *BridgeClient) ProcessAttestation(jwtToken string) (*AttestationResult, error) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	// Rate limit check.
	today := time.Now().YearDay()
	if today != bc.currentDay {
		bc.dailyCount = 0
		bc.currentDay = today
	}
	if bc.config.MaxAttestationsPerDay > 0 && bc.dailyCount >= bc.config.MaxAttestationsPerDay {
		return nil, fmt.Errorf("daily attestation limit of %d exceeded", bc.config.MaxAttestationsPerDay)
	}

	// Parse the JWT token.
	if bc.keyErr != nil {
		return nil, bc.keyErr
	}
	if bc.publicKey == nil && bc.config.JWTSecret == "" {
		return nil, ErrNoJWTSecret
	}
	token, err := jwt.ParseWithClaims(jwtToken, &AttestationClaims{}, func(token *jwt.Token) (interface{}, error) {
		if bc.publicKey != nil {
			// Asymmetric mode: the algorithm must be an asymmetric one. An HMAC token is refused
			// even if its signature would check out, so a public key cannot be used as a secret.
			switch token.Method.(type) {
			case *jwt.SigningMethodEd25519, *jwt.SigningMethodRSA, *jwt.SigningMethodECDSA:
				return bc.publicKey, nil
			}
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// Secret mode: only HMAC, only with the configured secret.
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(bc.config.JWTSecret), nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("JWT verification failed: %w", err)
	}

	claims, ok := token.Claims.(*AttestationClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid JWT claims")
	}

	// Replay protection: each attestation carries a unique jti, accepted once.
	if claims.ID == "" {
		return nil, fmt.Errorf("jti claim is required for replay protection")
	}
	now := time.Now()
	for id, exp := range bc.usedJTI {
		if now.After(exp) {
			delete(bc.usedJTI, id)
		}
	}
	if _, seen := bc.usedJTI[claims.ID]; seen {
		return nil, fmt.Errorf("attestation %q has already been used", claims.ID)
	}

	// Validate address format (basic Cosmos bech32 validation).
	if claims.Address == "" {
		return nil, fmt.Errorf("address claim is required")
	}
	if !strings.HasPrefix(claims.Address, "cosmos1") {
		return nil, fmt.Errorf("address %q is not a valid Cosmos bech32 address", claims.Address)
	}

	// Validate tier.
	tier := claims.Tier
	if tier == "" {
		tier = "kyc"
	}
	if !ValidTiers[IdentityTier(tier)] {
		return nil, fmt.Errorf("invalid identity tier: %q", tier)
	}

	// Validate provider.
	if len(bc.config.AllowedProviders) > 0 {
		provider := KYCProvider(claims.Provider)
		allowed := false
		for _, ap := range bc.config.AllowedProviders {
			if ap == provider {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("provider %q is not in the allowed list", claims.Provider)
		}
	}

	// Compute IdentityUUID = SHA-256 of the UUID (sub claim).
	uuid := claims.Subject
	if uuid == "" {
		return nil, fmt.Errorf("sub (UUID) claim is required")
	}
	hash := sha256.Sum256([]byte(uuid))
	identityUUID := hex.EncodeToString(hash[:])

	// Generate the result.
	result := &AttestationResult{
		Address:      claims.Address,
		IdentityUUID: identityUUID,
		Tier:         tier,
		Verified:     true,
		Provider:     claims.Provider,
		Timestamp:    time.Now().Unix(),
	}

	bc.usedJTI[claims.ID] = claims.ExpiresAt.Time
	bc.dailyCount++
	bc.attestationLog = append(bc.attestationLog, *result)

	return result, nil
}

// ComputeIdentityUUID computes the on-chain identity UUID from a raw UUID.
// This is a pure function for use by external services.
func ComputeIdentityUUID(uuid string) string {
	hash := sha256.Sum256([]byte(uuid))
	return hex.EncodeToString(hash[:])
}

// VerifyAttestationFormat validates that an attestation JWT is well-formed
// without processing it. This is a stateless validation.
func VerifyAttestationFormat(jwtToken string) error {
	// Check that it looks like a JWT (3 dot-separated base64 segments).
	parts := strings.Split(jwtToken, ".")
	if len(parts) != 3 {
		return fmt.Errorf("JWT must have 3 segments, got %d", len(parts))
	}

	// Validate each segment is non-empty.
	for i, part := range parts {
		if part == "" {
			return fmt.Errorf("JWT segment %d is empty", i+1)
		}
	}

	return nil
}

// GetDailyCount returns the current daily attestation count.
func (bc *BridgeClient) GetDailyCount() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return bc.dailyCount
}

// GetAttestationLog returns the attestation log.
func (bc *BridgeClient) GetAttestationLog() []AttestationResult {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	result := make([]AttestationResult, len(bc.attestationLog))
	copy(result, bc.attestationLog)
	return result
}

// MarshalJSON implements json.Marshaler for BridgeConfig (redacts JWT secret).
func (bc BridgeConfig) MarshalJSON() ([]byte, error) {
	type Alias BridgeConfig
	return json.Marshal(struct {
		Alias
		JWTSecret string `json:"jwt_secret"`
	}{
		Alias:     (Alias)(bc),
		JWTSecret: "[REDACTED]",
	})
}
