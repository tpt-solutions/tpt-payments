// tpt-payments/backend/adapters/bank_webhook_handler.go
package adapters

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// BankWebhookEvent represents a normalized webhook event from any bank sponsor
type BankWebhookEvent struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"`
	ObjectID        string                 `json:"object_id"`
	ObjectType      string                 `json:"object_type"`
	Status          string                 `json:"status"`
	CreatedAt       int64                  `json:"created_at"`
	RawData         map[string]interface{} `json:"raw_data"`
	BankReference   string                 `json:"bank_reference"`
	MerchantID      string                 `json:"merchant_id"`
}

// ColumnWebhookHandler validates and processes Column webhook events
type ColumnWebhookHandler struct {
	secret         []byte
	tolerance      time.Duration
	eventProcessor func(context.Context, BankWebhookEvent) error
}

// NewColumnWebhookHandler creates a new webhook handler for Column bank events
func NewColumnWebhookHandler(secret string, eventProcessor func(context.Context, BankWebhookEvent) error) *ColumnWebhookHandler {
	return &ColumnWebhookHandler{
		secret:         []byte(secret),
		tolerance:      5 * time.Minute,
		eventProcessor: eventProcessor,
	}
}

// ServeHTTP implements http.Handler interface for webhook endpoint
func (h *ColumnWebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	signatureHeader := r.Header.Get("Column-Signature")
	if signatureHeader == "" {
		http.Error(w, "missing signature header", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	if err := h.verifySignature(signatureHeader, body); err != nil {
		http.Error(w, fmt.Sprintf("invalid signature: %v", err), http.StatusUnauthorized)
		return
	}

	var event struct {
		ID         string                 `json:"id"`
		Type       string                 `json:"type"`
		Object     map[string]interface{} `json:"object"`
		CreatedAt  int64                  `json:"created_at"`
		Livemode   bool                   `json:"livemode"`
	}

	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "invalid event payload", http.StatusBadRequest)
		return
	}

	normalized, err := h.normalizeEvent(event)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to normalize event: %v", err), http.StatusBadRequest)
		return
	}

	normalized.RawData = event.Object

	if err := h.eventProcessor(ctx, normalized); err != nil {
		http.Error(w, fmt.Sprintf("failed to process event: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// verifySignature validates Column webhook HMAC signature
func (h *ColumnWebhookHandler) verifySignature(header string, body []byte) error {
	parts := strings.Split(header, ",")
	if len(parts) < 2 {
		return errors.New("invalid signature format")
	}

	var timestamp int64
	var signature string

	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			t, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return err
			}
			timestamp = t
		case "v1":
			signature = kv[1]
		}
	}

	if timestamp == 0 || signature == "" {
		return errors.New("missing timestamp or signature")
	}

	eventTime := time.Unix(timestamp, 0)
	if time.Since(eventTime) > h.tolerance {
		return errors.New("event timestamp outside tolerance window")
	}

	signedPayload := fmt.Sprintf("%d.%s", timestamp, body)

	mac := hmac.New(sha256.New, h.secret)
	mac.Write([]byte(signedPayload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return errors.New("signature mismatch")
	}

	return nil
}

// normalizeEvent converts Column event format to standard BankWebhookEvent
func (h *ColumnWebhookHandler) normalizeEvent(event struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	Object     map[string]interface{} `json:"object"`
	CreatedAt  int64                  `json:"created_at"`
}) (BankWebhookEvent, error) {
	normalized := BankWebhookEvent{
		ID:        event.ID,
		Type:      event.Type,
		CreatedAt: event.CreatedAt,
	}

	if id, ok := event.Object["id"].(string); ok {
		normalized.ObjectID = id
	}

	if merchantID, ok := event.Object["merchant_id"].(string); ok {
		normalized.MerchantID = merchantID
	}

	if networkID, ok := event.Object["network_transaction_id"].(string); ok {
		normalized.BankReference = networkID
	}

	if status, ok := event.Object["status"].(string); ok {
		normalized.Status = mapColumnStatus(status)
	}

	switch event.Type {
	case "authorization.created", "authorization.updated", "authorization.approved", "authorization.declined":
		normalized.ObjectType = "authorization"
	case "capture.created", "capture.completed", "capture.failed":
		normalized.ObjectType = "capture"
	case "void.created", "void.completed":
		normalized.ObjectType = "void"
	case "refund.created", "refund.completed", "refund.failed":
		normalized.ObjectType = "refund"
	case "settlement.completed":
		normalized.ObjectType = "settlement"
	}

	return normalized, nil
}

// BankWebhookReconciliationHandler processes webhook events to reconcile state
type BankWebhookReconciliationHandler struct {
	ledgerUpdateFunc       func(ctx context.Context, event BankWebhookEvent) error
	transactionUpdateFunc  func(ctx context.Context, event BankWebhookEvent) error
	webhookForwardFunc     func(ctx context.Context, event BankWebhookEvent) error
}

// NewBankWebhookReconciliationHandler creates the standard reconciliation handler
func NewBankWebhookReconciliationHandler(
	ledgerUpdate func(ctx context.Context, event BankWebhookEvent) error,
	transactionUpdate func(ctx context.Context, event BankWebhookEvent) error,
	webhookForward func(ctx context.Context, event BankWebhookEvent) error,
) *BankWebhookReconciliationHandler {
	return &BankWebhookReconciliationHandler{
		ledgerUpdateFunc:      ledgerUpdate,
		transactionUpdateFunc: transactionUpdate,
		webhookForwardFunc:    webhookForward,
	}
}

// ProcessEvent handles the full webhook processing pipeline
func (h *BankWebhookReconciliationHandler) ProcessEvent(ctx context.Context, event BankWebhookEvent) error {
	// 1. Update transaction state
	if err := h.transactionUpdateFunc(ctx, event); err != nil {
		return fmt.Errorf("transaction update failed: %w", err)
	}

	// 2. Post ledger entries
	if err := h.ledgerUpdateFunc(ctx, event); err != nil {
		return fmt.Errorf("ledger update failed: %w", err)
	}

	// 3. Forward event to merchant webhooks
	if err := h.webhookForwardFunc(ctx, event); err != nil {
		// Webhook delivery failure should not fail the webhook processing
		// Log error and continue
	}

	return nil
}