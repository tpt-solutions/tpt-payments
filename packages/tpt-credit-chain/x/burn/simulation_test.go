// Package burn_test contains simulation tests that exercise the burn
// module with large numbers of accounts and verify invariants.
package burn_test

import (
	"fmt"
	"math/rand"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-credit-chain/x/burn"
)

// ── Simulation test helpers ────────────────────────────────────────────────────

// SimIssuanceKeeper is a simplified issuance keeper for simulation tests.
type SimIssuanceKeeper struct {
	Accounts      map[string]*SimAccount
	HumanCount    int64
	GenesisHeight int64
}

type SimAccount struct {
	Address string
	Balance math.Int
}

func NewSimIssuanceKeeper(genesisHeight int64) *SimIssuanceKeeper {
	return &SimIssuanceKeeper{
		Accounts:      make(map[string]*SimAccount),
		HumanCount:    0,
		GenesisHeight: genesisHeight,
	}
}

func (sk *SimIssuanceKeeper) GetAccounts() []burn.IssuanceAccount {
	accounts := make([]burn.IssuanceAccount, 0, len(sk.Accounts))
	for _, acct := range sk.Accounts {
		accounts = append(accounts, burn.IssuanceAccount{
			Address: acct.Address,
			Balance: acct.Balance,
		})
	}
	return accounts
}

func (sk *SimIssuanceKeeper) GetHumanCount() int64 {
	return sk.HumanCount
}

func (sk *SimIssuanceKeeper) ResetYearCredits() {}

func (sk *SimIssuanceKeeper) GetGenesisHeight() int64 {
	return sk.GenesisHeight
}

func (sk *SimIssuanceKeeper) SetAccountBalance(address string, balance math.Int) {
	if acct, ok := sk.Accounts[address]; ok {
		acct.Balance = balance
	}
}

func (sk *SimIssuanceKeeper) GetAccountBalance(address string) math.Int {
	if acct, ok := sk.Accounts[address]; ok {
		return acct.Balance
	}
	return math.ZeroInt()
}

func (sk *SimIssuanceKeeper) RegisterAccount(address string, balance int64) {
	sk.Accounts[address] = &SimAccount{
		Address: address,
		Balance: math.NewInt(balance),
	}
	sk.HumanCount++
}

func (sk *SimIssuanceKeeper) RegisterAccountWithBalance(address string, balance math.Int) {
	sk.Accounts[address] = &SimAccount{
		Address: address,
		Balance: balance,
	}
	sk.HumanCount++
}

func (sk *SimIssuanceKeeper) HasAddress(address string) bool {
	_, ok := sk.Accounts[address]
	return ok
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

const simReserveAddr = "cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9"

// ── Simulation: 10,000 accounts burning simultaneously ─────────────────────────

func TestSimulation_TenThousandAccountsBurn(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	const numAccounts = 10_000

	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	// Register 10,000 accounts with random balances between 0 and MaxBalanceUcredit.
	addresses := make([]string, numAccounts)
	totalExpectedBurn := int64(0)

	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		addresses[i] = addr

		// Random balance between 0 and MaxBalanceUcredit.
		balance := rng.Int63n(burn.MaxBalanceUcredit + 1)
		mockIssuance.RegisterAccount(addr, balance)
		totalExpectedBurn += balance
	}

	require.Equal(t, int64(numAccounts), mockIssuance.GetHumanCount())
	require.Equal(t, int64(numAccounts), k.GetActiveHumans())

	// Execute burn at first burn block.
	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary, "burn should have occurred")
	assert.Equal(t, totalExpectedBurn, summary.TotalBurned,
		"total burned should equal sum of all balances")
	assert.Equal(t, int64(numAccounts), summary.ActiveHumans,
		"all 10,000 accounts should be active")

	// Verify redistribution math: perHuman = total / humans, remainder = total % humans.
	expectedPerHuman := totalExpectedBurn / numAccounts
	expectedRemainder := totalExpectedBurn - (expectedPerHuman * numAccounts)
	expectedTotalRedistributed := expectedPerHuman * numAccounts

	assert.Equal(t, expectedPerHuman, summary.PerHumanAmount,
		"per-human redistribution")
	assert.Equal(t, expectedRemainder, summary.Remainder,
		"remainder should go to protocol reserve")
	assert.Equal(t, expectedTotalRedistributed, summary.TotalRedistributed,
		"total redistributed")

	// Verify every account has the expected perHuman balance after redistribution.
	for _, addr := range addresses[:100] { // Sample first 100 accounts.
		bal := mockIssuance.GetAccountBalance(addr)
		assert.Equal(t, math.NewInt(expectedPerHuman), bal,
			"account %s should have perHuman balance after redistribution", addr)
	}

	// Verify burn record count.
	assert.Equal(t, numAccounts, k.GetBurnRecordCount(),
		"should have a burn record for each account")

	// Verify a few specific accounts' burn records.
	for _, addr := range addresses[:10] {
		record := k.GetBurnRecord(addr, 1)
		require.NotNil(t, record, "account %s should have year 1 burn record", addr)
		assert.Equal(t, firstBurn, record.YearHeight)
	}
}

