package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

const secret = "whsec_test_cli_secret"

// isolateConfig points every config location at a temp directory, so tests never touch the
// developer's profile.
func isolateConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("TPT_API_KEY", "")
	t.Setenv("TPT_BASE_URL", "")
	return dir
}

// The CLI's event list must match what the platform sends.
func TestEventList_MatchesKnownEventTypes(t *testing.T) {
	var known []string
	for _, e := range webhook.KnownEventTypes() {
		known = append(known, string(e))
	}
	assert.ElementsMatch(t, known, eventTypes)
}

func TestTrigger_SignatureVerifiesWithTheCoreVerifier(t *testing.T) {
	isolateConfig(t)
	var mu sync.Mutex
	var gotBody []byte
	var gotHeader, gotEventID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody, _ = io.ReadAll(r.Body)
		gotHeader = r.Header.Get(webhook.SignatureHeader)
		gotEventID = r.Header.Get("TPT-Event-Id")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := run([]string{"trigger", "payment_intent.succeeded", "--forward-to", srv.URL, "--secret", secret}, &out, &out)
	require.NoError(t, err)
	assert.Contains(t, out.String(), "HTTP 200")

	mu.Lock()
	defer mu.Unlock()
	require.NoError(t, webhook.Verify(gotBody, gotHeader, secret, 5*time.Minute),
		"a triggered event must verify with the same code the platform uses to sign")
	var event map[string]any
	require.NoError(t, json.Unmarshal(gotBody, &event))
	assert.Equal(t, "payment_intent.succeeded", event["type"])
	assert.Equal(t, event["id"], gotEventID)
}

func TestTrigger_ReportsNon2xxAsAnError(t *testing.T) {
	isolateConfig(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var out bytes.Buffer
	err := run([]string{"trigger", "charge.refunded", "--forward-to", srv.URL, "--secret", secret}, &out, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 500")
}

func TestTrigger_ValidatesArguments(t *testing.T) {
	isolateConfig(t)
	var out bytes.Buffer
	cases := [][]string{
		{"trigger", "refund.created", "--forward-to", "http://localhost:1", "--secret", secret},
		{"trigger", "payment_intent.created", "--forward-to", "http://example.com/hook", "--secret", secret},
		{"trigger", "payment_intent.created", "--forward-to", "http://localhost:1", "--secret", "nope"},
		{"trigger", "payment_intent.created"},
	}
	for _, args := range cases {
		assert.Error(t, run(args, &out, &out), "args %v should be refused", args)
	}
}

func TestLogin_RefusesNonKeysAndRemoteHTTP(t *testing.T) {
	isolateConfig(t)
	var out bytes.Buffer
	assert.Error(t, run([]string{"login", "--api-key", "Bearer xyz"}, &out, &out))
	assert.Error(t, run([]string{"login", "--api-key", "sk_test_abc", "--base-url", "http://api.example.com/v1"}, &out, &out))
}

func TestLogin_SavesProfileAndKeysCreateUsesIt(t *testing.T) {
	isolateConfig(t)
	var gotAuth string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		if r.URL.Path != "/v1/api_keys" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"key_1","secret":"sk_test_new"}`))
	}))
	defer srv.Close()

	var out bytes.Buffer
	require.NoError(t, run([]string{"login", "--api-key", "sk_test_saved", "--base-url", srv.URL + "/v1"}, &out, &out))

	path, err := configPath()
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(data), "sk_test_saved"))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "credentials file is private")
	}

	out.Reset()
	require.NoError(t, run([]string{"keys", "create", "--name", "ci", "--type", "secret"}, &out, &out))
	assert.Equal(t, "Bearer sk_test_saved", gotAuth)
	assert.Equal(t, "ci", gotBody["name"])
	assert.Contains(t, out.String(), "sk_test_new")
}

func TestKeysCreate_ShowsAPIMessageOnError(t *testing.T) {
	isolateConfig(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"A test key cannot create a live key."}}`))
	}))
	defer srv.Close()
	t.Setenv("TPT_API_KEY", "sk_test_x")
	t.Setenv("TPT_BASE_URL", srv.URL+"/v1")

	var out bytes.Buffer
	err := run([]string{"keys", "create", "--mode", "live"}, &out, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
	assert.Contains(t, err.Error(), "cannot create a live key")
}

func TestEventsCommandListsTheTypes(t *testing.T) {
	var out bytes.Buffer
	require.NoError(t, run([]string{"events"}, &out, &out))
	for _, e := range eventTypes {
		assert.Contains(t, out.String(), e)
	}

}
