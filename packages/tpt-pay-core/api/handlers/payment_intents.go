package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/middleware"
	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/internal/payment"
)

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

	captureMethod := payment.CaptureAutomatic
	if body.CaptureMethod == "manual" {
		captureMethod = payment.CaptureManual
	}

	intent, err := h.svc.Create(r.Context(), middleware.MerchantID(r.Context()), payment.CreateParams{
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
	intent, err := h.svc.Retrieve(r.Context(), id, middleware.MerchantID(r.Context()))
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

	intent, err := h.svc.Confirm(r.Context(), id, middleware.MerchantID(r.Context()),
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
	_ = decodeJSON(r, &body) // optional body

	intent, err := h.svc.Capture(r.Context(), id, middleware.MerchantID(r.Context()),
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
	_ = decodeJSON(r, &body)

	intent, err := h.svc.Cancel(r.Context(), id, middleware.MerchantID(r.Context()),
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
	resp := map[string]interface{}{
		"id":                   i.ID,
		"object":               "payment_intent",
		"amount":               i.Amount,
		"currency":             i.Currency,
		"status":               string(i.Status),
		"capture_method":       string(i.CaptureMethod),
		"confirmation_method":  i.ConfirmationMethod,
		"created":              i.CreatedAt.Unix(),
		"livemode":             false,
	}
	if i.PaymentMethodID != "" {
		resp["payment_method"] = i.PaymentMethodID
	}
	if i.Metadata != nil {
		resp["metadata"] = i.Metadata
	}
	if i.CanceledAt != nil {
		resp["canceled_at"] = i.CanceledAt.Unix()
	}
	if i.CancellationReason != "" {
		resp["cancellation_reason"] = i.CancellationReason
	}
	return resp
}
