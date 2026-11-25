package api_test

// The list endpoints for webhook endpoints and API keys, through the router against PostgreSQL.
// Skipped unless TEST_DATABASE_URL is set.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
)

type listPage struct {
	Object  string           `json:"object"`
	HasMore bool             `json:"has_more"`
	Data    []map[string]any `json:"data"`
}

func decodePage(t *testing.T, body string) listPage {
	t.Helper()
	var page listPage
	require.NoError(t, json.Unmarshal([]byte(body), &page), body)
	return page
}

func ids(page listPage) []string {
	out := make([]string, 0, len(page.Data))
	for _, d := range page.Data {
		out = append(out, typed[string](d["id"]))
	}
	return out
}

func TestList_WebhookEndpointsPagesAndNeverReturnsSecrets(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	var secrets []string
	for i := 0; i < 3; i++ {
		rec := s.do(http.MethodPost, "/v1/webhook_endpoints", secret,
			`{"url":"https://merchant.example.com/hook"}`)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		var created struct {
			Secret string `json:"secret"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
		secrets = append(secrets, created.Secret)
	}

	first := s.do(http.MethodGet, "/v1/webhook_endpoints?limit=2", secret, "")
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())
	page1 := decodePage(t, first.Body.String())
	assert.Equal(t, "list", page1.Object)
	assert.Len(t, page1.Data, 2)
	assert.True(t, page1.HasMore)
	for _, sec := range secrets {
		assert.NotContains(t, first.Body.String(), sec, "signing secrets are never listed")
	}
	assert.NotContains(t, first.Body.String(), "\"secret\"")

	second := s.do(http.MethodGet, "/v1/webhook_endpoints?limit=2&starting_after="+typed[string](page1.Data[1]["id"]), secret, "")
	require.Equal(t, http.StatusOK, second.Code, second.Body.String())
	page2 := decodePage(t, second.Body.String())
	assert.Len(t, page2.Data, 1)
	assert.False(t, page2.HasMore)

	seen := map[string]bool{}
	for _, id := range append(ids(page1), ids(page2)...) {
		assert.False(t, seen[id], "no endpoint appears twice across pages")
		seen[id] = true
	}
	assert.Len(t, seen, 3)
}

func TestList_WebhookEndpointsScopedByMode(t *testing.T) {
	s := newStack(t)
	test := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	live := s.key(t, apikey.TypeSecret, apikey.ModeLive)

	rec := s.do(http.MethodPost, "/v1/webhook_endpoints", test, `{"url":"https://merchant.example.com/hook"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	inLive := decodePage(t, s.do(http.MethodGet, "/v1/webhook_endpoints", live, "").Body.String())
	assert.Empty(t, inLive.Data, "a live key does not see test endpoints")
	inTest := decodePage(t, s.do(http.MethodGet, "/v1/webhook_endpoints", test, "").Body.String())
	assert.Len(t, inTest.Data, 1)
}

func TestList_WebhookEndpointCursorFromAnotherModeIsNotFound(t *testing.T) {
	s := newStack(t)
	test := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	live := s.key(t, apikey.TypeSecret, apikey.ModeLive)

	rec := s.do(http.MethodPost, "/v1/webhook_endpoints", test, `{"url":"https://merchant.example.com/hook"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	out := s.do(http.MethodGet, "/v1/webhook_endpoints?starting_after="+created.ID, live, "")
	assert.Equal(t, http.StatusNotFound, out.Code, out.Body.String())
}

func TestList_APIKeysNeedASecretKeyAndShowOnlyTheirMode(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	publishable := s.key(t, apikey.TypePublishable, apikey.ModeTest)
	live := s.key(t, apikey.TypeSecret, apikey.ModeLive)

	assert.Equal(t, http.StatusForbidden, s.do(http.MethodGet, "/v1/api_keys", publishable, "").Code,
		"a publishable key cannot list keys")

	page := decodePage(t, s.do(http.MethodGet, "/v1/api_keys", secret, "").Body.String())
	assert.Len(t, page.Data, 2, "the secret and publishable test keys are listed")
	for _, d := range page.Data {
		assert.Equal(t, "test", d["mode"])
		assert.NotContains(t, d, "key_hash")
		assert.NotContains(t, d, "secret")
		assert.NotContains(t, d, "key")
	}

	liveOnly := decodePage(t, s.do(http.MethodGet, "/v1/api_keys", live, "").Body.String())
	assert.Len(t, liveOnly.Data, 1, "a live key sees only live keys")
}

func TestList_APIKeysShowRevocationAndRefuseForeignCursors(t *testing.T) {
	s := newStack(t)
	secret := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	// Revoke a second test key through the API, then list: it stays, with its status.
	other := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	listed := decodePage(t, s.do(http.MethodGet, "/v1/api_keys", secret, "").Body.String())
	var otherID string
	for _, d := range listed.Data {
		if d["prefix"] != "" && strings.HasPrefix(other, typed[string](d["prefix"])) {
			otherID = typed[string](d["id"])
		}
	}
	require.NotEmpty(t, otherID, "the second key is in the list")
	require.Equal(t, http.StatusOK, s.do(http.MethodDelete, "/v1/api_keys/"+otherID, secret, "").Code)

	after := decodePage(t, s.do(http.MethodGet, "/v1/api_keys", secret, "").Body.String())
	statuses := map[string]string{}
	for _, d := range after.Data {
		statuses[typed[string](d["id"])] = typed[string](d["status"])
	}
	assert.Equal(t, "revoked", statuses[otherID], "a revoked key stays listed, marked revoked")

	// A cursor that is not a key of this account and mode is refused.
	bogus := s.do(http.MethodGet, "/v1/api_keys?starting_after=key_does_not_exist", secret, "")
	assert.Equal(t, http.StatusBadRequest, bogus.Code, bogus.Body.String())
}
