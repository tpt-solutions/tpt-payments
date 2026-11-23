package handlers

import (
	"net/http"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/payment"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/policy"
)

// PolicyEvaluator handles POST /v1/policy/evaluate, the dry run of the merchant rules.
type PolicyEvaluator struct {
	svc *payment.Service
}

// NewPolicyEvaluator returns the dry-run handler.
func NewPolicyEvaluator(svc *payment.Service) *PolicyEvaluator {
	return &PolicyEvaluator{svc: svc}
}

// Evaluate reports what each rule would do for a payment with the given amount, currency and
// metadata, in the caller's mode. Nothing is created and nothing is recorded.
func (h *PolicyEvaluator) Evaluate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Amount   int64             `json:"amount"`
		Currency string            `json:"currency"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := decodeJSON(r, &body); err != nil {
		respondError(w, r, errBadJSON(err))
		return
	}
	if body.Currency == "" {
		respondError(w, r, apierrors.MissingParam("currency"))
		return
	}
	code, err := payment.NormalizeCurrency(body.Currency)
	if err != nil {
		respondError(w, r, err)
		return
	}

	engine := h.svc.Policy()
	if engine == nil {
		respond(w, r, http.StatusOK, map[string]any{
			"object":   "policy_evaluation",
			"decision": policy.Allow,
			"rules":    []policy.RuleResult{},
			"note":     "no merchant rules are configured",
		})
		return
	}

	scope := scopeOf(r)
	decision, trace := engine.Trace(policy.Input{
		Amount:   body.Amount,
		Currency: code,
		Livemode: scope.Livemode,
		Metadata: body.Metadata,
	})
	respond(w, r, http.StatusOK, map[string]any{
		"object":   "policy_evaluation",
		"decision": decision,
		"rules":    trace,
	})
}
