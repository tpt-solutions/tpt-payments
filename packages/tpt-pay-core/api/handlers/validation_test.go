package handlers_test

// Request validation for the write handlers. Each case is rejected before the handler touches
// the payment service or the database: the services here are built with a nil pool, so a call
// that reached the database would panic and fail the test.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/api/handlers"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

func post(t *testing.T, h http.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func TestPaymentIntentCreate_RejectsBeforeService(t *testing.T) {
	h := handlers.NewPaymentIntents(payment.NewService(nil, nil, nil)).Create
	cases := map[string]string{
		"malformed json":     `{"amount":`,
		"unknown field":      `{"amount":100,"currency":"USD","bogus":true}`,
		"trailing data":      `{"amount":100,"currency":"USD"} {}`,
		"bad capture_method": `{"amount":100,"currency":"USD","capture_method":"later"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rr := post(t, h, body)
			assert.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		})
	}
}

func TestRefundCreate_RejectsBeforeService(t *testing.T) {
	h := handlers.NewRefunds(payment.NewService(nil, nil, nil)).Create

	rr := post(t, h, `{"amount":100}`)
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "payment_intent", "the missing parameter is named")

	rr = post(t, h, `{"payment_intent":`)
	assert.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestWebhookEndpointCreate_RejectsBadURLAndEvents(t *testing.T) {
	h := handlers.NewWebhookEndpoints(nil, nil).Create

	// Test-mode endpoints may be loopback; a plain-http public URL is refused.
	rr := post(t, h, `{"url":"http://example.com/hook"}`)
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "url")

	rr = post(t, h, `{"url":"http://127.0.0.1:9/hook","enabled_events":["not.a.real.event"]}`)
	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "enabled_events")
}

func TestAPIKeyCreate_RequiresAuthenticatedCaller(t *testing.T) {
	// Without an authenticated key in the context the handler must refuse before using the service.
	h := handlers.NewAPIKeys(nil).Create
	rr := post(t, h, `{"type":"secret"}`)
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}
