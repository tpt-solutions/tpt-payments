package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/subscription"
)

// fakeSubs records what the handlers ask for, and answers with a fixed subscription.
type fakeSubs struct {
	created   []subscription.CreateParams
	cancelled []bool
	err       error
}

func (f *fakeSubs) Create(_ context.Context, _ subscription.Scope, p subscription.CreateParams) (*subscription.Subscription, error) {
	f.created = append(f.created, p)
	if f.err != nil {
		return nil, f.err
	}
	return &subscription.Subscription{ID: "sub_1", Status: subscription.StatusActive, Currency: "USD", Interval: p.Interval}, nil
}

func (f *fakeSubs) Retrieve(_ context.Context, _ subscription.Scope, id string) (*subscription.Subscription, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &subscription.Subscription{ID: id, Status: subscription.StatusActive}, nil
}

func (f *fakeSubs) List(_ context.Context, _ subscription.ListParams) ([]*subscription.Subscription, bool, error) {
	if f.err != nil {
		return nil, false, f.err
	}
	return []*subscription.Subscription{{ID: "sub_1"}}, false, nil
}

func (f *fakeSubs) Cancel(_ context.Context, _ subscription.Scope, id string, atPeriodEnd bool) (*subscription.Subscription, error) {
	f.cancelled = append(f.cancelled, atPeriodEnd)
	if f.err != nil {
		return nil, f.err
	}
	return &subscription.Subscription{ID: id, Status: subscription.StatusCanceled}, nil
}

// withSubscriptionRoutes builds the four handlers behind a chi router, with the same key context
// the router would set, so the handlers can be exercised directly.
func withSubscriptionRoutes(svc SubscriptionService) http.Handler {
	h := NewSubscriptions(svc)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			key := &apikey.Key{ID: "key_1", MerchantID: "merch_1", Type: apikey.TypeSecret, Mode: apikey.ModeTest}
			next.ServeHTTP(w, req.WithContext(middleware.WithAuthenticatedKey(req.Context(), key)))
		})
	})
	r.Post("/v1/subscriptions", h.Create)
	r.Get("/v1/subscriptions", h.List)
	r.Get("/v1/subscriptions/{id}", h.Retrieve)
	r.Post("/v1/subscriptions/{id}/cancel", h.Cancel)
	return r
}

func TestSubscriptionHandlers_CreatePassesTheParameters(t *testing.T) {
	svc := &fakeSubs{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/subscriptions",
		strings.NewReader(`{"amount":1500,"currency":"usd","interval":"month","payment_method":"pm_card_visa","metadata":{"plan":"pro"}}`))
	withSubscriptionRoutes(svc).ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, svc.created, 1)
	assert.Equal(t, int64(1500), svc.created[0].Amount)
	assert.Equal(t, subscription.IntervalMonth, svc.created[0].Interval)
	assert.Equal(t, "pm_card_visa", svc.created[0].PaymentMethodID)
	assert.Equal(t, "pro", svc.created[0].Metadata["plan"])
}

func TestSubscriptionHandlers_CreateRefusesBadBodies(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field": `{"amount":1500,"currency":"usd","interval":"month","payment_method":"pm","surprise":1}`,
		"trailing data": `{"amount":1500,"currency":"usd","interval":"month","payment_method":"pm"} {}`,
		"not json":      `amount=1500`,
	} {
		svc := &fakeSubs{}
		rec := httptest.NewRecorder()
		withSubscriptionRoutes(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/subscriptions", strings.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, rec.Code, name)
		assert.Empty(t, svc.created, "%s must not reach the service", name)
	}
}

func TestSubscriptionHandlers_ServiceErrorsKeepTheirStatus(t *testing.T) {
	svc := &fakeSubs{err: apierrors.NotFound("subscription", "sub_missing")}
	rec := httptest.NewRecorder()
	withSubscriptionRoutes(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/subscriptions/sub_missing", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSubscriptionHandlers_ListValidatesTheLimit(t *testing.T) {
	for _, target := range []string{"/v1/subscriptions?limit=0", "/v1/subscriptions?limit=101", "/v1/subscriptions?limit=many"} {
		rec := httptest.NewRecorder()
		withSubscriptionRoutes(&fakeSubs{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code, target)
	}
}

func TestSubscriptionHandlers_CancelPassesTheChoice(t *testing.T) {
	svc := &fakeSubs{}
	rec := httptest.NewRecorder()
	withSubscriptionRoutes(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/subscriptions/sub_1/cancel", strings.NewReader(`{"at_period_end":true}`)))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []bool{true}, svc.cancelled)

	// An empty body cancels now.
	rec = httptest.NewRecorder()
	withSubscriptionRoutes(svc).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/subscriptions/sub_1/cancel", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []bool{true, false}, svc.cancelled)
}
