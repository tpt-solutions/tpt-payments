package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/policy"
)

const dryRunRules = `{"rules":[
  {"id":"big_block","when":[{"field":"amount","op":"gte","value":1000000}],"action":"block"},
  {"id":"vip_flag","when":[{"field":"metadata.tier","op":"eq","value":"vip"}],"action":"flag"}
]}`

func evaluate(t *testing.T, svc *payment.Service, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewPolicyEvaluator(svc)
	req := httptest.NewRequest(http.MethodPost, "/v1/policy/evaluate", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.Evaluate(rec, req)
	return rec
}

func TestPolicyEvaluate_ReturnsTheTrace(t *testing.T) {
	engine, err := policy.Parse([]byte(dryRunRules))
	require.NoError(t, err)
	svc := payment.NewService(nil, nil, nil)
	svc.SetPolicy(engine)

	rec := evaluate(t, svc, `{"amount":2000000,"currency":"usd","metadata":{"tier":"vip"}}`)
	require.Equal(t, http.StatusOK, rec.Code)
	var out struct {
		Decision string `json:"decision"`
		Rules    []struct {
			RuleID  string `json:"rule_id"`
			Matched bool   `json:"matched"`
		} `json:"rules"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
	assert.Equal(t, "block", out.Decision)
	require.Len(t, out.Rules, 2)
	assert.True(t, out.Rules[0].Matched)
	assert.True(t, out.Rules[1].Matched)
}

func TestPolicyEvaluate_WithoutRulesAllowsAndSaysSo(t *testing.T) {
	svc := payment.NewService(nil, nil, nil)
	rec := evaluate(t, svc, `{"amount":100,"currency":"usd"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "no merchant rules are configured")
	assert.Contains(t, rec.Body.String(), `"decision":"allow"`)
}

func TestPolicyEvaluate_ValidatesTheInput(t *testing.T) {
	svc := payment.NewService(nil, nil, nil)
	assert.Equal(t, http.StatusBadRequest, evaluate(t, svc, `{"amount":100}`).Code, "currency is required")
	assert.Equal(t, http.StatusBadRequest, evaluate(t, svc, `{"amount":100,"currency":"nope"}`).Code)
}