// ── Simulation: 10,000 accounts with variable balances near cap ─────────────────

func TestSimulation_TenThousandAccountsAtCap(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	const numAccounts = 10_000

	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	// Register 10,000 accounts with balances at or near the MaxBalanceUcredit cap.
	addresses := make([]string, numAccounts)
	totalExpectedBurn := int64(0)

	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		addresses[i] = addr

		// Balance between 90% and 100% of MaxBalanceUcredit.
		ratio := 9000 + rng.Int63n(1001) // 9000..10000 (representing 90%-100%)
		balance := burn.MaxBalanceUcredit * ratio / 10000
		mockIssuance.RegisterAccount(addr, balance)
		totalExpectedBurn += balance
	}

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, int64(numAccounts), summary.ActiveHumans)

	// Verify redistribution math.
	expectedPerHuman := totalExpectedBurn / numAccounts
	expectedRemainder := totalExpectedBurn - (expectedPerHuman * numAccounts)
	assert.Equal(t, expectedPerHuman, summary.PerHumanAmount)
	assert.Equal(t, expectedRemainder, summary.Remainder)

	// Verify the total burned + total redistributed + remainder = total burned.
	assert.Equal(t, totalExpectedBurn, summary.TotalRedistributed+summary.Remainder,
		"burned = redistributed + remainder")

	// Check records.
	assert.Equal(t, numAccounts, k.GetBurnRecordCount())
	require.Len(t, k.GetAllBurnSummaries(), 1)

	t.Logf("10K accounts at cap: totalBurned=%d, perHuman=%d, remainder=%d",
		summary.TotalBurned, summary.PerHumanAmount, summary.Remainder)
}

// ── Simulation: 10,000 accounts with zero balance ──────────────────────────────

func TestSimulation_TenThousandZeroBalances(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	const numAccounts = 10_000

	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	// Register 10,000 accounts with zero balance.
	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		mockIssuance.RegisterAccount(addr, 0)
	}

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, int64(0), summary.TotalBurned,
		"0 total burned with all zero balances")
	assert.Equal(t, int64(numAccounts), summary.ActiveHumans,
		"all 10K accounts active")
	assert.Equal(t, int64(0), summary.PerHumanAmount,
		"0 per human redistribution")
	assert.Equal(t, int64(0), summary.Remainder,
		"0 remainder")

	// All accounts should have 0 balance after burn.
	for _, acct := range mockIssuance.Accounts {
		assert.True(t, acct.Balance.IsZero(),
			"account %s should have 0 balance", acct.Address)
	}
}

// ── Simulation: Mixed accounts — zero, small, max, and mid-range balances ───────

