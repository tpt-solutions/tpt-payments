// tpt-payments/backend/payouts/notifications.go
package payouts

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)


// =============================================================================
// Payout Event Types
// =============================================================================

// PayoutEventType represents webhook event types for payouts
type PayoutEventType string

const (
	PayoutEventCreated   PayoutEventType = "payout.created"
	PayoutEventUpdated   PayoutEventType = "payout.updated"
	PayoutEventInTransit PayoutEventType = "payout.in_transit"
	PayoutEventPaid      PayoutEventType = "payout.paid"
	PayoutEventFailed    PayoutEventType = "payout.failed"
	PayoutEventCanceled  PayoutEventType = "payout.canceled"
	PayoutEventRetried   PayoutEventType = "payout.retried"
)

// PayoutEvent represents a webhook event for payout lifecycle changes
type PayoutEvent struct {
	ID        string          `json:"id"`
	Type      PayoutEventType `json:"type"`
	CreatedAt int64           `json:"created_at"`
	Data      PayoutEventData `json:"data"`
}

// PayoutEventData wraps the payout object in the event payload
type PayoutEventData struct {
	Object *Payout `json:"object"`
}

// =============================================================================
// Payout Event Dispatcher
// =============================================================================

// WebhookDeliveryFunc is a function that delivers events to webhook endpoints
// In production, this integrates with the platform's webhook delivery system
type WebhookDeliveryFunc func(ctx context.Context, eventType string, payload []byte) error

// PayoutEventDispatcher handles dispatching payout-related webhook events
type PayoutEventDispatcher struct {
	deliver WebhookDeliveryFunc
}

// NewPayoutEventDispatcher creates a new payout event dispatcher
func NewPayoutEventDispatcher(deliver WebhookDeliveryFunc) *PayoutEventDispatcher {
	return &PayoutEventDispatcher{
		deliver: deliver,
	}
}

// DispatchEvent creates and dispatches a payout event
func (d *PayoutEventDispatcher) DispatchEvent(ctx context.Context, eventType PayoutEventType, payout *Payout) error {
	event := PayoutEvent{
		ID:        fmt.Sprintf("evt_%s_%d", payout.ID, time.Now().Unix()),
		Type:      eventType,
		CreatedAt: time.Now().Unix(),
		Data: PayoutEventData{
			Object: payout,
		},
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal payout event: %w", err)
	}

	if d.deliver != nil {
		if err := d.deliver(ctx, string(eventType), payload); err != nil {
			// Log error but don't fail the payout operation
			// Webhook delivery failure should not block payout processing
			return fmt.Errorf("webhook delivery failed for event %s: %w", eventType, err)
		}
	}

	return nil
}

// DispatchFailureNotification sends immediate notification for payout failures
// This is used in addition to webhooks for critical failure alerts
func (d *PayoutEventDispatcher) DispatchFailureNotification(ctx context.Context, payout *Payout, reason string) error {
	// First dispatch the standard webhook event
	if err := d.DispatchEvent(ctx, PayoutEventFailed, payout); err != nil {
		return err
	}

	// Additional notification logic can be added here:
	// - Email to merchant
	// - SMS alert for high-value failures
	// - Slack/PagerDuty alert for platform operators
	// - Create notification record in database

	return nil
}

// =============================================================================
// Payout Failure Handler
// =============================================================================

// FailureHandler handles payout failures with retry logic and notifications
type FailureHandler struct {
	dispatcher *PayoutEventDispatcher
	maxRetries int
}

// NewFailureHandler creates a new failure handler
func NewFailureHandler(dispatcher *PayoutEventDispatcher, maxRetries int) *FailureHandler {
	return &FailureHandler{
		dispatcher: dispatcher,
		maxRetries: maxRetries,
	}
}

// ShouldRetry determines if a failed payout should be retried
func (h *FailureHandler) ShouldRetry(payout *Payout) bool {
	retryCount, _ := parseRetryCount(payout.Metadata)
	return retryCount < h.maxRetries
}

// CalculateRetryBackoff calculates exponential backoff with jitter for retries
func (h *FailureHandler) CalculateRetryBackoff(retryCount int) time.Duration {
	// Exponential backoff: 1h, 2h, 4h, 8h, etc.
	baseDelay := time.Duration(1<<retryCount) * time.Hour
	// Cap at 24 hours
	if baseDelay > 24*time.Hour {
		baseDelay = 24 * time.Hour
	}
	// Add jitter (±10%)
	jitter := time.Duration(float64(baseDelay) * 0.1)
	return baseDelay + jitter
}

// HandleFailure processes a payout failure, updates state, and notifies
func (h *FailureHandler) HandleFailure(ctx context.Context, payout *Payout, code string, message string) error {
	// Increment retry count
	retryCount, _ := parseRetryCount(payout.Metadata)
	retryCount++
	payout.Metadata["retry_count"] = fmt.Sprintf("%d", retryCount)

	if h.ShouldRetry(payout) {
		// Schedule retry
		backoff := h.CalculateRetryBackoff(retryCount)
		payout.Status = PayoutStatusPending
		payout.Metadata["retry_at"] = fmt.Sprintf("%d", time.Now().Add(backoff).Unix())
		payout.Metadata["last_failure_code"] = code
		payout.Metadata["last_failure_message"] = message

		// Dispatch retry event
		if h.dispatcher != nil {
			_ = h.dispatcher.DispatchEvent(ctx, PayoutEventRetried, payout)
		}
	} else {
		// Max retries exhausted - mark as permanently failed
		payout.MarkFailed(code, message)

		// Dispatch failure notification
		if h.dispatcher != nil {
			_ = h.dispatcher.DispatchFailureNotification(ctx, payout, message)
		}
	}

	payout.UpdatedAt = time.Now().Unix()
	return nil
}

// parseRetryCount extracts retry count from payout metadata
func parseRetryCount(metadata map[string]string) (int, error) {
	if val, ok := metadata["retry_count"]; ok {
		return strconv.Atoi(val)
	}
	return 0, nil
}
