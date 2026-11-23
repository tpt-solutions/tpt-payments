package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenAPIMatchesRouter fails when a route is added or removed without updating
// openapi.json, so the published contract cannot drift from what is served.
func TestOpenAPIMatchesRouter(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, prometheus.NewRegistry(), nil)
	chiRoutes, ok := router.(chi.Routes)
	require.True(t, ok, "router must be a chi router")
	routes := map[string]bool{}
	require.NoError(t, chi.Walk(chiRoutes, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		route = strings.TrimSuffix(route, "/")
		// Paths under /v1 are relative to the spec's /v1 server.
		route = strings.TrimPrefix(route, "/v1")
		if route == "" {
			route = "/"
		}
		routes[strings.ToUpper(method)+" "+route] = true
		return nil
	}))

	var spec struct {
		Paths map[string]map[string]json.RawMessage `json:"paths"`
	}
	require.NoError(t, json.Unmarshal(openAPISpec, &spec))

	documented := map[string]bool{}
	for path, ops := range spec.Paths {
		for method := range ops {
			documented[strings.ToUpper(method)+" "+path] = true
		}
	}

	var missing, undocumented []string
	for r := range routes {
		if strings.HasSuffix(r, "/*") || strings.Contains(r, "/*") {
			continue
		}
		if !documented[r] {
			missing = append(missing, r)
		}
	}
	for d := range documented {
		if !routes[d] {
			undocumented = append(undocumented, d)
		}
	}
	sort.Strings(missing)
	sort.Strings(undocumented)
	assert.Empty(t, missing, "served but not documented in openapi.json")
	assert.Empty(t, undocumented, "documented in openapi.json but not served")
}

func TestOpenAPIIsServed(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, prometheus.NewRegistry(), nil)
	rec := serve(router, http.MethodGet, "/openapi.json")
	require.Equal(t, http.StatusOK, rec.Code)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &doc))
	assert.Equal(t, "3.0.3", doc["openapi"])
}
