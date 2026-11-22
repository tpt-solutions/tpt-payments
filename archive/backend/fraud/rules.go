// tpt-payments/backend/fraud/rules.go
package fraud

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

// VelocityRule checks transaction velocity per card
type VelocityRule struct {
	ID            string
	Name          string
	WindowMinutes int
	MaxAttempts   int
	Score         int
}

func NewVelocityRule(windowMinutes int, maxAttempts int, score int) *VelocityRule {
	return &VelocityRule{
		ID:            "velocity_" + string(rune(windowMinutes)),
		Name:          "Transaction Velocity Check",
		WindowMinutes: windowMinutes,
		MaxAttempts:   maxAttempts,
		Score:         score,
	}
}

func (r *VelocityRule) GetID() string { return r.ID }
func (r *VelocityRule) GetName() string { return r.Name }
func (r *VelocityRule) GetSeverity() string {
	if r.Score >= 70 {
		return "critical"
	} else if r.Score >= 40 {
		return "high"
	}
	return "medium"
}

func (r *VelocityRule) Evaluate(ctx context.Context, payment *PaymentContext) RuleResult {
	// Actual implementation with Redis will be added
	// This is the rule interface implementation
	return RuleResult{
		RuleID:    r.ID,
		Name:      r.Name,
		Triggered: false,
		Score:     0,
		Severity:  r.GetSeverity(),
	}
}

// AmountRule checks unusual transaction amounts
type AmountRule struct {
	ID            string
	Name          string
	MinAmount     decimal.Decimal
	MaxAmount     decimal.Decimal
	Score         int
}

func NewAmountRule(minAmount decimal.Decimal, maxAmount decimal.Decimal, score int) *AmountRule {
	return &AmountRule{
		ID:        "amount_range",
		Name:      "Transaction Amount Check",
		MinAmount: minAmount,
		MaxAmount: maxAmount,
		Score:     score,
	}
}

func (r *AmountRule) GetID() string { return r.ID }
func (r *AmountRule) GetName() string { return r.Name }
func (r *AmountRule) GetSeverity() string {
	if r.Score >= 70 {
		return "critical"
	} else if r.Score >= 40 {
		return "high"
	}
	return "medium"
}

func (r *AmountRule) Evaluate(ctx context.Context, payment *PaymentContext) RuleResult {
	triggered := payment.Amount.LessThan(r.MinAmount) || payment.Amount.GreaterThan(r.MaxAmount)
	
	result := RuleResult{
		RuleID:    r.ID,
		Name:      r.Name,
		Triggered: triggered,
		Severity:  r.GetSeverity(),
	}
	
	if triggered {
		result.Score = r.Score
		result.Details = "Amount outside normal range"
	}
	
	return result
}

// GeolocationRule checks geographic risk factors
type GeolocationRule struct {
	ID            string
	Name          string
	HighRiskCountries map[string]bool
	Score         int
}

func NewGeolocationRule(highRiskCountries []string, score int) *GeolocationRule {
	countryMap := make(map[string]bool)
	for _, c := range highRiskCountries {
		countryMap[c] = true
	}
	
	return &GeolocationRule{
		ID:                "geolocation",
		Name:              "Geographic Risk Check",
		HighRiskCountries: countryMap,
		Score:             score,
	}
}

func (r *GeolocationRule) GetID() string { return r.ID }
func (r *GeolocationRule) GetName() string { return r.Name }
func (r *GeolocationRule) GetSeverity() string {
	if r.Score >= 70 {
		return "critical"
	} else if r.Score >= 40 {
		return "high"
	}
	return "medium"
}

func (r *GeolocationRule) Evaluate(ctx context.Context, payment *PaymentContext) RuleResult {
	_, isHighRisk := r.HighRiskCountries[payment.Country]
	
	result := RuleResult{
		RuleID:    r.ID,
		Name:      r.Name,
		Triggered: isHighRisk,
		Severity:  r.GetSeverity(),
	}
	
	if isHighRisk {
		result.Score = r.Score
		result.Details = "Transaction from high risk country: " + payment.Country
	}
	
	return result
}

// StandardRuleSet returns default production rule set
func StandardRuleSet() []Rule {
	return []Rule{
		NewVelocityRule(60, 10, 30),
		NewVelocityRule(1440, 100, 50),
		NewAmountRule(decimal.NewFromInt(50), decimal.NewFromInt(1000000), 25),
		NewGeolocationRule([]string{}, 40),
	}
}