func TestSimulation_MixedBalances(t *testing.T) {
	rng := rand.New(rand.NewSource(12345))
	const numAccounts = 10_000

	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	addresses := make([]string, numAccounts)
	totalExpectedBurn := int64(0)

	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		addresses[i] = addr

		var balance int64
		switch i % 5 {
		case 0:
			balance = 0 // zero balance
		case 1:
			balance = rng.Int63n(1_000_000) // small balance (< 1 credit)
		case 2:
			balance = rng.Int63n(burn.MaxBalanceUcredit) // mid-range
		case 3:
			balance = burn.MaxBalanceUcredit // at cap
		case 4:
			balance = rng.Int63n(burn.MaxBalanceUcredit/2) + burn.MaxBalanceUcredit/2 // high but not at cap
		}
		mockIssuance.RegisterAccount(addr, balance)
		totalExpectedBurn += balance
	}

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, totalExpectedBurn, summary.TotalBurned)
	assert.Equal(t, int64(numAccounts), summary.ActiveHumans)

	expectedPerHuman := totalExpectedBurn / numAccounts
	assert.Equal(t, expectedPerHuman, summary.PerHumanAmount)

	// Verify accounts with initial zero balance got their redistribution share.
	for _, addr := range addresses[:1000] {
		bal := mockIssuance.GetAccountBalance(addr)
		assert.Equal(t, math.NewInt(expectedPerHuman), bal,
			"account %s should have perHuman=%d after redistribution", addr, expectedPerHuman)
	}

	// Verify summary is queryable.
	s := k.GetBurnSummary(1)
	require.NotNil(t, s)
	assert.Equal(t, firstBurn, s.YearHeight)

	s = k.GetBurnSummaryByHeight(firstBurn)
	require.NotNil(t, s)
	assert.Equal(t, totalExpectedBurn, s.TotalBurned)
}

// ── Simulation: Burn event audit trail with 10,000 accounts ─────────────────────

func TestSimulation_BurnAuditTrailTenThousand(t *testing.T) {
	rng := rand.New(rand.NewSource(7777))
	const numAccounts = 10_000

	mockIssuance := NewSimIssuanceKeeper(1)
	bus := NewMockEventBus()
	k := burn.NewKeeperWithEvents(mockIssuance, bus, 1, simReserveAddr)

	// Register accounts with random balances.
	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		balance := rng.Int63n(burn.MaxBalanceUcredit + 1)
		mockIssuance.RegisterAccount(addr, balance)
	}

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, numAccounts, k.GetBurnRecordCount(),
		"should have burn record for each account")

	// Verify events were emitted for each account.
	require.Len(t, bus.AnnualBurnEvents, numAccounts,
		"should have an annual burn event for each account")

	// Verify summary event was emitted.
	require.Len(t, bus.AnnualBurnSummaryEvents, 1,
		"should have 1 summary event")

	// Verify summary event matches the summary.
	summaryEvent := bus.AnnualBurnSummaryEvents[0]
	assert.Equal(t, summary.TotalBurned, summaryEvent.TotalBurned)
	assert.Equal(t, summary.TotalRedistributed, summaryEvent.TotalRedistributed)
	assert.Equal(t, summary.ActiveHumans, summaryEvent.ActiveHumans)
	assert.Equal(t, summary.PerHumanAmount, summaryEvent.PerHumanAmount)
	assert.Equal(t, summary.Remainder, summaryEvent.Remainder)
	assert.Equal(t, simReserveAddr, summaryEvent.ReserveAddress)

	t.Logf("Audit trail: %d burn events, %d summary events, %d records",
		len(bus.AnnualBurnEvents), len(bus.AnnualBurnSummaryEvents), k.GetBurnRecordCount())
}

// ── Simulation: Multiple years with 10,000 accounts ────────────────────────────

