// Package webhook handles event creation, signing, and delivery scheduling.
//
// Signing is HMAC-SHA256 of "<timestamp>.<payload_bytes>" using the endpoint's
// secret. The resulting header format is:
//
//	TPT-Signature: t=<unix_timestamp>,v1=<hex_signature>
//
// This is intentionally compatible with Stripe's webhook signature scheme so
// that Stripe webhook verification libraries work unchanged.
package webhook

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	EventPaymentIntentFraud     EventType = "payment_intent.fraud_flagged"
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
// The format is compatible with Stripe's webhook signing scheme.
func Sign(payload []byte, secret string, t time.Time) string {
	ts := fmt.Sprintf("%d", t.Unix())
	signed := ts + "." + string(payload)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	sig := hex.EncodeToString(mac.Sum(nil))

	return "t=" + ts + ",v1=" + sig
}

// Verify checks a TPT-Signature header against the payload and secret.
// Returns an error if the signature is invalid or the timestamp is too old.
func Verify(payload []byte, header, secret string, tolerance time.Duration) error {
	parts := parseSignatureHeader(header)
	ts, ok := parts["t"]
	if !ok {
		return fmt.Errorf("webhook: missing timestamp in signature header")
	}
	v1, ok := parts["v1"]
	if !ok {
		return fmt.Errorf("webhook: missing v1 signature")
	}

	// Reconstruct the signed payload.
	signed := ts + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	expected := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(v1), []byte(expected)) {
		return fmt.Errorf("webhook: signature mismatch")
	}

	// Check timestamp tolerance.
	if tolerance > 0 {
		var tsUnix int64
		fmt.Sscanf(ts, "%d", &tsUnix)
		age := time.Since(time.Unix(tsUnix, 0))
		if age > tolerance {
			return fmt.Errorf("webhook: timestamp too old (%s > %s)", age, tolerance)
		}
	}
	return nil
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
		0,            // attempt 1: immediate
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

func parseSignatureHeader(header string) map[string]string {
	result := make(map[string]string)
	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		}
	}
	return result
}
