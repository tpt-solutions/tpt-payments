package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/secretbox"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/webhook"
)

// WebhookEndpoints groups the webhook endpoint HTTP handlers. Every route requires a
// secret key (enforced by the router) and is scoped to the key's merchant and mode.
type WebhookEndpoints struct {
	pool    *pgxpool.Pool
	secrets *secretbox.Box
}

// NewWebhookEndpoints creates a WebhookEndpoints handler group.
func NewWebhookEndpoints(pool *pgxpool.Pool, secrets *secretbox.Box) *WebhookEndpoints {
	return &WebhookEndpoints{pool: pool, secrets: secrets}
}

type webhookEndpointRow struct {
	ID            string
	MerchantID    string
	URL           string
	EnabledEvents []string
	SecretPrefix  string
	Status        string
	Livemode      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// secretDisplayChars is how much of the signing secret is shown after creation.
const secretDisplayChars = 12

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
	scope := scopeOf(r)

	if err := webhook.ValidateEndpointURL(body.URL, scope.Livemode); err != nil {
		respondError(w, r, apierrors.InvalidParam("url", err.Error()))
		return
	}
	if len(body.EnabledEvents) == 0 {
		body.EnabledEvents = []string{"*"}
	}
	if err := webhook.ValidateEvents(body.EnabledEvents); err != nil {
		respondError(w, r, apierrors.InvalidParam("enabled_events", err.Error()))
		return
	}

	secret := webhook.GenerateSecret()
	ciphertext, err := h.secrets.Seal([]byte(secret))
	if err != nil {
		respondError(w, r, err)
		return
	}
	id := "we_" + strings.ReplaceAll(uuid.New().String(), "-", "")

	var ep webhookEndpointRow
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO webhook_endpoints
			(id, merchant_id, url, enabled_events, secret_ciphertext, secret_prefix, livemode)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, merchant_id, url, enabled_events, secret_prefix, status, livemode, created_at, updated_at
	`, id, scope.MerchantID, body.URL, body.EnabledEvents, ciphertext,
		secret[:secretDisplayChars], scope.Livemode,
	).Scan(&ep.ID, &ep.MerchantID, &ep.URL, &ep.EnabledEvents,
		&ep.SecretPrefix, &ep.Status, &ep.Livemode, &ep.CreatedAt, &ep.UpdatedAt)
	if err != nil {
		respondError(w, r, err)
		return
	}

	// The full secret is returned only here. It is never retrievable again.
	resp := endpointResponse(&ep)
	resp["secret"] = secret
	respond(w, r, http.StatusOK, resp)
}

// Retrieve handles GET /v1/webhook_endpoints/:id
func (h *WebhookEndpoints) Retrieve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ep, err := h.getEndpoint(r, id)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, endpointResponse(ep))
}

// Delete handles DELETE /v1/webhook_endpoints/:id. The endpoint is disabled, not removed,
// so its delivery history remains available.
func (h *WebhookEndpoints) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scope := scopeOf(r)
	tag, err := h.pool.Exec(r.Context(), `
		UPDATE webhook_endpoints SET status = 'disabled'
		WHERE id = $1 AND merchant_id = $2 AND livemode = $3
	`, id, scope.MerchantID, scope.Livemode)
	if err != nil {
		respondError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, r, apierrors.NotFound("webhook_endpoint", id))
		return
	}
	respond(w, r, http.StatusOK, map[string]interface{}{
		"id": id, "object": "webhook_endpoint", "deleted": true,
	})
}

func (h *WebhookEndpoints) getEndpoint(r *http.Request, id string) (*webhookEndpointRow, error) {
	scope := scopeOf(r)
	var ep webhookEndpointRow
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, merchant_id, url, enabled_events, secret_prefix, status, livemode, created_at, updated_at
		FROM webhook_endpoints
		WHERE id = $1 AND merchant_id = $2 AND livemode = $3
	`, id, scope.MerchantID, scope.Livemode).Scan(
		&ep.ID, &ep.MerchantID, &ep.URL, &ep.EnabledEvents,
		&ep.SecretPrefix, &ep.Status, &ep.Livemode, &ep.CreatedAt, &ep.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apierrors.NotFound("webhook_endpoint", id)
	}
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

func endpointResponse(ep *webhookEndpointRow) map[string]interface{} {
	return map[string]interface{}{
		"id":             ep.ID,
		"object":         "webhook_endpoint",
		"url":            ep.URL,
		"enabled_events": ep.EnabledEvents,
		"status":         ep.Status,
		"livemode":       ep.Livemode,
		"secret_prefix":  ep.SecretPrefix,
		"created":        ep.CreatedAt.Unix(),
	}
}

// errBadJSON reports a body that is not valid JSON for this endpoint. It is a 400, not a
// 422, because no parameter was understood at all.
func errBadJSON(err error) *apierrors.APIError {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return &apierrors.APIError{
			StatusCode: http.StatusRequestEntityTooLarge,
			Type:       apierrors.ErrorTypeInvalidRequest,
			Message:    "Request body exceeds 1 MB.",
		}
	}
	return &apierrors.APIError{
		StatusCode: http.StatusBadRequest,
		Type:       apierrors.ErrorTypeInvalidRequest,
		Message:    "Invalid request body: " + err.Error(),
	}
}