func TestSimulation_MultipleYearsTenThousandAccounts(t *testing.T) {
	rng := rand.New(rand.NewSource(5555))
	const numAccounts = 10_000

	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	// Register 10,000 accounts.
	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		balance := rng.Int63n(burn.MaxBalanceUcredit + 1)
		mockIssuance.RegisterAccount(addr, balance)
	}

	// Simulate 5 years of burns.
	for year := int64(1); year <= 5; year++ {
		burnHeight := 1 + year*burn.BlocksPerYear

		// Before burning, set new random balances for all accounts.
		if year > 1 {
			for _, acct := range mockIssuance.Accounts {
				newBalance := rng.Int63n(burn.MaxBalanceUcredit + 1)
				mockIssuance.SetAccountBalance(acct.Address, math.NewInt(newBalance))
			}
		}

		summary := k.BeginBlock(burnHeight)
		require.NotNil(t, summary, "year %d burn should have occurred", year)

		// Verify redistribution math.
		perHuman, remainder := burn.CalculateRedistribution(summary.TotalBurned, numAccounts)
		assert.Equal(t, perHuman, summary.PerHumanAmount,
			"year %d per human", year)
		assert.Equal(t, remainder, summary.Remainder,
			"year %d remainder", year)

		// Verify all accounts have the perHuman balance.
		for _, acct := range mockIssuance.Accounts {
			assert.Equal(t, math.NewInt(perHuman), acct.Balance,
				"account %s after year %d burn", acct.Address, year)
		}

		t.Logf("Year %d: totalBurned=%d, perHuman=%d, remainder=%d",
			year, summary.TotalBurned, perHuman, remainder)
	}

	// Verify we have 5 summaries.
	summaries := k.GetAllBurnSummaries()
	require.Len(t, summaries, 5, "should have 5 yearly summaries")

	// Verify we have 5 years × 10,000 accounts = 50,000 burn records.
	assert.Equal(t, 5*numAccounts, k.GetBurnRecordCount(),
		"should have 50,000 burn records for 5 years")
}

// ── Simulation: Post-burn total supply invariant ───────────────────────────────

func TestSimulation_PostBurnSupplyInvariant(t *testing.T) {
	// After burn and redistribution, the total supply should equal
	// perHumanAmount * activeHumans (i.e., the redistribution pool).
	rng := rand.New(rand.NewSource(8888))
	const numAccounts = 10_000

	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	// Register accounts with random balances.
	for i := 0; i < numAccounts; i++ {
		addr := randomAddress(rng)
		balance := rng.Int63n(burn.MaxBalanceUcredit + 1)
		mockIssuance.RegisterAccount(addr, balance)
	}

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)

	// Post-burn supply = perHumanAmount * activeHumans.
	expectedSupply := summary.PerHumanAmount * summary.ActiveHumans
	assert.Equal(t, expectedSupply, summary.TotalRedistributed,
		"post-burn supply must equal total redristributed")
	assert.True(t, summary.TotalRedistributed <= summary.TotalBurned,
		"post-burn supply must be <= pre-burn total (remainder goes to reserve)")

	// Verify totalSupply = sum of all account balances.
	var sumBalances int64
	for _, acct := range mockIssuance.Accounts {
		sumBalances += acct.Balance.BigInt().Int64()
	}
	assert.Equal(t, expectedSupply, sumBalances,
		"sum of all account balances must equal expected post-burn supply")
}

// ── Simulation: Random burn block heights ───────────────────────────────────────

func TestSimulation_IsBurnBlockAtVariousHeights(t *testing.T) {
	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	// Verify IsBurnBlock behavior at various heights.
	for n := int64(1); n <= 100; n++ {
		burnHeight := 1 + n*burn.BlocksPerYear
		assert.True(t, k.IsBurnBlock(burnHeight),
			"burn block %d (n=%d) should be detected", burnHeight, n)

		// Block just before burn is not a burn block.
		assert.False(t, k.IsBurnBlock(burnHeight-1),
			"block before burn %d should not be burn block", burnHeight-1)

		// Block just after burn is not a burn block.
		assert.False(t, k.IsBurnBlock(burnHeight+1),
			"block after burn %d should not be burn block", burnHeight+1)
	}
}

