// Package issuance_test contains simulation tests that exercise the issuance
// module with random payout frequency combinations and verify invariants.
package issuance_test

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-credit-chain/x/issuance"
)

// ── Simulation test helpers ────────────────────────────────────────────────────

// randomPayoutFrequency returns a random payout frequency.
func randomPayoutFrequency(rng *rand.Rand) issuance.PayoutFrequency {
	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyWeekly,
		issuance.FrequencyBiweekly,
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}
	return freqs[rng.Intn(len(freqs))]
}

// randomUUID generates a random 64-char hex UUID.
func randomUUID(rng *rand.Rand) string {
	hexChars := "0123456789abcdef"
	buf := make([]byte, 64)
	for i := range buf {
		buf[i] = hexChars[rng.Intn(len(hexChars))]
	}
	return string(buf)
}

// randomAddress generates a random Cosmos-style bech32 address.
func randomAddress(rng *rand.Rand) string {
	hexChars := "0123456789abcdef"
	buf := make([]byte, 38)
	for i := range buf {
		buf[i] = hexChars[rng.Intn(len(hexChars))]
	}
	return "cosmos1" + string(buf)
}

// ── Simulation: random payout frequencies across 100 accounts ──────────────────

func TestSimulation_RandomFrequencyCombinations(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	k := issuance.NewKeeper()
	ctx := context.Background()

	const numAccounts = 100
	blocksPerYear := issuance.BlocksPerYear

	// Register 100 accounts with random payout frequencies.
	addresses := make([]string, numAccounts)
	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		uuid := randomUUID(rng)
		freq := randomPayoutFrequency(rng)
		addresses[i] = addr

		_, err := k.RegisterHuman(ctx, addr, uuid, "kyc", freq)
		require.NoError(t, err, "failed to register account %d", i)
	}

	require.Equal(t, int64(numAccounts), k.GetHumanCount())

	annual := issuance.AnnualAllocation()
	expectedMaxSupply := annual.MulRaw(numAccounts)

	// Simulate a full year of blocks.
	// Use monthly granularity for block advancement (worst-case for weekly accounts).
	blocksPerMonth := blocksPerYear / 12
	for month := int64(0); month < 12; month++ {
		blockHeight := int64(1) + month*blocksPerMonth
		k.SetBlockHeight(blockHeight)

		// Process payouts at this block height.
		events := k.ProcessPayouts()

		// Invariant check after each block.
		msg, broken := k.CheckInvariant(ctx)
		assert.False(t, broken,
			"invariant broken at month %d (block %d): %s", month, blockHeight, msg)

		// Log summary at end of each simulated "quarter".
		if (month+1)%3 == 0 {
			t.Logf("Month %d: %d events emitted, supply=%s / %s (%.1f%%)",
				month+1,
				len(events),
				k.GetSupply(),
				expectedMaxSupply,
				float64(k.GetSupply().BigInt().Int64())*100/float64(expectedMaxSupply.BigInt().Int64()),
			)
		}
	}

	// Final assertions.
	totalSupply := k.GetSupply()

	// No account should exceed annual allocation.
	for i := 0; i < numAccounts; i++ {
		acct := k.GetAccount(addresses[i])
		require.NotNil(t, acct)
		assert.True(t, acct.YearCreditsReceived.LTE(annual),
			"account %s: year credits %s exceeds annual allocation %s",
			addresses[i], acct.YearCreditsReceived, annual)
	}

	// Total supply must be ≤ annual × human_count.
	assert.True(t, totalSupply.LTE(expectedMaxSupply),
		"total supply %s must not exceed %s for %d accounts",
		totalSupply, expectedMaxSupply, numAccounts)

	t.Logf("Simulation complete: %d accounts, supply=%s / max=%s (%.1f%%)",
		numAccounts,
		totalSupply,
		expectedMaxSupply,
		float64(totalSupply.BigInt().Int64())*100/float64(expectedMaxSupply.BigInt().Int64()))
}

