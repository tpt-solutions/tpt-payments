package api_test

// HTTP-level tests for authentication and permissions. Skipped unless TEST_DATABASE_URL
// is set (see internal/testdb).

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/api"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

// testSecrets returns a box with a fresh random key.
func testSecrets(t *testing.T) *secretbox.Box {
	t.Helper()
	raw := make([]byte, 32)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	box, err := secretbox.New(base64.StdEncoding.EncodeToString(raw))
	require.NoError(t, err)
	return box
}

type stack struct {
	handler    http.Handler
	pool       *pgxpool.Pool
	keys       *apikey.Service
	merchantID string
}

func newStack(t *testing.T) *stack {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	var merchantID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Test Merchant', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid",
	).Scan(&merchantID))

	keys := apikey.NewService(pool)
	paySvc := payment.NewService(pool, sandbox.Default(), ledger.New(pool))
	return &stack{
		handler:    api.NewRouter(pool, paySvc, keys, testSecrets(t), prometheus.NewRegistry()),
		pool:       pool,
		keys:       keys,
		merchantID: merchantID.String(),
	}
}

// key creates a key of the given type and mode and returns its raw value.
func (s *stack) key(t *testing.T, typ apikey.KeyType, mode apikey.Mode) string {
	t.Helper()
	res, err := s.keys.Generate(context.Background(), s.merchantID, "test", typ, mode)
	require.NoError(t, err)
	return res.RawKey
}

