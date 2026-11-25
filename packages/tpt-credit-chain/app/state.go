package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/burn"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// stateFormat is the version of the state file. Bump it when the layout changes.
const stateFormat = 1

// stateFileName is written inside the node home directory.
const stateFileName = "state.json"

// persistedState is the on-disk form of the chain state.
type persistedState struct {
	Format   int               `json:"format"`
	ChainID  string            `json:"chain_id"`
	Issuance issuance.Snapshot `json:"issuance"`
	Burn     burn.Snapshot     `json:"burn"`
}

// expandHome resolves a leading "~/" against the user's home directory.
func expandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, path[2:]), nil
}

// StatePath returns the state file location, or "" when the app has no node home (tests,
// in-memory runs), in which case nothing is persisted.
func (app *App) StatePath() (string, error) {
	if app.NodeHome == "" {
		return "", nil
	}
	home, err := expandHome(app.NodeHome)
	if err != nil {
		return "", err
	}
	return filepath.Join(home, stateFileName), nil
}

// saveState writes the chain state atomically: a temp file in the same directory, then a rename,
// so a crash never leaves a half-written state file. Caller must hold app.mu.
func (app *App) saveState() error {
	path, err := app.StatePath()
	if err != nil || path == "" {
		return err
	}
	data, err := json.MarshalIndent(persistedState{
		Format:   stateFormat,
		ChainID:  app.ChainID,
		Issuance: app.IssuanceKeeper.Snapshot(),
		Burn:     app.BurnKeeper.Snapshot(),
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode chain state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), stateFileName+".*.tmp")
	if err != nil {
		return fmt.Errorf("create state temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write state: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("sync state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close state: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace state file: %w", err)
	}
	return nil
}

// loadState restores the chain state from disk when a state file exists. It returns false when
// there is nothing to load. A state file for another chain, or in an unknown format, is an error:
// the node must not silently start from a different state.
func (app *App) loadState() (bool, error) {
	path, err := app.StatePath()
	if err != nil || path == "" {
		return false, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read state: %w", err)
	}
	var st persistedState
	if err := json.Unmarshal(data, &st); err != nil {
		return false, fmt.Errorf("parse state file %s: %w", path, err)
	}
	if st.Format != stateFormat {
		return false, fmt.Errorf("state file %s has format %d; this binary reads format %d", path, st.Format, stateFormat)
	}
	if st.ChainID != app.ChainID {
		return false, fmt.Errorf("state file %s belongs to chain %q, not %q", path, st.ChainID, app.ChainID)
	}
	if err := app.IssuanceKeeper.Restore(st.Issuance); err != nil {
		return false, fmt.Errorf("restore issuance state: %w", err)
	}
	if err := app.BurnKeeper.Restore(st.Burn); err != nil {
		return false, fmt.Errorf("restore burn state: %w", err)
	}
	app.BlockHeight = st.Issuance.BlockHeight
	return true, nil
}

// RemoveState deletes the state file in a node home, so that a re-initialized chain does not
// resume the state of the one it replaced. A missing file is not an error.
func RemoveState(home string) error {
	if home == "" {
		return nil
	}
	dir, err := expandHome(home)
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, stateFileName))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove chain state: %w", err)
	}
	return nil
}
