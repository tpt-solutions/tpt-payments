// Package clock provides the time source used by the sandbox. Production code uses System;
// the sandbox can use a Test clock that a test-mode caller moves forward, so payment flows that
// depend on time (card expiry, failure windows, timestamps) can be exercised without waiting.
package clock

import (
	"errors"
	"sync"
	"time"
)

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// System is the wall clock, in UTC.
type System struct{}

// Now returns the current UTC time.
func (System) Now() time.Time { return time.Now().UTC() }

// ErrBackwards is returned when a test clock is asked to move to an earlier time. Time only moves
// forward, so a recorded event cannot be placed before one that already happened.
var ErrBackwards = errors.New("clock: a test clock can only move forward")

// Test is a clock that only moves when it is told to. It is safe for concurrent use.
type Test struct {
	mu  sync.Mutex
	now time.Time
}

// NewTest returns a test clock set to start, in UTC.
func NewTest(start time.Time) *Test {
	return &Test{now: start.UTC()}
}

// Now returns the clock's current time.
func (t *Test) Now() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.now
}

// Advance moves the clock forward by d. A negative d is refused.
func (t *Test) Advance(d time.Duration) (time.Time, error) {
	if d < 0 {
		return time.Time{}, ErrBackwards
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.now = t.now.Add(d)
	return t.now, nil
}

// Set moves the clock to the given time. Times before the current time are refused.
func (t *Test) Set(to time.Time) (time.Time, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if to.Before(t.now) {
		return time.Time{}, ErrBackwards
	}
	t.now = to.UTC()
	return t.now, nil
}
