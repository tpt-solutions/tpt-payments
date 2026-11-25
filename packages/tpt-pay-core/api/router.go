// Package api wires the HTTP router and all middleware.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/api/handlers"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ratelimit"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
)

// RouterOption adds optional routes to NewRouter.
type RouterOption func(*routerOptions)

type routerOptions struct {
	testClocks    *handlers.TestClocks
	subscriptions *handlers.Subscriptions
}

// WithSubscriptions serves the /v1/subscriptions routes.
func WithSubscriptions(svc handlers.SubscriptionService) RouterOption {
	return func(o *routerOptions) {
		o.subscriptions = handlers.NewSubscriptions(svc)
	}
}

// WithTestClocks serves the sandbox time-travel endpoints under /v1/test_clocks. Only the
// sandbox server enables them, and they accept test keys only.
func WithTestClocks(clk *clock.Test, outages handlers.OutageScheduler) RouterOption {
	return func(o *routerOptions) {
		o.testClocks = handlers.NewTestClocks(clk, outages)
	}
}

// NewRouter builds and returns the fully configured HTTP router.
//
// reg receives the HTTP metrics served at /metrics. Pass a fresh registry in tests.
func NewRouter(
	pool *pgxpool.Pool,
	paymentSvc *payment.Service,
	apiKeySvc *apikey.Service,
	secrets *secretbox.Box,
	reg *prometheus.Registry,
	limiter ratelimit.Limiter,
	opts ...RouterOption,
) http.Handler {
	var ro routerOptions
	for _, opt := range opts {
		opt(&ro)
	}
	r := chi.NewRouter()
	metrics := middleware.NewMetrics(reg)

	// ── Global middleware ─────────────────────────────────────────────────────
	// No RealIP: it trusts any client's X-Forwarded-For. Forwarded client addresses are only
	// believed from TRUSTED_PROXIES, applied by cmd/server around this router.
	r.Use(middleware.RequestID)
	r.Use(middleware.Tracing(otel.GetTracerProvider()))
	r.Use(chimw.Recoverer)
	r.Use(middleware.LimitBody)
	r.Use(metrics.Observe)

	// ── Operational endpoints (no authentication) ─────────────────────────────
	r.Get("/health", healthHandler)
	r.Get("/readyz", readyHandler(pool))
	r.Get("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}).ServeHTTP)
	r.Get("/openapi.json", openAPIHandler)

	// ── Authenticated API (all routes under /v1 require a valid API key) ──────
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(apiKeySvc))
		if limiter != nil {
			r.Use(middleware.RateLimit(limiter))
		}
		r.Use(middleware.Idempotency(pool))

		// Permissions: "secret" routes perform server-side operations and reject
		// publishable keys. Any valid key may retrieve an intent and confirm it, which is
		// what a browser checkout needs. Data access is always scoped to the key's mode.
		secret := middleware.RequireSecretKey
		// Every state-changing route writes an audit row on success.
		audit := middleware.NewAuditor(pool)

		// Payment intents
		pi := handlers.NewPaymentIntents(paymentSvc)
		r.With(secret, audit.Record("payment_intent.create", "payment_intent")).Post("/payment_intents", pi.Create)
		// Listing exposes every intent of the merchant, so it needs a secret key.
		r.With(secret).Get("/payment_intents", handlers.NewIntentList(paymentSvc).List)
		// Dry run of the merchant rules: shows what each rule would do, without creating anything.
		r.With(secret).Post("/policy/evaluate", handlers.NewPolicyEvaluator(paymentSvc).Evaluate)
		r.Get("/payment_intents/{id}", pi.Retrieve)
		r.With(audit.Record("payment_intent.confirm", "payment_intent")).Post("/payment_intents/{id}/confirm", pi.Confirm)
		r.With(secret, audit.Record("payment_intent.capture", "payment_intent")).Post("/payment_intents/{id}/capture", pi.Capture)
		r.With(secret, audit.Record("payment_intent.cancel", "payment_intent")).Post("/payment_intents/{id}/cancel", pi.Cancel)

		// Refunds
		rf := handlers.NewRefunds(paymentSvc)
		r.With(secret, audit.Record("refund.create", "refund")).Post("/refunds", rf.Create)
		// Listing exposes every refund of the merchant, so it needs a secret key.
		r.With(secret).Get("/refunds", handlers.NewRefundList(paymentSvc).List)
		r.Get("/refunds/{id}", rf.Retrieve)

		// Webhook endpoints
		we := handlers.NewWebhookEndpoints(pool, secrets)
		r.With(secret).Get("/webhook_endpoints", we.List)
		r.With(secret, audit.Record("webhook_endpoint.create", "webhook_endpoint")).Post("/webhook_endpoints", we.Create)
		r.With(secret).Get("/webhook_endpoints/{id}", we.Retrieve)
		r.With(secret, audit.Record("webhook_endpoint.delete", "webhook_endpoint")).Delete("/webhook_endpoints/{id}", we.Delete)

		// Delivery inspector: list what was sent, replay a delivery.
		wd := handlers.NewWebhookDeliveries(pool)
		r.With(secret).Get("/webhook_deliveries", wd.List)
		r.With(secret, audit.Record("webhook_delivery.replay", "webhook_delivery")).Post("/webhook_deliveries/{id}/replay", wd.Replay)

		// API keys
		ak := handlers.NewAPIKeys(apiKeySvc)
		r.With(secret).Get("/api_keys", ak.List)
		r.With(secret, audit.Record("api_key.create", "api_key")).Post("/api_keys", ak.Create)
		r.With(secret, audit.Record("api_key.revoke", "api_key")).Delete("/api_keys/{id}", ak.Revoke)

		// Subscriptions (only when the server enables them). Every route needs a secret key.
		if sc := ro.subscriptions; sc != nil {
			r.With(secret, audit.Record("subscription.create", "subscription")).Post("/subscriptions", sc.Create)
			r.With(secret).Get("/subscriptions", sc.List)
			r.With(secret).Get("/subscriptions/{id}", sc.Retrieve)
			r.With(secret, audit.Record("subscription.cancel", "subscription")).Post("/subscriptions/{id}/cancel", sc.Cancel)
		}

		// Sandbox time travel (only when the server enables it; test keys only).
		if tc := ro.testClocks; tc != nil {
			r.With(secret).Get("/test_clocks", tc.Get)
			r.With(secret, audit.Record("test_clock.advance", "test_clock")).Post("/test_clocks/advance", tc.Advance)
			r.With(secret, audit.Record("test_clock.outage_add", "test_clock")).Post("/test_clocks/outages", tc.AddOutage)
			r.With(secret, audit.Record("test_clock.outages_clear", "test_clock")).Delete("/test_clocks/outages", tc.ClearOutages)
		}
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	// Liveness: the process is up. It does not check dependencies.
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// readyHandler reports readiness: the database answers. Orchestrators use it to decide
// whether to route traffic here.
func readyHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		w.Header().Set("Content-Type", "application/json")
		if err := pool.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "unavailable", "database": "unreachable"})
			return
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready", "database": "ok"})
	}
}

func openAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(openAPISpec)
}
