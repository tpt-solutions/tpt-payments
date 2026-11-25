package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// WebhookDeliveries handles the delivery inspector: listing deliveries and replaying them. Every
// route requires a secret key and is scoped to the key's merchant and mode.
type WebhookDeliveries struct {
	pool *pgxpool.Pool
}

// NewWebhookDeliveries returns the delivery inspector handlers.
func NewWebhookDeliveries(pool *pgxpool.Pool) *WebhookDeliveries {
	return &WebhookDeliveries{pool: pool}
}

// List handles GET /v1/webhook_deliveries?limit=&starting_after=&endpoint_id=
func (h *WebhookDeliveries) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 10
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			respondError(w, r, apierrors.InvalidParam("limit", "limit must be between 1 and 100"))
			return
		}
		limit = n
	}
	includePayload := false
	switch inc := q.Get("include"); inc {
	case "":
	case "payload":
		includePayload = true
	default:
		respondError(w, r, apierrors.InvalidParam("include", "the only supported value is payload"))
		return
	}
	scope := scopeOf(r)
	records, hasMore, err := webhook.ListDeliveries(r.Context(), h.pool, webhook.DeliveryFilter{
		MerchantID:     scope.MerchantID,
		Livemode:       scope.Livemode,
		EndpointID:     q.Get("endpoint_id"),
		Limit:          limit,
		StartingAfter:  q.Get("starting_after"),
		IncludePayload: includePayload,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	data := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		data = append(data, deliveryJSON(rec))
	}
	respond(w, r, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/webhook_deliveries",
		"has_more": hasMore,
		"data":     data,
	})
}

// Replay handles POST /v1/webhook_deliveries/{id}/replay. It returns 202: the delivery is queued
// and sent by the workers, not inline.
func (h *WebhookDeliveries) Replay(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	scope := scopeOf(r)
	err := webhook.Replay(r.Context(), h.pool, scope.MerchantID, scope.Livemode, id)
	if errors.Is(err, webhook.ErrDeliveryNotFound) {
		respondError(w, r, apierrors.NotFound("webhook_delivery", id))
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusAccepted, map[string]any{
		"id":     id,
		"object": "webhook_delivery",
		"status": "pending",
	})
}

func deliveryJSON(rec webhook.DeliveryRecord) map[string]any {
	out := map[string]any{
		"id":               rec.ID,
		"object":           "webhook_delivery",
		"endpoint":         rec.EndpointID,
		"event_type":       rec.EventType,
		"event_id":         rec.EventID,
		"status":           rec.Status,
		"attempts":         rec.Attempts,
		"last_http_status": rec.LastHTTPStatus,
		"last_error":       rec.LastError,
		"created":          rec.CreatedAt.Unix(),
		"delivered_at":     nil,
	}
	if rec.DeliveredAt != nil {
		out["delivered_at"] = rec.DeliveredAt.Unix()
	}
	if rec.Payload != nil {
		out["payload"] = json.RawMessage(rec.Payload)
	}
	return out
}
