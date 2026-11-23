package issuance_test

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// A payout is due at NextDueAt and is not paid before it.
func TestBeginBlock_PayoutWaitsForNextDueAt(t *testing.T) {
	k := issuance.NewKeeperWithEvents(NewMockEventBus(), 1)
	_, err := k.RegisterHuman(context.Background(), testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	due := k.GetNextDueAt(testAddr1)
	require.Equal(t, int64(1)+issuance.BlocksPerPayout(issuance.FrequencyMonthly), due)

	assert.Len(t, k.BeginBlock(due-1), 0, "one block before the due height nothing is paid")
	assert.Len(t, k.BeginBlock(due), 1, "the payout is paid on the due height")
	assert.Equal(t, due+issuance.BlocksPerPayout(issuance.FrequencyMonthly), k.GetNextDueAt(testAddr1),
		"the next payout is scheduled one interval after the one just paid")
}

// A payout never takes the balance above MaxBalanceUcredit.
func TestBeginBlock_PayoutStopsAtBalanceCap(t *testing.T) {
	k := issuance.NewKeeperWithEvents(NewMockEventBus(), 1)
	_, err := k.RegisterHuman(context.Background(), testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	k.SetAccountBalance(testAddr1, math.NewInt(issuance.MaxBalanceUcredit-100))
	due := k.GetNextDueAt(testAddr1)

	events := k.BeginBlock(due)
	require.Len(t, events, 1)
	assert.Equal(t, "100", events[0].Amount, "the payout is trimmed to the room left under the cap")
	assert.True(t, k.GetBalance(testAddr1).Equal(math.NewInt(issuance.MaxBalanceUcredit)))

	// At the cap, the next due payout is not minted and stays due for a later block.
	next := k.GetNextDueAt(testAddr1)
	events = k.BeginBlock(next)
	assert.Len(t, events, 0, "no payout is minted above the cap")
	assert.True(t, k.GetBalance(testAddr1).Equal(math.NewInt(issuance.MaxBalanceUcredit)))
	msg, broken := k.CheckBalanceInvariant(context.Background())
	assert.False(t, broken, msg)
}
