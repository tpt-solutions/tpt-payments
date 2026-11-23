package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
)

func respond(w http.ResponseWriter, r *http.Request, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Request-ID", middleware.GetRequestID(r.Context()))
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// respondError writes an API error. An *APIError (possibly wrapped) is returned as its own
// status and message. Anything else is logged with its cause and returned as a generic 500,
// so internal details never reach the client.
func respondError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *apierrors.APIError
	if !errors.As(err, &apiErr) {
		slog.Error("unhandled error", "request_id", middleware.GetRequestID(r.Context()), "err", err)
		apiErr = apierrors.Internal("An unexpected error occurred.")
	}
	respond(w, r, apiErr.StatusCode, apierrors.Response{Error: apiErr})
}

// decodeJSON decodes a required JSON body. Unknown fields and trailing data are errors.
func decodeJSON(r *http.Request, dst interface{}) error {
	if err := decodeStrict(r, dst); err != nil {
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return err
	}
	return nil
}

// decodeOptionalJSON decodes a body that may be empty. An empty body is fine; a malformed
// one is an error.
func decodeOptionalJSON(r *http.Request, dst interface{}) error {
	err := decodeStrict(r, dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func decodeStrict(r *http.Request, dst interface{}) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	// Reject anything after the first JSON value.
	var extra json.RawMessage
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON body")
	}
	return nil
}
