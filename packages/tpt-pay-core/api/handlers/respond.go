package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/middleware"
)

func respond(w http.ResponseWriter, r *http.Request, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", middleware.GetRequestID(r.Context()))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func respondError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	switch e := err.(type) {
	case *apierrors.APIError:
		apiErr = e
	default:
		apiErr = apierrors.Internal("An unexpected error occurred.")
	}
	respond(w, r, apiErr.StatusCode, apierrors.Response{Error: apiErr})
}

func decodeJSON(r *http.Request, dst interface{}) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(dst)
}
