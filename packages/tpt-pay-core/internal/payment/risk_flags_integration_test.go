package payment_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/policy"
)

const riskDoc = `{"rules":[
	{"id":"large_review","when":[{"field":"amount","op":"gt","value":50000}],"action":"flag"},
	{"id":"vip","when":[{"field":"metadata.tier","op":"eq","value":"vip"}],"action":"flag"},
	{"id":"stop_huge","when":[{"field":"amount","op":"gte","value":1000000}],"action":"block"}
]}`

func TestIntegration_FlaggedPaymentStoresItsRules(t *testing.T) {
	e := setup(t)
	engine, err := policy.Parse([]byte(riskDoc))
	require.NoError(t, err)
	e.svc.SetPolicy(engine)
	ctx := context.Background()

	flagged, err := e.svc.Create(ctx, e.scope(), payment.CreateParams{
		Amount: 60000, Currency: "usd", Metadata: map[string]string{"tier": "vip"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"large_review", "vip"}, flagged.RiskFlags)

	// The flags survive a read back from the database.
	got, err := e.svc.Retrieve(ctx, flagged.ID, e.scope())
	require.NoError(t, err)
	assert.Equal(t, []string{"large_review", "vip"}, got.RiskFlags)

	clean, err := e.svc.Create(ctx, e.scope(), payment.CreateParams{Amount: 100, Currency: "usd"})
	require.NoError(t, err)
	assert.NotNil(t, clean.RiskFlags)
	assert.Empty(t, clean.RiskFlags)
}

func TestIntegration_BlockedPaymentIsNotStored(t *testing.T) {
	e := setup(t)
	engine, err := policy.Parse([]byte(riskDoc))
	require.NoError(t, err)
	e.svc.SetPolicy(engine)

	_, err = e.svc.Create(context.Background(), e.scope(), payment.CreateParams{Amount: 1000000, Currency: "usd"})
	var apiErr *apierrors.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, apierrors.CodePaymentBlocked, apiErr.Code)
}
