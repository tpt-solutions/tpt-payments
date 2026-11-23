package burn_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/burn"
)

func TestSnapshot_RoundTripKeepsBurnHistoryAndReserve(t *testing.T) {
	mock := NewMockIssuanceKeeper(1)
	mock.RegisterAccount(testAddrs[0], 10_000_000_001)
	mock.RegisterAccount(testAddrs[1], 6_000_000_000)
	k := burn.NewKeeper(mock, 1, testReserveAddr)
	require.NotNil(t, k.BeginBlock(1+burn.BlocksPerYear))

	snap := k.Snapshot()

	fresh := burn.NewKeeper(NewMockIssuanceKeeper(1), 1, testReserveAddr)
	require.NoError(t, fresh.Restore(snap))
	assert.Equal(t, k.ReserveBalance(), fresh.ReserveBalance())
	assert.Equal(t, k.GetBurnRecord(testAddrs[0], 1), fresh.GetBurnRecord(testAddrs[0], 1))
	require.NotNil(t, fresh.GetBurnSummary(1))
	assert.Equal(t, k.GetBurnSummary(1).TotalBurned, fresh.GetBurnSummary(1).TotalBurned)
}

func TestSnapshot_RestoreRefusesOtherReserve(t *testing.T) {
	k := burn.NewKeeper(NewMockIssuanceKeeper(1), 1, testReserveAddr)
	snap := k.Snapshot()
	snap.ReserveAddress = "cosmos1otherreserve"
	err := k.Restore(snap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserve address")
}

func TestSnapshot_RestoreRefusesNegativeReserve(t *testing.T) {
	k := burn.NewKeeper(NewMockIssuanceKeeper(1), 1, testReserveAddr)
	snap := k.Snapshot()
	snap.ReserveBalance = math.NewInt(-1)
	assert.Error(t, k.Restore(snap))
}

func TestSnapshot_IsACopy(t *testing.T) {
	k := burn.NewKeeper(NewMockIssuanceKeeper(1), 1, testReserveAddr)
	snap := k.Snapshot()
	snap.BurnRecords["x/1"] = burn.BurnRecord{Address: "x"}
	assert.Empty(t, k.GetBurnRecordsByAddress("x"), "changing a snapshot must not change the keeper")
}
