package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// adminFixture is a chain with two registered accounts, the first one funded by a payout, and an
// admin API on an ephemeral loopback port.
type adminFixture struct {
	app               *App
	addr              string
	token             string
	home              string
	sender, recipient string
}

func newAdminFixture(t *testing.T) *adminFixture {
	t.Helper()
	prev := GlobalApp
	GlobalApp = nil
	t.Cleanup(func() { GlobalApp = prev })

	home := t.TempDir()
	chain := newTestApp(t, home)
	ctx := context.Background()
	sender := testAddress(t, 21)
	recipient := testAddress(t, 22)
	_, err := chain.IssuanceKeeper.RegisterHuman(ctx, sender, testUUID(21), "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = chain.IssuanceKeeper.RegisterHuman(ctx, recipient, testUUID(22), "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	chain.IssuanceKeeper.SetBlockHeight(1 + issuance.BlocksPerYear/12)
	chain.IssuanceKeeper.ProcessPayouts()

	token, err := LoadOrCreateAdminToken(home)
	require.NoError(t, err)
	srv, ln, err := chain.ServeAdmin("127.0.0.1:0", token)
	require.NoError(t, err)
	t.Cleanup(func() { _ = srv.Close() })
	return &adminFixture{app: chain, addr: ln.Addr().String(), token: token, home: home, sender: sender, recipient: recipient}
}

func (f *adminFixture) post(t *testing.T, path, token string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, "http://"+f.addr+path, bytes.NewReader(raw))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestAdmin_RefusesNonLoopbackAddresses(t *testing.T) {
	chain := newTestApp(t, t.TempDir())
	token := strings.Repeat("t", 32)
	for _, addr := range []string{"0.0.0.0:0", "192.168.1.5:26670", ":26670"} {
		_, _, err := chain.ServeAdmin(addr, token)
		assert.Error(t, err, "%s must be refused", addr)
	}
	_, _, err := chain.ServeAdmin("127.0.0.1:0", "short")
	assert.Error(t, err, "a short admin token is refused")
}

func TestAdmin_EveryRouteNeedsTheToken(t *testing.T) {
	f := newAdminFixture(t)
	resp := f.post(t, "/admin/tx/bind-key", "", map[string]string{"address": f.sender})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp = f.post(t, "/admin/tx/bind-key", strings.Repeat("x", 32), map[string]string{"address": f.sender})
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAdmin_BindKeyOnceAndSignedSendOnce(t *testing.T) {
	f := newAdminFixture(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	resp := f.post(t, "/admin/tx/bind-key", f.token, BindKeyRequest{Address: f.sender, PublicKey: hex.EncodeToString(pub)})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp = f.post(t, "/admin/tx/bind-key", f.token, BindKeyRequest{Address: f.sender, PublicKey: hex.EncodeToString(pub)})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "a second binding is refused")

	amount := math.NewInt(100_000_000)
	sig := ed25519.Sign(priv, issuance.SendSigningBytes(f.sender, f.recipient, amount, 1))
	req := SendRequest{From: f.sender, To: f.recipient, Amount: amount.String(), Nonce: 1, Signature: hex.EncodeToString(sig)}
	resp = f.post(t, "/admin/tx/send", f.token, req)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp = f.post(t, "/admin/tx/send", f.token, req)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "a replayed signed send is refused")
	assert.Equal(t, uint64(1), f.app.IssuanceKeeper.GetAccount(f.sender).Nonce)
}

func TestAdmin_UnsignedOrForgedSendsAreRefused(t *testing.T) {
	f := newAdminFixture(t)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, f.post(t, "/admin/tx/bind-key", f.token,
		BindKeyRequest{Address: f.sender, PublicKey: hex.EncodeToString(pub)}).StatusCode)

	_, forger, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	amount := math.NewInt(100)
	forged := ed25519.Sign(forger, issuance.SendSigningBytes(f.sender, f.recipient, amount, 1))
	resp := f.post(t, "/admin/tx/send", f.token, SendRequest{From: f.sender, To: f.recipient, Amount: amount.String(), Nonce: 1, Signature: hex.EncodeToString(forged)})
	assert.Equal(t, http.StatusForbidden, resp.StatusCode, "a signature from another key is refused")
}

func TestAdmin_BodiesAreStrict(t *testing.T) {
	f := newAdminFixture(t)
	req, err := http.NewRequest(http.MethodPost, "http://"+f.addr+"/admin/tx/bind-key", strings.NewReader(`{"address":"x","surprise":1}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+f.token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "unknown fields are refused")
}

func TestAdmin_ClientPathUsesTheHomeToken(t *testing.T) {
	f := newAdminFixture(t)
	t.Setenv("TPTCREDIT_HOME", f.home)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// The command-line client finds the token in the home directory and reaches the node's admin API.
	_, err = submitRemote(f.addr, "/admin/tx/bind-key", BindKeyRequest{Address: f.sender, PublicKey: hex.EncodeToString(pub)})
	require.NoError(t, err)

	amount := math.NewInt(5_000)
	sig := ed25519.Sign(priv, issuance.SendSigningBytes(f.sender, f.recipient, amount, 1))
	_, err = submitRemote(f.addr, "/admin/tx/send", SendRequest{From: f.sender, To: f.recipient, Amount: amount.String(), Nonce: 1, Signature: hex.EncodeToString(sig)})
	require.NoError(t, err)

	// Without a token file there is nothing to submit with.
	require.NoError(t, os.Remove(mustTokenPath(t, f.home)))
	_, err = submitRemote(f.addr, "/admin/tx/bind-key", BindKeyRequest{Address: f.sender, PublicKey: hex.EncodeToString(pub)})
	assert.Error(t, err)
}

func mustTokenPath(t *testing.T, home string) string {
	t.Helper()
	p, err := AdminTokenPath(home)
	require.NoError(t, err)
	return p
}
