package secretbox_test

import (
	"crypto/rand"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/secretbox"
)

func newKey(t *testing.T) string {
	t.Helper()
	b := make([]byte, 32)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

func TestRoundTrip(t *testing.T) {
	box, err := secretbox.New(newKey(t))
	require.NoError(t, err)
	sealed, err := box.Seal([]byte("whsec_supersecret"))
	require.NoError(t, err)
	assert.NotContains(t, sealed, "supersecret", "ciphertext must not contain the plaintext")

	pt, err := box.Open(sealed)
	require.NoError(t, err)
	assert.Equal(t, "whsec_supersecret", string(pt))
}

func TestSealIsRandomised(t *testing.T) {
	box, err := secretbox.New(newKey(t))
	require.NoError(t, err)
	a, _ := box.Seal([]byte("same"))
	b, _ := box.Seal([]byte("same"))
	assert.NotEqual(t, a, b, "fresh nonce per seal")
}

func TestOpenRejectsWrongKeyAndTampering(t *testing.T) {
	boxA, _ := secretbox.New(newKey(t))
	boxB, _ := secretbox.New(newKey(t))
	sealed, _ := boxA.Seal([]byte("secret"))

	_, err := boxB.Open(sealed)
	assert.ErrorIs(t, err, secretbox.ErrOpen)

	_, err = boxA.Open(sealed[:len(sealed)-2] + "AA")
	assert.ErrorIs(t, err, secretbox.ErrOpen)
}

func TestOpenRejectsPlaintextRows(t *testing.T) {
	box, _ := secretbox.New(newKey(t))
	_, err := box.Open("whsec_plaintextsecretfromlegacyrow")
	assert.ErrorIs(t, err, secretbox.ErrOpen)
}

func TestNewRejectsBadKeys(t *testing.T) {
	_, err := secretbox.New("not base64!!")
	assert.Error(t, err)
	_, err = secretbox.New(base64.StdEncoding.EncodeToString([]byte("too short")))
	assert.Error(t, err)
}
