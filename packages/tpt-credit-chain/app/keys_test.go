package app

import (
	"crypto/ed25519"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyFile_RoundTripsAndIsNeverOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sender.key")
	pub, err := newKeyFile(path)
	require.NoError(t, err)
	require.Len(t, pub, ed25519.PublicKeySize)

	priv, err := loadSigningKey(path)
	require.NoError(t, err)
	msg := []byte("transfer")
	assert.True(t, ed25519.Verify(pub, msg, ed25519.Sign(priv, msg)), "the saved key signs for the published public key")

	_, err = newKeyFile(path)
	assert.Error(t, err, "an existing key file is never replaced")
	again, err := loadSigningKey(path)
	require.NoError(t, err)
	assert.Equal(t, priv, again, "the refused call left the key untouched")

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "a key file is readable by its owner only")
	}
}

func TestKeyFile_RefusesMalformedContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.key")
	require.NoError(t, os.WriteFile(path, []byte("not hex at all\n"), 0o600))
	_, err := loadSigningKey(path)
	assert.Error(t, err)
}
