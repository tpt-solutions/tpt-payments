package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureEnv_CopiesWhenMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "example")
	dst := filepath.Join(dir, "sub", ".env")
	require.NoError(t, os.WriteFile(src, []byte("DATABASE_URL=x\n"), 0o600))

	created, err := ensureEnv(src, dst)
	require.NoError(t, err)
	assert.True(t, created)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "DATABASE_URL=x\n", string(got))
}

func TestEnsureEnv_NeverOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "example")
	dst := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(src, []byte("FROM_EXAMPLE=1\n"), 0o600))
	require.NoError(t, os.WriteFile(dst, []byte("MINE=1\n"), 0o600))

	created, err := ensureEnv(src, dst)
	require.NoError(t, err)
	assert.False(t, created)

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "MINE=1\n", string(got), "a developer's existing .env must be left alone")
}

func TestEnsureEnv_MissingExampleIsAnError(t *testing.T) {
	dir := t.TempDir()
	_, err := ensureEnv(filepath.Join(dir, "nope"), filepath.Join(dir, ".env"))
	require.Error(t, err)
}

func TestExtractTestSecretKey_PicksTheSandboxKey(t *testing.T) {
	out := strings.Join([]string{
		"merchant_id: 1234",
		"test secret key (shown once): sk_test_AbC-123_xyz",
		"test publishable key (shown once): pk_test_ignored",
	}, "\n")
	key, err := extractTestSecretKey(out)
	require.NoError(t, err)
	assert.Equal(t, "sk_test_AbC-123_xyz", key)
}

func TestExtractTestSecretKey_NeverReturnsALiveKey(t *testing.T) {
	out := "live secret key (shown once): sk_live_SHOULD_NOT_BE_USED"
	_, err := extractTestSecretKey(out)
	require.Error(t, err, "the quickstart must never surface a live credential")
}

func TestWaitHealthy_ReturnsOnceHealthy(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	require.NoError(t, waitHealthy(srv.URL, 5*time.Second, 5*time.Millisecond))
	assert.GreaterOrEqual(t, calls.Load(), int32(3))
}

func TestWaitHealthy_TimesOut(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	err := waitHealthy(srv.URL, 50*time.Millisecond, 5*time.Millisecond)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not become healthy")
}

func TestFirstPaymentSteps_ContainsWorkingCommands(t *testing.T) {
	out := firstPaymentSteps("sk_test_KEY")
	assert.Contains(t, out, "sk_test_KEY")
	assert.Contains(t, out, "/v1/payment_intents")
	assert.Contains(t, out, "pm_card_visa")
	assert.Contains(t, out, "4000000000000002", "decline card must be documented")
}
