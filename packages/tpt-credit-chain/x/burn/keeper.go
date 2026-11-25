package burn

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"cosmossdk.io/math"
)

// EventBus is the interface for emitting events from the burn keeper.
type EventBus interface {
	// EmitAnnualBurn emits an annual burn event for a single account.
	EmitAnnualBurn(event EventAnnualBurn)
	// EmitAnnualBurnSummary emits the annual burn summary event.
	EmitAnnualBurnSummary(event EventAnnualBurnSummary)
}

// Account is the interface the burn module needs from an account implementation.
type Account struct {
	// Address is the Cosmos bech32 account address.
	Address string

	// Balance is the current ucredit balance of the account.
	Balance math.Int

	// IsRegistered indicates whether this account is a registered human.
	IsRegistered bool
}

// IssuanceKeeper is the interface the burn module needs from the issuance module.
type IssuanceKeeper interface {
	// GetAccounts returns all registered human accounts.
	GetAccounts() []IssuanceAccount

	// GetHumanCount returns the number of registered verified humans.
	GetHumanCount() int64

	// ResetYearCredits resets all year credits and payout counters to zero.
	ResetYearCredits()

	// GetGenesisHeight returns the block height at genesis.
	GetGenesisHeight() int64

	// SetAccountBalance sets the balance of an account.
	SetAccountBalance(address string, balance math.Int)

	// GetAccountBalance returns the balance of an account.
	GetAccountBalance(address string) math.Int
}

// IssuanceAccount is the minimal account info needed from the issuance module.
type IssuanceAccount struct {
	Address             string
	Balance             math.Int
	YearCreditsReceived math.Int
	YearPayoutCount     int64
	LastPaidAt          int64
	RegisteredAt        int64
}

// Keeper holds the burn module's state and business logic.
type Keeper struct {
	// issuanceKeeper is the reference to the issuance module.
	issuanceKeeper IssuanceKeeper

	// events is the event emitter (nil = no events emitted).
	events EventBus

	// burnRecords is the audit log of all burn events.
	// Keyed by "address/year" for queryable access.
	burnRecords map[string]BurnRecord

	// summaries is the audit log of burn summaries.
	summaries []BurnSummary

	// genesisHeight is the block height at genesis.
	genesisHeight int64

	// blockHeight is the current block height (set by BeginBlock).
	blockHeight int64

	// currentYear is the current year (1-based, computed from genesis).
	currentYear int64

	// reserveAddress is the address for protocol reserve remainder.
	reserveAddress string

	// reserveBalance holds burn remainders: the redistribution rounds down, and the leftover
	// ucredit is credited here so supply is conserved. It is not a human account.
	reserveBalance math.Int

	// lastBurnYear is the last year a burn was executed (0 = none yet).
	lastBurnYear int64
}

// NewKeeper creates a new burn Keeper with the given issuance keeper reference.
func NewKeeper(issuanceKeeper IssuanceKeeper, genesisHeight int64, reserveAddress string) *Keeper {
	return &Keeper{
		issuanceKeeper: issuanceKeeper,
		events:         nil,
		burnRecords:    make(map[string]BurnRecord),
		summaries:      make([]BurnSummary, 0),
		genesisHeight:  genesisHeight,
		blockHeight:    genesisHeight,
		currentYear:    1,
		reserveAddress: reserveAddress,
		reserveBalance: math.ZeroInt(),
		lastBurnYear:   0,
	}
}

// NewKeeperWithEvents creates a new burn Keeper with an event bus.
func NewKeeperWithEvents(issuanceKeeper IssuanceKeeper, events EventBus, genesisHeight int64, reserveAddress string) *Keeper {
	return &Keeper{
		issuanceKeeper: issuanceKeeper,
		events:         events,
		burnRecords:    make(map[string]BurnRecord),
		summaries:      make([]BurnSummary, 0),
		genesisHeight:  genesisHeight,
		blockHeight:    genesisHeight,
		currentYear:    1,
		reserveAddress: reserveAddress,
		reserveBalance: math.ZeroInt(),
		lastBurnYear:   0,
	}
}

// IsBurnBlock returns true if the given block height is an annual burn block.
// This function is provided to the issuance module's BeginBlock.
func (k *Keeper) IsBurnBlock(height int64) bool {
	if k.genesisHeight <= 0 {
		return false
	}

	// Burn blocks are at genesisHeight + n*BlocksPerYear for n = 1, 2, 3, ...
	elapsed := height - k.genesisHeight
	if elapsed <= 0 {
		return false
	}

	// Check if this height is exactly a multiple of BlocksPerYear from genesis.
	return elapsed%BlocksPerYear == 0
}