// ── Simulation: many accounts with weekly frequency (high frequency stress) ─────

func TestSimulation_AllWeeklyAccounts(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	k := issuance.NewKeeper()
	ctx := context.Background()

	const numAccounts = 50
	blocksPerYear := issuance.BlocksPerYear

	// Register 50 accounts, all weekly.
	addresses := make([]string, numAccounts)
	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		uuid := randomUUID(rng)
		addresses[i] = addr
		_, err := k.RegisterHuman(ctx, addr, uuid, "kyc", issuance.FrequencyWeekly)
		require.NoError(t, err)
	}

	annual := issuance.AnnualAllocation()
	expectedMaxSupply := annual.MulRaw(numAccounts)

	// Process payouts at weekly boundaries for a full year.
	blocksPerWeek := blocksPerYear / 52
	for week := int64(0); week < 52; week++ {
		blockHeight := int64(1) + week*blocksPerWeek
		k.SetBlockHeight(blockHeight)
		k.ProcessPayouts()

		msg, broken := k.CheckInvariant(ctx)
		assert.False(t, broken, "invariant broken at week %d (block %d): %s", week, blockHeight, msg)
	}

	totalSupply := k.GetSupply()
	assert.True(t, totalSupply.LTE(expectedMaxSupply),
		"total supply %s must not exceed %s", totalSupply, expectedMaxSupply)

	// All accounts must be at or under cap.
	for i := 0; i < numAccounts; i++ {
		acct := k.GetAccount(addresses[i])
		require.NotNil(t, acct)
		assert.True(t, acct.YearCreditsReceived.LTE(annual),
			"account %s exceeds cap: %s", addresses[i], acct.YearCreditsReceived)
	}

	t.Logf("All weekly: %d accounts, supply=%s / %s", numAccounts, totalSupply, expectedMaxSupply)
}

// ── Simulation: mid-year registration + payout stress ──────────────────────────

func TestSimulation_MidYearRegistration(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	k := issuance.NewKeeper()
	ctx := context.Background()

	blocksPerYear := issuance.BlocksPerYear
	blocksPerMonth := blocksPerYear / 12

	// Register initial accounts at the start.
	initialAddrs := make([]string, 20)
	for i := 0; i < 20; i++ {
		addr := randomAddress(rng)
		uuid := randomUUID(rng)
		freq := randomPayoutFrequency(rng)
		initialAddrs[i] = addr
		_, err := k.RegisterHuman(ctx, addr, uuid, "kyc", freq)
		require.NoError(t, err)
	}

	annual := issuance.AnnualAllocation()

	// Simulate 3 months of payouts.
	for month := int64(0); month < 3; month++ {
		blockHeight := int64(1) + month*blocksPerMonth
		k.SetBlockHeight(blockHeight)
		k.ProcessPayouts()
	}

	// Register 10 more accounts mid-year.
	midAddrs := make([]string, 10)
	for i := 0; i < 10; i++ {
		addr := randomAddress(rng)
		uuid := randomUUID(rng)
		freq := randomPayoutFrequency(rng)
		midAddrs[i] = addr
		_, err := k.RegisterHuman(ctx, addr, uuid, "kyc", freq)
		require.NoError(t, err)
	}

	// Simulate remaining 9 months of payouts.
	for month := int64(3); month < 12; month++ {
		blockHeight := int64(1) + month*blocksPerMonth
		k.SetBlockHeight(blockHeight)
		k.ProcessPayouts()

		msg, broken := k.CheckInvariant(ctx)
		assert.False(t, broken, "invariant broken at month %d: %s", month, msg)
	}

	// Verify no account exceeds annual cap.
	totalHumans := int64(30)
	expectedMax := annual.MulRaw(totalHumans)
	totalSupply := k.GetSupply()
	assert.True(t, totalSupply.LTE(expectedMax),
		"total supply %s must not exceed %s for %d humans",
		totalSupply, expectedMax, totalHumans)

	// Check early registrants.
	for _, addr := range initialAddrs {
		acct := k.GetAccount(addr)
		require.NotNil(t, acct)
		assert.True(t, acct.YearCreditsReceived.LTE(annual),
			"initial account %s exceeds cap: %s", addr, acct.YearCreditsReceived)
	}

	// Check mid-year registrants — they registered at month 3, so they
	// only received ~9 months of payouts. Still shouldn't exceed annual cap.
	for _, addr := range midAddrs {
		acct := k.GetAccount(addr)
		require.NotNil(t, acct)
		assert.True(t, acct.YearCreditsReceived.LTE(annual),
			"mid-year account %s exceeds cap: %s", addr, acct.YearCreditsReceived)
	}

	t.Logf("Mid-year registration: %d initial + %d mid-year = %d total, supply=%s / %s",
		len(initialAddrs), len(midAddrs), totalHumans, totalSupply, expectedMax)
}

