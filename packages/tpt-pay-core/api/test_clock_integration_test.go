package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/api"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

// clockStack is the HTTP stack with the sandbox running on a test clock, served with the
// /v1/test_clocks routes. It is what the sandbox server runs.
func clockStack(t *testing.T) (*stack, *clock.Test) {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()

	var merchantID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Clock Merchant', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid",
	).Scan(&merchantID))

	clk := clock.NewTest(time.Now().UTC().Truncate(time.Second))
	sb := sandbox.New(sandbox.Config{Clock: clk})
	keys := apikey.NewService(pool)
	paySvc := payment.NewService(pool, sb, ledger.New(pool))
	handler := api.NewRouter(pool, paySvc, keys, testSecrets(t), prometheus.NewRegistry(), nil,
		api.WithTestClocks(clk, sb))
	return &stack{handler: handler, pool: pool, keys: keys, merchantID: merchantID.String()}, clk
}

func TestTestClocks_AdvanceAndOutageThroughTheApi(t *testing.T) {
	s, clk := clockStack(t)
	key := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	start := clk.Now()

	// Reading the clock.
	rec := s.do(http.MethodGet, "/v1/test_clocks", key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Advancing a day moves the sandbox clock.
	rec = s.do(http.MethodPost, "/v1/test_clocks/advance", key, `{"seconds":86400}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, clk.Now().Equal(start.Add(24*time.Hour)))

	// A window covering the next hour makes a payment fail with a network error, not a decline.
	from := clk.Now().Format(time.RFC3339)
	to := clk.Now().Add(time.Hour).Format(time.RFC3339)
	rec = s.do(http.MethodPost, "/v1/test_clocks/outages", key, `{"from":"`+from+`","to":"`+to+`"}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	id := createIntent(t, s, key)
	confirm := s.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", key, `{"payment_method":"pm_card_visa"}`)
	assert.NotEqual(t, http.StatusOK, confirm.Code, "a payment inside an outage must not succeed")

	// Clearing the schedule lets the same kind of payment succeed.
	rec = s.do(http.MethodDelete, "/v1/test_clocks/outages", key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	id = createIntent(t, s, key)
	confirm = s.do(http.MethodPost, "/v1/payment_intents/"+id+"/confirm", key, `{"payment_method":"pm_card_visa"}`)
	require.Equal(t, http.StatusOK, confirm.Code, confirm.Body.String())
	var intent struct {
		Status string `json:"status"`
	}
	require.NoError(t, json.Unmarshal(confirm.Body.Bytes(), &intent))
	assert.Equal(t, "succeeded", intent.Status)
}

func TestTestClocks_LiveKeyIsRefusedThroughTheApi(t *testing.T) {
	s, _ := clockStack(t)
	live := s.key(t, apikey.TypeSecret, apikey.ModeLive)
	rec := s.do(http.MethodPost, "/v1/test_clocks/advance", live, `{"seconds":60}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