// BeginBlock is called at the start of each block.
// It checks if the current block is a burn block and executes the burn if so.
// Returns the annual burn summary if a burn occurred, nil otherwise.
//
// Every burn that is due and has not run yet is executed, in year order. Normally that
// is the single burn on the boundary block. If the node was down across one or more
// boundaries, the missed burns run on the first block after recovery, so no year is
// skipped. Returns the summary of the last burn executed, or nil.
func (k *Keeper) BeginBlock(height int64) *BurnSummary {
	k.blockHeight = height

	if k.genesisHeight <= 0 {
		return nil
	}
	elapsed := height - k.genesisHeight
	if elapsed <= 0 {
		k.currentYear = 1
		return nil
	}

	completedYears := elapsed / BlocksPerYear
	k.currentYear = 1 + completedYears

	var last *BurnSummary
	for year := k.lastBurnYear + 1; year <= completedYears; year++ {
		last = k.executeBurn(year)
	}
	return last
}

// executeBurn performs the annual burn and redistribution.
func (k *Keeper) executeBurn(year int64) *BurnSummary {
	k.lastBurnYear = year

	accounts := k.issuanceKeeper.GetAccounts()
	// Process accounts in address order so events and records do not depend on the
	// order in which accounts were stored.
	sort.SliceStable(accounts, func(i, j int) bool { return accounts[i].Address < accounts[j].Address })
	activeHumans := k.issuanceKeeper.GetHumanCount()
	burnHeight := k.blockHeight

	// Phase 1: Calculate the total burn pool (sum of all balances).
	var totalBurned int64
	burnResults := make([]struct {
		address       string
		burnedAmount  int64
		balanceBefore math.Int
	}, len(accounts))

	for i, acct := range accounts {
		bal := acct.Balance
		// A balance outside int64 cannot be burned without truncation. Halt rather than
		// burn the wrong amount.
		if !bal.IsInt64() {
			panic(fmt.Sprintf("burn: balance of %s (%s) does not fit in int64; refusing to truncate", acct.Address, bal))
		}
		burnAmount := CalculateBurnAmount(bal.Int64())
		burnResults[i].address = acct.Address
		burnResults[i].burnedAmount = burnAmount
		burnResults[i].balanceBefore = bal
		totalBurned += burnAmount
	}

	// Phase 2: Calculate redistribution.
	perHuman, remainder := CalculateRedistribution(totalBurned, activeHumans)
	totalRedistributed := activeHumans * perHuman

	// Phase 3: Execute burn and redistribution for each account.
	perHumanInt := math.NewInt(perHuman)

	for _, br := range burnResults {
		// Set the account's new balance = redistribution share.
		k.issuanceKeeper.SetAccountBalance(br.address, perHumanInt)

		// Log the burn record.
		record := BurnRecord{
			Address:             br.address,
			BurnedAmount:        br.burnedAmount,
			RedistributedAmount: perHuman,
			YearHeight:          burnHeight,
		}
		key := fmt.Sprintf("%s/%d", br.address, year)
		k.burnRecords[key] = record

		// Build burn event.
		burnEvent := EventAnnualBurn{
			Address:             br.address,
			BurnedAmount:        br.burnedAmount,
			RedistributedAmount: perHuman,
			YearHeight:          burnHeight,
			Year:                year,
		}

		if k.events != nil {
			k.events.EmitAnnualBurn(burnEvent)
		}
	}

	// Credit the rounding remainder to the reserve so the burned total is conserved:
	// humans' new balances plus the reserve equal the amount burned.
	k.reserveBalance = k.reserveBalance.Add(math.NewInt(remainder))

	// Phase 4: Reset year credits in the issuance module.
	k.issuanceKeeper.ResetYearCredits()

	// Phase 5: Build and log the burn summary.
	summary := BurnSummary{
		YearHeight:         burnHeight,
		TotalBurned:        totalBurned,
		TotalRedistributed: totalRedistributed,
		ActiveHumans:       activeHumans,
		PerHumanAmount:     perHuman,
		Remainder:          remainder,
	}
	k.summaries = append(k.summaries, summary)

	// Emit summary event.
	if k.events != nil {
		summaryEvent := EventAnnualBurnSummary{
			YearHeight:         burnHeight,
			Year:               year,
			TotalBurned:        totalBurned,
			TotalRedistributed: totalRedistributed,
			ActiveHumans:       activeHumans,
			PerHumanAmount:     perHuman,
			Remainder:          remainder,
			ReserveAddress:     k.reserveAddress,
		}
		k.events.EmitAnnualBurnSummary(summaryEvent)
	}

	return &summary
}

// ReserveBalance returns the accumulated burn remainder held by the protocol reserve.
func (k *Keeper) ReserveBalance() math.Int {
	return k.reserveBalance
}

// GetBurnRecord returns the burn record for a specific account and year.
// Returns nil if no record exists.
func (k *Keeper) GetBurnRecord(address string, year int64) *BurnRecord {
	key := fmt.Sprintf("%s/%d", address, year)
	record, exists := k.burnRecords[key]
	if !exists {
		return nil
	}
	return &record
}

