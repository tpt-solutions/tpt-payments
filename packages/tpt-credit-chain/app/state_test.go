package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

func testAddress(t *testing.T, seed byte) string {
	t.Helper()
	payload := make([]byte, 20)
	for i := range payload {
		payload[i] = seed + byte(i)
	}
	addr, err := Bech32Encode("cosmos", payload)
	require.NoError(t, err)
	return addr
}

func testUUID(seed byte) string {
	return strings.Repeat(string([]byte{'a' + seed%6}), 64)
}

func newTestApp(t *testing.T, home string) *App {
	t.Helper()
	genesis := NewDefaultGenesis("tpt-test-1", "node")
	app, err := NewAppFromGenesis(home, genesis)
	require.NoError(t, err)
	return app
}

// A saved state restores the same accounts, heights and year in a fresh process.
func TestState_SaveAndRestoreRoundTrip(t *testing.T) {
	home := t.TempDir()
	first := newTestApp(t, home)
	addr := testAddress(t, 1)
	_, err := first.IssuanceKeeper.RegisterHuman(context.Background(), addr, testUUID(1), "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	first.IssuanceKeeper.BeginBlock(first.GenesisHeight + 500)
	first.BlockHeight = first.GenesisHeight + 500
	require.NoError(t, first.saveState())

	second := newTestApp(t, home)
	restored, err := second.loadState()
	require.NoError(t, err)
	require.True(t, restored)
	assert.Equal(t, first.BlockHeight, second.BlockHeight)
	assert.Equal(t, first.IssuanceKeeper.GetHumanCount(), second.IssuanceKeeper.GetHumanCount())
	acct := second.IssuanceKeeper.GetAccount(addr)
	require.NotNil(t, acct)
	assert.Equal(t, first.IssuanceKeeper.GetAccount(addr).NextDueAt, acct.NextDueAt)
	assert.Equal(t, first.IssuanceKeeper.GetCurrentYear(), second.IssuanceKeeper.GetCurrentYear())
}

// No state file means nothing to restore, and that is not an error.
func TestState_NoFileIsNotAnError(t *testing.T) {
	app := newTestApp(t, t.TempDir())
	restored, err := app.loadState()
	require.NoError(t, err)
	assert.False(t, restored)
}

// An app without a node home persists nothing.
func TestState_NoHomeMeansNoPersistence(t *testing.T) {
	app := newTestApp(t, "")
	path, err := app.StatePath()
	require.NoError(t, err)
	assert.Equal(t, "", path)
	require.NoError(t, app.saveState())
}

// A state file for another chain must stop startup rather than be loaded.
func TestState_RefusesAnotherChain(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, newTestApp(t, home).saveState())

	other := NewDefaultGenesis("tpt-other-9", "node")
	app, err := NewAppFromGenesis(home, other)
	require.NoError(t, err)
	_, err = app.loadState()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "belongs to chain")
}

// A corrupt file stops startup, and the error names the file.
func TestState_CorruptFileIsAnError(t *testing.T) {
	home := t.TempDir()
	app := newTestApp(t, home)
	path, err := app.StatePath()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte("{not json"), 0o600))
	_, err = app.loadState()
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Base(path))
}

// Saving leaves no temp files behind, and the file is replaced atomically.
func TestState_SaveLeavesNoTempFiles(t *testing.T) {
	home := t.TempDir()
	app := newTestApp(t, home)
	for i := 0; i < 3; i++ {
		require.NoError(t, app.saveState())
	}
	entries, err := os.ReadDir(home)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, stateFileName, entries[0].Name())
}
