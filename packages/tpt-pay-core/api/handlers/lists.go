package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

// IntentLister is the part of the payment service the list handler needs.
type IntentLister interface {
	ListIntents(ctx context.Context, p payment.ListParams) (payment.ListPage, error)
}

// IntentList handles GET /v1/payment_intents.
type IntentList struct {
	svc IntentLister
}

// NewIntentList returns the list handler.
func NewIntentList(svc IntentLister) *IntentList {
	return &IntentList{svc: svc}
}

// List returns one page of the caller's payment intents, newest first.
//
// Query parameters: limit (1-100, default 10), starting_after (an intent ID from the previous
// page), status (a payment intent status). The response is a list object with has_more.
func (h *IntentList) List(w http.ResponseWriter, r *http.Request) {
	params, err := parseListParams(r, scopeOf(r))
	if err != nil {
		respondError(w, r, err)
		return
	}
	page, err := h.svc.ListIntents(r.Context(), params)
	if err != nil {
		respondError(w, r, err)
		return
	}
	data := make([]map[string]interface{}, 0, len(page.Intents))
	for _, intent := range page.Intents {
		data = append(data, intentResponse(intent))
	}
	respond(w, r, http.StatusOK, map[string]interface{}{
		"object":   "list",
		"url":      "/v1/payment_intents",
		"has_more": page.HasMore,
		"data":     data,
	})
}

// parseListParams reads and checks the query string. Errors are client errors (400).
func parseListParams(r *http.Request, scope payment.Scope) (payment.ListParams, error) {
	q := r.URL.Query()
	params := payment.ListParams{
		Scope:         scope,
		StartingAfter: q.Get("starting_after"),
		Status:        payment.Status(q.Get("status")),
	}
	limit, err := parseLimit(r)
	if err != nil {
		return params, err
	}
	params.Limit = limit
	return payment.ValidateListParams(params)
}

// parseLimit reads the limit query parameter. An explicit limit is checked here: zero is an
// error, not a request for the default. Absent means zero, which the service replaces with the
// default page size.
func parseLimit(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apierrors.InvalidParam("limit", "limit must be an integer")
	}
	if n < 1 || n > payment.MaxListLimit {
		return 0, apierrors.InvalidParam("limit", fmt.Sprintf("limit must be between 1 and %d", payment.MaxListLimit))
	}
	return n, nil
}
