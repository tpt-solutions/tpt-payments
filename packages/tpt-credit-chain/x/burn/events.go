// Package burn implements the TPT Credit annual burn module.
//
// Event types for burn operations.
package burn

import "fmt"

// EventTypeAnnualBurn is emitted when an account's credits are burned and
// redistribution is received at the annual reset.
const EventTypeAnnualBurn = "annual_burn"

// EventTypeAnnualBurnSummary is emitted once per annual burn cycle,
// summarizing the total burned, redistributed, and remainder sent to reserve.
const EventTypeAnnualBurnSummary = "annual_burn_summary"

// EventAnnualBurn is emitted for each account at the annual burn.
type EventAnnualBurn struct {
	// Address is the account that was burned.
	Address string `json:"address"`

	// BurnedAmount is the ucredit amount that was burned from this account.
	BurnedAmount int64 `json:"burned_amount"`

	// RedistributedAmount is the ucredit amount redistributed to this account.
	RedistributedAmount int64 `json:"redistributed_amount"`

	// YearHeight is the block height at which the burn occurred.
	YearHeight int64 `json:"year_height"`

	// Year is the year number (1-based from genesis) that just ended.
	Year int64 `json:"year"`
}

// Attributes returns the event attributes as key-value pairs.
func (e EventAnnualBurn) Attributes() []string {
	return []string{
		"address", e.Address,
		"burned_amount", fmt.Sprintf("%d", e.BurnedAmount),
		"redistributed_amount", fmt.Sprintf("%d", e.RedistributedAmount),
		"year_height", fmt.Sprintf("%d", e.YearHeight),
		"year", fmt.Sprintf("%d", e.Year),
	}
}

// EventAnnualBurnSummary is emitted once per burn cycle with aggregate data.
type EventAnnualBurnSummary struct {
	// YearHeight is the block height at which the burn occurred.
	YearHeight int64 `json:"year_height"`

	// Year is the year number that just ended.
	Year int64 `json:"year"`

	// TotalBurned is the sum of all balances burned (ucredit).
	TotalBurned int64 `json:"total_burned"`

	// TotalRedistributed is the total ucredit redistributed to accounts.
	TotalRedistributed int64 `json:"total_redistributed"`

	// ActiveHumans is the number of active accounts that received redistribution.
	ActiveHumans int64 `json:"active_humans"`

	// PerHumanAmount is the ucredit amount each active account received.
	PerHumanAmount int64 `json:"per_human_amount"`

	// Remainder is the ucredit amount sent to the protocol reserve.
	Remainder int64 `json:"remainder"`

	// ReserveAddress is the protocol reserve address.
	ReserveAddress string `json:"reserve_address"`
}

// Attributes returns the event attributes as key-value pairs.
func (e EventAnnualBurnSummary) Attributes() []string {
	return []string{
		"year_height", fmt.Sprintf("%d", e.YearHeight),
		"year", fmt.Sprintf("%d", e.Year),
		"total_burned", fmt.Sprintf("%d", e.TotalBurned),
		"total_redistributed", fmt.Sprintf("%d", e.TotalRedistributed),
		"active_humans", fmt.Sprintf("%d", e.ActiveHumans),
		"per_human_amount", fmt.Sprintf("%d", e.PerHumanAmount),
		"remainder", fmt.Sprintf("%d", e.Remainder),
		"reserve_address", e.ReserveAddress,
	}
}