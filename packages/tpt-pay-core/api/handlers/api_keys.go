package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/middleware"
)

// APIKeys groups the API key HTTP handlers.
type APIKeys struct {
	svc *apikey.Service
}

// NewAPIKeys creates an APIKeys handler group.
func NewAPIKeys(svc *apikey.Service) *APIKeys {
	return &APIKeys{svc: svc}
}

// Create handles POST /v1/api_keys
func (h *APIKeys) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type string `json:"type"`
		Mode string `json:"mode"`
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, r, errBadJSON(err))
		return
	}

	keyType := apikey.TypeSecret
	if body.Type == "publishable" {
		keyType = apikey.TypePublishable
	}
	mode := apikey.ModeTest
	if body.Mode == "live" {
		mode = apikey.ModeLive
	}

	result, err := h.svc.Generate(r.Context(), middleware.MerchantID(r.Context()), body.Name, keyType, mode)
	if err != nil {
		respondError(w, r, err)
		return
	}

	// Include raw key only in create response — it is never retrievable again.
	resp := keyResponse(result.Key)
	resp["secret"] = result.RawKey
	respond(w, r, http.StatusOK, resp)
}

// Revoke handles DELETE /v1/api_keys/:id
func (h *APIKeys) Revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.svc.Revoke(r.Context(), id, middleware.MerchantID(r.Context()))
	if err != nil {
		respondError(w, r, apierrors.NotFound("api_key", id))
		return
	}
	respond(w, r, http.StatusOK, map[string]interface{}{
		"id":      id,
		"object":  "api_key",
		"deleted": true,
	})
}

func keyResponse(k *apikey.Key) map[string]interface{} {
	return map[string]interface{}{
		"id":         k.ID,
		"object":     "api_key",
		"type":       string(k.Type),
		"mode":       string(k.Mode),
		"name":       k.Name,
		"created":    k.CreatedAt.Unix(),
	}
}