// ── Simulation: random walk across many operations ─────────────────────────────

func TestSimulation_RandomWalkAccountsAndPayouts(t *testing.T) {
	rng := rand.New(rand.NewSource(12345))
	k := issuance.NewKeeper()
	ctx := context.Background()

	blocksPerYear := issuance.BlocksPerYear
	blocksPerMonth := blocksPerYear / 12
	annual := issuance.AnnualAllocation()

	const steps = 500
	var registeredAddrs []string
	nextUUID := 0

	for step := 0; step < steps; step++ {
		op := rng.Intn(3)

		switch op {
		case 0:
			// Register a new account.
			addr := randomAddress(rng)
			uuid := randomUUID(rng)
			freq := randomPayoutFrequency(rng)
			_, err := k.RegisterHuman(ctx, addr, uuid, "kyc", freq)
			if err == nil {
				registeredAddrs = append(registeredAddrs, addr)
			}
			nextUUID++

		case 1:
			// Advance time by a random number of blocks.
			blocksAhead := rng.Int63n(blocksPerMonth) + 1
			k.SetBlockHeight(k.GetBlockHeight() + blocksAhead)
			payouts := k.ProcessPayouts()

			// Verify each payout event.
			for _, event := range payouts {
				assert.Equal(t, k.GetCurrentYear(), event.Year)
				// Parse amount from event to verify non-negative.
				eventAmount, ok := math.NewIntFromString(event.Amount)
				assert.True(t, ok, "event amount %q must be parseable", event.Amount)
				assert.True(t, eventAmount.GT(math.ZeroInt()), "event amount must be positive")
			}

			// Verify no account exceeds annual allocation.
			for _, addr := range registeredAddrs {
				acct := k.GetAccount(addr)
				if acct != nil {
					assert.True(t, acct.YearCreditsReceived.LTE(annual),
						"account %s exceeds cap at step %d: %s",
						addr, step, acct.YearCreditsReceived)
				}
			}

		case 2:
			// Simulate a year-end / burn transition (reset credits).
			k.ResetYearCredits()
		}

		// Invariant check after every operation.
		msg, broken := k.CheckInvariant(ctx)
		assert.False(t, broken,
			"invariant broken at step %d (op=%d, humans=%d, supply=%s): %s",
			step, op, len(registeredAddrs), k.GetSupply(), msg)
	}

	t.Logf("Random walk: %d steps, %d registered accounts, supply=%s",
		steps, len(registeredAddrs), k.GetSupply())

	// Final invariant.
	expectedMax := annual.MulRaw(int64(len(registeredAddrs)))
	assert.True(t, k.GetSupply().LTE(expectedMax),
		"final supply %s must not exceed %s", k.GetSupply(), expectedMax)
}

// ── Simulation: extreme stress with 1000 accounts ──────────────────────────────

