package burn

import (
	"fmt"

	"cosmossdk.io/math"
)

// Snapshot is the complete persisted state of the burn module.
type Snapshot struct {
	GenesisHeight  int64                 `json:"genesis_height"`
	BlockHeight    int64                 `json:"block_height"`
	CurrentYear    int64                 `json:"current_year"`
	LastBurnYear   int64                 `json:"last_burn_year"`
	ReserveAddress string                `json:"reserve_address"`
	ReserveBalance math.Int              `json:"reserve_balance"`
	BurnRecords    map[string]BurnRecord `json:"burn_records"` // keyed "address/year"
	Summaries      []BurnSummary         `json:"summaries"`
}

// Snapshot returns a copy of the module state.
func (k *Keeper) Snapshot() Snapshot {
	records := make(map[string]BurnRecord, len(k.burnRecords))
	for key, rec := range k.burnRecords {
		records[key] = rec
	}
	summaries := make([]BurnSummary, len(k.summaries))
	copy(summaries, k.summaries)
	return Snapshot{
		GenesisHeight:  k.genesisHeight,
		BlockHeight:    k.blockHeight,
		CurrentYear:    k.currentYear,
		LastBurnYear:   k.lastBurnYear,
		ReserveAddress: k.reserveAddress,
		ReserveBalance: k.reserveBalance,
		BurnRecords:    records,
		Summaries:      summaries,
	}
}

// Restore replaces the module state with a snapshot after validating it. The keeper is left
// unchanged when validation fails.
func (k *Keeper) Restore(s Snapshot) error {
	if s.GenesisHeight <= 0 {
		return fmt.Errorf("snapshot genesis height must be positive, got %d", s.GenesisHeight)
	}
	if s.ReserveAddress != k.reserveAddress {
		return fmt.Errorf("snapshot reserve address %q does not match the configured reserve %q",
			s.ReserveAddress, k.reserveAddress)
	}
	if s.ReserveBalance.IsNil() || s.ReserveBalance.IsNegative() {
		return fmt.Errorf("snapshot reserve balance is missing or negative")
	}
	if s.LastBurnYear < 0 {
		return fmt.Errorf("snapshot last burn year is negative")
	}

	k.genesisHeight = s.GenesisHeight
	k.blockHeight = s.BlockHeight
	k.currentYear = s.CurrentYear
	k.lastBurnYear = s.LastBurnYear
	k.reserveBalance = s.ReserveBalance
	k.burnRecords = make(map[string]BurnRecord, len(s.BurnRecords))
	for key, rec := range s.BurnRecords {
		k.burnRecords[key] = rec
	}
	k.summaries = append(make([]BurnSummary, 0, len(s.Summaries)), s.Summaries...)
	return nil
}
