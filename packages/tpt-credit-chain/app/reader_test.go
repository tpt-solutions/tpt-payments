package app

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// The read-only view must report what the node last saved, with no node running in this process.
func TestReader_ReportsTheLastSavedState(t *testing.T) {
	prev := GlobalApp
	GlobalApp = nil
	t.Cleanup(func() { GlobalApp = prev })

	home := t.TempDir()
	node := newTestApp(t, home)
	addr := testAddress(t, 7)
	_, err := node.IssuanceKeeper.RegisterHuman(context.Background(), addr, testUUID(7), "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	node.IssuanceKeeper.BeginBlock(node.GenesisHeight + 42) // the issuance keeper holds the chain height
	node.BlockHeight = node.GenesisHeight + 42
	require.NoError(t, node.saveState())

	view, err := readStateFile(home)
	require.NoError(t, err)
	assert.Equal(t, node.ChainID, view.ChainID)
	assert.Equal(t, node.BlockHeight, view.BlockHeight)
	assert.Equal(t, node.GenesisHeight, view.GenesisHeight)
	acct := view.IssuanceKeeper.GetAccount(addr)
	require.NotNil(t, acct, "the registered human must be visible to the query")
	assert.Equal(t, node.IssuanceKeeper.GetHumanCount(), view.IssuanceKeeper.GetHumanCount())

	// The status report is built from the same view the node would report.
	snapshot := view.GetStateSnapshot()
	assert.Equal(t, node.ChainID, snapshot["chain_id"])
}

// The view never writes the state back: reading it leaves the file as it was.
func TestReader_DoesNotChangeTheStateFile(t *testing.T) {
	prev := GlobalApp
	GlobalApp = nil
	t.Cleanup(func() { GlobalApp = prev })

	home := t.TempDir()
	node := newTestApp(t, home)
	require.NoError(t, node.saveState())
	path, err := node.StatePath()
	require.NoError(t, err)

	before := mustRead(t, path)
	_, err = readStateFile(home)
	require.NoError(t, err)
	assert.Equal(t, before, mustRead(t, path))
}

// Without a state file the view says so, instead of reporting an empty chain.
func TestReader_NoStateIsAnError(t *testing.T) {
	prev := GlobalApp
	GlobalApp = nil
	t.Cleanup(func() { GlobalApp = prev })

	_, err := readStateFile(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no chain state")
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
