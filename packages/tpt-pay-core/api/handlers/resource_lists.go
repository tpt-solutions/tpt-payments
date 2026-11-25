package handlers

import (
	"errors"
	"net/http"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
)

// defaultResourceLimit is the page size when a list request names no limit.
const defaultResourceLimit = 25

// List handles GET /v1/api_keys. The caller sees the keys of its own merchant and mode, newest
// first, including revoked keys. Key hashes are never returned, and the secret is never shown
// after creation.
func (h *APIKeys) List(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		respondError(w, r, err)
		return
	}
	if limit == 0 {
		limit = defaultResourceLimit
	}
	scope := scopeOf(r)
	mode := apikey.ModeTest
	if scope.Livemode {
		mode = apikey.ModeLive
	}
	keys, hasMore, err := h.svc.List(r.Context(), scope.MerchantID, mode, r.URL.Query().Get("starting_after"), limit)
	if errors.Is(err, apikey.ErrCursorNotFound) {
		respondError(w, r, apierrors.InvalidParam("starting_after", "starting_after is not a key in this account and mode"))
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	data := make([]map[string]interface{}, 0, len(keys))
	for i := range keys {
		data = append(data, keyResponse(&keys[i]))
	}
	respond(w, r, http.StatusOK, map[string]interface{}{
		"object":   "list",
		"url":      "/v1/api_keys",
		"has_more": hasMore,
		"data":     data,
	})
}

// List handles GET /v1/webhook_endpoints. The caller sees its own merchant's endpoints in its
// own mode, newest first. Signing secrets are never returned; only the prefix is.
func (h *WebhookEndpoints) List(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		respondError(w, r, err)
		return
	}
	if limit == 0 {
		limit = defaultResourceLimit
	}
	scope := scopeOf(r)
	after := r.URL.Query().Get("starting_after")
	if after != "" {
		// Checking the cursor first gives a clear 404 instead of an empty page.
		if _, err := h.getEndpoint(r, after); err != nil {
			respondError(w, r, err)
			return
		}
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, merchant_id, url, enabled_events, secret_prefix, status, livemode, created_at, updated_at
		FROM webhook_endpoints
		WHERE merchant_id = $1 AND livemode = $2
		  AND ($3 = '' OR (created_at, id) < (
		      SELECT created_at, id FROM webhook_endpoints WHERE id = $3 AND merchant_id = $1 AND livemode = $2))
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, scope.MerchantID, scope.Livemode, after, limit+1)
	if err != nil {
		respondError(w, r, err)
		return
	}
	defer rows.Close()

	var endpoints []*webhookEndpointRow
	for rows.Next() {
		var ep webhookEndpointRow
		if err := rows.Scan(&ep.ID, &ep.MerchantID, &ep.URL, &ep.EnabledEvents,
			&ep.SecretPrefix, &ep.Status, &ep.Livemode, &ep.CreatedAt, &ep.UpdatedAt); err != nil {
			respondError(w, r, err)
			return
		}
		endpoints = append(endpoints, &ep)
	}
	if err := rows.Err(); err != nil {
		respondError(w, r, err)
		return
	}
	hasMore := len(endpoints) > limit
	if hasMore {
		endpoints = endpoints[:limit]
	}
	data := make([]map[string]interface{}, 0, len(endpoints))
	for _, ep := range endpoints {
		data = append(data, endpointResponse(ep))
	}
	respond(w, r, http.StatusOK, map[string]interface{}{
		"object":   "list",
		"url":      "/v1/webhook_endpoints",
		"has_more": hasMore,
		"data":     data,
	})
}
