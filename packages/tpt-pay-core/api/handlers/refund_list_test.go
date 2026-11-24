package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

type fakeRefundLister struct {
	params payment.RefundListParams
	page   payment.RefundListPage
	err    error
}

func (f *fakeRefundLister) ListRefunds(_ context.Context, p payment.RefundListParams) (payment.RefundListPage, error) {
	f.params = p
	return f.page, f.err
}

func TestRefundList_PassesFiltersAndCursorToTheService(t *testing.T) {
	fake := &fakeRefundLister{}
	rec := httptest.NewRecorder()
	NewRefundList(fake).List(rec, listRequest(t,
		"/v1/refunds?limit=25&starting_after=re_1&status=succeeded&payment_intent=pi_9"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 25, fake.params.Limit)
	assert.Equal(t, "re_1", fake.params.StartingAfter)
	assert.Equal(t, payment.RefundSucceeded, fake.params.Status)
	assert.Equal(t, "pi_9", fake.params.PaymentIntentID)
}

func TestRefundList_DefaultsTheLimit(t *testing.T) {
	fake := &fakeRefundLister{}
	rec := httptest.NewRecorder()
	NewRefundList(fake).List(rec, listRequest(t, "/v1/refunds"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, payment.DefaultListLimit, fake.params.Limit)
	assert.Empty(t, fake.params.Status)
	assert.Empty(t, fake.params.PaymentIntentID)
}

func TestRefundList_RejectsBadValuesBeforeTheService(t *testing.T) {
	cases := map[string]string{
		"non-integer limit": "/v1/refunds?limit=ten",
		"zero limit":        "/v1/refunds?limit=0",
		"limit over max":    "/v1/refunds?limit=101",
		"unknown status":    "/v1/refunds?status=canceled",
		"intent status":     "/v1/refunds?status=requires_capture",
	}
	for name, target := range cases {
		fake := &fakeRefundLister{}
		rec := httptest.NewRecorder()
		NewRefundList(fake).List(rec, listRequest(t, target))
		assert.Equal(t, http.StatusBadRequest, rec.Code, name)
		assert.Zero(t, fake.params.Limit, "%s: the service must not be called", name)
	}
}

func TestRefundList_ReturnsListEnvelope(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	fake := &fakeRefundLister{page: payment.RefundListPage{
		HasMore: true,
		Refunds: []*payment.Refund{
			{ID: "re_2", PaymentIntentID: "pi_1", MerchantID: "m1", Amount: 500, Currency: "USD",
				Status: payment.RefundSucceeded, CreatedAt: now, UpdatedAt: now},
			{ID: "re_1", PaymentIntentID: "pi_1", MerchantID: "m1", Amount: 700, Currency: "USD",
				Status: payment.RefundPending, CreatedAt: now, UpdatedAt: now},
		},
	}}
	rec := httptest.NewRecorder()
	NewRefundList(fake).List(rec, listRequest(t, "/v1/refunds?limit=2"))

	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Object  string           `json:"object"`
		URL     string           `json:"url"`
		HasMore bool             `json:"has_more"`
		Data    []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "list", body.Object)
	assert.Equal(t, "/v1/refunds", body.URL)
	assert.True(t, body.HasMore)
	require.Len(t, body.Data, 2)
	assert.Equal(t, "re_2", body.Data[0]["id"], "newest first, as the service returns them")
	assert.Equal(t, "refund", body.Data[0]["object"])
	assert.Equal(t, "pi_1", body.Data[0]["payment_intent"])
}

func TestRefundList_PropagatesServiceErrors(t *testing.T) {
	fake := &fakeRefundLister{err: apierrors.InvalidParam("starting_after", "no refund with that id")}
	rec := httptest.NewRecorder()
	NewRefundList(fake).List(rec, listRequest(t, "/v1/refunds?starting_after=re_missing"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
