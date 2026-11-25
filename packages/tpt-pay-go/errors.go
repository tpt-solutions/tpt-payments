package tptpay

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Error is an error returned by the API, or a failure to reach it.
type Error struct {
	Status    int    // HTTP status; 0 when the API could not be reached
	Type      string // e.g. invalid_request_error, card_error, api_connection_error
	Code      string // machine-readable code, e.g. card_declined, payment_blocked
	Message   string
	Param     string // the request parameter at fault, when there is one
	RequestID string // X-Request-ID, for support requests
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.RequestID != "" {
		return fmt.Sprintf("tptpay: %s (status %d, code %s, request %s)", e.Message, e.Status, e.Code, e.RequestID)
	}
	return fmt.Sprintf("tptpay: %s (status %d, code %s)", e.Message, e.Status, e.Code)
}

// IsCardError reports whether the payment was declined by the card or the network. The
// customer can usually fix it, for example with another card.
func IsCardError(err error) bool {
	e, ok := err.(*Error)
	return ok && e.Type == "card_error"
}

// IsNotFound reports whether the requested object does not exist, or is not visible to the caller.
func IsNotFound(err error) bool {
	e, ok := err.(*Error)
	return ok && e.Status == http.StatusNotFound
}

// IsConnectionError reports whether the API could not be reached after the retries.
func IsConnectionError(err error) bool {
	e, ok := err.(*Error)
	return ok && e.Type == "api_connection_error"
}

// decodeError builds an Error from an error response body.
func decodeError(status int, body []byte, requestID string) *Error {
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Code    string `json:"code"`
			Message string `json:"message"`
			Param   string `json:"param"`
		} `json:"error"`
	}
	e := &Error{Status: status, RequestID: requestID}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Message != "" {
		e.Type = envelope.Error.Type
		e.Code = envelope.Error.Code
		e.Message = envelope.Error.Message
		e.Param = envelope.Error.Param
		return e
	}
	e.Type = "api_error"
	e.Message = http.StatusText(status)
	if e.Message == "" {
		e.Message = "the API returned an error"
	}
	return e
}