// GetBurnRecordsByAddress returns all burn records for a given account,
// sorted by year ascending.
func (k *Keeper) GetBurnRecordsByAddress(address string) []BurnRecord {
	var records []BurnRecord
	for _, record := range k.burnRecords {
		if record.Address == address {
			records = append(records, record)
		}
	}
	// Sort by year height ascending.
	sort.Slice(records, func(i, j int) bool {
		return records[i].YearHeight < records[j].YearHeight
	})
	return records
}

// GetBurnRecordsByYear returns all burn records for a given year.
func (k *Keeper) GetBurnRecordsByYear(year int64) []BurnRecord {
	yearPrefix := fmt.Sprintf("/%d", year)
	var records []BurnRecord
	for key, record := range k.burnRecords {
		// Key format: "address/year"
		if strings.HasSuffix(key, yearPrefix) {
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Address < records[j].Address })
	return records
}

// GetBurnSummary returns the burn summary for a given year index (1-based).
// Returns nil if no summary exists for that year.
func (k *Keeper) GetBurnSummary(yearIndex int64) *BurnSummary {
	// year 1 burn is at index 0 in summaries, year 2 at index 1, etc.
	idx := yearIndex - 1
	if idx < 0 || idx >= int64(len(k.summaries)) {
		return nil
	}
	return &k.summaries[idx]
}

// GetAllBurnSummaries returns all burn summaries.
func (k *Keeper) GetAllBurnSummaries() []BurnSummary {
	result := make([]BurnSummary, len(k.summaries))
	copy(result, k.summaries)
	return result
}

// GetCurrentYear returns the current year.
func (k *Keeper) GetCurrentYear() int64 {
	return k.currentYear
}

// GetBlockHeight returns the current block height.
func (k *Keeper) GetBlockHeight() int64 {
	return k.blockHeight
}

// GetGenesisHeight returns the genesis block height.
func (k *Keeper) GetGenesisHeight() int64 {
	return k.genesisHeight
}

// SetBlockHeight updates the keeper's block height (used in testing).
func (k *Keeper) SetBlockHeight(height int64) {
	k.blockHeight = height
}

// SetCurrentYear updates the keeper's current year (used in testing).
func (k *Keeper) SetCurrentYear(year int64) {
	k.currentYear = year
}

// SetLastBurnYear sets the last burn year (used in testing).
func (k *Keeper) SetLastBurnYear(year int64) {
	k.lastBurnYear = year
}

// GetLastBurnYear returns the last year a burn was executed.
func (k *Keeper) GetLastBurnYear() int64 {
	return k.lastBurnYear
}

// GetBurnRecordCount returns the total number of burn records.
func (k *Keeper) GetBurnRecordCount() int {
	return len(k.burnRecords)
}

// IsRegisteredAccount checks if an address is a registered human.
func (k *Keeper) IsRegisteredAccount(address string) bool {
	for _, acct := range k.issuanceKeeper.GetAccounts() {
		if acct.Address == address {
			return true
		}
	}
	return false
}

// GetActiveHumans returns the current count of active verified humans.
func (k *Keeper) GetActiveHumans() int64 {
	return k.issuanceKeeper.GetHumanCount()
}

// GetBurnSummaryByHeight returns the burn summary for a given block height.
// Returns nil if no burn occurred at that height.
func (k *Keeper) GetBurnSummaryByHeight(height int64) *BurnSummary {
	for _, s := range k.summaries {
		if s.YearHeight == height {
			return &s
		}
	}
	return nil
}

// DefaultGenesisState returns the default genesis state for the burn module.
func (k *Keeper) DefaultGenesisState() *GenesisState {
	return DefaultGenesis()
}

// ValidateGenesis validates the burn module's genesis state.
func (k *Keeper) ValidateGenesis(state *GenesisState) error {
	// Burn records at genesis should be empty for mainnet.
	// This is a no-op validation since records are appended during operation.
	if state.BurnRecords == nil {
		return fmt.Errorf("burn_records cannot be nil in genesis state")
	}
	return nil
}

// ExportGenesis exports the burn module's state for genesis export.
func (k *Keeper) ExportGenesis() *GenesisState {
	records := make([]BurnRecord, 0, len(k.burnRecords))
	for _, record := range k.burnRecords {
		records = append(records, record)
	}
	return &GenesisState{
		BurnRecords: records,
	}
}

// InitializeGenesis initializes the burn module from a genesis state.
func (k *Keeper) InitializeGenesis(ctx context.Context, state *GenesisState) error {
	if state == nil {
		return fmt.Errorf("genesis state cannot be nil")
	}

	// Initialize burn records from genesis.
	// Derive the year from YearHeight: year = (YearHeight - genesisHeight) / BlocksPerYear
	for _, record := range state.BurnRecords {
		year := record.YearHeight
		if k.genesisHeight > 0 {
			year = (record.YearHeight - k.genesisHeight) / BlocksPerYear
		}
		key := fmt.Sprintf("%s/%d", record.Address, year)
		k.burnRecords[key] = record
	}

	return nil
}
