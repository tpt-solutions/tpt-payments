package vault_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/vault"
)

const goodToken = "test-token"

// fakeVault implements the transit encrypt and decrypt endpoints. "Encryption" is reversible
// base64 with a prefix, which is enough to test the protocol, not the cryptography.
func fakeVault(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != goodToken {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
			return
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch {
		case r.URL.Path == "/v1/transit/encrypt/tpt-webhooks":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]string{"ciphertext": "vault:v1:" + body["plaintext"]},
			})
		case r.URL.Path == "/v1/transit/decrypt/tpt-webhooks":
			ct := strings.TrimPrefix(body["ciphertext"], "vault:v1:")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"plaintext": ct}})
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors":["unsupported path"]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTransit_EncryptThenDecryptRoundTrip(t *testing.T) {
	srv := fakeVault(t)
	client, err := vault.NewTransit(srv.URL, goodToken, "transit", "tpt-webhooks")
	require.NoError(t, err)

	ct, err := client.Encrypt(context.Background(), []byte("secret-material"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(ct, "vault:v1:"))

	pt, err := client.Decrypt(context.Background(), ct)
	require.NoError(t, err)
	assert.Equal(t, "secret-material", string(pt))
}

// The wrapped webhook key unwraps to a key the secretbox accepts, which is how the server uses it.
func TestTransit_UnwrappedKeyOpensSecretbox(t *testing.T) {
	srv := fakeVault(t)
	client, err := vault.NewTransit(srv.URL, goodToken, "", "tpt-webhooks")
	require.NoError(t, err)

	raw := make([]byte, 32)
	_, _ = rand.Read(raw)
	keyB64 := base64.StdEncoding.EncodeToString(raw)
	wrapped, err := client.Encrypt(context.Background(), []byte(keyB64))
	require.NoError(t, err)

	unwrapped, err := client.Decrypt(context.Background(), wrapped)
	require.NoError(t, err)
	box, err := secretbox.New(string(unwrapped))
	require.NoError(t, err)
	sealed, err := box.Seal([]byte("whsec_abc"))
	require.NoError(t, err)
	opened, err := box.Open(sealed)
	require.NoError(t, err)
	assert.Equal(t, "whsec_abc", string(opened))
}

// A rejected token is reported with the status, and the token is not echoed.
func TestTransit_ErrorsDoNotLeakToken(t *testing.T) {
	srv := fakeVault(t)
	client, err := vault.NewTransit(srv.URL, "wrong-token-value", "transit", "tpt-webhooks")
	require.NoError(t, err)

	_, err = client.Encrypt(context.Background(), []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
	assert.Contains(t, err.Error(), "permission denied")
	assert.NotContains(t, err.Error(), "wrong-token-value")
}

func TestTransit_UnknownKeyIsAnError(t *testing.T) {
	srv := fakeVault(t)
	client, err := vault.NewTransit(srv.URL, goodToken, "transit", "missing-key")
	require.NoError(t, err)
	_, err = client.Encrypt(context.Background(), []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 404")
}

func TestNewTransit_RequiresSettingsAndTLSUnlessLoopback(t *testing.T) {
	_, err := vault.NewTransit("", goodToken, "transit", "k")
	assert.ErrorIs(t, err, vault.ErrNotConfigured)
	_, err = vault.NewTransit("https://vault.example.com", "", "transit", "k")
	assert.ErrorIs(t, err, vault.ErrNotConfigured)

	_, err = vault.NewTransit("http://vault.example.com:8200", goodToken, "transit", "k")
	require.Error(t, err, "a remote plaintext address must be refused")

	_, err = vault.NewTransit("http://127.0.0.1:8200", goodToken, "transit", "k")
	assert.NoError(t, err, "loopback is allowed for development")

	_, err = vault.NewTransit("https://vault.example.com", goodToken, "transit", "k")
	assert.NoError(t, err)
}
