// Package api wires the HTTP router and all middleware.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/api/handlers"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/payment"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewRouter builds and returns the fully configured HTTP router.
func NewRouter(
	pool *pgxpool.Pool,
	paymentSvc *payment.Service,
	apiKeySvc *apikey.Service,
) http.Handler {
	r := chi.NewRouter()

	// ── Global middleware ─────────────────────────────────────────────────────
	r.Use(chimw.RealIP)
	r.Use(middleware.RequestID)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Compress(5))

	// ── Public endpoints ──────────────────────────────────────────────────────
	r.Get("/health", healthHandler)
	r.Get("/openapi.json", openAPIHandler)

	// ── Authenticated API (all routes under /v1 require a valid API key) ──────
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(apiKeySvc))
		r.Use(middleware.Idempotency(pool))

		// Payment intents
		pi := handlers.NewPaymentIntents(paymentSvc)
		r.Post("/payment_intents", pi.Create)
		r.Get("/payment_intents/{id}", pi.Retrieve)
		r.Post("/payment_intents/{id}/confirm", pi.Confirm)
		r.Post("/payment_intents/{id}/capture", pi.Capture)
		r.Post("/payment_intents/{id}/cancel", pi.Cancel)

		// Webhook endpoints
		we := handlers.NewWebhookEndpoints(pool)
		r.Post("/webhook_endpoints", we.Create)
		r.Get("/webhook_endpoints/{id}", we.Retrieve)
		r.Delete("/webhook_endpoints/{id}", we.Delete)

		// API keys
		ak := handlers.NewAPIKeys(apiKeySvc)
		r.Post("/api_keys", ak.Create)
		r.Delete("/api_keys/{id}", ak.Revoke)
	})

	return r
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func openAPIHandler(w http.ResponseWriter, r *http.Request) {
	// TODO: serve the generated OpenAPI spec from api/openapi.json
	http.Error(w, "OpenAPI spec not yet generated — run `make openapi`", http.StatusNotImplemented)
}
