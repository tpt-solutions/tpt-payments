package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/webhook"
)

// WebhookEndpoints groups the webhook endpoint HTTP handlers.
type WebhookEndpoints struct {
	pool *pgxpool.Pool
}

// NewWebhookEndpoints creates a WebhookEndpoints handler group.
func NewWebhookEndpoints(pool *pgxpool.Pool) *WebhookEndpoints {
	return &WebhookEndpoints{pool: pool}
}

type webhookEndpointRow struct {
	ID            string
	MerchantID    string
	URL           string
	EnabledEvents []string
	SecretPrefix  string
	Status        string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Create handles POST /v1/webhook_endpoints
func (h *WebhookEndpoints) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL           string   `json:"url"`
		EnabledEvents []string `json:"enabled_events"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, r, errBadJSON(err))
		return
	}
	if body.URL == "" {
		respondError(w, r, apierrors.MissingParam("url"))
		return
	}
	if len(body.EnabledEvents) == 0 {
		body.EnabledEvents = []string{"*"}
	}

	secret := webhook.GenerateSecret()
	secretHash := secret[:8] // store only prefix for display; hash stored separately
	id := "we_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	var ep webhookEndpointRow
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO webhook_endpoints
			(id, merchant_id, url, enabled_events, secret_hash, secret_prefix)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, merchant_id, url, enabled_events, secret_prefix, status, created_at, updated_at
	`, id, middleware.MerchantID(r.Context()), body.URL, body.EnabledEvents,
		secret, // storing raw secret here for now — Phase 1 will hash it properly
		secretHash,
	).Scan(&ep.ID, &ep.MerchantID, &ep.URL, &ep.EnabledEvents,
		&ep.SecretPrefix, &ep.Status, &ep.CreatedAt, &ep.UpdatedAt)
	if err != nil {
		respondError(w, r, fmt.Errorf("create webhook endpoint: %w", err))
		return
	}

	// Include the full secret in the create response — never retrievable again.
	resp := endpointResponse(&ep)
	resp["secret"] = secret
	respond(w, r, http.StatusOK, resp)
}

// Retrieve handles GET /v1/webhook_endpoints/:id
func (h *WebhookEndpoints) Retrieve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := h.getEndpoint(r.Context(), id, middleware.MerchantID(r.Context()))
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, endpointResponse(ep))
}

// Delete handles DELETE /v1/webhook_endpoints/:id
func (h *WebhookEndpoints) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE webhook_endpoints SET status = 'disabled' WHERE id = $1 AND merchant_id = $2`,
		id, middleware.MerchantID(r.Context()),
	)
	if err != nil || tag.RowsAffected() == 0 {
		respondError(w, r, apierrors.NotFound("webhook_endpoint", id))
		return
	}
	respond(w, r, http.StatusOK, map[string]interface{}{
		"id": id, "object": "webhook_endpoint", "deleted": true,
	})
}

func (h *WebhookEndpoints) getEndpoint(ctx context.Context, id, merchantID string) (*webhookEndpointRow, error) {
	var ep webhookEndpointRow
	err := h.pool.QueryRow(ctx, `
		SELECT id, merchant_id, url, enabled_events, secret_prefix, status, created_at, updated_at
		FROM webhook_endpoints
		WHERE id = $1 AND merchant_id = $2
	`, id, merchantID).Scan(
		&ep.ID, &ep.MerchantID, &ep.URL, &ep.EnabledEvents,
		&ep.SecretPrefix, &ep.Status, &ep.CreatedAt, &ep.UpdatedAt,
	)
	if err != nil {
		return nil, apierrors.NotFound("webhook_endpoint", id)
	}
	return &ep, nil
}

func endpointResponse(ep *webhookEndpointRow) map[string]interface{} {
	events, _ := json.Marshal(ep.EnabledEvents)
	_ = events
	return map[string]interface{}{
		"id":             ep.ID,
		"object":         "webhook_endpoint",
		"url":            ep.URL,
		"enabled_events": ep.EnabledEvents,
		"status":         ep.Status,
		"created":        ep.CreatedAt.Unix(),
	}
}

// errBadJSON wraps a JSON decode error into an API error.
func errBadJSON(err error) *apierrors.APIError {
	return apierrors.InvalidParam("", "Invalid JSON: "+err.Error())
}
