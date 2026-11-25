package apierrors

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstructors(t *testing.T) {
	tests := []struct {
		name       string
		err        *APIError
		status     int
		typ        ErrorType
		code       ErrorCode
		msgContain string
		param      string
	}{
		{"PaymentBlocked", PaymentBlocked(), http.StatusPaymentRequired, ErrorTypeCardError, CodePaymentBlocked, "merchant rule", ""},
		{"NotFound", NotFound("payment_intent", "pi_1"), http.StatusNotFound, ErrorTypeInvalidRequest, CodeResourceNotFound, `No such payment_intent: "pi_1"`, ""},
		{"InvalidParam", InvalidParam("amount", "must be positive"), http.StatusBadRequest, ErrorTypeInvalidRequest, CodeInvalidParam, "must be positive", "amount"},
		{"MissingParam", MissingParam("currency"), http.StatusBadRequest, ErrorTypeInvalidRequest, CodeMissingParam, `"currency"`, "currency"},
		{"Unauthorized", Unauthorized("bad key"), http.StatusUnauthorized, ErrorTypeAuthentication, CodeAPIKeyInvalid, "bad key", ""},
		{"PermissionDenied", PermissionDenied("test key in live mode"), http.StatusForbidden, ErrorTypePermission, CodePermissionDenied, "live mode", ""},
		{"Internal", Internal("boom"), http.StatusInternalServerError, ErrorTypeAPIError, "", "boom", ""},
		{"IntentInvalidState", IntentInvalidState("succeeded", "cancel"), http.StatusBadRequest, ErrorTypeInvalidRequest, CodeIntentInvalidState, "(succeeded) does not support cancel", ""},
		{"CardVelocityExceeded", CardVelocityExceeded(), http.StatusTooManyRequests, ErrorTypeCardError, CodeCardVelocityExceeded, "Too many payment attempts", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.err)
			assert.Equal(t, tt.status, tt.err.StatusCode)
			assert.Equal(t, tt.typ, tt.err.Type)
			assert.Equal(t, tt.code, tt.err.Code)
			assert.Contains(t, tt.err.Message, tt.msgContain)
			assert.Equal(t, tt.param, tt.err.Param)
		})
	}
}

func TestErrorString(t *testing.T) {
	withParam := InvalidParam("amount", "must be positive")
	assert.Equal(t, "parameter_invalid_string: must be positive (param: amount)", withParam.Error())

	noParam := Unauthorized("bad key")
	assert.Equal(t, "api_key_invalid: bad key", noParam.Error())
}

func TestResponseJSON(t *testing.T) {
	raw, err := json.Marshal(Response{Error: NotFound("refund", "re_9")})
	require.NoError(t, err)

	var got struct {
		Error map[string]any `json:"error"`
	}
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "invalid_request_error", got.Error["type"])
	assert.Equal(t, "resource_missing", got.Error["code"])
	assert.NotContains(t, got.Error, "StatusCode", "status is a transport concern and must not leak into the body")
	assert.NotContains(t, got.Error, "param", "empty param must be omitted")
}
