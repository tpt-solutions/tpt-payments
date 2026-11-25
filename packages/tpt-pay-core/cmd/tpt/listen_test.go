package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

const listenSecret = "whsec_listen_test"

func TestListenForwardsOnlyNewEventsOnceAndSignsThem(t *testing.T) {
	older := `{"id":"evt_old","type":"payment_intent.succeeded"}`
	newer := `{"id":"evt_new","type":"refund.created"}`

	// The fake API: the first poll shows only the existing event; later polls show the new one
	// twice (once per endpoint), newest first, as the real list does.
	var polls int
	var mu sync.Mutex
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		polls++
		n := polls
		mu.Unlock()
		assert.Equal(t, "/webhook_deliveries", r.URL.Path)
		assert.Equal(t, "payload", r.URL.Query().Get("include"))
		data := []map[string]any{{"event_id": "evt_old", "event_type": "payment_intent.succeeded", "payload": json.RawMessage(older)}}
		if n > 1 {
			data = []map[string]any{
				{"event_id": "evt_new", "event_type": "refund.created", "payload": json.RawMessage(newer)},
				{"event_id": "evt_new", "event_type": "refund.created", "payload": json.RawMessage(newer)},
				{"event_id": "evt_old", "event_type": "payment_intent.succeeded", "payload": json.RawMessage(older)},
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "list", "data": data, "has_more": false})
	}))
	defer api.Close()

	type received struct {
		body   []byte
		header string
		id     string
	}
	var got []received
	gotCh := make(chan struct{}, 10)
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{body: body, header: r.Header.Get(webhook.SignatureHeader), id: r.Header.Get("TPT-Event-Id")})
		mu.Unlock()
		gotCh <- struct{}{}
		w.WriteHeader(http.StatusOK)
	}))
	defer local.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out bytes.Buffer
	l := &listener{
		profile:   profile{APIKey: "sk_test_abc", BaseURL: api.URL},
		forwardTo: local.URL,
		secret:    listenSecret,
		interval:  10 * time.Millisecond,
		client:    &http.Client{Timeout: 5 * time.Second},
	}
	done := make(chan error, 1)
	go func() { done <- l.run(ctx, &out) }()

	select {
	case <-gotCh:
	case <-time.After(5 * time.Second):
		t.Fatal("no event was forwarded")
	}
	// Give the loop a few more polls: the duplicate must not be forwarded again.
	time.Sleep(60 * time.Millisecond)
	cancel()
	require.NoError(t, <-done)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1, "the existing event and the duplicate must not be forwarded")
	assert.Equal(t, newer, string(got[0].body))
	assert.Equal(t, "evt_new", got[0].id)
	assert.NoError(t, webhook.Verify(got[0].body, got[0].header, listenSecret, time.Minute),
		"forwarded events must verify with the secret the listener was given")
	assert.True(t, strings.Contains(out.String(), "forwarded refund.created (evt_new): HTTP 200"), out.String())
}

func TestListenRejectsBadFlags(t *testing.T) {
	ctx := context.Background()
	var out bytes.Buffer
	assert.Error(t, cmdListen(ctx, []string{}, &out), "forward and secret are required")
	assert.Error(t, cmdListen(ctx, []string{"--forward-to", "http://example.com/hook", "--secret", "whsec_x"}, &out),
		"plain http is refused for non-local hosts")
	assert.Error(t, cmdListen(ctx, []string{"--forward-to", "http://localhost:4242/hook", "--secret", "sk_x"}, &out))
	assert.Error(t, cmdListen(ctx, []string{"--forward-to", "http://localhost:4242/hook", "--secret", "whsec_x", "--interval", "100ms"}, &out))
}
