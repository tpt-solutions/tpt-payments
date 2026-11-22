// tpt-payments/backend/fraud/engine.go
package fraud

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// RiskScore represents calculated fraud risk
type RiskScore struct {
	Score      int               `json:"score"` // 0-100
	RiskLevel  string            `json:"risk_level"` // low, medium, high, critical
	Rules      []RuleResult      `json:"rules"`
	Timestamp  int64             `json:"timestamp"`
	RequestID  string            `json:"request_id"`
}

// RuleResult represents individual rule evaluation
type RuleResult struct {
	RuleID     string `json:"rule_id"`
	Name       string `json:"name"`
	Triggered  bool   `json:"triggered"`
	Score      int    `json:"score"`
	Severity   string `json:"severity"`
	Details    string `json:"details,omitempty"`
}

// RuleAction represents action to take based on risk
type RuleAction string

const (
	ActionAllow    RuleAction = "allow"
	ActionReview   RuleAction = "review"
	ActionBlock    RuleAction = "block"
	ActionFlag     RuleAction = "flag"
)

// Rule defines a fraud detection rule
type Rule interface {
	Evaluate(ctx context.Context, payment *PaymentContext) RuleResult
	GetID() string
	GetName() string
	GetSeverity() string
}

// PaymentContext contains all data for fraud evaluation
type PaymentContext struct {
	PaymentIntentID string          `json:"payment_intent_id"`
	Amount          decimal.Decimal `json:"amount"`
	Currency        string          `json:"currency"`
	CustomerID      string          `json:"customer_id"`
	CardFingerprint string          `json:"card_fingerprint"`
	IPAddress       string          `json:"ip_address"`
	Email           string          `json:"email"`
	BIN             string          `json:"bin"`
	Country         string          `json:"country"`
	UserAgent       string          `json:"user_agent"`
	MerchantID      string          `json:"merchant_id"`
	Metadata        map[string]string `json:"metadata"`
	Timestamp       int64           `json:"timestamp"`
}

// Engine is the main fraud scoring engine
type Engine struct {
	rules       []Rule
	thresholds  map[string]int
	actions     map[string]RuleAction
}

// NewEngine creates a new fraud scoring engine
func NewEngine() *Engine {
	return &Engine{
		rules: make([]Rule, 0),
		thresholds: map[string]int{
			"low":     30,
			"medium":  60,
			"high":    80,
			"critical": 100,
		},
		actions: make(map[string]RuleAction),
	}
}

// AddRule adds a rule to the engine
func (e *Engine) AddRule(rule Rule) {
	e.rules = append(e.rules, rule)
}

// Evaluate runs all rules against payment context
func (e *Engine) Evaluate(ctx context.Context, payment *PaymentContext) *RiskScore {
	score := &RiskScore{
		Timestamp: time.Now().Unix(),
		Rules:     make([]RuleResult, 0, len(e.rules)),
	}

	totalScore := 0

	for _, rule := range e.rules {
		result := rule.Evaluate(ctx, payment)
		score.Rules = append(score.Rules, result)
		if result.Triggered {
			totalScore += result.Score
		}
	}

	score.Score = min(totalScore, 100)

	switch {
	case score.Score < e.thresholds["low"]:
		score.RiskLevel = "low"
	case score.Score < e.thresholds["medium"]:
		score.RiskLevel = "medium"
	case score.Score < e.thresholds["high"]:
		score.RiskLevel = "high"
	default:
		score.RiskLevel = "critical"
	}

	return score
}

// GetAction returns recommended action based on risk score
func (e *Engine) GetAction(score *RiskScore) RuleAction {
	switch score.RiskLevel {
	case "low":
		return ActionAllow
	case "medium":
		return ActionFlag
	case "high":
		return ActionReview
	case "critical":
		return ActionBlock
	default:
		return ActionReview
	}
}

// SetMerchantThresholds sets custom thresholds per merchant
func (e *Engine) SetMerchantThresholds(merchantID string, thresholds map[string]int, actions map[string]RuleAction) {
	// Implementation will store per-merchant configuration
}

// EvaluatePayment runs fraud evaluation and returns action recommendation
func (e *Engine) EvaluatePayment(ctx context.Context, payment *PaymentContext) (RuleAction, *RiskScore) {
	score := e.Evaluate(ctx, payment)
	action := e.GetAction(score)
	return action, score
}

// GetMerchantActions returns action configuration for a merchant
func (e *Engine) GetMerchantActions(merchantID string) map[string]RuleAction {
	// Return merchant-specific action mappings
	return map[string]RuleAction{
		"low":      ActionAllow,
		"medium":   ActionFlag,
		"high":     ActionReview,
		"critical": ActionBlock,
	}
}

// min returns minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}