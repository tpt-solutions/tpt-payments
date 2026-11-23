package middleware_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ratelimit"
)

type failingLimiter struct{}

func (failingLimiter) Allow(context.Context, string) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, errors.New("redis down")
}

func ok() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func call(h http.Handler) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/v1/payment_intents/pi_1", nil)
	req.RemoteAddr = "203.0.113.7:4444"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRateLimit_RejectsOverLimitWith429(t *testing.T) {
	lim, err := ratelimit.NewMemory(2, time.Minute)
	require.NoError(t, err)
	h := middleware.RateLimit(lim)(ok())

	assert.Equal(t, http.StatusOK, call(h).Code)
	assert.Equal(t, http.StatusOK, call(h).Code)

	rec := call(h)
	require.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("Retry-After"))
	assert.Equal(t, "2", rec.Header().Get("X-RateLimit-Limit"))

	var body apierrors.Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, apierrors.CodeRateLimitExceeded, body.Error.Code)
	assert.Equal(t, apierrors.ErrorTypeRateLimit, body.Error.Type)
}

func TestRateLimit_FailsOpenWhenLimiterIsDown(t *testing.T) {
	h := middleware.RateLimit(failingLimiter{})(ok())
	assert.Equal(t, http.StatusOK, call(h).Code, "a limiter outage must not become an API outage")
}
