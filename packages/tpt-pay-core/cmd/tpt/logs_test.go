package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogs_PrintsOnePagePerDelivery(t *testing.T) {
	isolateConfig(t)
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.RequestURI()
		_, _ = w.Write([]byte(`{"object":"list","has_more":true,"data":[
			{"id":"wd_1","event_type":"payment_intent.succeeded","status":"failed","attempts":15,"last_http_status":500,"last_error":"HTTP 500"},
			{"id":"wd_2","event_type":"charge.refunded","status":"delivered","attempts":1,"last_http_status":null,"last_error":""}
		]}`))
	}))
	defer srv.Close()
	t.Setenv("TPT_API_KEY", "sk_test_x")
	t.Setenv("TPT_BASE_URL", srv.URL+"/v1")

	var out bytes.Buffer
	require.NoError(t, run([]string{"logs", "--limit", "2", "--endpoint", "we_9"}, &out, &out))
	assert.Equal(t, "/v1/webhook_deliveries?limit=2&endpoint_id=we_9", gotPath)
	text := out.String()
	assert.Contains(t, text, "wd_1")
	assert.Contains(t, text, "failed")
	assert.Contains(t, text, "http=500")
	assert.Contains(t, text, "wd_2")
	assert.Contains(t, text, "more deliveries exist")
}

func TestReplay_PostsToTheDeliveryAndReportsQueued(t *testing.T) {
	isolateConfig(t)
	var method, path, auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, path, auth = r.Method, r.URL.Path, r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"id":"wd_1","status":"pending"}`))
	}))
	defer srv.Close()
	t.Setenv("TPT_API_KEY", "sk_test_x")
	t.Setenv("TPT_BASE_URL", srv.URL+"/v1")

	var out bytes.Buffer
	require.NoError(t, run([]string{"replay", "wd_1"}, &out, &out))
	assert.Equal(t, http.MethodPost, method)
	assert.Equal(t, "/v1/webhook_deliveries/wd_1/replay", path)
	assert.Equal(t, "Bearer sk_test_x", auth)
	assert.True(t, strings.Contains(out.String(), "queued delivery wd_1"))
}

func TestReplay_ShowsNotFound(t *testing.T) {
	isolateConfig(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"No such webhook_delivery"}}`))
	}))
	defer srv.Close()
	t.Setenv("TPT_API_KEY", "sk_test_x")
	t.Setenv("TPT_BASE_URL", srv.URL+"/v1")

	var out bytes.Buffer
	err := run([]string{"replay", "wd_missing"}, &out, &out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No such webhook_delivery")
}
