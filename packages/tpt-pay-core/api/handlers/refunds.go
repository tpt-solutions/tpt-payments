package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
)

// Refunds groups the refund HTTP handlers.
type Refunds struct {
	svc *payment.Service
}

// NewRefunds creates a Refunds handler group.
func NewRefunds(svc *payment.Service) *Refunds {
	return &Refunds{svc: svc}
}

// Create handles POST /v1/refunds
func (h *Refunds) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PaymentIntent string `json:"payment_intent"`
		Amount        int64  `json:"amount"`
		Reason        string `json:"reason"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, r, errBadJSON(err))
		return
	}
	if body.PaymentIntent == "" {
		respondError(w, r, apierrors.MissingParam("payment_intent"))
		return
	}

	refund, err := h.svc.CreateRefund(r.Context(), scopeOf(r), payment.CreateRefundParams{
		PaymentIntentID: body.PaymentIntent,
		Amount:          body.Amount,
		Reason:          body.Reason,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, payment.RefundWire(refund))
}

// Retrieve handles GET /v1/refunds/:id
func (h *Refunds) Retrieve(w http.ResponseWriter, r *http.Request) {
	refund, err := h.svc.RetrieveRefund(r.Context(), chi.URLParam(r, "id"), scopeOf(r))
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, payment.RefundWire(refund))
}
