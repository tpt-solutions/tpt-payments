package handlers

import (
	"context"
	"net/http"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

// RefundLister is the part of the payment service the refund list handler needs.
type RefundLister interface {
	ListRefunds(ctx context.Context, p payment.RefundListParams) (payment.RefundListPage, error)
}

// RefundList handles GET /v1/refunds.
type RefundList struct {
	svc RefundLister
}

// NewRefundList returns the refund list handler.
func NewRefundList(svc RefundLister) *RefundList {
	return &RefundList{svc: svc}
}

// List returns one page of the caller's refunds, newest first.
//
// Query parameters: limit (1-100, default 10), starting_after (a refund ID from the previous
// page), status (pending, succeeded or failed), payment_intent (only the refunds of that intent).
// The response is a list object with has_more.
func (h *RefundList) List(w http.ResponseWriter, r *http.Request) {
	limit, err := parseLimit(r)
	if err != nil {
		respondError(w, r, err)
		return
	}
	q := r.URL.Query()
	params := payment.RefundListParams{
		Scope:           scopeOf(r),
		Limit:           limit,
		StartingAfter:   q.Get("starting_after"),
		Status:          payment.RefundStatus(q.Get("status")),
		PaymentIntentID: q.Get("payment_intent"),
	}
	params, err = payment.ValidateRefundListParams(params)
	if err != nil {
		respondError(w, r, err)
		return
	}

	page, err := h.svc.ListRefunds(r.Context(), params)
	if err != nil {
		respondError(w, r, err)
		return
	}
	data := make([]map[string]interface{}, 0, len(page.Refunds))
	for _, refund := range page.Refunds {
		data = append(data, payment.RefundWire(refund))
	}
	respond(w, r, http.StatusOK, map[string]interface{}{
		"object":   "list",
		"url":      "/v1/refunds",
		"has_more": page.HasMore,
		"data":     data,
	})
}
