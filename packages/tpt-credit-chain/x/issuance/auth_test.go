package issuance_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// funded returns a keeper with two registered accounts, the first one holding credits from a payout.
func funded(t *testing.T) *issuance.Keeper {
	t.Helper()
	k := issuance.NewKeeperWithEvents(NewMockEventBus(), 1)
	ctx := context.Background()
	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	k.SetBlockHeight(1 + issuance.BlocksPerYear/12)
	k.ProcessPayouts()
	return k
}

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return pub, priv
}

func TestSignedSend_AcceptsAValidSignatureOnce(t *testing.T) {
	k := funded(t)
	pub, priv := newKey(t)
	require.NoError(t, k.BindKey(testAddr1, pub))

	amount := math.NewInt(100_000_000)
	sig := ed25519.Sign(priv, issuance.SendSigningBytes(testAddr1, testAddr2, amount, 1))
	_, err := k.SendCreditsSigned(issuance.SignedSend{From: testAddr1, To: testAddr2, Amount: amount, Nonce: 1, Signature: sig})
	require.NoError(t, err)
	assert.Equal(t, uint64(1), k.GetAccount(testAddr1).Nonce)

	// The same message again is a replay: its nonce is no longer the next one.
	_, err = k.SendCreditsSigned(issuance.SignedSend{From: testAddr1, To: testAddr2, Amount: amount, Nonce: 1, Signature: sig})
	assert.ErrorIs(t, err, issuance.ErrBadNonce)
}

func TestSignedSend_NoncesMustBeInOrder(t *testing.T) {
	k := funded(t)
	pub, priv := newKey(t)
	require.NoError(t, k.BindKey(testAddr1, pub))
	amount := math.NewInt(1_000)

	// Skipping a nonce is refused, even with a valid signature.
	sig := ed25519.Sign(priv, issuance.SendSigningBytes(testAddr1, testAddr2, amount, 2))
	_, err := k.SendCreditsSigned(issuance.SignedSend{From: testAddr1, To: testAddr2, Amount: amount, Nonce: 2, Signature: sig})
	assert.ErrorIs(t, err, issuance.ErrBadNonce)
	assert.Equal(t, uint64(0), k.GetAccount(testAddr1).Nonce, "a refused transfer changes nothing")
}

func TestSignedSend_TamperedOrForeignSignaturesAreRefused(t *testing.T) {
	k := funded(t)
	pub, priv := newKey(t)
	require.NoError(t, k.BindKey(testAddr1, pub))
	amount := math.NewInt(1_000)
	sig := ed25519.Sign(priv, issuance.SendSigningBytes(testAddr1, testAddr2, amount, 1))

	// A different amount does not match the signed message.
	_, err := k.SendCreditsSigned(issuance.SignedSend{From: testAddr1, To: testAddr2, Amount: math.NewInt(2_000), Nonce: 1, Signature: sig})
	assert.ErrorIs(t, err, issuance.ErrBadSignature)

	// A different recipient does not match either.
	_, err = k.SendCreditsSigned(issuance.SignedSend{From: testAddr1, To: testAddr1, Amount: amount, Nonce: 1, Signature: sig})
	assert.ErrorIs(t, err, issuance.ErrBadSignature)

	// Another key cannot authorize the account's transfers.
	_, otherPriv := newKey(t)
	forged := ed25519.Sign(otherPriv, issuance.SendSigningBytes(testAddr1, testAddr2, amount, 1))
	_, err = k.SendCreditsSigned(issuance.SignedSend{From: testAddr1, To: testAddr2, Amount: amount, Nonce: 1, Signature: forged})
	assert.ErrorIs(t, err, issuance.ErrBadSignature)

	assert.Equal(t, uint64(0), k.GetAccount(testAddr1).Nonce)
}

func TestSignedSend_AccountWithoutAKeyCannotSend(t *testing.T) {
	k := funded(t)
	_, priv := newKey(t)
	amount := math.NewInt(1_000)
	sig := ed25519.Sign(priv, issuance.SendSigningBytes(testAddr1, testAddr2, amount, 1))
	_, err := k.SendCreditsSigned(issuance.SignedSend{From: testAddr1, To: testAddr2, Amount: amount, Nonce: 1, Signature: sig})
	assert.ErrorIs(t, err, issuance.ErrNoKey)
}

func TestBindKey_OnceAndOnlyWithAValidKey(t *testing.T) {
	k := funded(t)
	pub, _ := newKey(t)
	other, _ := newKey(t)

	assert.Error(t, k.BindKey(testAddr1, ed25519.PublicKey([]byte("short"))), "a key must be 32 bytes")
	assert.Error(t, k.BindKey(unknownAddr, pub), "an unknown account has no key to bind")

	require.NoError(t, k.BindKey(testAddr1, pub))
	assert.Error(t, k.BindKey(testAddr1, other), "a bound key cannot be replaced")
}

func TestSignedBatch_AuthorizesEveryEntryAtOnce(t *testing.T) {
	k := funded(t)
	pub, priv := newKey(t)
	require.NoError(t, k.BindKey(testAddr1, pub))
	entries := []issuance.BatchSendEntry{{To: testAddr2, Amount: math.NewInt(500)}}

	sig := ed25519.Sign(priv, issuance.BatchSigningBytes(testAddr1, entries, 1))
	_, err := k.BatchSendSigned(issuance.SignedBatch{From: testAddr1, Entries: entries, Nonce: 1, Signature: sig})
	require.NoError(t, err)

	// Changing an entry after signing invalidates the signature.
	changed := []issuance.BatchSendEntry{{To: testAddr2, Amount: math.NewInt(501)}}
	_, err = k.BatchSendSigned(issuance.SignedBatch{From: testAddr1, Entries: changed, Nonce: 2, Signature: sig})
	assert.ErrorIs(t, err, issuance.ErrBadSignature)
}

func TestSigningBytes_AreUnambiguous(t *testing.T) {
	// Length prefixes keep "ab"+"c" and "a"+"bc" apart.
	a := issuance.SendSigningBytes("ab", "c", math.NewInt(1), 1)
	b := issuance.SendSigningBytes("a", "bc", math.NewInt(1), 1)
	assert.NotEqual(t, a, b)
}
