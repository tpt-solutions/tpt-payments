// Package policy evaluates merchant-defined routing and fraud rules before a payment is created.
//
// Rules are data, not code: a JSON document lists rules, each rule is a set of conditions that
// must all hold, and an action. The document is validated when it is loaded, so a bad rule stops
// the server from starting instead of failing on the first payment. Evaluation is pure and
// deterministic, and a dry run returns the trace of every rule, so a rule can be checked before it
// is enforced.
//
// Actions: "flag" marks a payment for review; "block" refuses it. If any matching rule blocks, the
// decision is block. Otherwise any match makes it flag. Otherwise it is allow.
package policy

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Decision is the outcome of evaluating a payment.
type Decision string

const (
	Allow Decision = "allow"
	Flag  Decision = "flag"
	Block Decision = "block"
)

// Input is what the rules see about a payment.
type Input struct {
	Amount   int64
	Currency string
	Livemode bool
	Metadata map[string]string
}

// Condition is one test on one field.
type Condition struct {
	// Field is "amount", "currency", "livemode", or "metadata.<key>".
	Field string `json:"field"`
	// Op is one of eq, neq, gt, gte, lt, lte, in, not_in.
	Op string `json:"op"`
	// Value is a number (amount), a string (currency, metadata), a boolean (livemode), or a list
	// of strings for in and not_in.
	Value any `json:"value"`
}

// Rule is a named set of conditions and the action taken when all of them hold.
type Rule struct {
	ID          string      `json:"id"`
	Description string      `json:"description,omitempty"`
	When        []Condition `json:"when"`
	Action      string      `json:"action"` // "flag" or "block"
}

// Document is the file format: {"rules": [...]}.
type Document struct {
	Rules []Rule `json:"rules"`
}

// Engine is a validated set of rules.
type Engine struct {
	rules []Rule
}

// RuleResult is one line of a dry-run trace.
type RuleResult struct {
	RuleID  string `json:"rule_id"`
	Matched bool   `json:"matched"`
	Action  string `json:"action"`
}

var validOps = map[string]bool{
	"eq": true, "neq": true, "gt": true, "gte": true, "lt": true, "lte": true, "in": true, "not_in": true,
}

// Parse reads and validates a policy document. Every problem is reported, not only the first.
func Parse(data []byte) (*Engine, error) {
	var doc Document
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("policy: parse document: %w", err)
	}
	var problems []string
	seen := map[string]bool{}
	for i, r := range doc.Rules {
		where := fmt.Sprintf("rule %d (%q)", i, r.ID)
		if r.ID == "" {
			problems = append(problems, fmt.Sprintf("rule %d: id is required", i))
		} else if seen[r.ID] {
			problems = append(problems, where+": duplicate id")
		}
		seen[r.ID] = true
		if r.Action != "flag" && r.Action != "block" {
			problems = append(problems, where+`: action must be "flag" or "block"`)
		}
		if len(r.When) == 0 {
			problems = append(problems, where+": at least one condition is required")
		}
		for j, c := range r.When {
			if err := validateCondition(c); err != nil {
				problems = append(problems, fmt.Sprintf("%s condition %d: %v", where, j, err))
			}
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("policy: %s", strings.Join(problems, "; "))
	}
	return &Engine{rules: doc.Rules}, nil
}

func validateCondition(c Condition) error {
	if !validOps[c.Op] {
		return fmt.Errorf("unknown operator %q", c.Op)
	}
	switch {
	case c.Field == "amount":
		if c.Op == "in" || c.Op == "not_in" {
			return fmt.Errorf("amount takes a number, not a list")
		}
		if _, ok := c.Value.(float64); !ok {
			return fmt.Errorf("amount takes a number")
		}
	case c.Field == "currency":
		if c.Op == "gt" || c.Op == "gte" || c.Op == "lt" || c.Op == "lte" {
			return fmt.Errorf("currency does not support %s", c.Op)
		}
		if err := checkStringValue(c); err != nil {
			return err
		}
	case c.Field == "livemode":
		if c.Op != "eq" && c.Op != "neq" {
			return fmt.Errorf("livemode supports eq and neq")
		}
		if _, ok := c.Value.(bool); !ok {
			return fmt.Errorf("livemode takes true or false")
		}
	case strings.HasPrefix(c.Field, "metadata.") && len(c.Field) > len("metadata."):
		if c.Op == "gt" || c.Op == "gte" || c.Op == "lt" || c.Op == "lte" {
			return fmt.Errorf("metadata fields are strings and do not support %s", c.Op)
		}
		if err := checkStringValue(c); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown field %q (use amount, currency, livemode, or metadata.<key>)", c.Field)
	}
	return nil
}

