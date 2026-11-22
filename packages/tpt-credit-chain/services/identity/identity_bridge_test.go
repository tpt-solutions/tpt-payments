package identity

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-secret-with-enough-entropy-for-hs256"

func testConfig() BridgeConfig {
	cfg := DefaultBridgeConfig()
	cfg.JWTSecret = testSecret
	return cfg
}

// mint signs an attestation with the given secret and claim overrides.
func mint(t *testing.T, secret string, mutate func(*AttestationClaims)) string {
	t.Helper()
	claims := &AttestationClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-uuid-1",
			ID:        "jti-" + time.Now().Format("150405.000000000"),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Address:  "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
		Tier:     "kyc",
		Provider: string(ProviderOnfido),
	}
	if mutate != nil {
		mutate(claims)
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestAttestation_ValidTokenIsAccepted(t *testing.T) {
	bc := NewBridgeClient(testConfig())
	res, err := bc.ProcessAttestation(mint(t, testSecret, nil))
	require.NoError(t, err)
	assert.True(t, res.Verified)
	assert.Equal(t, ComputeIdentityUUID("user-uuid-1"), res.IdentityUUID)
}

func TestAttestation_UnconfiguredBridgeRefusesEverything(t *testing.T) {
	cfg := testConfig()
	cfg.JWTSecret = ""
	bc := NewBridgeClient(cfg)

	// Even a token signed with the old hard-coded fallback key must be refused.
	_, err := bc.ProcessAttestation(mint(t, "dev-secret-do-not-use-in-production", nil))
	require.ErrorIs(t, err, ErrNoJWTSecret)
}

func TestAttestation_WrongSecretIsRejected(t *testing.T) {
	bc := NewBridgeClient(testConfig())
	_, err := bc.ProcessAttestation(mint(t, "some-other-secret", nil))
	require.Error(t, err)
}

func TestAttestation_MissingExpiryIsRejected(t *testing.T) {
	bc := NewBridgeClient(testConfig())
	token := mint(t, testSecret, func(c *AttestationClaims) { c.ExpiresAt = nil })
	_, err := bc.ProcessAttestation(token)
	require.Error(t, err, "a token without exp must not be accepted")
}

func TestAttestation_ExpiredTokenIsRejected(t *testing.T) {
	bc := NewBridgeClient(testConfig())
	token := mint(t, testSecret, func(c *AttestationClaims) {
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	})
	_, err := bc.ProcessAttestation(token)
	require.Error(t, err)
}

func TestAttestation_ReplayIsRejected(t *testing.T) {
	bc := NewBridgeClient(testConfig())
	token := mint(t, testSecret, func(c *AttestationClaims) { c.ID = "fixed-jti" })

	_, err := bc.ProcessAttestation(token)
	require.NoError(t, err)

	_, err = bc.ProcessAttestation(token)
	require.Error(t, err, "the same attestation must not be accepted twice")
	assert.Contains(t, err.Error(), "already been used")
}

func TestAttestation_MissingJTIIsRejected(t *testing.T) {
	bc := NewBridgeClient(testConfig())
	token := mint(t, testSecret, func(c *AttestationClaims) { c.ID = "" })
	_, err := bc.ProcessAttestation(token)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jti")
}

func TestAttestation_NonHMACAlgorithmIsRejected(t *testing.T) {
	bc := NewBridgeClient(testConfig())
	claims := &AttestationClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-uuid-2",
			ID:        "jti-none",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		},
		Address: "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = bc.ProcessAttestation(token)
	require.Error(t, err, "alg=none must never be accepted")
}
