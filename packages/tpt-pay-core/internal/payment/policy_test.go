package payment_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/policy"
)

// A merchant rule that blocks refuses the payment before anything is written, so this runs without a
// database.
func TestPolicy_BlockRefusesCreateBeforeAnyWrite(t *testing.T) {
	engine, err := policy.Parse([]byte(`{"rules":[
	  {"id":"big","when":[{"field":"amount","op":"gte","value":1000000}],"action":"block"}]}`))
	require.NoError(t, err)

	svc := payment.NewService(nil, nil, nil)
	svc.SetPolicy(engine)
	require.NotNil(t, svc.Policy())

	_, err = svc.Create(context.Background(), payment.Scope{MerchantID: "merch_1"}, payment.CreateParams{
		Amount: 2000000, Currency: "usd",
	})
	require.Error(t, err)
	var apiErr *apierrors.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusPaymentRequired, apiErr.StatusCode)
	assert.Equal(t, apierrors.CodePaymentBlocked, apiErr.Code)
	assert.NotContains(t, apiErr.Message, "big", "the rule is not revealed to the caller")
}
