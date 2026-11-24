// Package webhook handles event creation, signing, and delivery scheduling.
//
// Signing is HMAC-SHA256 of "<timestamp>.<payload_bytes>" using the endpoint's
// secret. The header format is:
//
//	TPT-Signature: t=<unix_timestamp>,v1=<hex_signature>[,v1=<hex_signature>...]
//
// The scheme follows Stripe's webhook signing (same payload construction, same
// t=/v1= structure). The header NAME differs: Stripe uses "Stripe-Signature", so
// Stripe's verification libraries need their header option pointed at TPT-Signature.
package webhook

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EventType is the type of a webhook event.
type EventType string

const (
	EventPaymentIntentCreated   EventType = "payment_intent.created"
	EventPaymentIntentSucceeded EventType = "payment_intent.succeeded"
	EventPaymentIntentCanceled  EventType = "payment_intent.canceled"
	EventPaymentIntentFailed    EventType = "payment_intent.payment_failed"
	// EventChargeRefunded uses Stripe's name so existing receivers work unchanged.
	EventChargeRefunded EventType = "charge.refunded"
)

// Event is the webhook event envelope — matches Stripe's event object shape.
type Event struct {
	ID         string          `json:"id"`
	Type       EventType       `json:"type"`
	Created    int64           `json:"created"` // Unix timestamp
	Livemode   bool            `json:"livemode"`
	Data       json.RawMessage `json:"data"`
	APIVersion string          `json:"api_version"`
}

// NewEvent constructs an Event wrapping the given resource.
func NewEvent(eventType EventType, data interface{}, livemode bool) (*Event, error) {
	raw, err := json.Marshal(map[string]interface{}{"object": data})
	if err != nil {
		return nil, fmt.Errorf("marshal event data: %w", err)
	}
	return &Event{
		ID:         "evt_" + strings.ReplaceAll(uuid.New().String(), "-", ""),
		Type:       eventType,
		Created:    time.Now().Unix(),
		Livemode:   livemode,
		Data:       raw,
		APIVersion: "2024-06-20",
	}, nil
}

// Sign produces a TPT-Signature header value for the given payload and secret.
func Sign(payload []byte, secret string, t time.Time) string {
	ts := fmt.Sprintf("%d", t.Unix())
	return "t=" + ts + ",v1=" + computeSignature(payload, ts, secret)
}

// GenerateSecret creates a random webhook signing secret (whsec_ prefixed).
func GenerateSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("webhook: rand.Read failed: " + err.Error())
	}
	return "whsec_" + base64.StdEncoding.EncodeToString(b)
}

// RetrySchedule returns the delay before attempt n (1-indexed).
// Follows exponential backoff with a 72-hour total window.
func RetrySchedule(attempt int) time.Duration {
	delays := []time.Duration{
		0, // attempt 1: immediate
		5 * time.Minute,
		30 * time.Minute,
		2 * time.Hour,
		5 * time.Hour,
		10 * time.Hour,
		12 * time.Hour,
		12 * time.Hour,
		12 * time.Hour,
	}
	if attempt < 1 {
		return 0
	}
	if attempt-1 < len(delays) {
		return delays[attempt-1]
	}
	return 12 * time.Hour
}