func TestSimulation_ThousandAccountsStress(t *testing.T) {
	rng := rand.New(rand.NewSource(9999))
	k := issuance.NewKeeper()
	ctx := context.Background()

	const numAccounts = 1000
	blocksPerYear := issuance.BlocksPerYear

	// Register 1000 accounts with mixed frequencies.
	addresses := make([]string, numAccounts)
	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		uuid := randomUUID(rng)
		freq := randomPayoutFrequency(rng)
		addresses[i] = addr
		_, err := k.RegisterHuman(ctx, addr, uuid, "kyc", freq)
		require.NoError(t, err)
	}

	annual := issuance.AnnualAllocation()
	expectedMax := annual.MulRaw(numAccounts)

	// Simulate a full year with biweekly granularity.
	blocksPerPeriod := blocksPerYear / 26
	for period := int64(0); period < 26; period++ {
		blockHeight := int64(1) + period*blocksPerPeriod
		k.SetBlockHeight(blockHeight)
		k.ProcessPayouts()

		msg, broken := k.CheckInvariant(ctx)
		assert.False(t, broken,
			"invariant broken at period %d (block %d): %s", period, blockHeight, msg)
	}

	totalSupply := k.GetSupply()
	assert.True(t, totalSupply.LTE(expectedMax),
		"total supply %s must not exceed %s for %d accounts",
		totalSupply, expectedMax, numAccounts)

	// Check a random sample of accounts for cap compliance.
	sampleSize := 50
	for i := 0; i < sampleSize; i++ {
		idx := rng.Intn(numAccounts)
		acct := k.GetAccount(addresses[idx])
		require.NotNil(t, acct)
		assert.True(t, acct.YearCreditsReceived.LTE(annual),
			"account %s at index %d exceeds cap: %s",
			addresses[idx], idx, acct.YearCreditsReceived)
	}

	t.Logf("1000-account stress: supply=%s / %s (%.1f%%)",
		totalSupply,
		expectedMax,
		float64(totalSupply.BigInt().Int64())*100/float64(expectedMax.BigInt().Int64()))
}

// ── Test: Payout math correctness for all frequency combinations ──────────────

func TestSimulation_AllFrequencyPayoutMath(t *testing.T) {
	// For each frequency, verify that per-payout × payouts_per_year
	// produces exactly the right arithmetic for integer math.
	annual := issuance.AnnualAllocation()
	annualInt := annual.BigInt().Int64()

	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyWeekly,
		issuance.FrequencyBiweekly,
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}

	for _, freq := range freqs {
		t.Run(string(freq), func(t *testing.T) {
			payoutAmount := issuance.PayoutAmount(freq)
			payoutsPerYear := issuance.PayoutsPerYear(freq)

			// Per-payout amount must be positive.
			assert.True(t, payoutAmount.GT(math.ZeroInt()),
				"payout amount must be positive for %s", freq)

			// Total = per_payout × payouts_per_year.
			totalPossible := payoutAmount.MulRaw(payoutsPerYear)
			totalPossibleInt := totalPossible.BigInt().Int64()

			// Verify divide-then-multiply property.
			// For frequencies that divide evenly (monthly, quarterly, annual),
			// totalPossible should exactly equal annual allocation.
			// For frequencies with remainders (weekly, biweekly),
			// totalPossible < annual allocation (dust).
			switch freq {
			case issuance.FrequencyMonthly, issuance.FrequencyQuarterly, issuance.FrequencyAnnual:
				assert.Equal(t, annualInt, totalPossibleInt,
					"%s: total payouts %d must equal annual allocation %d",
					freq, totalPossibleInt, annualInt)
			case issuance.FrequencyWeekly, issuance.FrequencyBiweekly:
				// These have dust — total < annual allocation.
				assert.True(t, totalPossibleInt < annualInt,
					"%s: total payouts %d must be less than annual allocation %d (dust expected)",
					freq, totalPossibleInt, annualInt)
				// Dust must be less than one payout.
				dust := annual.Sub(totalPossible)
				assert.True(t, dust.LT(payoutAmount),
					"%s: dust %s must be less than one payout %s",
					freq, dust, payoutAmount)
			}

			t.Logf("%s: perPayout=%s × %d = %d (annual=%d, dust=%d)",
				freq,
				payoutAmount,
				payoutsPerYear,
				totalPossibleInt,
				annualInt,
				annualInt-totalPossibleInt,
			)
		})
	}
}

