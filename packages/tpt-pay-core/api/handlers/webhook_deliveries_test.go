package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Validation runs before any SQL, so these checks need no database.
func TestWebhookDeliveries_ListRejectsBadLimit(t *testing.T) {
	h := NewWebhookDeliveries(nil)
	for _, target := range []string{
		"/v1/webhook_deliveries?limit=0",
		"/v1/webhook_deliveries?limit=101",
		"/v1/webhook_deliveries?limit=many",
	} {
		rec := httptest.NewRecorder()
		h.List(rec, httptest.NewRequest(http.MethodGet, target, nil))
		assert.Equal(t, http.StatusBadRequest, rec.Code, target)
	}
}

// A malformed delivery ID is "not found", never a database error.
func TestWebhookDeliveries_ReplayMalformedIDIsNotFound(t *testing.T) {
	h := NewWebhookDeliveries(nil)
	req := httptest.NewRequest(http.MethodPost, "/v1/webhook_deliveries/not-a-uuid/replay", nil)
	ctx := chi.NewRouteContext()
	ctx.URLParams.Add("id", "not-a-uuid")
	req = req.WithContext(chiContext(req, ctx))
	rec := httptest.NewRecorder()
	h.Replay(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

func chiContext(r *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
}
