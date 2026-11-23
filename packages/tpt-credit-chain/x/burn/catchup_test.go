package burn_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/burn"
)

// If the node was down across two year boundaries, both burns run on the first block
// back, in year order, and neither year is skipped.
func TestBeginBlock_CatchesUpMissedBurns(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)
	summary := k.BeginBlock(1 + 2*burn.BlocksPerYear)
	require.NotNil(t, summary, "a burn runs when boundaries were missed")

	assert.NotNil(t, k.GetBurnSummary(1), "year 1 burn ran")
	assert.NotNil(t, k.GetBurnSummary(2), "year 2 burn ran")
	assert.Nil(t, k.GetBurnSummary(3))
	assert.Equal(t, 1+2*burn.BlocksPerYear, summary.YearHeight,
		"the returned summary is the last burn executed")

	// Nothing is pending afterwards.
	assert.Nil(t, k.BeginBlock(1+2*burn.BlocksPerYear+1))
}
