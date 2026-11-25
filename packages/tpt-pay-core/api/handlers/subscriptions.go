package handlers

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/subscription"
)

// SubscriptionService is the part of the subscription service the HTTP handlers use.
type SubscriptionService interface {
	Create(ctx context.Context, scope subscription.Scope, p subscription.CreateParams) (*subscription.Subscription, error)
	Retrieve(ctx context.Context, scope subscription.Scope, id string) (*subscription.Subscription, error)
	List(ctx context.Context, p subscription.ListParams) ([]*subscription.Subscription, bool, error)
	Cancel(ctx context.Context, scope subscription.Scope, id string, atPeriodEnd bool) (*subscription.Subscription, error)
}

// Subscriptions handles the /v1/subscriptions routes. Every route needs a secret key.
type Subscriptions struct {
	svc SubscriptionService
}

// NewSubscriptions returns the subscription handlers.
func NewSubscriptions(svc SubscriptionService) *Subscriptions {
	return &Subscriptions{svc: svc}
}

type createSubscriptionBody struct {
	Amount        int64             `json:"amount"`
	Currency      string            `json:"currency"`
	Interval      string            `json:"interval"`
	PaymentMethod string            `json:"payment_method"`
	Metadata      map[string]string `json:"metadata"`
}

// Create handles POST /v1/subscriptions. The first period is charged before the response; the
// response status shows whether that charge succeeded (active) or is being retried (past_due).
func (h *Subscriptions) Create(w http.ResponseWriter, r *http.Request) {
	var body createSubscriptionBody
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, r, apierrors.InvalidParam("body", err.Error()))
		return
	}
	sub, err := h.svc.Create(r.Context(), scopeOf(r), subscription.CreateParams{
		Amount:          body.Amount,
		Currency:        body.Currency,
		Interval:        subscription.Interval(body.Interval),
		PaymentMethodID: body.PaymentMethod,
		Metadata:        body.Metadata,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, subscription.Wire(sub))
}

// Retrieve handles GET /v1/subscriptions/{id}.
func (h *Subscriptions) Retrieve(w http.ResponseWriter, r *http.Request) {
	sub, err := h.svc.Retrieve(r.Context(), scopeOf(r), chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, subscription.Wire(sub))
}

// List handles GET /v1/subscriptions?limit=&starting_after=.
func (h *Subscriptions) List(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			respondError(w, r, apierrors.InvalidParam("limit", "limit must be between 1 and 100"))
			return
		}
		limit = n
	}
	subs, hasMore, err := h.svc.List(r.Context(), subscription.ListParams{
		Scope:         scopeOf(r),
		Limit:         limit,
		StartingAfter: r.URL.Query().Get("starting_after"),
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	data := make([]map[string]any, 0, len(subs))
	for _, s := range subs {
		data = append(data, subscription.Wire(s))
	}
	respond(w, r, http.StatusOK, map[string]any{
		"object":   "list",
		"url":      "/v1/subscriptions",
		"has_more": hasMore,
		"data":     data,
	})
}

type cancelSubscriptionBody struct {
	AtPeriodEnd bool `json:"at_period_end"`
}

// Cancel handles POST /v1/subscriptions/{id}/cancel. With at_period_end the subscription keeps
// running until the paid period ends.
func (h *Subscriptions) Cancel(w http.ResponseWriter, r *http.Request) {
	var body cancelSubscriptionBody
	if err := decodeOptionalJSON(r, &body); err != nil {
		respondError(w, r, apierrors.InvalidParam("body", err.Error()))
		return
	}
	sub, err := h.svc.Cancel(r.Context(), scopeOf(r), chi.URLParam(r, "id"), body.AtPeriodEnd)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, subscription.Wire(sub))
}
