// Package ratelimit implements fixed-window request limits.
//
// Two backends share one interface. Redis is the backend for production: every API replica
// counts against the same window. Memory keeps counts in one process, for tests and single-node
// development only.
package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Decision is the outcome of one Allow call.
type Decision struct {
	Allowed bool
	// Limit is the number of requests allowed per window.
	Limit int
	// Remaining is the number of requests left in the current window after this one.
	Remaining int
	// RetryAfter is how long until the window resets. It is set when Allowed is false.
	RetryAfter time.Duration
}

// Limiter decides whether a request for a key may proceed.
type Limiter interface {
	Allow(ctx context.Context, key string) (Decision, error)
}

// windowStart returns the start of the fixed window containing now, aligned to the window size.
func windowStart(now time.Time, window time.Duration) time.Time {
	return now.Truncate(window)
}

func decide(count, limit int, now time.Time, window time.Duration) Decision {
	start := windowStart(now, window)
	d := Decision{
		Allowed:    count <= limit,
		Limit:      limit,
		Remaining:  max(limit-count, 0),
		RetryAfter: start.Add(window).Sub(now),
	}
	return d
}

// Redis counts requests per key in fixed windows stored in Redis.
type Redis struct {
	client *redis.Client
	limit  int
	window time.Duration
	prefix string
	now    func() time.Time
}

// NewRedis returns a Redis-backed limiter that allows limit requests per window for each key.
func NewRedis(client *redis.Client, limit int, window time.Duration) (*Redis, error) {
	if client == nil {
		return nil, fmt.Errorf("ratelimit: redis client is nil")
	}
	if limit <= 0 {
		return nil, fmt.Errorf("ratelimit: limit must be positive, got %d", limit)
	}
	if window <= 0 {
		return nil, fmt.Errorf("ratelimit: window must be positive, got %s", window)
	}
	return &Redis{client: client, limit: limit, window: window, prefix: "tpt:rl:", now: time.Now}, nil
}

// Allow increments the counter for key in the current window and reports the decision.
func (r *Redis) Allow(ctx context.Context, key string) (Decision, error) {
	now := r.now()
	start := windowStart(now, r.window)
	redisKey := fmt.Sprintf("%s%s:%d", r.prefix, key, start.UnixMilli())

	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, redisKey)
	// The key outlives its window by a second so a late read still sees the count.
	pipe.PExpire(ctx, redisKey, r.window+time.Second)
	if _, err := pipe.Exec(ctx); err != nil {
		return Decision{}, fmt.Errorf("ratelimit: redis: %w", err)
	}
	return decide(int(incr.Val()), r.limit, now, r.window), nil
}

// Memory counts requests in process memory.
type Memory struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	counts   map[string]memCount
	now      func() time.Time
	lastSwep time.Time
}

type memCount struct {
	window time.Time
	count  int
}

// NewMemory returns an in-process limiter. It is not shared between processes.
func NewMemory(limit int, window time.Duration) (*Memory, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("ratelimit: limit must be positive, got %d", limit)
	}
	if window <= 0 {
		return nil, fmt.Errorf("ratelimit: window must be positive, got %s", window)
	}
	return &Memory{limit: limit, window: window, counts: make(map[string]memCount), now: time.Now}, nil
}

// Allow increments the counter for key in the current window and reports the decision.
func (m *Memory) Allow(_ context.Context, key string) (Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := m.now()
	start := windowStart(now, m.window)

	// Drop counters from earlier windows once per window, so memory stays bounded by the
	// number of keys active in the current window.
	if now.Sub(m.lastSwep) >= m.window {
		for k, c := range m.counts {
			if !c.window.Equal(start) {
				delete(m.counts, k)
			}
		}
		m.lastSwep = now
	}

	c := m.counts[key]
	if !c.window.Equal(start) {
		c = memCount{window: start}
	}
	c.count++
	m.counts[key] = c
	return decide(c.count, m.limit, now, m.window), nil
}
