package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
)

var clockStart = time.Date(2030, 3, 1, 0, 0, 0, 0, time.UTC)

func newClockHandler() (*TestClocks, *clock.Test, *sandbox.Sandbox) {
	clk := clock.NewTest(clockStart)
	s := sandbox.New(sandbox.Config{Clock: clk})
	return NewTestClocks(clk, s), clk, s
}

func withKey(r *http.Request, mode apikey.Mode) *http.Request {
	key := &apikey.Key{ID: "key_1", Type: apikey.TypeSecret, Mode: mode}
	return r.WithContext(middleware.WithAuthenticatedKey(r.Context(), key))
}

func do(t *testing.T, fn http.HandlerFunc, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

func TestTestClocks_LiveKeysAreRefused(t *testing.T) {
	h, _, _ := newClockHandler()
	for name, fn := range map[string]http.HandlerFunc{"get": h.Get, "advance": h.Advance, "outage": h.AddOutage, "clear": h.ClearOutages} {
		req := withKey(httptest.NewRequest(http.MethodPost, "/v1/test_clocks", strings.NewReader(`{"seconds":60}`)), apikey.ModeLive)
		rec := do(t, fn, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, name)
	}
	// No key at all is refused too.
	rec := do(t, h.Get, httptest.NewRequest(http.MethodGet, "/v1/test_clocks", nil))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestTestClocks_AdvanceBySeconds(t *testing.T) {
	h, clk, _ := newClockHandler()
	rec := do(t, h.Advance, withKey(httptest.NewRequest(http.MethodPost, "/v1/test_clocks/advance", strings.NewReader(`{"seconds":3600}`)), apikey.ModeTest))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, clk.Now().Equal(clockStart.Add(time.Hour)))

	var body struct {
		Object     string    `json:"object"`
		FrozenTime time.Time `json:"frozen_time"`
		Outages    []any     `json:"outages"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "test_clock", body.Object)
	assert.True(t, body.FrozenTime.Equal(clockStart.Add(time.Hour)))
	assert.NotNil(t, body.Outages, "an empty schedule is an empty list, not null")
}

func TestTestClocks_AdvanceValidation(t *testing.T) {
	h, clk, _ := newClockHandler()
	cases := map[string]string{
		"neither given":      `{}`,
		"both given":         `{"seconds":60,"frozen_time":"2030-04-01T00:00:00Z"}`,
		"zero seconds":       `{"seconds":0}`,
		"too far in seconds": `{"seconds":31622401}`,
		"backwards time":     `{"frozen_time":"2029-01-01T00:00:00Z"}`,
		"too far in time":    `{"frozen_time":"2035-01-01T00:00:00Z"}`,
		"unknown field":      `{"seconds":60,"speed":"fast"}`,
	}
	for name, body := range cases {
		rec := do(t, h.Advance, withKey(httptest.NewRequest(http.MethodPost, "/v1/test_clocks/advance", strings.NewReader(body)), apikey.ModeTest))
		assert.Equal(t, http.StatusBadRequest, rec.Code, name)
		assert.True(t, clk.Now().Equal(clockStart), "%s must not move the clock", name)
	}
}

func TestTestClocks_AdvanceToFrozenTime(t *testing.T) {
	h, clk, _ := newClockHandler()
	target := clockStart.AddDate(0, 6, 0)
	body := `{"frozen_time":"` + target.Format(time.RFC3339) + `"}`
	rec := do(t, h.Advance, withKey(httptest.NewRequest(http.MethodPost, "/v1/test_clocks/advance", strings.NewReader(body)), apikey.ModeTest))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.True(t, clk.Now().Equal(target))
}

func TestTestClocks_OutagesAddListClear(t *testing.T) {
	h, _, s := newClockHandler()
	from := clockStart.Add(time.Hour).Format(time.RFC3339)
	to := clockStart.Add(2 * time.Hour).Format(time.RFC3339)

	rec := do(t, h.AddOutage, withKey(httptest.NewRequest(http.MethodPost, "/v1/test_clocks/outages",
		strings.NewReader(`{"from":"`+from+`","to":"`+to+`","decline_code":"do_not_honor"}`)), apikey.ModeTest))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Len(t, s.Outages(), 1)
	assert.Contains(t, rec.Body.String(), "do_not_honor")

	rec = do(t, h.AddOutage, withKey(httptest.NewRequest(http.MethodPost, "/v1/test_clocks/outages",
		strings.NewReader(`{"from":"`+to+`","to":"`+from+`"}`)), apikey.ModeTest))
	assert.Equal(t, http.StatusBadRequest, rec.Code, "an outage must end after it starts")

	rec = do(t, h.AddOutage, withKey(httptest.NewRequest(http.MethodPost, "/v1/test_clocks/outages",
		strings.NewReader(`{"from":"`+from+`"}`)), apikey.ModeTest))
	assert.Equal(t, http.StatusBadRequest, rec.Code, "both ends are required")

	rec = do(t, h.ClearOutages, withKey(httptest.NewRequest(http.MethodDelete, "/v1/test_clocks/outages", nil), apikey.ModeTest))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, s.Outages())
}
