package issuance_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

func TestSnapshot_RoundTripKeepsAccounts(t *testing.T) {
	k := issuance.NewKeeperWithEvents(NewMockEventBus(), 1)
	_, err := k.RegisterHuman(context.Background(), testAddr1, testUUID1, "kyc", issuance.FrequencyWeekly)
	require.NoError(t, err)
	k.BeginBlock(1 + issuance.BlocksPerPayout(issuance.FrequencyWeekly))

	fresh := issuance.NewKeeper()
	require.NoError(t, fresh.Restore(k.Snapshot()))
	assert.Equal(t, k.GetBalance(testAddr1), fresh.GetBalance(testAddr1))
	assert.Equal(t, k.GetNextDueAt(testAddr1), fresh.GetNextDueAt(testAddr1))
	assert.Equal(t, k.GetBlockHeight(), fresh.GetBlockHeight())
}

func TestSnapshot_RestoreRefusesDuplicateIdentity(t *testing.T) {
	k := issuance.NewKeeperWithEvents(NewMockEventBus(), 1)
	_, err := k.RegisterHuman(context.Background(), testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	snap := k.Snapshot()
	snap.Accounts = append(snap.Accounts, snap.Accounts[0])
	snap.Accounts[1].Address = testAddr2
	err = issuance.NewKeeper().Restore(snap)
	require.Error(t, err)
}

func TestSnapshot_RestoreRejectsZeroGenesisHeight(t *testing.T) {
	snap := issuance.NewKeeper().Snapshot()
	snap.GenesisHeight = 0
	assert.Error(t, issuance.NewKeeper().Restore(snap))
}
