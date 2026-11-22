package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The genesis files shipped in config/ must load with the same code path the node uses.
func TestShippedGenesisFilesLoad(t *testing.T) {
	for _, name := range []string{"devnet-genesis.json", "testnet-genesis.json"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "config", name)
			if _, err := os.Stat(path); err != nil {
				path = filepath.Join("..", "config", name)
			}
			raw, err := os.ReadFile(path)
			require.NoError(t, err)

			var doc GenesisDoc
			require.NoError(t, json.Unmarshal(raw, &doc), "GenesisDoc must parse %s", name)
			require.NotEmpty(t, doc.ChainID)
			require.NotEmpty(t, doc.AppState)

			// Every address in the bank balances must be a valid bech32 address.
			var state struct {
				Bank struct {
					Balances []struct {
						Address string `json:"address"`
					} `json:"balances"`
				} `json:"bank"`
			}
			require.NoError(t, json.Unmarshal(doc.AppState, &state))
			for _, b := range state.Bank.Balances {
				require.NoError(t, ValidAddress(b.Address, "cosmos"), "balance address %q in %s", b.Address, name)
			}
		})
	}
}
