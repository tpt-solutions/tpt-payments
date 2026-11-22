package app

import (
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Valid vectors from BIP-173.
func TestBech32_BIP173Vectors(t *testing.T) {
	for _, v := range []string{
		"A12UEL5L",
		"a12uel5l",
		"abcdef1qpzry9x8gf2tvdw0s3jn54khce6mua7lmqqqxw",
	} {
		_, _, err := Bech32Decode(v)
		assert.NoError(t, err, "vector %q must decode", v)
	}
}

func TestBech32_RejectsBadChecksumAndMixedCase(t *testing.T) {
	_, _, err := Bech32Decode("abcdef1qpzry9x8gf2tvdw0s3jn54khce6mua7lmqqqxx") // last char changed
	assert.Error(t, err)
	_, _, err = Bech32Decode("A12uEL5L")
	assert.Error(t, err)
}

func TestBech32_EncodeDecodeRoundTrip(t *testing.T) {
	payload := sha256.Sum256([]byte("round trip"))
	enc, err := Bech32Encode("cosmos", payload[:20])
	require.NoError(t, err)
	hrp, got, err := Bech32Decode(enc)
	require.NoError(t, err)
	assert.Equal(t, "cosmos", hrp)
	assert.Equal(t, payload[:20], got)
}

func TestValidAddress_ChecksPrefixAndLength(t *testing.T) {
	payload := sha256.Sum256([]byte("addr"))
	addr, err := Bech32Encode("cosmos", payload[:20])
	require.NoError(t, err)

	assert.NoError(t, ValidAddress(addr, "cosmos"))
	assert.Error(t, ValidAddress(addr, "osmo"), "wrong prefix")

	short, err := Bech32Encode("cosmos", payload[:10])
	require.NoError(t, err)
	assert.Error(t, ValidAddress(short, "cosmos"), "wrong payload length")
}

// The protocol reserve address must be a valid address. Previously it was a hand-typed string
// with a bad checksum.
func TestReserveAddressIsValid(t *testing.T) {
	require.NoError(t, ValidAddress(DefaultReserveAddress, "cosmos"))
	assert.True(t, strings.HasPrefix(DefaultReserveAddress, "cosmos1"))
}
