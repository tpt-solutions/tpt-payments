package clock

import (
	"errors"
	"testing"
	"time"
)

func TestTestClockOnlyMovesForward(t *testing.T) {
	start := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewTest(start)

	if got := c.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
	if _, err := c.Advance(-time.Second); !errors.Is(err, ErrBackwards) {
		t.Fatalf("negative Advance: got %v, want ErrBackwards", err)
	}
	now, err := c.Advance(36 * time.Hour)
	if err != nil || !now.Equal(start.Add(36*time.Hour)) {
		t.Fatalf("Advance: got %v, %v", now, err)
	}
	if _, err := c.Set(start); !errors.Is(err, ErrBackwards) {
		t.Fatalf("Set to the past: got %v, want ErrBackwards", err)
	}
	target := time.Date(2031, 6, 1, 12, 0, 0, 0, time.FixedZone("x", 3600))
	if got, err := c.Set(target); err != nil || !got.Equal(target.UTC()) {
		t.Fatalf("Set forward: got %v, %v", got, err)
	}
}

func TestSystemClockIsUTC(t *testing.T) {
	if _, offset := (System{}).Now().Zone(); offset != 0 {
		t.Fatalf("system clock is not UTC (offset %d)", offset)
	}
}
