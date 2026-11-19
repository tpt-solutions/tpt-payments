package payment

// Wire returns the Stripe-compatible wire representation of an intent. The API and webhook
// events use the same shape, so a receiver sees identical fields in both places.
func Wire(i *Intent) map[string]any {
	resp := map[string]any{
		"id":                  i.ID,
		"object":              "payment_intent",
		"amount":              i.Amount,
		"currency":            i.Currency,
		"status":              string(i.Status),
		"capture_method":      string(i.CaptureMethod),
		"confirmation_method": i.ConfirmationMethod,
		"created":             i.CreatedAt.Unix(),
		"livemode":            i.Livemode,
	}
	if i.PaymentMethodID != "" {
		resp["payment_method"] = i.PaymentMethodID
	}
	if i.Metadata != nil {
		resp["metadata"] = i.Metadata
	}
	if i.AmountCaptured > 0 {
		resp["amount_captured"] = i.AmountCaptured
	}
	if i.LedgerTxnID != "" {
		resp["ledger_transaction_id"] = i.LedgerTxnID
	}
	if i.CanceledAt != nil {
		resp["canceled_at"] = i.CanceledAt.Unix()
	}
	if i.CancellationReason != "" {
		resp["cancellation_reason"] = i.CancellationReason
	}
	return resp
}
