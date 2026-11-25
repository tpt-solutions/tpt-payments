package sandbox

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter"
)

// errOutage is what an adapter call returns during a network outage window. Like chaos, it is an
// unknown outcome, not a card decision.
var errOutage = errors.New("sandbox: simulated network outage")

// Outage is a window of sandbox time in which adapter calls fail. With an empty DeclineCode it is
// a network outage: every call returns an error. With a DeclineCode, Authorize returns that card
// decline instead (Capture, Void and Refund still fail with the network error, because a decline
// code does not apply to them).
type Outage struct {
	From        time.Time `json:"from"`
	To          time.Time `json:"to"`
	DeclineCode string    `json:"decline_code,omitempty"`
}

// ErrInvalidOutage is returned for a window that ends before it starts, or is empty.
var ErrInvalidOutage = errors.New("sandbox: an outage must end after it starts")

// AddOutage schedules a failure window. Windows may overlap; a call inside any window fails.
func (s *Sandbox) AddOutage(o Outage) error {
	if !o.To.After(o.From) {
		return ErrInvalidOutage
	}
	o.From = o.From.UTC()
	o.To = o.To.UTC()
	s.outMu.Lock()
	defer s.outMu.Unlock()
	s.outages = append(s.outages, o)
	return nil
}

// ClearOutages removes every scheduled window.
func (s *Sandbox) ClearOutages() {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	s.outages = nil
}

// Outages returns the scheduled windows, earliest first.
func (s *Sandbox) Outages() []Outage {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	out := append([]Outage(nil), s.outages...)
	sort.Slice(out, func(i, j int) bool { return out[i].From.Before(out[j].From) })
	return out
}

// injectOutage fails the call if the sandbox clock is inside a window. authorize reports whether
// the call is an Authorize, the only call a decline code applies to.
func (s *Sandbox) injectOutage(authorize bool) error {
	now := s.clock.Now()
	s.outMu.Lock()
	defer s.outMu.Unlock()
	for _, o := range s.outages {
		if now.Before(o.From) || !now.Before(o.To) {
			continue
		}
		if authorize && o.DeclineCode != "" {
			return &adapter.CardError{
				Code:    o.DeclineCode,
				Message: fmt.Sprintf("Declined during a scheduled sandbox window (%s).", o.DeclineCode),
			}
		}
		return errOutage
	}
	return nil
}

// expiredCard returns a card_expired decline when the token's expiry month has passed on the
// sandbox clock. The card is valid through the last day of its expiry month.
func (s *Sandbox) expiredCard(pmID string) error {
	s.mu.RLock()
	tok, ok := s.tokens[pmID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}
	now := s.clock.Now()
	endOfExpiry := time.Date(tok.expYear, time.Month(tok.expMonth)+1, 1, 0, 0, 0, 0, time.UTC)
	if !now.Before(endOfExpiry) {
		return &adapter.CardError{Code: "expired_card", Message: "Your card has expired."}
	}
	return nil
}