func (s *stack) do(method, path, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func createIntent(t *testing.T, s *stack, key string) string {
	t.Helper()
	rec := s.do(http.MethodPost, "/v1/payment_intents", key, `{"amount":1000,"currency":"usd"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	return out.ID
}

func TestAuth_MissingAndUnknownKeysAreRejected(t *testing.T) {
	s := newStack(t)
	assert.Equal(t, http.StatusUnauthorized, s.do(http.MethodGet, "/v1/payment_intents/pi_x", "", "").Code)
	assert.Equal(t, http.StatusUnauthorized,
		s.do(http.MethodGet, "/v1/payment_intents/pi_x", "sk_test_thisisnotarealkey", "").Code)
}

func TestAuth_RevokedKeyIsRejected(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	require.Equal(t, http.StatusOK, s.do(http.MethodPost, "/v1/payment_intents", secret,
		`{"amount":100,"currency":"usd"}`).Code)

	res, err := s.keys.Generate(context.Background(), s.merchantID, "to revoke", apikey.TypeSecret, apikey.ModeTest)
	require.NoError(t, err)
	require.NoError(t, s.keys.Revoke(context.Background(), res.Key.ID, s.merchantID))

	assert.Equal(t, http.StatusUnauthorized,
		s.do(http.MethodGet, "/v1/payment_intents/pi_x", res.RawKey, "").Code)
}

func TestPermission_PublishableKeyCannotCreateIntents(t *testing.T) {
	s := newStack(t)
	pk := s.key(t, apikey.TypePublishable, apikey.ModeTest)
	rec := s.do(http.MethodPost, "/v1/payment_intents", pk, `{"amount":100,"currency":"usd"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Contains(t, rec.Body.String(), "permission_denied")
}

func TestPermission_PublishableKeyCannotManageKeysOrWebhooks(t *testing.T) {
	s := newStack(t)
	pk := s.key(t, apikey.TypePublishable, apikey.ModeTest)
	assert.Equal(t, http.StatusForbidden,
		s.do(http.MethodPost, "/v1/api_keys", pk, `{"type":"secret"}`).Code)
	assert.Equal(t, http.StatusForbidden,
		s.do(http.MethodPost, "/v1/webhook_endpoints", pk, `{"url":"https://example.com/hook"}`).Code)
}

func TestPermission_PublishableKeyCanRetrieveAndConfirm(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	pk := s.key(t, apikey.TypePublishable, apikey.ModeTest)
	id := createIntent(t, s, secret)

	assert.Equal(t, http.StatusOK, s.do(http.MethodGet, "/v1/payment_intents/"+id, pk, "").Code)
	rec := s.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", pk, `{"payment_method":"pm_card_visa"}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

func TestPermission_TestKeyCannotMintLiveKey(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	rec := s.do(http.MethodPost, "/v1/api_keys", secret, `{"type":"secret","mode":"live"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec = s.do(http.MethodPost, "/v1/api_keys", secret, `{"type":"secret"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		Mode   string `json:"mode"`
		Secret string `json:"secret"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "test", out.Mode, "keys default to the calling key's mode")
	assert.True(t, strings.HasPrefix(out.Secret, "sk_test_"))
}

func TestMode_LiveAndTestDataAreIsolated(t *testing.T) {
	s := newStack(t)
	testKey := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	liveKey := s.key(t, apikey.TypeSecret, apikey.ModeLive)

	testIntent := createIntent(t, s, testKey)
	liveIntent := createIntent(t, s, liveKey)

	assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, "/v1/payment_intents/"+liveIntent, testKey, "").Code,
		"a test key must not see live intents")
	assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, "/v1/payment_intents/"+testIntent, liveKey, "").Code,
		"a live key must not see test intents")

	rec := s.do(http.MethodGet, "/v1/payment_intents/"+liveIntent, liveKey, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, true, out["livemode"])
}

// ── Idempotency ───────────────────────────────────────────────────────────────

func (s *stack) doWithKey(method, path, key, body, idempotencyKey string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec
}

func (s *stack) intentCount(t *testing.T) int {
	t.Helper()
	var n int
	require.NoError(t, s.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM payment_intents WHERE merchant_id = $1`, s.merchantID).Scan(&n))
	return n
}

func TestIdempotency_ConcurrentDuplicatesCreateOneIntent(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	const body = `{"amount":2500,"currency":"usd"}`

	const workers = 15
	ids := make([]string, workers)
	codes := make([]int, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec := s.doWithKey(http.MethodPost, "/v1/payment_intents", secret, body, "order-42")
			codes[i] = rec.Code
			var out struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			ids[i] = out.ID
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 1, s.intentCount(t), "concurrent duplicates must create exactly one intent")
	var first string
	for i := range ids {
		switch codes[i] {
		case http.StatusOK:
			if first == "" {
				first = ids[i]
			}
			assert.Equal(t, first, ids[i], "every success must return the same intent")
		case http.StatusConflict:
			// Still in flight when this duplicate arrived; acceptable.
		default:
			t.Fatalf("unexpected status %d", codes[i])
		}
	}
	assert.NotEmpty(t, first, "at least one request must succeed")
}

func TestIdempotency_ReplayReturnsStoredResponse(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	const body = `{"amount":900,"currency":"usd"}`

	first := s.doWithKey(http.MethodPost, "/v1/payment_intents", secret, body, "replay-1")
	require.Equal(t, http.StatusOK, first.Code)
	second := s.doWithKey(http.MethodPost, "/v1/payment_intents", secret, body, "replay-1")

	assert.Equal(t, http.StatusOK, second.Code)
	assert.Equal(t, "true", second.Header().Get("Idempotent-Replayed"))
	assert.Equal(t, first.Body.String(), second.Body.String())
	assert.Equal(t, 1, s.intentCount(t))
}

func TestIdempotency_KeyReuseWithDifferentBodyIsRejected(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	require.Equal(t, http.StatusOK, s.doWithKey(http.MethodPost, "/v1/payment_intents", secret,
		`{"amount":900,"currency":"usd"}`, "reuse-1").Code)

	rec := s.doWithKey(http.MethodPost, "/v1/payment_intents", secret,
		`{"amount":9999,"currency":"usd"}`, "reuse-1")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "idempotency_key_in_use")
	assert.Equal(t, 1, s.intentCount(t))
}

func TestIdempotency_KeyReuseOnAnotherPathIsRejected(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	id := createIntent(t, s, secret)

	// Same empty body on two different endpoints must not replay across them.
	require.Equal(t, http.StatusOK, s.doWithKey(http.MethodPost, "/v1/payment_intents/"+id+"/cancel", secret, `{}`, "path-1").Code)
	rec := s.doWithKey(http.MethodPost, "/v1/payment_intents/"+id+"/capture", secret, `{}`, "path-1")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestIdempotency_OversizedBodyIsRejectedNotTruncated(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	big := `{"amount":100,"currency":"usd","metadata":{"blob":"` + strings.Repeat("x", 1<<20) + `"}}`
	rec := s.doWithKey(http.MethodPost, "/v1/payment_intents", secret, big, "big-1")
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, 0, s.intentCount(t))
}

// ── Webhook endpoints ─────────────────────────────────────────────────────────

func TestWebhookEndpoint_SecretReturnedOnceAndStoredEncrypted(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	rec := s.do(http.MethodPost, "/v1/webhook_endpoints", secret,
		`{"url":"http://localhost:3000/hook","enabled_events":["payment_intent.succeeded"]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var out struct {
		ID     string `json:"id"`
		Secret string `json:"secret"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	require.True(t, strings.HasPrefix(out.Secret, "whsec_"))

	var stored string
	require.NoError(t, s.pool.QueryRow(context.Background(),
		`SELECT secret_ciphertext FROM webhook_endpoints WHERE id = $1`, out.ID).Scan(&stored))
	assert.True(t, strings.HasPrefix(stored, "v1:"), "secret must be stored encrypted")
	assert.NotContains(t, stored, out.Secret)

	get := s.do(http.MethodGet, "/v1/webhook_endpoints/"+out.ID, secret, "")
	require.Equal(t, http.StatusOK, get.Code)
	assert.NotContains(t, get.Body.String(), out.Secret, "secret must not be returned again")
}

func TestWebhookEndpoint_ValidationRejectsUnsafeTargets(t *testing.T) {
	s := newStack(t)
	live := s.key(t, apikey.TypeSecret, apikey.ModeLive)

	rec := s.do(http.MethodPost, "/v1/webhook_endpoints", live,
		`{"url":"http://merchant.example.com/hook"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "live endpoints must use https")

	rec = s.do(http.MethodPost, "/v1/webhook_endpoints", live,
		`{"url":"https://169.254.169.254/latest"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "link-local targets must be refused")

	test := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	rec = s.do(http.MethodPost, "/v1/webhook_endpoints", test,
		`{"url":"https://merchant.example.com/hook","enabled_events":["payment_intent.typo"]}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "unknown event types must be refused")
}

func TestWebhookEndpoint_ScopedByMode(t *testing.T) {
	s := newStack(t)
	test := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	live := s.key(t, apikey.TypeSecret, apikey.ModeLive)

	rec := s.do(http.MethodPost, "/v1/webhook_endpoints", test, `{"url":"https://merchant.example.com/hook"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var out struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))

	assert.Equal(t, http.StatusNotFound, s.do(http.MethodGet, "/v1/webhook_endpoints/"+out.ID, live, "").Code)
	assert.Equal(t, http.StatusOK, s.do(http.MethodGet, "/v1/webhook_endpoints/"+out.ID, test, "").Code)
}

// ── Hardening and operations ──────────────────────────────────────────────────

func TestOps_ReadyzReportsDatabaseState(t *testing.T) {
	s := newStack(t)
	rec := s.do(http.MethodGet, "/readyz", "", "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"ready"`)
}

func TestOps_MetricsCountRequestsByRoutePattern(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	id := createIntent(t, s, secret)
	require.Equal(t, http.StatusOK, s.do(http.MethodGet, "/v1/payment_intents/"+id, secret, "").Code)

	rec := s.do(http.MethodGet, "/metrics", "", "")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, `tpt_http_requests_total{method="GET",route="/v1/payment_intents/{id}",status="2xx"}`)
	assert.NotContains(t, body, id, "resource IDs must not become metric labels")
}

func TestHardening_TrailingJSONIsRejected(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	rec := s.do(http.MethodPost, "/v1/payment_intents", secret, `{"amount":100,"currency":"usd"} {"x":1}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHardening_MalformedCaptureBodyIsRejected(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	id := createIntent(t, s, secret)
	rec := s.do(http.MethodPost, "/v1/payment_intents/"+id+"/capture", secret, `{"amount_to_capture":`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a malformed body must not silently mean full capture")
}

func TestHardening_UnknownCurrencyAndCaptureMethodAreRejected(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	rec := s.do(http.MethodPost, "/v1/payment_intents", secret, `{"amount":100,"currency":"usdx"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	assert.Contains(t, rec.Body.String(), "currency")

	rec = s.do(http.MethodPost, "/v1/payment_intents", secret, `{"amount":100,"currency":"usd","capture_method":"sometimes"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestHardening_OversizedBodyFromLimitBodyIsRejected(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	// No Idempotency-Key, so the idempotency middleware's own check is not involved.
	big := `{"amount":100,"currency":"usd","metadata":{"blob":"` + strings.Repeat("x", 2<<20) + `"}}`
	rec := s.do(http.MethodPost, "/v1/payment_intents", secret, big)
	assert.NotEqual(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, s.intentCount(t))
}
