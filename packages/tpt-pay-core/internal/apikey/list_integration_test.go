package apikey_test

// Listing keys against a real database. Skipped unless TEST_DATABASE_URL is set.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/apikey"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

func TestList_ScopesByMerchantAndModeAndPages(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	svc := apikey.NewService(pool)

	var merchant, other string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('List A', $1) RETURNING id`, uuid.New().String()+"@test.invalid").Scan(&merchant))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('List B', $1) RETURNING id`, uuid.New().String()+"@test.invalid").Scan(&other))

	var testIDs []string
	for i := 0; i < 3; i++ {
		res, err := svc.Generate(ctx, merchant, "test key", apikey.TypeSecret, apikey.ModeTest)
		require.NoError(t, err)
		testIDs = append(testIDs, res.Key.ID)
	}
	live, err := svc.Generate(ctx, merchant, "live key", apikey.TypeSecret, apikey.ModeLive)
	require.NoError(t, err)
	_, err = svc.Generate(ctx, other, "not ours", apikey.TypeSecret, apikey.ModeTest)
	require.NoError(t, err)

	// Test mode lists only the merchant's test keys.
	page1, more, err := svc.List(ctx, merchant, apikey.ModeTest, "", 2)
	require.NoError(t, err)
	assert.Len(t, page1, 2)
	assert.True(t, more, "a third key exists, so there is a next page")
	for _, k := range page1 {
		assert.Equal(t, apikey.ModeTest, k.Mode)
		assert.Equal(t, merchant, k.MerchantID)
	}

	page2, more, err := svc.List(ctx, merchant, apikey.ModeTest, page1[len(page1)-1].ID, 2)
	require.NoError(t, err)
	assert.Len(t, page2, 1)
	assert.False(t, more)

	seen := map[string]bool{}
	for _, k := range append(page1, page2...) {
		seen[k.ID] = true
	}
	for _, id := range testIDs {
		assert.True(t, seen[id], "every test key appears exactly once across pages")
	}
	assert.False(t, seen[live.Key.ID], "live keys are not in test mode")

	// Live mode sees only the live key.
	liveList, _, err := svc.List(ctx, merchant, apikey.ModeLive, "", 10)
	require.NoError(t, err)
	require.Len(t, liveList, 1)
	assert.Equal(t, live.Key.ID, liveList[0].ID)
}

func TestList_RevokedKeysStayListedWithTheirTime(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	svc := apikey.NewService(pool)

	var merchant string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Revoke', $1) RETURNING id`, uuid.New().String()+"@test.invalid").Scan(&merchant))
	res, err := svc.Generate(ctx, merchant, "soon revoked", apikey.TypeSecret, apikey.ModeTest)
	require.NoError(t, err)
	require.NoError(t, svc.Revoke(ctx, res.Key.ID, merchant))

	keys, _, err := svc.List(ctx, merchant, apikey.ModeTest, "", 10)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.NotNil(t, keys[0].RevokedAt, "a revoked key is still listed, with its revocation time")
}

func TestList_CursorFromAnotherMerchantIsRefused(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	svc := apikey.NewService(pool)

	var a, b string
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Cur A', $1) RETURNING id`, uuid.New().String()+"@test.invalid").Scan(&a))
	require.NoError(t, pool.QueryRow(ctx,
		`INSERT INTO merchants (name, email) VALUES ('Cur B', $1) RETURNING id`, uuid.New().String()+"@test.invalid").Scan(&b))
	foreign, err := svc.Generate(ctx, b, "theirs", apikey.TypeSecret, apikey.ModeTest)
	require.NoError(t, err)

	_, _, err = svc.List(ctx, a, apikey.ModeTest, foreign.Key.ID, 10)
	assert.ErrorIs(t, err, apikey.ErrCursorNotFound)
}
