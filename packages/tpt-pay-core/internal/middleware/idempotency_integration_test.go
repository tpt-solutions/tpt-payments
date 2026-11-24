package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

func newIdemMerchant(t *testing.T) (string, func(h http.Handler) http.Handler) {
	t.Helper()
	pool := testdb.Open(t)
	var merchantID uuid.UUID
	require.NoError(t, pool.QueryRow(context.Background(),
		`INSERT INTO merchants (name, email) VALUES ('Idempotency Test', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid",
	).Scan(&merchantID))
	return merchantID.String(), Idempotency(pool)
}

// post sends one POST with idempotencyKey through h, authenticated as merchantID. A panic from the
// handler is recovered and reported, so the test can continue with the next request.
func post(t *testing.T, h http.Handler, merchantID, idempotencyKey string) (rec *httptest.ResponseRecorder, panicked bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/payment_intents", strings.NewReader(`{"amount":1000}`))
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req = req.WithContext(context.WithValue(req.Context(), authKey{}, &apikey.Key{MerchantID: merchantID}))
	rec = httptest.NewRecorder()
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		h.ServeHTTP(rec, req)
	}()
	return rec, panicked
}

// A handler that panics must release the key. Otherwise the client's retry would see "in progress"
// (409) forever, although nothing was done.
func TestIntegration_IdempotencyPanicReleasesTheKey(t *testing.T) {
	merchantID, idem := newIdemMerchant(t)
	var calls atomic.Int32
	h := idem(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			panic("fault injected by the test")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))

	_, panicked := post(t, h, merchantID, "panic-key-1")
	require.True(t, panicked, "the first attempt should panic")

	rec, panicked := post(t, h, merchantID, "panic-key-1")
	require.False(t, panicked)
	assert.Equal(t, http.StatusOK, rec.Code, "the retry runs, rather than seeing the key as in progress")
	assert.Equal(t, int32(2), calls.Load())
}

// A 5xx response is not stored either: the client may retry and the operation must run again.
func TestIntegration_IdempotencyServerErrorIsNotStored(t *testing.T) {
	merchantID, idem := newIdemMerchant(t)
	var calls atomic.Int32
	h := idem(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))

	rec, _ := post(t, h, merchantID, "five-hundred-key")
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	rec, _ = post(t, h, merchantID, "five-hundred-key")
	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Empty(t, rec.Header().Get("Idempotent-Replayed"), "a 5xx is not replayed")
	assert.Equal(t, int32(2), calls.Load())
}

// A successful response is stored and replayed byte for byte, without running the handler again.
func TestIntegration_IdempotencySuccessIsReplayedWithoutRunningAgain(t *testing.T) {
	merchantID, idem := newIdemMerchant(t)
	var calls atomic.Int32
	h := idem(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"pi_once"}`))
	}))

	first, _ := post(t, h, merchantID, "success-key")
	second, _ := post(t, h, merchantID, "success-key")
	assert.Equal(t, first.Body.String(), second.Body.String())
	assert.Equal(t, "true", second.Header().Get("Idempotent-Replayed"))
	assert.Equal(t, int32(1), calls.Load())
}
