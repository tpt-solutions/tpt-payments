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

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/api/handlers"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ratelimit"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/secretbox"
)

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
) http.Handler {
	r := chi.NewRouter()
	metrics := middleware.NewMetrics(reg)

	// ── Global middleware ─────────────────────────────────────────────────────
	// No RealIP: it trusts any client's X-Forwarded-For. Client addresses come from the
	// connection until trusted proxies are configured.
	r.Use(middleware.RequestID)
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
		r.Get("/refunds/{id}", rf.Retrieve)

		// Webhook endpoints
		we := handlers.NewWebhookEndpoints(pool, secrets)
		r.With(secret, audit.Record("webhook_endpoint.create", "webhook_endpoint")).Post("/webhook_endpoints", we.Create)
		r.With(secret).Get("/webhook_endpoints/{id}", we.Retrieve)
		r.With(secret, audit.Record("webhook_endpoint.delete", "webhook_endpoint")).Delete("/webhook_endpoints/{id}", we.Delete)

		// Delivery inspector: list what was sent, replay a delivery.
		wd := handlers.NewWebhookDeliveries(pool)
		r.With(secret).Get("/webhook_deliveries", wd.List)
		r.With(secret, audit.Record("webhook_delivery.replay", "webhook_delivery")).Post("/webhook_deliveries/{id}/replay", wd.Replay)

		// API keys
		ak := handlers.NewAPIKeys(apiKeySvc)
		r.With(secret, audit.Record("api_key.create", "api_key")).Post("/api_keys", ak.Create)
		r.With(secret, audit.Record("api_key.revoke", "api_key")).Delete("/api_keys/{id}", ak.Revoke)
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
