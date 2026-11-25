package handlers

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
)

// APIKeys groups the API key HTTP handlers. Both routes require a secret key (enforced
// by the router).
type APIKeys struct {
	svc *apikey.Service
}

// NewAPIKeys creates an APIKeys handler group.
func NewAPIKeys(svc *apikey.Service) *APIKeys {
	return &APIKeys{svc: svc}
}

// Create handles POST /v1/api_keys
//
// New keys are always created in the calling key's mode: a test key cannot mint a live
// key, and a live key cannot mint a test key.
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

	caller := middleware.AuthenticatedKey(r.Context())
	if caller == nil {
		respondError(w, r, apierrors.Unauthorized("No API key provided."))
		return
	}

	keyType := apikey.TypeSecret
	switch body.Type {
	case "", "secret":
	case "publishable":
		keyType = apikey.TypePublishable
	default:
		respondError(w, r, apierrors.InvalidParam("type", "type must be 'secret' or 'publishable'"))
		return
	}

	mode := caller.Mode
	if body.Mode != "" {
		requested := apikey.Mode(body.Mode)
		if requested != apikey.ModeTest && requested != apikey.ModeLive {
			respondError(w, r, apierrors.InvalidParam("mode", "mode must be 'test' or 'live'"))
			return
		}
		if requested != caller.Mode {
			respondError(w, r, apierrors.PermissionDenied(
				"A "+string(caller.Mode)+" key cannot create a "+string(requested)+" key."))
			return
		}
	}

	result, err := h.svc.Generate(r.Context(), caller.MerchantID, body.Name, keyType, mode)
	if err != nil {
		respondError(w, r, err)
		return
	}

	// The raw key is returned only here; it cannot be retrieved again.
	resp := keyResponse(result.Key)
	resp["secret"] = result.RawKey
	respond(w, r, http.StatusOK, resp)
}

// Revoke handles DELETE /v1/api_keys/:id
func (h *APIKeys) Revoke(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.svc.Revoke(r.Context(), id, middleware.MerchantID(r.Context()))
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, r, apierrors.NotFound("api_key", id))
		return
	}
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, map[string]interface{}{
		"id":      id,
		"object":  "api_key",
		"deleted": true,
	})
}

// keyResponse describes a key. It carries the prefix and status, never the hash or the secret.
func keyResponse(k *apikey.Key) map[string]interface{} {
	status := "active"
	if k.RevokedAt != nil {
		status = "revoked"
	}
	out := map[string]interface{}{
		"id":      k.ID,
		"object":  "api_key",
		"type":    string(k.Type),
		"mode":    string(k.Mode),
		"name":    k.Name,
		"prefix":  k.Prefix,
		"status":  status,
		"created": k.CreatedAt.Unix(),
	}
	if k.LastUsedAt != nil {
		out["last_used"] = k.LastUsedAt.Unix()
	}
	if k.RevokedAt != nil {
		out["revoked"] = k.RevokedAt.Unix()
	}
	return out
}
