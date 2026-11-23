package middleware

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
)

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if !errors.As(err, &apiErr) {
		apiErr = apierrors.Internal("An unexpected error occurred.")
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", GetRequestID(r.Context()))
	w.WriteHeader(apiErr.StatusCode)
	_ = json.NewEncoder(w).Encode(apierrors.Response{Error: apiErr})
}
