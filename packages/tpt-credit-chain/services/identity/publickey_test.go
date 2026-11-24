package identity

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func publicPEM(t *testing.T, pub any) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

func claimsFor(id string) *AttestationClaims {
	return &AttestationClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-uuid-pk",
			ID:        id,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		Address:  "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
		Tier:     "kyc",
		Provider: string(ProviderOnfido),
	}
}

func TestPublicKey_Ed25519AttestationIsAccepted(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cfg := DefaultBridgeConfig()
	cfg.JWTPublicKeyPEM = publicPEM(t, pub)
	bc := NewBridgeClient(cfg)

	signed, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claimsFor("jti-ed-1")).SignedString(priv)
	require.NoError(t, err)
	res, err := bc.ProcessAttestation(signed)
	require.NoError(t, err)
	assert.True(t, res.Verified)
}

func TestPublicKey_RSAAndECDSAKeysAreSupported(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	cfg := DefaultBridgeConfig()
	cfg.JWTPublicKeyPEM = publicPEM(t, &rsaKey.PublicKey)
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claimsFor("jti-rsa-1")).SignedString(rsaKey)
	require.NoError(t, err)
	_, err = NewBridgeClient(cfg).ProcessAttestation(signed)
	require.NoError(t, err)

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cfg.JWTPublicKeyPEM = publicPEM(t, &ecKey.PublicKey)
	signed, err = jwt.NewWithClaims(jwt.SigningMethodES256, claimsFor("jti-ec-1")).SignedString(ecKey)
	require.NoError(t, err)
	_, err = NewBridgeClient(cfg).ProcessAttestation(signed)
	require.NoError(t, err)
}

// In asymmetric mode an HMAC token is refused, even one whose HMAC key is the public key bytes. This is
// the algorithm-confusion attack: a verifier that accepts HS256 with its public key as the secret can be
// forged by anyone who has the public key.
func TestPublicKey_HMACTokenIsRefusedEvenWithThePublicKeyAsSecret(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pemText := publicPEM(t, pub)
	cfg := DefaultBridgeConfig()
	cfg.JWTPublicKeyPEM = pemText
	bc := NewBridgeClient(cfg)

	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claimsFor("jti-forge-1")).SignedString([]byte(pemText))
	require.NoError(t, err)
	_, err = bc.ProcessAttestation(forged)
	require.Error(t, err)
}

func TestPublicKey_TokenFromAnotherKeyIsRejected(t *testing.T) {
	trusted, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, attacker, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cfg := DefaultBridgeConfig()
	cfg.JWTPublicKeyPEM = publicPEM(t, trusted)

	signed, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claimsFor("jti-other-1")).SignedString(attacker)
	require.NoError(t, err)
	_, err = NewBridgeClient(cfg).ProcessAttestation(signed)
	require.Error(t, err)
}

func TestPublicKey_BadOrDoubledConfigurationIsReported(t *testing.T) {
	cfg := DefaultBridgeConfig()
	cfg.JWTPublicKeyPEM = "not a key"
	_, err := NewBridgeClient(cfg).ProcessAttestation("x.y.z")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "JWTPublicKeyPEM")

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	both := DefaultBridgeConfig()
	both.JWTPublicKeyPEM = publicPEM(t, pub)
	both.JWTSecret = testSecret
	_, err = NewBridgeClient(both).ProcessAttestation("x.y.z")
	assert.ErrorIs(t, err, ErrBothKeyKinds)
}

// During a key rotation both the old and the new key are configured as PEM blocks in one value.
// An attestation signed by either verifies; one signed by a key that is not listed does not.
func TestPublicKey_RotationAcceptsEitherConfiguredKey(t *testing.T) {
	oldPub, oldPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	newPub, newPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_, strayPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	cfg := DefaultBridgeConfig()
	cfg.JWTPublicKeyPEM = publicPEM(t, oldPub) + publicPEM(t, newPub)
	bc := NewBridgeClient(cfg)

	fromOld, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claimsFor("jti-rot-old")).SignedString(oldPriv)
	require.NoError(t, err)
	_, err = bc.ProcessAttestation(fromOld)
	require.NoError(t, err, "a token from the retiring key still verifies during rotation")

	fromNew, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claimsFor("jti-rot-new")).SignedString(newPriv)
	require.NoError(t, err)
	_, err = bc.ProcessAttestation(fromNew)
	require.NoError(t, err, "a token from the new key verifies")

	stray, err := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claimsFor("jti-rot-stray")).SignedString(strayPriv)
	require.NoError(t, err)
	_, err = bc.ProcessAttestation(stray)
	require.Error(t, err, "a key that is not configured is refused")
}

// A rotation value is only as good as its parsing: a block that is not a key, or stray data after
// the last block, is reported rather than silently dropping a key.
func TestPublicKey_MalformedRotationValueIsReported(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	good := publicPEM(t, pub)

	cases := map[string]string{
		"trailing junk after a key": good + "not-a-pem-block\n",
		"a certificate block":       "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n",
	}
	for name, value := range cases {
		cfg := DefaultBridgeConfig()
		cfg.JWTPublicKeyPEM = value
		_, err := NewBridgeClient(cfg).ProcessAttestation("header.payload.sig")
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "JWTPublicKeyPEM", name)
	}
}
