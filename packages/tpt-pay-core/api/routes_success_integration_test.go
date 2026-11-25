package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
)

// decode reads a JSON response body into a map.
func decode(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(body, &out), string(body))
	return out
}

// The success paths of the routes that the other integration tests only touch on their
// failure or permission edges.

func TestRefunds_CreateRetrieveAndList(t *testing.T) {
	s := newStack(t)
	key := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	id := createIntent(t, s, key)
	rec := s.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", key, `{"payment_method":"pm_card_visa"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = s.do(http.MethodPost, "/v1/refunds", key, `{"payment_intent":"`+id+`","amount":400,"reason":"requested_by_customer"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	refund := decode(t, rec.Body.Bytes())
	refundID, _ := refund["id"].(string)
	require.NotEmpty(t, refundID)
	assert.EqualValues(t, 400, refund["amount"])

	rec = s.do(http.MethodGet, "/v1/refunds/"+refundID, key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, refundID, decode(t, rec.Body.Bytes())["id"])

	rec = s.do(http.MethodGet, "/v1/refunds?payment_intent="+id, key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	list := decode(t, rec.Body.Bytes())
	assert.Equal(t, "list", list["object"])
	assert.Len(t, list["data"], 1)
}

func TestWebhookEndpoints_CreateListRetrieveDelete(t *testing.T) {
	s := newStack(t)
	key := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	rec := s.do(http.MethodPost, "/v1/webhook_endpoints", key,
		`{"url":"https://example.com/hooks","enabled_events":["payment_intent.succeeded"]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	created := decode(t, rec.Body.Bytes())
	endpointID, _ := created["id"].(string)
	require.NotEmpty(t, endpointID)
	assert.NotEmpty(t, created["secret"], "the signing secret is returned on creation")

	rec = s.do(http.MethodGet, "/v1/webhook_endpoints", key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Len(t, decode(t, rec.Body.Bytes())["data"], 1)

	rec = s.do(http.MethodGet, "/v1/webhook_endpoints/"+endpointID, key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	got := decode(t, rec.Body.Bytes())
	assert.Equal(t, "https://example.com/hooks", got["url"])
	assert.Nil(t, got["secret"], "the secret is never returned after creation")

	rec = s.do(http.MethodDelete, "/v1/webhook_endpoints/"+endpointID, key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	// Deleting disables the endpoint rather than removing it, so its delivery history stays readable.
	rec = s.do(http.MethodGet, "/v1/webhook_endpoints/"+endpointID, key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, "disabled", decode(t, rec.Body.Bytes())["status"])
}

func TestAPIKeys_CreateListAndRevokeThroughTheApi(t *testing.T) {
	s := newStack(t)
	key := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	rec := s.do(http.MethodPost, "/v1/api_keys", key, `{"type":"secret","mode":"test","name":"worker"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	created := decode(t, rec.Body.Bytes())
	newKey, _ := created["secret"].(string)
	keyID, _ := created["id"].(string)
	require.NotEmpty(t, newKey, "the raw key is returned once on creation")

	// The new key works.
	rec = s.do(http.MethodGet, "/v1/payment_intents", newKey, "")
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = s.do(http.MethodGet, "/v1/api_keys", key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	for _, k := range typed[[]any](decode(t, rec.Body.Bytes())["data"]) {
		assert.NotContains(t, k, "secret", "listing never returns a secret")
		assert.NotContains(t, k, "key_digest", "listing never returns the digest")
	}

	rec = s.do(http.MethodDelete, "/v1/api_keys/"+keyID, key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = s.do(http.MethodGet, "/v1/payment_intents", newKey, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "a revoked key stops working")
}

func TestWebhookDeliveries_ListAndReplayThroughTheApi(t *testing.T) {
	s := newStack(t)
	key := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	rec := s.do(http.MethodPost, "/v1/webhook_endpoints", key,
		`{"url":"https://example.com/hooks","enabled_events":["*"]}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	createIntent(t, s, key) // queues a payment_intent.created delivery

	rec = s.do(http.MethodGet, "/v1/webhook_deliveries?limit=10", key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	page := decode(t, rec.Body.Bytes())
	data := typed[[]any](page["data"])
	require.NotEmpty(t, data)
	first := typed[map[string]any](data[0])
	assert.NotContains(t, first, "payload", "payloads are only returned with include=payload")

	rec = s.do(http.MethodGet, "/v1/webhook_deliveries?include=payload&limit=10", key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	withPayload := typed[map[string]any](typed[[]any](decode(t, rec.Body.Bytes())["data"])[0])
	assert.Contains(t, withPayload, "payload")

	rec = s.do(http.MethodPost, "/v1/webhook_deliveries/"+typed[string](first["id"])+"/replay", key, `{}`)
	assert.Equal(t, http.StatusAccepted, rec.Code, rec.Body.String())
}
