package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// MaxRequestBody bounds every request body. Handlers never read more than this.
const MaxRequestBody = 1 << 20 // 1 MB

// LimitBody caps the request body. A larger body fails to read, and the decoder reports it
// as an invalid request, instead of the server buffering it.
func LimitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBody)
		next.ServeHTTP(w, r)
	})
}

// statusRecorder captures the status code written by a handler.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

// Metrics records request counts and latencies per route pattern, so high-cardinality
// path parameters (IDs) never become label values.
type Metrics struct {
	requests *prometheus.CounterVec
	latency  *prometheus.HistogramVec
}

// NewMetrics registers HTTP metrics with reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "tpt_http_requests_total",
			Help: "HTTP requests by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		latency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "tpt_http_request_duration_seconds",
			Help:    "HTTP request latency by method and route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
	}
	reg.MustRegister(m.requests, m.latency)
	return m
}

// Observe logs each request (method, route, status, latency, request id) and records metrics.
func (m *Metrics) Observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)

		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		route := routePattern(r)
		elapsed := time.Since(start)

		m.requests.WithLabelValues(r.Method, route, statusLabel(status)).Inc()
		m.latency.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		}
		attrs := append([]any{
			"method", r.Method,
			"route", route,
			"status", status,
			"duration_ms", elapsed.Milliseconds(),
		}, LogAttrs(r.Context())...)
		slog.Log(r.Context(), level, "request", attrs...)
	})
}

func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if pattern := rctx.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unmatched"
}

func statusLabel(status int) string {
	switch {
	case status >= 500:
		return "5xx"
	case status >= 400:
		return "4xx"
	default:
		return "2xx"
	}
}

