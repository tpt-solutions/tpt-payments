// tpt-payments/backend/fraud/events.go
package fraud

import (
	"context"
	"time"
)

// FraudEvent represents a fraud detection event
type FraudEvent struct {
	EventID          string     `json:"event_id"`
	Type             string     `json:"type"`
	PaymentIntentID  string     `json:"payment_intent_id"`
	MerchantID       string     `json:"merchant_id"`
	RiskScore        int        `json:"risk_score"`
	RiskLevel        string     `json:"risk_level"`
	RulesTriggered   []string   `json:"rules_triggered"`
	Action           RuleAction `json:"action"`
	Timestamp        int64      `json:"timestamp"`
}

// FraudWebhookEmitter interface for sending fraud events
type FraudWebhookEmitter interface {
	EmitFraudFlagged(ctx context.Context, event *FraudEvent) error
}

// EventService handles fraud event delivery
type EventService struct {
	webhookEmitter FraudWebhookEmitter
}

// NewEventService creates fraud event service
func NewEventService(webhookEmitter FraudWebhookEmitter) *EventService {
	return &EventService{
		webhookEmitter: webhookEmitter,
	}
}

// SendFraudFlaggedEvent sends a payment_intent.fraud_flagged webhook
func (s *EventService) SendFraudFlaggedEvent(ctx context.Context, payment *PaymentContext, score *RiskScore, action RuleAction) error {
	var rulesTriggered []string
	for _, rule := range score.Rules {
		if rule.Triggered {
			rulesTriggered = append(rulesTriggered, rule.RuleID)
		}
	}

	event := &FraudEvent{
		EventID:         "evt_fraud_" + generateID(),
		Type:            "payment_intent.fraud_flagged",
		PaymentIntentID: payment.PaymentIntentID,
		MerchantID:      payment.MerchantID,
		RiskScore:       score.Score,
		RiskLevel:       score.RiskLevel,
		RulesTriggered:  rulesTriggered,
		Action:          action,
		Timestamp:       time.Now().Unix(),
	}

	return s.webhookEmitter.EmitFraudFlagged(ctx, event)
}

// generateID generates a unique event ID
func generateID() string {
	return time.Now().Format("20060102150405")
}

// DisputeFeedback represents chargeback feedback for ML training
type DisputeFeedback struct {
	PaymentIntentID string `json:"payment_intent_id"`
	Disputed        bool   `json:"disputed"`
	DisputeReason   string `json:"dispute_reason"`
	FeedbackTime    int64  `json:"feedback_time"`
}

// MLModelHook interface for plugging in custom fraud models
type MLModelHook interface {
	GetScore(ctx context.Context, payment *PaymentContext) (int, error)
	Train(ctx context.Context, feedback []DisputeFeedback) error
}