// ── Simulation: Edge case — single account, extreme values ──────────────────────

func TestSimulation_SingleAccountExtremeValues(t *testing.T) {
	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	// Max possible balance.
	mockIssuance.RegisterAccount("max-account", burn.MaxBalanceUcredit)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, burn.MaxBalanceUcredit, summary.TotalBurned)
	assert.Equal(t, int64(1), summary.ActiveHumans)
	assert.Equal(t, burn.MaxBalanceUcredit, summary.PerHumanAmount)
	assert.Equal(t, int64(0), summary.Remainder)

	bal := mockIssuance.GetAccountBalance("max-account")
	assert.Equal(t, math.NewInt(burn.MaxBalanceUcredit), bal)
}

// ── Simulation: Accounts registered at different times ─────────────────────────

func TestSimulation_AccountsRegisteredOverTime(t *testing.T) {
	rng := rand.New(rand.NewSource(333))

	// Phase 1: Initial 5000 accounts, full year burn.
	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	for i := 0; i < 5000; i++ {
		addr := randomAddress(rng)
		balance := rng.Int63n(burn.MaxBalanceUcredit + 1)
		mockIssuance.RegisterAccount(addr, balance)
	}

	// First burn.
	firstBurn := 1 + burn.BlocksPerYear
	summary1 := k.BeginBlock(firstBurn)
	require.NotNil(t, summary1)
	assert.Equal(t, int64(5000), summary1.ActiveHumans)

	// Phase 2: Mid-year, add 5000 more accounts.
	// Set new balances for original accounts.
	for _, acct := range mockIssuance.Accounts {
		newBalance := rng.Int63n(burn.MaxBalanceUcredit + 1)
		mockIssuance.SetAccountBalance(acct.Address, math.NewInt(newBalance))
	}

	// Add new accounts mid-year-2.
	for i := 0; i < 5000; i++ {
		addr := randomAddress(rng)
		balance := rng.Int63n(burn.MaxBalanceUcredit + 1)
		mockIssuance.RegisterAccount(addr, balance)
	}

	// Second burn — should include all 10,000 accounts.
	secondBurn := 1 + 2*burn.BlocksPerYear
	summary2 := k.BeginBlock(secondBurn)
	require.NotNil(t, summary2)
	assert.Equal(t, int64(10_000), summary2.ActiveHumans,
		"all 10,000 accounts should participate in year 2 burn")

	// Verify each burn record count.
	assert.Equal(t, 5_000+10_000, k.GetBurnRecordCount(),
		"5000 records from year 1 + 10,000 from year 2 = 15,000")
}

// ── Simulation: Redistribution edge case — large remainder ──────────────────────

func TestSimulation_LargeRemainderWithManyAccounts(t *testing.T) {
	mockIssuance := NewSimIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, simReserveAddr)

	const numAccounts = 10_000

	// Set balances such that totalBurned % numAccounts produces a large remainder.
	// totalBurned = 10,000 * 1,000,000 + 9,999 = remainder 9,999
	baseBalance := int64(1_000_000) // 1 credit each
	for i := 0; i < numAccounts; i++ {
		addr := fmt.Sprintf("addr-%d", i)
		var balance int64
		if i == 0 {
			balance = baseBalance + 9_999 // add remainder to first account
		} else {
			balance = baseBalance
		}
		mockIssuance.RegisterAccount(addr, balance)
	}

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	totalBurned := int64(10_000_000_000 + 9_999) // 10 billion + 9999
	assert.Equal(t, totalBurned, summary.TotalBurned)

	expectedPerHuman := totalBurned / numAccounts // 1_000_000
	expectedRemainder := totalBurned % numAccounts // 9_999
	assert.Equal(t, expectedPerHuman, summary.PerHumanAmount)
	assert.Equal(t, expectedRemainder, summary.Remainder)
	assert.Equal(t, totalBurned, summary.PerHumanAmount*numAccounts+summary.Remainder)
}