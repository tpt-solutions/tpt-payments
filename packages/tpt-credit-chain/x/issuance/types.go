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

	// BlocksPerYear is the number of blocks in one year at 2-second block time.
	// 365.25 days × 24 h × 60 min × 30 blocks/min = 15,778,800 blocks/year.
	BlocksPerYear = int64(15_778_800)
)

// AnnualAllocation returns the per-human annual issuance as a math.Int.
func AnnualAllocation() math.Int {
	return math.NewInt(AnnualAllocationUcredit)
}

// PayoutFrequency controls how often a human receives their credits.
type PayoutFrequency string

// MaxBalanceUcredit is the maximum balance cap (3 × annual allocation).
// 3 × 12,000 × 1,000,000 = 36,000,000,000 ucredit
const MaxBalanceUcredit int64 = 3 * AnnualAllocationUcredit

// MaxBalanceCredits is MaxBalanceUcredit in credit units.
const MaxBalanceCredits int64 = 3 * AnnualAllocationCredits

// FeePolicy defines the transaction fee model for the credit chain.
type FeePolicy string

const (
	// FeeZero is Option A: 0 fees (pure credit system). This is the
	// hardcoded policy for TPT Credit — credits are a basic income
	// instrument, not a revenue source for validators. Validators
	// are compensated separately through the protocol's operational
	// budget (off-chain).
	//
	// Decision documented per TODO: Option A selected. Rationale:
	// - Credits are a UBI/merit instrument, not a speculative token.
	// - Fees create friction for the unbanked user base.
	// - Validator compensation is a separate concern (operational grants).
	FeeZero FeePolicy = "zero"

	// FeeOptionB would be 0.1% fee to validator pool. NOT SELECTED.
	// FeeOptionB FeePolicy = "fee_0.1pct"
)

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

// BlocksPerPayout returns the number of blocks between payouts for a given frequency.
func BlocksPerPayout(f PayoutFrequency) int64 {
	return BlocksPerYear / PayoutsPerYear(f)
}

// HumanAccount is the on-chain record for a registered human.
type HumanAccount struct {
	// Address is the Cosmos bech32 account address.
	Address string `json:"address"`

	// PublicKey is the Ed25519 key that authorizes this account's signed transfers. It is bound
	// once (see BindKey) and empty until then.
	PublicKey []byte `json:"public_key,omitempty"`

	// Nonce is the last nonce accepted from a signed transfer by this account.
	Nonce uint64 `json:"nonce"`

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

	// YearPayoutCount tracks the number of payouts received this year.
	// Resets to 0 after the annual burn.
	YearPayoutCount int64 `json:"year_payout_count"`

	// LastPaidAt is the block height of the most recent payout.
	LastPaidAt int64 `json:"last_paid_at"`

	// NextDueAt is the block height at which the next payout is due.
	// This is computed as LastPaidAt + BlocksPerPayout.
	NextDueAt int64 `json:"next_due_at"`

	// RegisteredAt is the block height of registration.
	RegisteredAt int64 `json:"registered_at"`

	// Balance is the current spendable ucredit balance.
	// This is the amount available for transfers (separate from YearCreditsReceived
	// which tracks issuance for cap enforcement).
	Balance math.Int `json:"balance"`
}

// SendLimit defines a per-day or per-transaction send limit.
type SendLimit struct {
	// PerTransaction is the maximum ucredit amount allowed per single transfer.
	// 0 means no limit.
	PerTransaction int64 `json:"per_transaction"`

	// PerDay is the maximum total ucredit amount allowed per day (rolling 24h).
	// 0 means no limit.
	PerDay int64 `json:"per_day"`
}

// TransferRecord tracks a credits transfer for audit and limit enforcement.
type TransferRecord struct {
	// From is the sender address.
	From string `json:"from"`

	// To is the recipient address.
	To string `json:"to"`

	// Amount is the ucredit amount transferred.
	Amount math.Int `json:"amount"`

	// BlockHeight is the block height of the transfer.
	BlockHeight int64 `json:"block_height"`
}

// BatchSendEntry is a single recipient in a batch send operation.
type BatchSendEntry struct {
	// To is the recipient address.
	To string `json:"to"`

	// Amount is the ucredit amount to send.
	Amount math.Int `json:"amount"`
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