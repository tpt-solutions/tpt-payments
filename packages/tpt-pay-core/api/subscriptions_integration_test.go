package api_test

import (
	"context"
	"net/http"
	"testing"

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
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/subscription"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

// subscriptionStack is the HTTP stack with the subscription routes enabled.
func subscriptionStack(t *testing.T) *stack {
	t.Helper()
	pool := testdb.Open(t)
	ctx := context.Background()
	var merchantID uuid.UUID
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Subscriber', $1) RETURNING id`,
		uuid.New().String()+"@test.invalid").Scan(&merchantID))

	keys := apikey.NewService(pool)
	paySvc := payment.NewService(pool, sandbox.Default(), ledger.New(pool))
	subSvc := subscription.NewService(pool, paySvc, clock.System{})
	handler := api.NewRouter(pool, paySvc, keys, testSecrets(t), prometheus.NewRegistry(), nil,
		api.WithSubscriptions(subSvc))
	return &stack{handler: handler, pool: pool, keys: keys, merchantID: merchantID.String()}
}

func TestSubscriptions_LifecycleThroughTheApi(t *testing.T) {
	s := subscriptionStack(t)
	key := s.key(t, apikey.TypeSecret, apikey.ModeTest)

	rec := s.do(http.MethodPost, "/v1/subscriptions", key,
		`{"amount":1500,"currency":"usd","interval":"month","payment_method":"pm_card_visa","metadata":{"plan":"pro"}}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	created := decode(t, rec.Body.Bytes())
	id, _ := created["id"].(string)
	require.NotEmpty(t, id)
	assert.Equal(t, "subscription", created["object"])
	assert.Equal(t, "active", created["status"], "the first charge succeeded")
	assert.EqualValues(t, 1500, created["amount"])
	assert.Equal(t, "pro", typed[map[string]any](created["metadata"])["plan"])

	rec = s.do(http.MethodGet, "/v1/subscriptions/"+id, key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, id, decode(t, rec.Body.Bytes())["id"])

	rec = s.do(http.MethodGet, "/v1/subscriptions?limit=10", key, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Len(t, decode(t, rec.Body.Bytes())["data"], 1)

	rec = s.do(http.MethodPost, "/v1/subscriptions/"+id+"/cancel", key, `{"at_period_end":false}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	canceled := decode(t, rec.Body.Bytes())
	assert.Equal(t, "canceled", canceled["status"])
	assert.Equal(t, "requested", canceled["cancel_reason"])

	rec = s.do(http.MethodPost, "/v1/subscriptions/"+id+"/cancel", key, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code, "a canceled subscription cannot be canceled again")
}

func TestSubscriptions_ValidationAndModeThroughTheApi(t *testing.T) {
	s := subscriptionStack(t)
	test := s.key(t, apikey.TypeSecret, apikey.ModeTest)
	live := s.key(t, apikey.TypeSecret, apikey.ModeLive)

	rec := s.do(http.MethodPost, "/v1/subscriptions", live,
		`{"amount":1500,"currency":"usd","interval":"month","payment_method":"pm_card_visa"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code, "live mode is refused until a billing policy exists")

	rec = s.do(http.MethodPost, "/v1/subscriptions", test,
		`{"amount":1500,"currency":"usd","interval":"day","payment_method":"pm_card_visa"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "only week, month and year are offered")

	rec = s.do(http.MethodPost, "/v1/subscriptions", test,
		`{"amount":1500,"currency":"usd","interval":"month","payment_method":"pm_card_visa","unknown":true}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code, "unknown fields are refused")

	rec = s.do(http.MethodGet, "/v1/subscriptions/sub_nope", test, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
