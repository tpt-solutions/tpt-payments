package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runInit(t *testing.T, args ...string) error {
	t.Helper()
	cmd := InitCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd.Execute()
}

func TestInit_HonorsHomeFlag(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TPTCREDIT_HOME", filepath.Join(t.TempDir(), "should-not-be-used"))

	require.NoError(t, runInit(t, "node0", "--home", home, "--chain-id", "tpt-test-1"))

	_, err := os.Stat(filepath.Join(home, "config", "genesis.json"))
	require.NoError(t, err, "genesis must be written to --home")
}

func TestInit_FallsBackToEnvironment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("TPTCREDIT_HOME", home)

	require.NoError(t, runInit(t, "node0"))
	_, err := os.Stat(filepath.Join(home, "config", "genesis.json"))
	require.NoError(t, err)
}

func TestInit_RefusesToReplaceExistingGenesis(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, runInit(t, "node0", "--home", home))

	err := runInit(t, "node0", "--home", home)
	require.Error(t, err, "a second init must not silently replace the genesis")
	assert.Contains(t, err.Error(), "--overwrite")

	require.NoError(t, runInit(t, "node0", "--home", home, "--overwrite"))
}
