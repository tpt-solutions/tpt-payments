package issuance_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

func TestAnnualAllocation_MatchesSpec(t *testing.T) {
	assert.Equal(t, int64(12_000), issuance.AnnualAllocationCredits)
	assert.Equal(t, int64(12_000_000_000), issuance.AnnualAllocationUcredit)
	assert.Equal(t, math.NewInt(12_000_000_000), issuance.AnnualAllocation())
}

func TestOnOffRamps_AlwaysFalse(t *testing.T) {
	// These are compile-time constants. This test documents the intent
	// and will fail to compile if someone removes the constants.
	assert.False(t, issuance.OnRampEnabled,  "on-ramps must never be enabled")
	assert.False(t, issuance.OffRampEnabled, "off-ramps must never be enabled")
}

func TestPayoutsPerYear(t *testing.T) {
	tests := []struct {
		freq  issuance.PayoutFrequency
		count int64
	}{
		{issuance.FrequencyWeekly, 52},
		{issuance.FrequencyBiweekly, 26},
		{issuance.FrequencyMonthly, 12},
		{issuance.FrequencyQuarterly, 4},
		{issuance.FrequencyAnnual, 1},
	}
	for _, tc := range tests {
		t.Run(string(tc.freq), func(t *testing.T) {
			assert.Equal(t, tc.count, issuance.PayoutsPerYear(tc.freq))
		})
	}
}

func TestPayoutAmount_SumsToAnnual(t *testing.T) {
	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}
	for _, f := range freqs {
		t.Run(string(f), func(t *testing.T) {
			perPayout := issuance.PayoutAmount(f)
			perYear := issuance.PayoutsPerYear(f)
			total := perPayout.MulRaw(perYear)
			// Total must equal or be less than annual allocation (integer division may lose dust).
			annual := issuance.AnnualAllocation()
			assert.True(t, total.LTE(annual),
				"total payouts %s must not exceed annual allocation %s", total, annual)
			// Dust must be less than one payout period.
			dust := annual.Sub(total)
			assert.True(t, dust.LT(perPayout),
				"dust %s should be less than one payout %s", dust, perPayout)
		})
	}
}

func TestDefaultGenesis_Empty(t *testing.T) {
	genesis := issuance.DefaultGenesis()
	assert.Empty(t, genesis.HumanAccounts)
}
