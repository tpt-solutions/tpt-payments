package burn_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/burn"
)

func TestCalculateRedistribution_EvenSplit(t *testing.T) {
	perHuman, remainder := burn.CalculateRedistribution(1_000_000, 4)
	assert.Equal(t, int64(250_000), perHuman)
	assert.Equal(t, int64(0), remainder)
}

func TestCalculateRedistribution_WithRemainder(t *testing.T) {
	perHuman, remainder := burn.CalculateRedistribution(1_000_001, 4)
	assert.Equal(t, int64(250_000), perHuman)
	assert.Equal(t, int64(1), remainder)
}

func TestCalculateRedistribution_TotalAccountedFor(t *testing.T) {
	total := int64(12_000_000_000) // 12,000 credits in ucredit
	humans := int64(7)
	perHuman, remainder := burn.CalculateRedistribution(total, humans)
	assert.Equal(t, total, perHuman*humans+remainder)
}

func TestCalculateRedistribution_ZeroHumans(t *testing.T) {
	perHuman, remainder := burn.CalculateRedistribution(1_000, 0)
	assert.Equal(t, int64(0), perHuman)
	assert.Equal(t, int64(1_000), remainder) // all goes to reserve
}

func TestCalculateRedistribution_SingleHuman(t *testing.T) {
	perHuman, remainder := burn.CalculateRedistribution(5_000, 1)
	assert.Equal(t, int64(5_000), perHuman)
	assert.Equal(t, int64(0), remainder)
}

func TestDefaultGenesis_Empty(t *testing.T) {
	g := burn.DefaultGenesis()
	assert.Empty(t, g.BurnRecords)
}

func TestGlobalAnnualReset_AlwaysTrue(t *testing.T) {
	assert.True(t, burn.GlobalAnnualReset,
		"burn must always be a global annual reset, never per-account anniversary")
}

func TestBurnBlock_Sequence(t *testing.T) {
	const genesis = int64(1)

	year1 := burn.BurnBlock(genesis, 1)
	year2 := burn.BurnBlock(genesis, 2)
	year3 := burn.BurnBlock(genesis, 3)

	// Each burn is exactly BlocksPerYear apart.
	assert.Equal(t, burn.BlocksPerYear, year2-year1)
	assert.Equal(t, burn.BlocksPerYear, year3-year2)

	// Burn blocks are deterministic from genesis.
	assert.Equal(t, genesis+burn.BlocksPerYear, year1)
}

func TestCalculateBurnAmount_FullBurn(t *testing.T) {
	// The entire balance burns at the annual reset — no carryover.
	cases := []int64{0, 1_000, 12_000_000_000, burn.MaxBalanceUcredit}
	for _, bal := range cases {
		assert.Equal(t, bal, burn.CalculateBurnAmount(bal),
			"expected full burn for balance %d", bal)
	}
}

func TestNoCarryover_Invariant(t *testing.T) {
	// CalculateBurnAmount(x) must equal x for all non-negative balances.
	for _, bal := range []int64{0, 1, 500_000_000, burn.MaxBalanceUcredit} {
		assert.Equal(t, bal, burn.CalculateBurnAmount(bal))
	}
}

func TestMaxBalance_IsThreeTimesAnnual(t *testing.T) {
	// Mid-year cap is exactly 3 × annual allocation (36,000 credits).
	// Bounds the max per-account contribution to the burn pool at year-end.
	const annualCredits = int64(12_000)
	assert.Equal(t, 3*annualCredits, burn.MaxBalanceCredits)
}
