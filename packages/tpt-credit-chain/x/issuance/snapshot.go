package issuance

import (
	"context"
	"fmt"
)

// Snapshot is the complete persisted state of the issuance module.
type Snapshot struct {
	GenesisHeight   int64            `json:"genesis_height"`
	BlockHeight     int64            `json:"block_height"`
	CurrentYear     int64            `json:"current_year"`
	Accounts        []HumanAccount   `json:"accounts"`
	TransferRecords []TransferRecord `json:"transfer_records"`
}

// Snapshot returns a copy of the module state. The copy shares no memory with the keeper.
func (k *Keeper) Snapshot() Snapshot {
	accounts := make([]HumanAccount, len(k.accounts))
	copy(accounts, k.accounts)
	records := make([]TransferRecord, len(k.transferRecords))
	copy(records, k.transferRecords)
	return Snapshot{
		GenesisHeight:   k.genesisHeight,
		BlockHeight:     k.blockHeight,
		CurrentYear:     k.currentYear,
		Accounts:        accounts,
		TransferRecords: records,
	}
}

// Restore replaces the module state with a snapshot. The snapshot is validated first, and the
// keeper is left unchanged if validation fails.
func (k *Keeper) Restore(s Snapshot) error {
	if s.GenesisHeight <= 0 {
		return fmt.Errorf("snapshot genesis height must be positive, got %d", s.GenesisHeight)
	}
	if s.CurrentYear < 1 {
		return fmt.Errorf("snapshot current year must be at least 1, got %d", s.CurrentYear)
	}
	seenAddr := make(map[string]bool, len(s.Accounts))
	seenUUID := make(map[string]bool, len(s.Accounts))
	for i, acct := range s.Accounts {
		if err := validateAccountFields(acct.Address, acct.IdentityUUID); err != nil {
			return fmt.Errorf("snapshot account %d: %w", i, err)
		}
		if seenAddr[acct.Address] || seenUUID[acct.IdentityUUID] {
			return fmt.Errorf("snapshot account %d: duplicate address or identity", i)
		}
		seenAddr[acct.Address] = true
		seenUUID[acct.IdentityUUID] = true
		if acct.YearCreditsReceived.IsNil() || acct.Balance.IsNil() {
			return fmt.Errorf("snapshot account %d: missing amounts", i)
		}
	}
	if err := k.InitializeGenesis(context.Background(), &GenesisState{HumanAccounts: s.Accounts}); err != nil {
		return err
	}
	k.genesisHeight = s.GenesisHeight
	k.blockHeight = s.BlockHeight
	k.currentYear = s.CurrentYear
	k.transferRecords = append(make([]TransferRecord, 0, len(s.TransferRecords)), s.TransferRecords...)
	return nil
}
