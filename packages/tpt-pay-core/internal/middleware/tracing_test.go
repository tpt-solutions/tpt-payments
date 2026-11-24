package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
)

func newRecordingProvider() (*sdktrace.TracerProvider, *tracetest.SpanRecorder) {
	rec := tracetest.NewSpanRecorder()
	return sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)), rec
}

func TestTracing_NamesSpanByRoutePatternNotRawPath(t *testing.T) {
	tp, rec := newRecordingProvider()
	r := chi.NewRouter()
	r.Use(middleware.Tracing(tp))
	r.Get("/v1/payment_intents/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v1/payment_intents/pi_secret123?expand=x", nil))

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "GET /v1/payment_intents/{id}", spans[0].Name())
	assert.NotContains(t, spans[0].Name(), "pi_secret123", "IDs must not become span names")
	assert.NotContains(t, attrString(spans[0].Attributes(), "http.route"), "pi_secret123")
}

func TestTracing_RecordsStatusAndMarksServerErrors(t *testing.T) {
	tp, rec := newRecordingProvider()
	r := chi.NewRouter()
	r.Use(middleware.Tracing(tp))
	r.Post("/v1/refunds", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	r.Get("/v1/missing-ok", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) })

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/refunds", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/missing-ok", nil))

	spans := rec.Ended()
	require.Len(t, spans, 2)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
	assert.Equal(t, int64(500), attrInt(spans[0].Attributes(), "http.response.status_code"))
	assert.Equal(t, codes.Unset, spans[1].Status().Code, "a 4xx is the caller's error, not the server's")
}

func TestTracing_ContinuesIncomingTraceContext(t *testing.T) {
	tp, rec := newRecordingProvider()
	r := chi.NewRouter()
	r.Use(middleware.Tracing(tp))
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	r.ServeHTTP(httptest.NewRecorder(), req)

	spans := rec.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "4bf92f3577b34da6a3ce929d0e0e4736", spans[0].SpanContext().TraceID().String())
	assert.Equal(t, "00f067aa0ba902b7", spans[0].Parent().SpanID().String())
}

func attrString(attrs []attribute.KeyValue, key string) string {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsString()
		}
	}
	return ""
}

func attrInt(attrs []attribute.KeyValue, key string) int64 {
	for _, a := range attrs {
		if string(a.Key) == key {
			return a.Value.AsInt64()
		}
	}
	return -1
}