func checkStringValue(c Condition) error {
	if c.Op == "in" || c.Op == "not_in" {
		list, ok := c.Value.([]any)
		if !ok || len(list) == 0 {
			return fmt.Errorf("%s takes a non-empty list of strings", c.Op)
		}
		for _, v := range list {
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%s takes a list of strings", c.Op)
			}
		}
		return nil
	}
	if _, ok := c.Value.(string); !ok {
		return fmt.Errorf("%s takes a string", c.Op)
	}
	return nil
}

// Rules returns the rules in the order they were defined.
func (e *Engine) Rules() []Rule {
	out := make([]Rule, len(e.rules))
	copy(out, e.rules)
	return out
}

// Evaluate returns the decision for a payment.
func (e *Engine) Evaluate(in Input) Decision {
	decision, _ := e.EvaluateWithFlags(in)
	return decision
}

// EvaluateWithFlags returns the decision and the IDs of the flag rules that matched. The IDs are
// empty when the decision is Allow or Block; a blocked payment is refused, so nothing is recorded.
func (e *Engine) EvaluateWithFlags(in Input) (Decision, []string) {
	decision := Allow
	var flags []string
	for _, r := range e.rules {
		if !matches(r, in) {
			continue
		}
		if r.Action == "block" {
			return Block, nil
		}
		decision = Flag
		flags = append(flags, r.ID)
	}
	return decision, flags
}

// Trace evaluates every rule and reports each one, for dry runs. The decision is the same one
// Evaluate returns.
func (e *Engine) Trace(in Input) (Decision, []RuleResult) {
	results := make([]RuleResult, 0, len(e.rules))
	decision := Allow
	for _, r := range e.rules {
		m := matches(r, in)
		results = append(results, RuleResult{RuleID: r.ID, Matched: m, Action: r.Action})
		if m && r.Action == "block" {
			decision = Block
		} else if m && decision != Block {
			decision = Flag
		}
	}
	return decision, results
}

func matches(r Rule, in Input) bool {
	for _, c := range r.When {
		if !holds(c, in) {
			return false
		}
	}
	return true
}

// holds reports whether one condition is true. A metadata key that is absent never matches,
// except for not_in, where absence means the value is not in the list.
func holds(c Condition, in Input) bool {
	switch {
	case c.Field == "amount":
		return compareNumber(float64(in.Amount), c)
	case c.Field == "currency":
		return compareString(in.Currency, c)
	case c.Field == "livemode":
		want := c.Value.(bool)
		if c.Op == "eq" {
			return in.Livemode == want
		}
		return in.Livemode != want
	default:
		key := strings.TrimPrefix(c.Field, "metadata.")
		value, present := in.Metadata[key]
		if !present {
			return c.Op == "not_in" || c.Op == "neq"
		}
		return compareString(value, c)
	}
}

func compareNumber(v float64, c Condition) bool {
	want := c.Value.(float64)
	switch c.Op {
	case "eq":
		return v == want
	case "neq":
		return v != want
	case "gt":
		return v > want
	case "gte":
		return v >= want
	case "lt":
		return v < want
	case "lte":
		return v <= want
	}
	return false
}

func compareString(v string, c Condition) bool {
	switch c.Op {
	case "eq":
		return v == c.Value.(string)
	case "neq":
		return v != c.Value.(string)
	case "in":
		return containsString(c.Value.([]any), v)
	case "not_in":
		return !containsString(c.Value.([]any), v)
	}
	return false
}

func containsString(list []any, v string) bool {
	for _, item := range list {
		if item.(string) == v {
			return true
		}
	}
	return false
}
