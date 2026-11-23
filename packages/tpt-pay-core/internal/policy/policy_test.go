package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleDoc = `{
  "rules": [
    {
      "id": "large_card_block",
      "description": "Block very large payments in test-mode currencies we do not serve",
      "when": [
        {"field": "amount", "op": "gte", "value": 1000000},
        {"field": "currency", "op": "in", "value": ["xyz", "abc"]}
      ],
      "action": "block"
    },
    {
      "id": "large_payment_review",
      "when": [{"field": "amount", "op": "gt", "value": 50000}],
      "action": "flag"
    },
    {
      "id": "vip_metadata",
      "when": [{"field": "metadata.tier", "op": "eq", "value": "vip"}],
      "action": "flag"
    }
  ]
}`

func mustParse(t *testing.T, doc string) *Engine {
	t.Helper()
	e, err := Parse([]byte(doc))
	require.NoError(t, err)
	return e
}

func TestEvaluate_AllowFlagAndBlock(t *testing.T) {
	e := mustParse(t, sampleDoc)
	assert.Equal(t, Allow, e.Evaluate(Input{Amount: 1000, Currency: "usd"}))
	assert.Equal(t, Flag, e.Evaluate(Input{Amount: 60000, Currency: "usd"}))
	assert.Equal(t, Flag, e.Evaluate(Input{Amount: 1000, Currency: "usd", Metadata: map[string]string{"tier": "vip"}}))
	assert.Equal(t, Block, e.Evaluate(Input{Amount: 2000000, Currency: "xyz"}))
}

// A block beats any number of flags, whatever the order of the rules.
func TestEvaluate_BlockBeatsFlag(t *testing.T) {
	e := mustParse(t, `{"rules":[
	  {"id":"a","when":[{"field":"amount","op":"gt","value":0}],"action":"flag"},
	  {"id":"b","when":[{"field":"amount","op":"gt","value":0}],"action":"block"}]}`)
	assert.Equal(t, Block, e.Evaluate(Input{Amount: 10}))

	reversed := mustParse(t, `{"rules":[
	  {"id":"b","when":[{"field":"amount","op":"gt","value":0}],"action":"block"},
	  {"id":"a","when":[{"field":"amount","op":"gt","value":0}],"action":"flag"}]}`)
	assert.Equal(t, Block, reversed.Evaluate(Input{Amount: 10}))
}

// All conditions in a rule must hold.
func TestEvaluate_ConditionsAreAnded(t *testing.T) {
	e := mustParse(t, sampleDoc)
	// Large amount but a currency the block rule does not list: only the flag rule applies.
	assert.Equal(t, Flag, e.Evaluate(Input{Amount: 2000000, Currency: "usd"}))
}

func TestEvaluate_LivemodeAndNotIn(t *testing.T) {
	e := mustParse(t, `{"rules":[
	  {"id":"live_only","when":[{"field":"livemode","op":"eq","value":true},{"field":"currency","op":"not_in","value":["usd"]}],"action":"block"}]}`)
	assert.Equal(t, Block, e.Evaluate(Input{Livemode: true, Currency: "eur"}))
	assert.Equal(t, Allow, e.Evaluate(Input{Livemode: true, Currency: "usd"}))
	assert.Equal(t, Allow, e.Evaluate(Input{Livemode: false, Currency: "eur"}))
}

// A missing metadata key does not match an eq rule, but does match not_in.
func TestEvaluate_MissingMetadata(t *testing.T) {
	eq := mustParse(t, `{"rules":[{"id":"r","when":[{"field":"metadata.tier","op":"eq","value":"vip"}],"action":"flag"}]}`)
	assert.Equal(t, Allow, eq.Evaluate(Input{}))

	notIn := mustParse(t, `{"rules":[{"id":"r","when":[{"field":"metadata.tier","op":"not_in","value":["basic"]}],"action":"flag"}]}`)
	assert.Equal(t, Flag, notIn.Evaluate(Input{}), "an absent key is not in the list")
}

// The dry run reports each rule and the same decision Evaluate returns.
func TestTrace_ReportsEachRuleAndAgreesWithEvaluate(t *testing.T) {
	e := mustParse(t, sampleDoc)
	in := Input{Amount: 60000, Currency: "usd", Metadata: map[string]string{"tier": "vip"}}
	decision, results := e.Trace(in)
	assert.Equal(t, e.Evaluate(in), decision)
	require.Len(t, results, 3)
	assert.Equal(t, "large_card_block", results[0].RuleID)
	assert.False(t, results[0].Matched)
	assert.True(t, results[1].Matched)
	assert.True(t, results[2].Matched)
}

func TestParse_RejectsBadRulesAndReportsEveryProblem(t *testing.T) {
	_, err := Parse([]byte(`{"rules":[
	  {"id":"","when":[{"field":"amount","op":"gt","value":1}],"action":"flag"},
	  {"id":"x","when":[{"field":"amount","op":"matches","value":1}],"action":"flag"},
	  {"id":"x","when":[{"field":"color","op":"eq","value":"red"}],"action":"deny"},
	  {"id":"y","when":[],"action":"flag"},
	  {"id":"z","when":[{"field":"currency","op":"gt","value":"usd"}],"action":"flag"}
	]}`))
	require.Error(t, err)
	msg := err.Error()
	for _, want := range []string{"id is required", "unknown operator", "duplicate id", "unknown field",
		`action must be "flag" or "block"`, "at least one condition", "currency does not support"} {
		assert.Contains(t, msg, want)
	}
}

func TestParse_TypeChecksValues(t *testing.T) {
	cases := map[string]string{
		"amount as string": `{"id":"r","when":[{"field":"amount","op":"eq","value":"1000"}],"action":"flag"}`,
		"livemode as text": `{"id":"r","when":[{"field":"livemode","op":"eq","value":"true"}],"action":"flag"}`,
		"in with a number": `{"id":"r","when":[{"field":"currency","op":"in","value":"usd"}],"action":"flag"}`,
		"empty in list":    `{"id":"r","when":[{"field":"currency","op":"in","value":[]}],"action":"flag"}`,
	}
	for name, rule := range cases {
		_, err := Parse([]byte(`{"rules":[` + rule + `]}`))
		assert.Error(t, err, name)
	}
}

func TestParse_RejectsUnknownFields(t *testing.T) {
	_, err := Parse([]byte(`{"rules":[{"id":"r","when":[{"field":"amount","op":"gt","value":1,"extra":true}],"action":"flag"}]}`))
	assert.Error(t, err, "a misspelt key must not be silently ignored")
}

func TestParse_EmptyDocumentAllowsEverything(t *testing.T) {
	e := mustParse(t, `{"rules":[]}`)
	assert.Equal(t, Allow, e.Evaluate(Input{Amount: 1}))
}
