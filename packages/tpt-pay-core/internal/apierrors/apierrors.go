// Package apierrors defines the error types returned by the TPT Pay API.
// The wire format is Stripe-compatible so that clients written for Stripe
// work unchanged against this API.
package apierrors

import (
	"fmt"
	"net/http"
)

// ErrorType mirrors Stripe's error type taxonomy.
type ErrorType string

const (
	ErrorTypeAPIError         ErrorType = "api_error"
	ErrorTypeCardError        ErrorType = "card_error"
	ErrorTypeIdempotency      ErrorType = "idempotency_error"
	ErrorTypeInvalidRequest   ErrorType = "invalid_request_error"
	ErrorTypeAuthentication   ErrorType = "authentication_error"
	ErrorTypePermission       ErrorType = "invalid_request_error"
	ErrorTypeRateLimit        ErrorType = "rate_limit_error"
)

// ErrorCode is the machine-readable code within an error.
type ErrorCode string

const (
	CodeCardDeclined          ErrorCode = "card_declined"
	CodeCardExpired           ErrorCode = "expired_card"
	CodeCVCIncorrect          ErrorCode = "incorrect_cvc"
	CodeInsufficientFunds     ErrorCode = "insufficient_funds"
	CodeAuthenticationRequired ErrorCode = "authentication_required"
	CodeInvalidAmount         ErrorCode = "amount_too_small"
	CodeMissingParam          ErrorCode = "parameter_missing"
	CodeInvalidParam          ErrorCode = "parameter_invalid_string"
	CodeResourceNotFound      ErrorCode = "resource_missing"
	CodeIdempotencyKeyInUse   ErrorCode = "idempotency_key_in_use"
	CodeIntentInvalidState    ErrorCode = "payment_intent_invalid_state"
	CodeCardVelocityExceeded  ErrorCode = "card_velocity_exceeded"
	CodeRateLimitExceeded     ErrorCode = "rate_limit_exceeded"
	CodeAPIKeyInvalid         ErrorCode = "api_key_invalid"
)

// APIError is the structured error returned in all error responses.
type APIError struct {
	StatusCode int       `json:"-"`
	Type       ErrorType `json:"type"`
	Code       ErrorCode `json:"code,omitempty"`
	Message    string    `json:"message"`
	Param      string    `json:"param,omitempty"`
}

func (e *APIError) Error() string {
	if e.Param != "" {
		return fmt.Sprintf("%s: %s (param: %s)", e.Code, e.Message, e.Param)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Response is the JSON envelope for error responses.
type Response struct {
	Error *APIError `json:"error"`
}

// ── Constructors ──────────────────────────────────────────────────────────────

func NotFound(resource, id string) *APIError {
	return &APIError{
		StatusCode: http.StatusNotFound,
		Type:       ErrorTypeInvalidRequest,
		Code:       CodeResourceNotFound,
		Message:    fmt.Sprintf("No such %s: %q", resource, id),
	}
}

func InvalidParam(param, message string) *APIError {
	return &APIError{
		StatusCode: http.StatusUnprocessableEntity,
		Type:       ErrorTypeInvalidRequest,
		Code:       CodeInvalidParam,
		Message:    message,
		Param:      param,
	}
}

func MissingParam(param string) *APIError {
	return &APIError{
		StatusCode: http.StatusUnprocessableEntity,
		Type:       ErrorTypeInvalidRequest,
		Code:       CodeMissingParam,
		Message:    fmt.Sprintf("Missing required param: %q", param),
		Param:      param,
	}
}

func Unauthorized(message string) *APIError {
	return &APIError{
		StatusCode: http.StatusUnauthorized,
		Type:       ErrorTypeAuthentication,
		Code:       CodeAPIKeyInvalid,
		Message:    message,
	}
}

func Internal(message string) *APIError {
	return &APIError{
		StatusCode: http.StatusInternalServerError,
		Type:       ErrorTypeAPIError,
		Message:    message,
	}
}

func IntentInvalidState(current, attempted string) *APIError {
	return &APIError{
		StatusCode: http.StatusBadRequest,
		Type:       ErrorTypeInvalidRequest,
		Code:       CodeIntentInvalidState,
		Message: fmt.Sprintf(
			"This PaymentIntent's status (%s) does not support %s",
			current, attempted,
		),
	}
}

func CardVelocityExceeded() *APIError {
	return &APIError{
		StatusCode: http.StatusTooManyRequests,
		Type:       ErrorTypeCardError,
		Code:       CodeCardVelocityExceeded,
		Message:    "Too many payment attempts with this card. Please try again later.",
	}
}
