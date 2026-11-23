package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemory_AllowsLimitThenRejects(t *testing.T) {
	m, err := NewMemory(3, time.Minute)
	require.NoError(t, err)
	base := time.Date(2026, 1, 1, 12, 0, 10, 0, time.UTC)
	m.now = func() time.Time { return base }

	for i := 1; i <= 3; i++ {
		d, err := m.Allow(context.Background(), "k")
		require.NoError(t, err)
		assert.True(t, d.Allowed, "request %d", i)
		assert.Equal(t, 3-i, d.Remaining)
	}
	d, err := m.Allow(context.Background(), "k")
	require.NoError(t, err)
	assert.False(t, d.Allowed)
	assert.Equal(t, 0, d.Remaining)
	assert.Equal(t, 50*time.Second, d.RetryAfter, "window resets at 12:01:00")
}

func TestMemory_KeysAreIndependentAndWindowsReset(t *testing.T) {
	m, err := NewMemory(1, time.Minute)
	require.NoError(t, err)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m.now = func() time.Time { return now }

	d, _ := m.Allow(context.Background(), "a")
	assert.True(t, d.Allowed)
	d, _ = m.Allow(context.Background(), "b")
	assert.True(t, d.Allowed, "a different key has its own budget")
	d, _ = m.Allow(context.Background(), "a")
	assert.False(t, d.Allowed)

	now = now.Add(time.Minute)
	d, _ = m.Allow(context.Background(), "a")
	assert.True(t, d.Allowed, "a new window resets the count")
}

func TestRedis_SharedAcrossClients(t *testing.T) {
	srv := miniredis.RunT(t)
	newClient := func() *redis.Client { return redis.NewClient(&redis.Options{Addr: srv.Addr()}) }

	a, err := NewRedis(newClient(), 2, time.Minute)
	require.NoError(t, err)
	b, err := NewRedis(newClient(), 2, time.Minute)
	require.NoError(t, err)
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	b.now = func() time.Time { return now }

	ctx := context.Background()
	d, err := a.Allow(ctx, "key-1")
	require.NoError(t, err)
	assert.True(t, d.Allowed)
	d, err = b.Allow(ctx, "key-1")
	require.NoError(t, err)
	assert.True(t, d.Allowed)

	// The third request, arriving through the other replica, is over the shared limit.
	d, err = a.Allow(ctx, "key-1")
	require.NoError(t, err)
	assert.False(t, d.Allowed, "replicas share one counter")
	assert.Equal(t, time.Minute, d.RetryAfter)
}

func TestRedis_ReturnsErrorWhenUnavailable(t *testing.T) {
	srv := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	l, err := NewRedis(client, 5, time.Minute)
	require.NoError(t, err)
	srv.Close()

	_, err = l.Allow(context.Background(), "k")
	assert.Error(t, err, "an unreachable Redis is an error the caller can act on")
}

func TestConstructorsRejectBadConfig(t *testing.T) {
	_, err := NewMemory(0, time.Minute)
	assert.Error(t, err)
	_, err = NewMemory(1, 0)
	assert.Error(t, err)
	_, err = NewRedis(nil, 1, time.Minute)
	assert.Error(t, err)
}
