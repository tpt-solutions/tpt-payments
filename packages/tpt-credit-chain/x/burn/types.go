// Package burn implements the TPT Credit annual burn module.
//
// This module is IMMUTABLE. The burn policy is hardcoded and cannot be changed
// by governance.
//
// Burn timing: GLOBAL ANNUAL RESET. Every account burns on the same block,
// once per year. There are no per-account anniversaries.
//
// Full-burn design: at the annual reset, 100% of every account's balance burns
// into the redistribution pool. There is NO carryover between years. The pool
// is then split evenly among all active accounts, and the new year's issuance
// schedule (12,000 credits) begins for everyone.
//
// Mid-year circulation cap: accounts may not hold more than MaxBalanceUcredit
// (36,000 credits) at any moment during the year. Transfers or payouts that
// would exceed this ceiling are rejected by the transaction module. This keeps
// credits circulating — a holder at the cap must spend before receiving more.
//
// The burn mechanics:
//  1. At the global reset block, every account's full balance is transferred to
//     the burn pool (CalculateBurnAmount returns the entire balance).
//  2. The burn pool is divided equally among all active verified humans.
//  3. Each active account receives burn_pool / active_human_count.
//  4. Remainder (due to integer division) goes to the protocol reserve account.
//  5. New year's 12,000-credit issuance schedule begins for all accounts.
package burn

const ModuleName = "burn"

// GlobalAnnualReset is true and must never be changed. The burn fires on the
// same block for every account, once per year. Per-account anniversary burns
// are not supported and would undermine the anti-hoarding guarantee.
const GlobalAnnualReset = true

// BlocksPerYear is the number of blocks in one year at 2-second block time.
// Used to calculate the burn block from genesis height.
// 365.25 days × 24 h × 60 min × 30 blocks/min = 15,778,800 blocks/year.
const BlocksPerYear = int64(15_778_800)

// MaxBalanceCredits is the maximum balance an account may hold at any moment
// during the year. Transfers or payouts that would push a balance above this
// value are rejected by the transaction module. Keeps credits in circulation —
// a holder at the cap must spend before they can receive more.
// Set to 3 × annual allocation (36,000) to accommodate commerce surplus while
// preventing indefinite mid-year accumulation.
const MaxBalanceCredits = int64(36_000)
const MaxBalanceUcredit = MaxBalanceCredits * 1_000_000

// BurnBlock returns the block height of the Nth annual burn after genesis.
// n=1 is the first burn (end of year 1), n=2 is the second, etc.
func BurnBlock(genesisHeight, n int64) int64 {
	return genesisHeight + n*BlocksPerYear
}

// BurnRecord logs a single account's annual burn event.
type BurnRecord struct {
	Address           string `json:"address"`
	BurnedAmount      int64  `json:"burned_amount"`       // ucredit burned
	RedistributedAmount int64 `json:"redistributed_amount"` // ucredit received from pool
	YearHeight        int64  `json:"year_height"`         // block height of the burn event
}

// BurnSummary aggregates a year's burn across all accounts.
type BurnSummary struct {
	YearHeight        int64  `json:"year_height"`
	TotalBurned       int64  `json:"total_burned"`
	TotalRedistributed int64 `json:"total_redistributed"`
	ActiveHumans      int64  `json:"active_humans"`
	PerHumanAmount    int64  `json:"per_human_amount"`  // ucredit redistributed per human
	Remainder         int64  `json:"remainder"`         // goes to protocol reserve
}

// GenesisState is the initial state for the burn module.
type GenesisState struct {
	// BurnRecords are pre-existing records (empty on mainnet genesis).
	BurnRecords []BurnRecord `json:"burn_records"`
}

// DefaultGenesis returns the genesis state for a fresh chain.
func DefaultGenesis() *GenesisState {
	return &GenesisState{BurnRecords: []BurnRecord{}}
}

// CalculateBurnAmount returns the amount that burns at the annual reset.
// The entire balance burns — there is no carryover between years.
func CalculateBurnAmount(balance int64) int64 {
	return balance
}

// CalculateRedistribution computes the per-human redistribution amount and
// the remainder. Pure function — no side effects.
func CalculateRedistribution(totalBurned, activeHumans int64) (perHuman, remainder int64) {
	if activeHumans == 0 {
		return 0, totalBurned
	}
	perHuman = totalBurned / activeHumans
	remainder = totalBurned - (perHuman * activeHumans)
	return
}
