package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/burn"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// queryView returns the app whose state the read-only query commands report, and a release
// function to call when they finish. In the process running the node, that is the live app, locked
// for the duration of the query. Anywhere else, it is a read-only copy of the last state the node
// wrote to its state file, so `query` works while the node runs in another process or is stopped.
//
// The copy is never written back and never produces blocks; it only reflects the last saved block.
func queryView(home string, lock bool) (*App, func(), error) {
	if GlobalApp != nil {
		if lock {
			GlobalApp.mu.Lock()
			return GlobalApp, GlobalApp.mu.Unlock, nil
		}
		return GlobalApp, func() {}, nil
	}
	view, err := readStateFile(home)
	if err != nil {
		return nil, nil, err
	}
	return view, func() {}, nil
}

// readStateFile restores a read-only app from the state file in a node home.
func readStateFile(home string) (*App, error) {
	if home == "" {
		home = DefaultNodeHome
	}
	dir, err := expandHome(home)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, stateFileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("no chain state at %s: start the node with 'tptcreditd start' first", path)
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var st persistedState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, fmt.Errorf("parse state file %s: %w", path, err)
	}
	if st.Format != stateFormat {
		return nil, fmt.Errorf("state file %s has format %d; this binary reads format %d", path, st.Format, stateFormat)
	}

	ik := issuance.NewKeeper()
	if err := ik.Restore(st.Issuance); err != nil {
		return nil, fmt.Errorf("restore issuance state: %w", err)
	}
	bk := burn.NewKeeper(NewIssuanceKeeperAdapter(ik), st.Burn.GenesisHeight, DefaultReserveAddress)
	if err := bk.Restore(st.Burn); err != nil {
		return nil, fmt.Errorf("restore burn state: %w", err)
	}
	return &App{
		ChainID:        st.ChainID,
		NodeHome:       "",
		GenesisHeight:  st.Burn.GenesisHeight,
		BlockHeight:    st.Issuance.BlockHeight,
		IssuanceKeeper: ik,
		BurnKeeper:     bk,
		// Events are not persisted, so a view read from disk has none. Its event counts are zero.
		events: &EventBus{},
	}, nil
}
