package ledger_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/ledger"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/testdb"
)

// These periods are long past, so they are settled. The ledger is empty for them, which makes
// the pack's root deterministic and keeps the tests independent of each other.
var (
	periodA = [2]time.Time{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)}
	periodB = [2]time.Time{time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)}
)

func TestCheckpoint_RecordIsIdempotentAndReadable(t *testing.T) {
	pool := testdb.Open(t)
	lgr := ledger.New(pool)
	ctx := context.Background()

	pack, err := lgr.BuildAuditPack(ctx, periodA[0], periodA[1])
	require.NoError(t, err)

	require.NoError(t, lgr.RecordCheckpoint(ctx, pack))
	require.NoError(t, lgr.RecordCheckpoint(ctx, pack), "recording the same root again is not an error")

	got, err := lgr.GetCheckpoint(ctx, "2024-01-01T00:00:00Z", "2024-02-01T00:00:00Z")
	require.NoError(t, err)
	assert.Equal(t, pack.MerkleRoot, got.MerkleRoot)
	assert.Equal(t, 0, got.TransactionCount)

	_, err = lgr.GetCheckpoint(ctx, "2019-01-01T00:00:00Z", "2019-02-01T00:00:00Z")
	assert.ErrorIs(t, err, ledger.ErrNoCheckpoint)
}

func TestCheckpoint_ChangedRootIsRefused(t *testing.T) {
	pool := testdb.Open(t)
	lgr := ledger.New(pool)
	ctx := context.Background()

	// An earlier record for the period says the root was something else.
	_, err := pool.Exec(ctx, `
		INSERT INTO ledger_checkpoints (period_start, period_end, merkle_root, transaction_count)
		VALUES ($1, $2, $3, 1)
	`, periodB[0], periodB[1], strings.Repeat("ab", 32))
	require.NoError(t, err)

	pack, err := lgr.BuildAuditPack(ctx, periodB[0], periodB[1])
	require.NoError(t, err)
	err = lgr.RecordCheckpoint(ctx, pack)
	assert.True(t, errors.Is(err, ledger.ErrCheckpointMismatch), "got %v", err)
}

func TestCheckpoint_RowsAreAppendOnly(t *testing.T) {
	pool := testdb.Open(t)
	lgr := ledger.New(pool)
	ctx := context.Background()

	pack, err := lgr.BuildAuditPack(ctx, periodA[0], periodA[1])
	require.NoError(t, err)
	require.NoError(t, lgr.RecordCheckpoint(ctx, pack))

	_, err = pool.Exec(ctx, `UPDATE ledger_checkpoints SET merkle_root = $1`, strings.Repeat("00", 32))
	assert.ErrorContains(t, err, "append-only")
	_, err = pool.Exec(ctx, `DELETE FROM ledger_checkpoints`)
	assert.ErrorContains(t, err, "append-only")
}
