package sandbox

import (
	"errors"
	"math/rand/v2"
	"sync"
	"time"
)

// errChaos is the error an injected failure returns. It is a network error, not a card decision,
// so the payment service treats the outcome as unknown and leaves it for reconciliation, which is
// what a real timeout does.
var errChaos = errors.New("sandbox: chaos: simulated network failure")

// chaos fails a configured fraction of adapter calls at random. It is for testing that a
// merchant's integration and the payment service cope with unreliable networks. The random source
// is seeded, so a given seed fails the same calls in the same order.
type chaos struct {
	rate float64

	mu  sync.Mutex
	rng *rand.Rand
}

// newChaos returns nil when rate is zero, so a sandbox without chaos pays nothing for it. A zero
// seed means "pick one from the clock", which gives a different failure pattern on each start.
func newChaos(rate float64, seed int64) *chaos {
	if rate <= 0 {
		return nil
	}
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return &chaos{
		rate: rate,
		rng:  rand.New(rand.NewPCG(uint64(seed), uint64(seed)^0x9e3779b97f4a7c15)),
	}
}

// fail reports whether this call should fail. A nil chaos never fails.
func (c *chaos) fail() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rng.Float64() < c.rate
}

// injectChaos returns errChaos for a call chosen to fail, and nil otherwise.
func (s *Sandbox) injectChaos() error {
	if s.chaos.fail() {
		return errChaos
	}
	return nil
}
