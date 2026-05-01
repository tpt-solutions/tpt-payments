// Package issuance implements the TPT Credit issuance module.
//
// This module is IMMUTABLE. The following parameters are hardcoded at genesis
// and CANNOT be changed by governance, upgrade proposals, or any transaction:
//
//   - AnnualAllocation: 12,000 credits per verified human per year
//   - OnRampEnabled:    false (no fiat-to-credit on-ramps, ever)
//   - OffRampEnabled:   false (no credit-to-fiat off-ramps, ever)
//
// Any pull request that adds a governance upgrade path to these parameters
// will be rejected.
package issuance

import (
	"cosmossdk.io/math"
)

const ModuleName = "issuance"

// Immutable protocol constants. These are not stored in the params store —
// they are compiled into the binary. Changing them requires a new binary
// and a coordinated chain upgrade with social consensus (hard fork).
const (
	// AnnualAllocationCredits is the number of whole credits issued per
	// verified human per year.
	AnnualAllocationCredits = int64(12_000)

	// AnnualAllocationUcredit is AnnualAllocationCredits in the base unit.
	// 1 credit = 1,000,000 ucredit (micro-credit, 6 decimal places).
	AnnualAllocationUcredit = AnnualAllocationCredits * 1_000_000

	// Denom is the on-chain denomination for the base unit.
	Denom = "ucredit"

	// OnRampEnabled controls whether fiat→credit conversions are permitted.
	// This is false and must never be changed.
	OnRampEnabled = false

	// OffRampEnabled controls whether credit→fiat conversions are permitted.
	// This is false and must never be changed.
	OffRampEnabled = false
)

// AnnualAllocation returns the per-human annual issuance as a math.Int.
func AnnualAllocation() math.Int {
	return math.NewInt(AnnualAllocationUcredit)
}

// PayoutFrequency controls how often a human receives their credits.
type PayoutFrequency string

const (
	FrequencyWeekly    PayoutFrequency = "weekly"     // 52 payouts/year ≈ 230.77 ucredit each
	FrequencyBiweekly  PayoutFrequency = "biweekly"   // 26 payouts/year ≈ 461.54 ucredit each
	FrequencyMonthly   PayoutFrequency = "monthly"    // 12 payouts/year = 1,000 credits each
	FrequencyQuarterly PayoutFrequency = "quarterly"  // 4 payouts/year  = 3,000 credits each
	FrequencyAnnual    PayoutFrequency = "annual"     // 1 payout/year   = 12,000 credits
)

// PayoutsPerYear returns how many payouts occur per year for a given frequency.
func PayoutsPerYear(f PayoutFrequency) int64 {
	switch f {
	case FrequencyWeekly:
		return 52
	case FrequencyBiweekly:
		return 26
	case FrequencyMonthly:
		return 12
	case FrequencyQuarterly:
		return 4
	case FrequencyAnnual:
		return 1
	default:
		return 12 // default to monthly
	}
}

// PayoutAmount calculates the ucredit amount for a single payout at the given frequency.
// Uses integer arithmetic — any remainder carries forward to the next year's pool.
func PayoutAmount(f PayoutFrequency) math.Int {
	perYear := PayoutsPerYear(f)
	return math.NewInt(AnnualAllocationUcredit / perYear)
}

// HumanAccount is the on-chain record for a registered human.
type HumanAccount struct {
	// Address is the Cosmos bech32 account address.
	Address string `json:"address"`

	// IdentityUUID is the SHA-256 hash of the KYC provider's unique person ID.
	// It is the uniqueness constraint — duplicate UUIDs are rejected at consensus.
	IdentityUUID string `json:"identity_uuid"`

	// IdentityTier is the verification level: "kyc" | "biometric" | "dna"
	IdentityTier string `json:"identity_tier"`

	// PayoutFrequency controls how often this account receives credits.
	PayoutFrequency PayoutFrequency `json:"payout_frequency"`

	// YearCreditsReceived tracks total ucredit issued in the current year.
	// Resets to 0 after the annual burn.
	YearCreditsReceived math.Int `json:"year_credits_received"`

	// LastPaidAt is the block height of the most recent payout.
	LastPaidAt int64 `json:"last_paid_at"`

	// RegisteredAt is the block height of registration.
	RegisteredAt int64 `json:"registered_at"`
}

// GenesisState is the initial state for the issuance module.
type GenesisState struct {
	HumanAccounts []HumanAccount `json:"human_accounts"`
}

// DefaultGenesis returns the genesis state for a fresh chain.
// The accounts list is empty — humans register by submitting MsgRegisterHuman.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		HumanAccounts: []HumanAccount{},
	}
}
