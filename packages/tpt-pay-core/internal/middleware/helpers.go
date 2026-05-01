package middleware

import (
	"encoding/json"
	"net/http"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apierrors"
)

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	switch e := err.(type) {
	case *apierrors.APIError:
		apiErr = e
	case *idempotencyError:
		apiErr = &apierrors.APIError{
			StatusCode: http.StatusUnprocessableEntity,
			Type:       apierrors.ErrorTypeIdempotency,
			Message:    e.msg,
		}
	default:
		apiErr = apierrors.Internal("An unexpected error occurred.")
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", GetRequestID(r.Context()))
	w.WriteHeader(apiErr.StatusCode)
	_ = json.NewEncoder(w).Encode(apierrors.Response{Error: apiErr})
}
