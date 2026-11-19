// Command compatreport measures which Stripe-compatible endpoints the core service serves.
//
// It builds the real HTTP router, walks its registered routes, and compares them with
// the reference endpoint list below. A row is "served" only if the router registers that
// method and path. This is a measurement of route presence, not of behaviour: request
// semantics are covered by the integration tests, and this report does not claim them.
//
// Usage (from the repository root):
//
//	go run ./packages/tpt-pay-core/cmd/compatreport --output compatibility-report.md
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/tpt-platform/tpt-platform/packages/tpt-pay-core/api"
)

// referenceEndpoints is the Stripe-compatible surface the project targets (API version
// 2024-06-20). Extend it as compatibility work is planned.
var referenceEndpoints = []endpoint{
	{"POST", "/v1/payment_intents"},
	{"GET", "/v1/payment_intents/{id}"},
	{"POST", "/v1/payment_intents/{id}/confirm"},
	{"POST", "/v1/payment_intents/{id}/capture"},
	{"POST", "/v1/payment_intents/{id}/cancel"},
	{"GET", "/v1/payment_intents"},
	{"POST", "/v1/refunds"},
	{"GET", "/v1/refunds/{id}"},
	{"GET", "/v1/refunds"},
	{"POST", "/v1/subscriptions"},
	{"GET", "/v1/subscriptions/{id}"},
	{"POST", "/v1/subscriptions/{id}/cancel"},
	{"GET", "/v1/subscriptions"},
	{"POST", "/v1/invoices"},
	{"GET", "/v1/invoices/{id}"},
	{"POST", "/v1/invoices/{id}/finalize"},
	{"POST", "/v1/invoices/{id}/pay"},
	{"GET", "/v1/invoices"},
	{"POST", "/v1/customers"},
	{"GET", "/v1/customers/{id}"},
	{"POST", "/v1/webhook_endpoints"},
	{"GET", "/v1/webhook_endpoints/{id}"},
	{"DELETE", "/v1/webhook_endpoints/{id}"},
	{"POST", "/v1/webhook_endpoints/{id}/test"},
	{"POST", "/v1/api_keys"},
	{"GET", "/v1/api_keys"},
	{"POST", "/v1/api_keys/{id}/revoke"},
}

type endpoint struct {
	Method string
	Path   string
}

func (e endpoint) key() string { return e.Method + " " + e.Path }

func main() {
	output := flag.String("output", "", "write the markdown report to this file (default: stdout)")
	flag.Parse()

	served, err := servedRoutes()
	if err != nil {
		fmt.Fprintln(os.Stderr, "compatreport:", err)
		os.Exit(1)
	}
	report := render(served)

	if *output == "" {
		fmt.Print(report)
		return
	}
	if err := os.WriteFile(*output, []byte(report), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "compatreport:", err)
		os.Exit(1)
	}
}

// servedRoutes returns the "METHOD /path" set registered by the real router.
// Dependencies are nil: route registration does not touch them.
func servedRoutes() (map[string]bool, error) {
	router := api.NewRouter(nil, nil, nil, nil, prometheus.NewRegistry())
	routes, ok := router.(chi.Routes)
	if !ok {
		return nil, fmt.Errorf("router does not expose its routes")
	}
	served := map[string]bool{}
	err := chi.Walk(routes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		served[method+" "+strings.TrimSuffix(route, "/")] = true
		return nil
	})
	return served, err
}

func render(served map[string]bool) string {
	var b strings.Builder
	var hit, miss int

	fmt.Fprintf(&b, "# Stripe-compatible route coverage\n\n")
	fmt.Fprintf(&b, "Generated: %s · Target: Stripe API 2024-06-20\n\n", time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "> This report measures **route presence** only: whether the router serves the method and path. ")
	fmt.Fprintf(&b, "It does not measure request or response behaviour; see the integration tests for that.\n\n")

	fmt.Fprintf(&b, "| Method | Path | Served |\n|---|---|---|\n")
	for _, e := range referenceEndpoints {
		mark := "❌ missing"
		if served[e.key()] {
			mark = "✅ served"
			hit++
		} else {
			miss++
		}
		fmt.Fprintf(&b, "| %s | `%s` | %s |\n", e.Method, e.Path, mark)
	}

	total := len(referenceEndpoints)
	pct := 0.0
	if total > 0 {
		pct = float64(hit) / float64(total) * 100
	}
	fmt.Fprintf(&b, "\n**Route coverage: %d of %d reference endpoints (%.0f%%).**\n", hit, total, pct)

	extra := extraRoutes(served)
	if len(extra) > 0 {
		fmt.Fprintf(&b, "\n## Served routes outside the reference list\n\n")
		for _, r := range extra {
			fmt.Fprintf(&b, "- `%s`\n", r)
		}
	}
	return b.String()
}

func extraRoutes(served map[string]bool) []string {
	reference := map[string]bool{}
	for _, e := range referenceEndpoints {
		reference[e.key()] = true
	}
	var extra []string
	for r := range served {
		if !reference[r] && !strings.HasSuffix(r, "/*") {
			extra = append(extra, r)
		}
	}
	sort.Strings(extra)
	return extra
}
