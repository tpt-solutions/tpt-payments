package apikey_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

// seedMerchant inserts a merchant into the same database as pool, so keys can reference it.
func seedMerchant(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ($1, $2) RETURNING id`, name, uuid.New().String()+"@test.invalid").Scan(&id))
	return id
}

func TestKeyLifecycle_GenerateAuthenticateRevoke(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	svc := apikey.NewService(pool)
	merchant := seedMerchant(t, ctx, pool, "Lifecycle")

	res, err := svc.Generate(ctx, merchant, "server", apikey.TypeSecret, apikey.ModeTest)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(res.RawKey, "sk_test_"))

	// The raw key is never stored: only its digest is.
	var stored int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM api_keys WHERE key_digest = $1`, apikey.Digest(res.RawKey)).Scan(&stored))
	assert.Equal(t, 1, stored)
	var raw int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM api_keys WHERE key_digest = $1 OR key_prefix = $1`, res.RawKey).Scan(&raw))
	assert.Equal(t, 0, raw, "the raw key must not appear in the table")

	key, err := svc.Authenticate(ctx, res.RawKey)
	require.NoError(t, err)
	assert.Equal(t, merchant, key.MerchantID)
	assert.Equal(t, apikey.TypeSecret, key.Type)
	assert.Equal(t, apikey.ModeTest, key.Mode)

	// Revoked keys stop working at once.
	require.NoError(t, svc.Revoke(ctx, key.ID, merchant))
	_, err = svc.Authenticate(ctx, res.RawKey)
	assert.ErrorIs(t, err, apikey.ErrInvalidKey)

	// Revoking again, or revoking another merchant's key, finds nothing to revoke.
	assert.ErrorIs(t, svc.Revoke(ctx, key.ID, merchant), pgx.ErrNoRows)
	other := seedMerchant(t, ctx, pool, "Other")
	res2, err := svc.Generate(ctx, merchant, "second", apikey.TypeSecret, apikey.ModeTest)
	require.NoError(t, err)
	assert.ErrorIs(t, svc.Revoke(ctx, res2.Key.ID, other), pgx.ErrNoRows, "a merchant cannot revoke another's key")
	_, err = svc.Authenticate(ctx, res2.RawKey)
	assert.NoError(t, err, "the failed cross-merchant revoke must not revoke the key")
}

func TestKeyLifecycle_UnknownAndMalformedKeysAreRejected(t *testing.T) {
	ctx := context.Background()
	svc := apikey.NewService(testdb.Open(t))
	for _, raw := range []string{"", "not-a-key", "sk_test_doesnotexist", "pk_live_zzz"} {
		_, err := svc.Authenticate(ctx, raw)
		assert.True(t, errors.Is(err, apikey.ErrInvalidKey), "%q must be refused as an invalid key", raw)
	}
}

func TestKeyLifecycle_LastUsedIsRecordedAsynchronously(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	svc := apikey.NewService(pool)
	merchant := seedMerchant(t, ctx, pool, "Usage")
	res, err := svc.Generate(ctx, merchant, "usage", apikey.TypeSecret, apikey.ModeTest)
	require.NoError(t, err)

	_, err = svc.Authenticate(ctx, res.RawKey)
	require.NoError(t, err)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var used *time.Time
		require.NoError(t, pool.QueryRow(ctx, `SELECT last_used_at FROM api_keys WHERE id = $1`, res.Key.ID).Scan(&used))
		if used != nil {
			break
		}
		require.True(t, time.Now().Before(deadline), "last_used_at was never recorded")
		time.Sleep(20 * time.Millisecond)
	}
}

func TestKeyLifecycle_ConcurrentAuthenticationIsSafe(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Open(t)
	svc := apikey.NewService(pool)
	merchant := seedMerchant(t, ctx, pool, "Concurrent")
	res, err := svc.Generate(ctx, merchant, "c", apikey.TypeSecret, apikey.ModeTest)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := svc.Authenticate(ctx, res.RawKey); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("authentication failed under concurrency: %v", err)
	}
}
