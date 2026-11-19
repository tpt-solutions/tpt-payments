package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/payment"
)

// scopeOf derives the merchant and mode the request may act on from its API key.
func scopeOf(r *http.Request) payment.Scope {
	key := middleware.AuthenticatedKey(r.Context())
	return payment.Scope{
		MerchantID: middleware.MerchantID(r.Context()),
		Livemode:   key != nil && key.Mode == apikey.ModeLive,
	}
}

// PaymentIntents groups the payment intent HTTP handlers.
type PaymentIntents struct {
	svc *payment.Service
}

// NewPaymentIntents creates a PaymentIntents handler group.
func NewPaymentIntents(svc *payment.Service) *PaymentIntents {
	return &PaymentIntents{svc: svc}
}

// Create handles POST /v1/payment_intents
func (h *PaymentIntents) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Amount             int64             `json:"amount"`
		Currency           string            `json:"currency"`
		CaptureMethod      string            `json:"capture_method"`
		ConfirmationMethod string            `json:"confirmation_method"`
		Metadata           map[string]string `json:"metadata"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, r, errBadJSON(err))
		return
	}

	if err := payment.ValidCaptureMethod(body.CaptureMethod); err != nil {
		respondError(w, r, err)
		return
	}
	captureMethod := payment.CaptureAutomatic
	if body.CaptureMethod == "manual" {
		captureMethod = payment.CaptureManual
	}

	intent, err := h.svc.Create(r.Context(), scopeOf(r), payment.CreateParams{
		Amount:             body.Amount,
		Currency:           body.Currency,
		CaptureMethod:      captureMethod,
		ConfirmationMethod: body.ConfirmationMethod,
		Metadata:           body.Metadata,
	})
	if err != nil {
		respondError(w, r, err)
		return
	}

	respond(w, r, http.StatusOK, intentResponse(intent))
}

// Retrieve handles GET /v1/payment_intents/:id
func (h *PaymentIntents) Retrieve(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	intent, err := h.svc.Retrieve(r.Context(), id, scopeOf(r))
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, intentResponse(intent))
}

// Confirm handles POST /v1/payment_intents/:id/confirm
func (h *PaymentIntents) Confirm(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		PaymentMethod string `json:"payment_method"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, r, errBadJSON(err))
		return
	}

	intent, err := h.svc.Confirm(r.Context(), id, scopeOf(r),
		payment.ConfirmParams{PaymentMethodID: body.PaymentMethod},
	)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, intentResponse(intent))
}

// Capture handles POST /v1/payment_intents/:id/capture
func (h *PaymentIntents) Capture(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		AmountToCapture int64 `json:"amount_to_capture"`
	}
	if err := decodeOptionalJSON(r, &body); err != nil {
		respondError(w, r, errBadJSON(err))
		return
	}

	intent, err := h.svc.Capture(r.Context(), id, scopeOf(r),
		payment.CaptureParams{AmountToCapture: body.AmountToCapture},
	)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, intentResponse(intent))
}

// Cancel handles POST /v1/payment_intents/:id/cancel
func (h *PaymentIntents) Cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var body struct {
		CancellationReason string `json:"cancellation_reason"`
	}
	if err := decodeOptionalJSON(r, &body); err != nil {
		respondError(w, r, errBadJSON(err))
		return
	}

	intent, err := h.svc.Cancel(r.Context(), id, scopeOf(r),
		payment.CancelParams{Reason: body.CancellationReason},
	)
	if err != nil {
		respondError(w, r, err)
		return
	}
	respond(w, r, http.StatusOK, intentResponse(intent))
}

// intentResponse converts an internal Intent to the Stripe-compatible wire format.
func intentResponse(i *payment.Intent) map[string]interface{} {
	return payment.Wire(i)
}
