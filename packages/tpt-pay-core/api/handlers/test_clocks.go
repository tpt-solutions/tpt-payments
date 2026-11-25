package handlers

import (
	"net/http"
	"time"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apierrors"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/clock"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/middleware"
)

// maxAdvance bounds one advance call. Larger jumps are made in steps, so a typo cannot move the
// sandbox decades ahead in a single request.
const maxAdvance = 366 * 24 * time.Hour

// OutageScheduler is the part of the sandbox adapter the test clock endpoints control.
type OutageScheduler interface {
	AddOutage(o sandbox.Outage) error
	ClearOutages()
	Outages() []sandbox.Outage
}

// TestClocks handles the time-travel endpoints under /v1/test_clocks. They exist only for test
// keys: moving the sandbox clock or scheduling failures would otherwise be a way to break a live
// merchant's sandbox traffic, and the sandbox is never used for live money.
type TestClocks struct {
	clk     *clock.Test
	outages OutageScheduler
}

// NewTestClocks returns the handler for the sandbox clock and its outage schedule.
func NewTestClocks(clk *clock.Test, outages OutageScheduler) *TestClocks {
	return &TestClocks{clk: clk, outages: outages}
}

type testClockResponse struct {
	Object     string           `json:"object"`
	FrozenTime time.Time        `json:"frozen_time"`
	Outages    []sandbox.Outage `json:"outages"`
}

func (h *TestClocks) state() testClockResponse {
	outages := h.outages.Outages()
	if outages == nil {
		outages = []sandbox.Outage{}
	}
	return testClockResponse{Object: "test_clock", FrozenTime: h.clk.Now(), Outages: outages}
}

// requireTestKey refuses live keys. It returns false after writing the error response.
func requireTestKey(w http.ResponseWriter, r *http.Request) bool {
	key := middleware.AuthenticatedKey(r.Context())
	if key == nil || key.Mode != apikey.ModeTest {
		respondError(w, r, apierrors.PermissionDenied("Test clocks are only available with test keys (sk_test_...)."))
		return false
	}
	return true
}

// Get handles GET /v1/test_clocks: the sandbox time and the scheduled outages.
func (h *TestClocks) Get(w http.ResponseWriter, r *http.Request) {
	if !requireTestKey(w, r) {
		return
	}
	respond(w, r, http.StatusOK, h.state())
}

type advanceParams struct {
	Seconds    *int64     `json:"seconds,omitempty"`
	FrozenTime *time.Time `json:"frozen_time,omitempty"`
}

// Advance handles POST /v1/test_clocks/advance. Give exactly one of seconds (1 to 366 days) or
// frozen_time (an RFC 3339 time that is not before the current sandbox time).
func (h *TestClocks) Advance(w http.ResponseWriter, r *http.Request) {
	if !requireTestKey(w, r) {
		return
	}
	var p advanceParams
	if err := decodeJSON(r, &p); err != nil {
		respondError(w, r, apierrors.InvalidParam("body", err.Error()))
		return
	}
	if (p.Seconds == nil) == (p.FrozenTime == nil) {
		respondError(w, r, apierrors.InvalidParam("body", "give exactly one of seconds or frozen_time"))
		return
	}

	// Validate everything before the clock moves, so a refused request changes nothing.
	if p.Seconds != nil {
		d := time.Duration(*p.Seconds) * time.Second
		if *p.Seconds < 1 || d > maxAdvance {
			respondError(w, r, apierrors.InvalidParam("seconds", "must be between 1 and 31622400 (366 days) per call"))
			return
		}
		if _, err := h.clk.Advance(d); err != nil {
			respondError(w, r, apierrors.InvalidParam("seconds", "sandbox time only moves forward"))
			return
		}
	} else {
		now := h.clk.Now()
		if p.FrozenTime.Before(now) {
			respondError(w, r, apierrors.InvalidParam("frozen_time", "sandbox time only moves forward"))
			return
		}
		if p.FrozenTime.Sub(now) > maxAdvance {
			respondError(w, r, apierrors.InvalidParam("frozen_time", "must be within 366 days of the current sandbox time"))
			return
		}
		if _, err := h.clk.Set(*p.FrozenTime); err != nil {
			respondError(w, r, apierrors.InvalidParam("frozen_time", "sandbox time only moves forward"))
			return
		}
	}
	respond(w, r, http.StatusOK, h.state())
}

type outageParams struct {
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	DeclineCode string    `json:"decline_code,omitempty"`
}

// AddOutage handles POST /v1/test_clocks/outages: schedule a failure window in sandbox time.
func (h *TestClocks) AddOutage(w http.ResponseWriter, r *http.Request) {
	if !requireTestKey(w, r) {
		return
	}
	var p outageParams
	if err := decodeJSON(r, &p); err != nil {
		respondError(w, r, apierrors.InvalidParam("body", err.Error()))
		return
	}
	if p.From.IsZero() || p.To.IsZero() {
		respondError(w, r, apierrors.MissingParam("from and to"))
		return
	}
	if err := h.outages.AddOutage(sandbox.Outage{From: p.From, To: p.To, DeclineCode: p.DeclineCode}); err != nil {
		respondError(w, r, apierrors.InvalidParam("to", "must be after from"))
		return
	}
	respond(w, r, http.StatusOK, h.state())
}

// ClearOutages handles DELETE /v1/test_clocks/outages: remove every scheduled window.
func (h *TestClocks) ClearOutages(w http.ResponseWriter, r *http.Request) {
	if !requireTestKey(w, r) {
		return
	}
	h.outages.ClearOutages()
	respond(w, r, http.StatusOK, h.state())
}
