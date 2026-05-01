// tpt-payments/backend/webhooks/webhook.go
package webhooks

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Event represents a webhook event
type Event struct {
	ID              string                 `json:"id"`
	Type            string                 `json:"type"`
	Created         int64                  `json:"created"`
	Data            map[string]interface{} `json:"data"`
	PreviousData    map[string]interface{} `json:"previous_data,omitempty"`
	Livemode        bool                   `json:"livemode"`
	PendingWebhooks int32                  `json:"pending_webhooks"`
	Request         string                 `json:"request,omitempty"`
}

// WebhookEndpoint represents a registered webhook endpoint for a merchant
type WebhookEndpoint struct {
	ID            string   `json:"id"`
	MerchantID    string   `json:"merchant_id"`
	URL           string   `json:"url"`
	EnabledEvents []string `json:"enabled_events"`
	Status        string   `json:"status"` // enabled, disabled
	Secret        string   `json:"secret"`
	CreatedAt     int64    `json:"created_at"`
}

// DeliveryAttempt represents a single attempt to deliver an event to an endpoint
type DeliveryAttempt struct {
	ID                string `json:"id"`
	EventID           string `json:"event_id"`
	EndpointID        string `json:"endpoint_id"`
	Status            string `json:"status"` // pending, succeeded, failed
	AttemptNumber     int    `json:"attempt_number"`
	ResponseStatusCode int   `json:"response_status_code,omitempty"`
	ResponseBody      string `json:"response_body,omitempty"`
	Error             string `json:"error,omitempty"`
	CreatedAt         int64  `json:"created_at"`
	ScheduledAt       int64  `json:"scheduled_at"`
}

// GenerateSignature generates a Stripe-compatible webhook signature
func GenerateSignature(payload []byte, secret string, timestamp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(fmt.Sprintf("%d.%s", timestamp, payload)))
	signature := hex.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("t=%d,v1=%s", timestamp, signature)
}

// VerifySignature verifies that a webhook payload has a valid signature
func VerifySignature(payload []byte, signatureHeader string, secret string, tolerance time.Duration) (bool, error) {
	var timestamp int64
	var receivedSignature string

	_, err := fmt.Sscanf(signatureHeader, "t=%d,v1=%s", &timestamp, &receivedSignature)
	if err != nil {
		return false, fmt.Errorf("invalid signature format")
	}

	if time.Now().UTC().Sub(time.Unix(timestamp, 0)) > tolerance {
		return false, fmt.Errorf("signature timestamp outside tolerance window")
	}

	expectedSignature := GenerateSignature(payload, secret, timestamp)
	return hmac.Equal([]byte(signatureHeader), []byte(expectedSignature)), nil
}

// NewEvent creates a new webhook event
func NewEvent(eventType string, data map[string]interface{}, livemode bool) *Event {
	return &Event{
		ID:              "evt_" + uuid.NewString(),
		Type:            eventType,
		Created:         time.Now().UTC().Unix(),
		Data:            data,
		Livemode:        livemode,
		PendingWebhooks: 0,
	}
}

// Serialize converts an event to JSON bytes
func (e *Event) Serialize() ([]byte, error) {
	return json.Marshal(e)
}

// NewWebhookEndpoint creates a new webhook endpoint
func NewWebhookEndpoint(merchantID string, url string, enabledEvents []string) *WebhookEndpoint {
	return &WebhookEndpoint{
		ID:            "we_" + uuid.NewString(),
		MerchantID:    merchantID,
		URL:           url,
		EnabledEvents: enabledEvents,
		Status:        "enabled",
		Secret:        "whsec_" + randomHex(32),
		CreatedAt:     time.Now().UTC().Unix(),
	}
}

// TestEvent generates a standard test event for endpoint testing
func TestEvent(endpointID string) *Event {
	return &Event{
		ID:              "evt_test_" + uuid.NewString(),
		Type:            "test.event",
		Created:         time.Now().UTC().Unix(),
		Data: map[string]interface{}{
			"test":        true,
			"endpoint_id": endpointID,
			"message":     "This is a test event from TPT Pay",
		},
		Livemode:        false,
		PendingWebhooks: 1,
	}
}

func randomHex(n int) string {
	bytes := make([]byte, n)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
