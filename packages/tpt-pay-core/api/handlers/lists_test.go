package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

type fakeLister struct {
	params payment.ListParams
	page   payment.ListPage
	err    error
}

func (f *fakeLister) ListIntents(_ context.Context, p payment.ListParams) (payment.ListPage, error) {
	f.params = p
	return f.page, f.err
}

func listRequest(t *testing.T, target string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, target, nil)
}

func TestParseListParams_Defaults(t *testing.T) {
	p, err := parseListParams(listRequest(t, "/v1/payment_intents"), payment.Scope{MerchantID: "m1"})
	require.NoError(t, err)
	assert.Equal(t, payment.DefaultListLimit, p.Limit)
	assert.Equal(t, "m1", p.Scope.MerchantID)
	assert.Empty(t, p.Status)
}

func TestParseListParams_RejectsBadValues(t *testing.T) {
	cases := map[string]string{
		"non-integer limit": "/v1/payment_intents?limit=ten",
		"zero limit":        "/v1/payment_intents?limit=0",
		"limit over max":    "/v1/payment_intents?limit=101",
		"unknown status":    "/v1/payment_intents?status=paid",
	}
	for name, target := range cases {
		_, err := parseListParams(listRequest(t, target), payment.Scope{})
		require.Error(t, err, name)
		var apiErr *apierrors.APIError
		require.ErrorAs(t, err, &apiErr, name)
		assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode, name)
	}
}

func TestParseListParams_PassesCursorAndStatus(t *testing.T) {
	p, err := parseListParams(listRequest(t,
		"/v1/payment_intents?limit=25&starting_after=pi_123&status=requires_capture"), payment.Scope{})
	require.NoError(t, err)
	assert.Equal(t, 25, p.Limit)
	assert.Equal(t, "pi_123", p.StartingAfter)
	assert.Equal(t, payment.StatusRequiresCapture, p.Status)
}

func TestIntentList_ReturnsListEnvelope(t *testing.T) {
	fake := &fakeLister{page: payment.ListPage{
		HasMore: true,
		Intents: []*payment.Intent{
			{ID: "pi_2", Amount: 2000, Currency: "usd", Status: payment.StatusSucceeded},
			{ID: "pi_1", Amount: 1000, Currency: "usd", Status: payment.StatusCanceled},
		},
	}}
	h := NewIntentList(fake)
	rec := httptest.NewRecorder()
	h.List(rec, listRequest(t, "/v1/payment_intents?limit=2"))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Object  string           `json:"object"`
		HasMore bool             `json:"has_more"`
		Data    []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "list", body.Object)
	assert.True(t, body.HasMore)
	require.Len(t, body.Data, 2)
	assert.Equal(t, "pi_2", body.Data[0]["id"], "newest first, as the service returns them")
	assert.Equal(t, 2, fake.params.Limit)
}

func TestIntentList_PropagatesServiceErrors(t *testing.T) {
	fake := &fakeLister{err: apierrors.InvalidParam("starting_after", "no payment intent with that id")}
	rec := httptest.NewRecorder()
	NewIntentList(fake).List(rec, listRequest(t, "/v1/payment_intents?starting_after=pi_missing"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
