package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// exchange sends newline-delimited requests through Serve and returns the decoded responses.
func exchange(t *testing.T, cfg Config, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	require.NoError(t, Serve(context.Background(), cfg, strings.NewReader(strings.Join(lines, "\n")+"\n"), &out))
	var resps []map[string]any
	dec := json.NewDecoder(&out)
	for dec.More() {
		var m map[string]any
		require.NoError(t, dec.Decode(&m))
		resps = append(resps, m)
	}
	return resps
}

func toolText(t *testing.T, resp map[string]any) (text string, isError bool) {
	t.Helper()
	result := resp["result"].(map[string]any)
	content := result["content"].([]any)
	return content[0].(map[string]any)["text"].(string), result["isError"].(bool)
}

func TestInitializeAndNotifications(t *testing.T) {
	resps := exchange(t, Config{},
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`, // no id: no response
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`,
	)
	require.Len(t, resps, 2, "a notification must not produce a response")
	result := resps[0]["result"].(map[string]any)
	assert.Equal(t, protocolVersion, result["protocolVersion"])
	assert.Equal(t, "tpt-mcp", result["serverInfo"].(map[string]any)["name"])
}

func TestToolsList_ReadDocsOnlyWhenConfigured(t *testing.T) {
	names := func(cfg Config) []string {
		resp := exchange(t, cfg, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)[0]
		var out []string
		for _, tool := range resp["result"].(map[string]any)["tools"].([]any) {
			out = append(out, tool.(map[string]any)["name"].(string))
		}
		return out
	}
	assert.NotContains(t, names(Config{}), "read_docs")
	assert.Contains(t, names(Config{DocsPath: "llms.txt"}), "read_docs")
	assert.Contains(t, names(Config{}), "create_test_payment")
}

func TestListTestPaymentMethods_MatchesSandbox(t *testing.T) {
	resp := exchange(t, Config{}, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_test_payment_methods"}}`)[0]
	text, isErr := toolText(t, resp)
	require.False(t, isErr)
	assert.Contains(t, text, "pm_card_visa: approves")
	assert.Contains(t, text, "pm_card_declined (also pm_test_declined): declines with card_declined")
	assert.Contains(t, text, "pm_card_slow (also pm_test_slow): approves after 2s")
}

func TestWebhookEventTypes_ListsCatalog(t *testing.T) {
	resp := exchange(t, Config{}, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_webhook_event_types"}}`)[0]
	text, isErr := toolText(t, resp)
	require.False(t, isErr)
	assert.Contains(t, text, "payment_intent.succeeded")
	assert.Contains(t, text, "charge.refunded")
}

func TestCreateTestPayment_RefusesLiveKeyWithoutCallingAPI(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer srv.Close()

	resp := exchange(t, Config{APIBase: srv.URL, APIKey: "sk_live_abc"},
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_test_payment","arguments":{"amount":1000}}}`)[0]
	text, isErr := toolText(t, resp)
	assert.True(t, isErr)
	assert.Contains(t, text, "not a test secret key")
	assert.Zero(t, calls, "no request may reach the API with a live key")
}

func TestCreateTestPayment_RefusesNonSandboxTokenWithoutCallingAPI(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer srv.Close()

	resp := exchange(t, Config{APIBase: srv.URL, APIKey: "sk_test_abc"},
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_test_payment","arguments":{"amount":1000,"payment_method":"pm_real_card_123"}}}`)[0]
	_, isErr := toolText(t, resp)
	assert.True(t, isErr)
	assert.Zero(t, calls)
}

func TestCreateTestPayment_CreatesThenConfirmsWithTestKey(t *testing.T) {
	var mu sync.Mutex
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		seen = append(seen, r.URL.Path+" "+r.Header.Get("Authorization")+" "+string(body))
		mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/confirm") {
			_, _ = w.Write([]byte(`{"id":"pi_1","status":"requires_capture"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"pi_1","status":"requires_payment_method"}`))
	}))
	defer srv.Close()

	resp := exchange(t, Config{APIBase: srv.URL + "/v1", APIKey: "sk_test_abc"},
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_test_payment","arguments":{"amount":2500,"currency":"eur","payment_method":"pm_card_declined"}}}`)[0]
	text, isErr := toolText(t, resp)
	require.False(t, isErr, text)
	assert.JSONEq(t, `{"id":"pi_1","status":"requires_capture"}`, text)

	require.Len(t, seen, 2)
	assert.True(t, strings.HasPrefix(seen[0], "/v1/payment_intents Bearer sk_test_abc "))
	assert.Contains(t, seen[0], `"amount":2500`)
	assert.Contains(t, seen[0], `"currency":"EUR"`)
	assert.True(t, strings.HasPrefix(seen[1], "/v1/payment_intents/pi_1/confirm "))
	assert.Contains(t, seen[1], `"payment_method":"pm_card_declined"`)
}

func TestReadDocs_ReturnsFileOrReportsMissing(t *testing.T) {
	path := t.TempDir() + "/llms.txt"
	require.NoError(t, os.WriteFile(path, []byte("# TPT Pay index"), 0o600))

	resp := exchange(t, Config{DocsPath: path}, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_docs"}}`)[0]
	text, isErr := toolText(t, resp)
	assert.False(t, isErr)
	assert.Equal(t, "# TPT Pay index", text)

	resp = exchange(t, Config{DocsPath: path + ".missing"}, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_docs"}}`)[0]
	_, isErr = toolText(t, resp)
	assert.True(t, isErr)
}

func TestProtocolErrors(t *testing.T) {
	resps := exchange(t, Config{},
		`{not json`,
		`{"jsonrpc":"2.0","id":7,"method":"no/such"}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{}}`,
	)
	require.Len(t, resps, 3)
	assert.Equal(t, float64(-32700), resps[0]["error"].(map[string]any)["code"])
	assert.Equal(t, float64(-32601), resps[1]["error"].(map[string]any)["code"])
	assert.Equal(t, float64(-32602), resps[2]["error"].(map[string]any)["code"])
}