// ── Simulation: Edge cases for payout timing ───────────────────────────────────

func TestSimulation_PayoutTiming(t *testing.T) {
	// Verify that accounts are paid at the correct block intervals.
	blocksPerYear := issuance.BlocksPerYear

	tests := []struct {
		freq            issuance.PayoutFrequency
		expectedPayouts int64
		blocksPerPayout int64
	}{
		{issuance.FrequencyWeekly, 52, blocksPerYear / 52},
		{issuance.FrequencyBiweekly, 26, blocksPerYear / 26},
		{issuance.FrequencyMonthly, 12, blocksPerYear / 12},
		{issuance.FrequencyQuarterly, 4, blocksPerYear / 4},
		{issuance.FrequencyAnnual, 1, blocksPerYear},
	}

	for _, tc := range tests {
		t.Run(string(tc.freq), func(t *testing.T) {
			k := issuance.NewKeeper()
			ctx := context.Background()

			_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", tc.freq)
			require.NoError(t, err)

			// Process payouts at each expected payout block.
			var totalPayouts int64
			for block := tc.blocksPerPayout; block <= blocksPerYear; block += tc.blocksPerPayout {
				k.SetBlockHeight(int64(1) + block)
				payouts := k.ProcessPayouts()
				if len(payouts) > 0 {
					totalPayouts++
				}
			}

			assert.Equal(t, tc.expectedPayouts, totalPayouts,
				"expected %d payouts for %s frequency, got %d",
				tc.expectedPayouts, tc.freq, totalPayouts)
		})
	}
}

// ── Simulation: Verify event attributes are complete ───────────────────────────

func TestSimulation_EventAttributes(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerMonth := blocksPerYear / 12

	// Pay out.
	k.SetBlockHeight(1 + blocksPerMonth)
	k.ProcessPayouts()

	// Verify HumanRegistered event.
	require.Len(t, bus.HumanRegisteredEvents, 1)
	regEvent := bus.HumanRegisteredEvents[0]
	assert.Equal(t, testAddr1, regEvent.Address)
	assert.Equal(t, testUUID1, regEvent.IdentityUUID)
	assert.Equal(t, "kyc", regEvent.IdentityTier)
	assert.Equal(t, issuance.FrequencyMonthly, regEvent.PayoutFrequency)

	// Verify CreditsIssued event.
	require.Len(t, bus.CreditsIssuedEvents, 1)
	creditEvent := bus.CreditsIssuedEvents[0]
	assert.Equal(t, testAddr1, creditEvent.Address)
	assert.Equal(t, int64(1), creditEvent.PayoutNumber)
	assert.Equal(t, int64(1), creditEvent.Year)

	// Amount should be positive.
	amount, ok := math.NewIntFromString(creditEvent.Amount)
	require.True(t, ok, "event amount %q must be parseable", creditEvent.Amount)
	assert.True(t, amount.GT(math.ZeroInt()), "amount must be positive")

	// Verify Attributes() method produces expected key-value pairs.
	attrs := regEvent.Attributes()
	require.Len(t, attrs, 10) // 5 pairs
	assert.Contains(t, attrs, "address")
	assert.Contains(t, attrs, testAddr1)

	attrs = creditEvent.Attributes()
	require.Len(t, attrs, 10) // 5 pairs
	assert.Contains(t, attrs, "block_height")
	assert.Contains(t, attrs, fmt.Sprintf("%d", creditEvent.BlockHeight))
}