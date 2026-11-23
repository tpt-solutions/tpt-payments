package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The fingerprint must change when any of method, path or body changes, so a key cannot be
// reused for a different request.
func TestRequestFingerprint_BindsMethodPathAndBody(t *testing.T) {
	base := requestFingerprint(http.MethodPost, "/v1/refunds", []byte(`{"amount":100}`))

	assert.Equal(t, base, requestFingerprint(http.MethodPost, "/v1/refunds", []byte(`{"amount":100}`)),
		"identical requests fingerprint the same")
	assert.NotEqual(t, base, requestFingerprint(http.MethodPut, "/v1/refunds", []byte(`{"amount":100}`)), "method")
	assert.NotEqual(t, base, requestFingerprint(http.MethodPost, "/v1/refunds/x", []byte(`{"amount":100}`)), "path")
	assert.NotEqual(t, base, requestFingerprint(http.MethodPost, "/v1/refunds", []byte(`{"amount":101}`)), "body")
}

// The separator byte stops "POST" + "/a" from colliding with "POS" + "T/a" style splits.
func TestRequestFingerprint_NoConcatenationCollisions(t *testing.T) {
	a := requestFingerprint("POST", "/a", []byte("b"))
	b := requestFingerprint("POST", "/ab", []byte(""))
	assert.NotEqual(t, a, b)
}

func TestIsMutating(t *testing.T) {
	for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		assert.True(t, isMutating(m), m)
	}
	for _, m := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		assert.False(t, isMutating(m), m)
	}
}
