package burn_test

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-credit-chain/x/burn"
	"github.com/tpt-platform/tpt-platform/packages/tpt-credit-chain/x/issuance"
)

// ── MockIssuanceKeeper ─────────────────────────────────────────────────────────

// MockIssuanceKeeper implements burn.IssuanceKeeper for testing.
type MockIssuanceKeeper struct {
	Accounts      map[string]*MockAccount
	HumanCount    int64
	GenesisHeight int64
}

type MockAccount struct {
	Address             string
	Balance             math.Int
	YearCreditsReceived math.Int
	YearPayoutCount     int64
	LastPaidAt          int64
	RegisteredAt        int64
}

func NewMockIssuanceKeeper(genesisHeight int64) *MockIssuanceKeeper {
	return &MockIssuanceKeeper{
		Accounts:      make(map[string]*MockAccount),
		HumanCount:    0,
		GenesisHeight: genesisHeight,
	}
}

func (mk *MockIssuanceKeeper) GetAccounts() []burn.IssuanceAccount {
	accounts := make([]burn.IssuanceAccount, 0, len(mk.Accounts))
	for _, acct := range mk.Accounts {
		accounts = append(accounts, burn.IssuanceAccount{
			Address:             acct.Address,
			Balance:             acct.Balance,
			YearCreditsReceived: acct.YearCreditsReceived,
			YearPayoutCount:     acct.YearPayoutCount,
			LastPaidAt:          acct.LastPaidAt,
			RegisteredAt:        acct.RegisteredAt,
		})
	}
	return accounts
}

func (mk *MockIssuanceKeeper) GetHumanCount() int64 {
	return mk.HumanCount
}

func (mk *MockIssuanceKeeper) ResetYearCredits() {
	for _, acct := range mk.Accounts {
		acct.YearCreditsReceived = math.ZeroInt()
		acct.YearPayoutCount = 0
	}
}

func (mk *MockIssuanceKeeper) GetGenesisHeight() int64 {
	return mk.GenesisHeight
}

func (mk *MockIssuanceKeeper) SetAccountBalance(address string, balance math.Int) {
	if acct, ok := mk.Accounts[address]; ok {
		acct.Balance = balance
	}
}

func (mk *MockIssuanceKeeper) GetAccountBalance(address string) math.Int {
	if acct, ok := mk.Accounts[address]; ok {
		return acct.Balance
	}
	return math.ZeroInt()
}

func (mk *MockIssuanceKeeper) RegisterAccount(address string, balance int64) {
	mk.Accounts[address] = &MockAccount{
		Address:             address,
		Balance:             math.NewInt(balance),
		YearCreditsReceived: math.NewInt(balance),
		YearPayoutCount:     12,
		LastPaidAt:          mk.GenesisHeight,
		RegisteredAt:        mk.GenesisHeight,
	}
	mk.HumanCount++
}

func (mk *MockIssuanceKeeper) RegisterAccountWithBalance(address string, balance math.Int) {
	mk.Accounts[address] = &MockAccount{
		Address:             address,
		Balance:             balance,
		YearCreditsReceived: math.ZeroInt(),
		YearPayoutCount:     0,
		LastPaidAt:          mk.GenesisHeight,
		RegisteredAt:        mk.GenesisHeight,
	}
	mk.HumanCount++
}

// ── MockEventBus ───────────────────────────────────────────────────────────────

// MockEventBus collects emitted burn events for test assertions.
type MockEventBus struct {
	AnnualBurnEvents        []burn.EventAnnualBurn
	AnnualBurnSummaryEvents []burn.EventAnnualBurnSummary
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		AnnualBurnEvents:        make([]burn.EventAnnualBurn, 0),
		AnnualBurnSummaryEvents: make([]burn.EventAnnualBurnSummary, 0),
	}
}

func (m *MockEventBus) EmitAnnualBurn(event burn.EventAnnualBurn) {
	m.AnnualBurnEvents = append(m.AnnualBurnEvents, event)
}

func (m *MockEventBus) EmitAnnualBurnSummary(event burn.EventAnnualBurnSummary) {
	m.AnnualBurnSummaryEvents = append(m.AnnualBurnSummaryEvents, event)
}

// ── Test constants ─────────────────────────────────────────────────────────────

const testReserveAddr = "cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9"

var testAddrs = []string{
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqg7f8d8",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqp8n9e7",
}

// ── Test: IsBurnBlock ──────────────────────────────────────────────────────────

func TestIsBurnBlock_GenesisNotBurn(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	assert.False(t, k.IsBurnBlock(1), "genesis block is not a burn block")
}

func TestIsBurnBlock_FirstBurnAtYearEnd(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	assert.True(t, k.IsBurnBlock(firstBurn), "first burn occurs at genesis + BlocksPerYear")
}

func TestIsBurnBlock_SecondBurnAtTwoYears(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	secondBurn := 1 + 2*burn.BlocksPerYear
	assert.True(t, k.IsBurnBlock(secondBurn), "second burn occurs at genesis + 2*BlocksPerYear")
}

func TestIsBurnBlock_BetweenBurnsNotBurn(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	midYear := 1 + burn.BlocksPerYear/2
	assert.False(t, k.IsBurnBlock(midYear), "mid-year blocks are not burn blocks")
}

// ── Test: Basic burn with multiple accounts ────────────────────────────────────

func TestBeginBlock_SingleAccountFullBurn(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000) // 12,000 credits

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	// Advance to first burn block.
	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary, "burn should have occurred")
	assert.Equal(t, int64(12_000_000_000), summary.TotalBurned, "total burned = 12,000 credits")
	assert.Equal(t, int64(1), summary.ActiveHumans, "1 active human")
	assert.Equal(t, int64(12_000_000_000), summary.PerHumanAmount, "entire pool redistributed to single human")
	assert.Equal(t, int64(0), summary.Remainder, "no remainder with 1 human")
	assert.Equal(t, int64(12_000_000_000), summary.TotalRedistributed, "total redistributed")

	// Verify balance was set to redistribution share.
	bal := mockIssuance.GetAccountBalance(testAddrs[0])
	assert.Equal(t, math.NewInt(12_000_000_000), bal, "balance after burn = redistribution share")
}

func TestBeginBlock_MultipleAccountsEvenSplit(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 10_000_000_000) // 10,000 credits
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)  // 6,000 credits

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, int64(16_000_000_000), summary.TotalBurned, "total burned = 16,000 credits")
	assert.Equal(t, int64(2), summary.ActiveHumans)

	// Per human = 16,000,000,000 / 2 = 8,000,000,000
	assert.Equal(t, int64(8_000_000_000), summary.PerHumanAmount)

	// Each account should now have 8 billion ucredit.
	bal0 := mockIssuance.GetAccountBalance(testAddrs[0])
	bal1 := mockIssuance.GetAccountBalance(testAddrs[1])
	assert.Equal(t, math.NewInt(8_000_000_000), bal0)
	assert.Equal(t, math.NewInt(8_000_000_000), bal1)
}

func TestBeginBlock_MultipleAccountsWithRemainder(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 10_000_000_001) // 10,000 credits + 1 ucredit
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)  // 6,000 credits
	mockIssuance.RegisterAccount(testAddrs[2], 2_000_000_000)  // 2,000 credits

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	totalBurned := int64(10_000_000_001 + 6_000_000_000 + 2_000_000_000) // 18,000,000,001
	assert.Equal(t, totalBurned, summary.TotalBurned)
	assert.Equal(t, int64(3), summary.ActiveHumans)

	// Per human = 18,000,000,001 / 3 = 6,000,000,000
	// Remainder = 18,000,000,001 - (6,000,000,000 * 3) = 1
	perHuman := int64(6_000_000_000)
	remainder := int64(1)

	assert.Equal(t, perHuman, summary.PerHumanAmount)
	assert.Equal(t, remainder, summary.Remainder)
	assert.Equal(t, perHuman*3, summary.TotalRedistributed)

	// Each account should have perHuman after redistribution.
	for _, addr := range testAddrs {
		bal := mockIssuance.GetAccountBalance(addr)
		assert.Equal(t, math.NewInt(perHuman), bal, "account %s should have perHuman balance", addr)
	}
}

// ── Test: Zero balance accounts ─────────────────────────────────────────────────

func TestBeginBlock_ZeroBalanceAccounts(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 0) // zero balance
	mockIssuance.RegisterAccount(testAddrs[1], 0) // zero balance

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, int64(0), summary.TotalBurned, "total burned = 0")
	assert.Equal(t, int64(2), summary.ActiveHumans, "2 active humans")
	assert.Equal(t, int64(0), summary.PerHumanAmount, "per human = 0")
	assert.Equal(t, int64(0), summary.Remainder, "remainder = 0")

	// Both accounts should still have 0 balance.
	for _, addr := range []string{testAddrs[0], testAddrs[1]} {
		bal := mockIssuance.GetAccountBalance(addr)
		assert.True(t, bal.IsZero(), "zero balance account should remain at 0")
	}
}

func TestBeginBlock_SomeZeroBalanceAccounts(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000) // 12,000 credits
	mockIssuance.RegisterAccount(testAddrs[1], 0)              // zero balance

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, int64(12_000_000_000), summary.TotalBurned)
	assert.Equal(t, int64(2), summary.ActiveHumans)

	// Per human = 12,000,000,000 / 2 = 6,000,000,000
	perHuman := int64(6_000_000_000)
	assert.Equal(t, perHuman, summary.PerHumanAmount)

	// Both accounts get redistribution share.
	bal0 := mockIssuance.GetAccountBalance(testAddrs[0])
	bal1 := mockIssuance.GetAccountBalance(testAddrs[1])
	assert.Equal(t, math.NewInt(perHuman), bal0, "account with balance gets redistribution share")
	assert.Equal(t, math.NewInt(perHuman), bal1, "zero balance account also gets redistribution share")
}

// ── Test: Newly registered accounts (mid-year) ──────────────────────────────────

func TestBeginBlock_NewlyRegisteredAccounts(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000) // registered at genesis

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	// Half way through the year, register a new account.
	midYear := 1 + burn.BlocksPerYear/2
	k.SetBlockHeight(midYear)

	// Register mid-year account directly (balance = 5 credits from partial payouts).
	mockIssuance.RegisterAccountWithBalance(testAddrs[1], math.NewInt(5_000_000_000))

	// Burn at year end.
	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, int64(2), summary.ActiveHumans)

	// Total burned = 12,000 + 5,000 = 17,000 credits
	assert.Equal(t, int64(12_000_000_000+5_000_000_000), summary.TotalBurned)

	// Per human = 17,000,000,000 / 2 = 8,500,000,000
	perHuman := int64(8_500_000_000)
	assert.Equal(t, perHuman, summary.PerHumanAmount)

	// Both accounts get redistribution share.
	bal0 := mockIssuance.GetAccountBalance(testAddrs[0])
	bal1 := mockIssuance.GetAccountBalance(testAddrs[1])
	assert.Equal(t, math.NewInt(perHuman), bal0)
	assert.Equal(t, math.NewInt(perHuman), bal1)
}

// ── Test: No accounts edge case ────────────────────────────────────────────────

func TestBeginBlock_NoAccounts(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary, "burn should occur even with 0 accounts")
	assert.Equal(t, int64(0), summary.TotalBurned)
	assert.Equal(t, int64(0), summary.ActiveHumans)
	assert.Equal(t, int64(0), summary.PerHumanAmount)
	assert.Equal(t, int64(0), summary.Remainder)
}

// ── Test: Accounts at MaxBalanceUcredit (cap) ───────────────────────────────────

func TestBeginBlock_AccountAtMaxBalance(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	// Account at exactly the mid-year cap (36,000 credits).
	mockIssuance.RegisterAccount(testAddrs[0], burn.MaxBalanceUcredit)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	assert.Equal(t, burn.MaxBalanceUcredit, summary.TotalBurned)
	assert.Equal(t, int64(1), summary.ActiveHumans)

	// Entire balance burns and redistributes back to the same account.
	assert.Equal(t, burn.MaxBalanceUcredit, summary.PerHumanAmount)
	assert.Equal(t, int64(0), summary.Remainder)

	// Account should have 36,000 credits again after redistribution.
	bal := mockIssuance.GetAccountBalance(testAddrs[0])
	assert.Equal(t, math.NewInt(burn.MaxBalanceUcredit), bal)
}

func TestBeginBlock_MultipleAccountsAtMaxBalance(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], burn.MaxBalanceUcredit)
	mockIssuance.RegisterAccount(testAddrs[1], burn.MaxBalanceUcredit)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	totalBurned := 2 * burn.MaxBalanceUcredit
	assert.Equal(t, totalBurned, summary.TotalBurned)
	assert.Equal(t, int64(2), summary.ActiveHumans)

	// Per human = total / 2 = MaxBalanceUcredit
	assert.Equal(t, burn.MaxBalanceUcredit, summary.PerHumanAmount)

	// Both accounts get perHuman back.
	for _, addr := range []string{testAddrs[0], testAddrs[1]} {
		bal := mockIssuance.GetAccountBalance(addr)
		assert.Equal(t, math.NewInt(burn.MaxBalanceUcredit), bal)
	}
}

// ── Test: Event emission ───────────────────────────────────────────────────────

func TestBeginBlock_EventEmittedForEachAccount(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)

	bus := NewMockEventBus()
	k := burn.NewKeeperWithEvents(mockIssuance, bus, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)
	require.Len(t, bus.AnnualBurnEvents, 2, "should have 2 burn events")

	// Verify each event has correct data.
	for _, event := range bus.AnnualBurnEvents {
		assert.Equal(t, firstBurn, event.YearHeight)
		assert.Equal(t, int64(1), event.Year, "year 1")
		assert.Equal(t, int64(9_000_000_000), event.RedistributedAmount, "each account gets 9,000,000,000")
	}

	// Verify specific burned amounts.
	event0 := bus.AnnualBurnEvents[0]
	event1 := bus.AnnualBurnEvents[1]
	assert.Equal(t, int64(18_000_000_000), event0.BurnedAmount+event1.BurnedAmount,
		"sum of burned amounts should be 18,000,000,000")
}

func TestBeginBlock_SummaryEventEmitted(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	bus := NewMockEventBus()
	k := burn.NewKeeperWithEvents(mockIssuance, bus, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	k.BeginBlock(firstBurn)

	require.Len(t, bus.AnnualBurnSummaryEvents, 1, "should have 1 summary event")

	summaryEvent := bus.AnnualBurnSummaryEvents[0]
	assert.Equal(t, firstBurn, summaryEvent.YearHeight)
	assert.Equal(t, int64(1), summaryEvent.Year)
	assert.Equal(t, int64(12_000_000_000), summaryEvent.TotalBurned)
	assert.Equal(t, int64(12_000_000_000), summaryEvent.TotalRedistributed)
	assert.Equal(t, int64(1), summaryEvent.ActiveHumans)
	assert.Equal(t, int64(12_000_000_000), summaryEvent.PerHumanAmount)
	assert.Equal(t, int64(0), summaryEvent.Remainder)
	assert.Equal(t, testReserveAddr, summaryEvent.ReserveAddress)
}

func TestBeginBlock_NoEventsWithoutEventBus(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	// Keeper created without event bus.
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)
	require.NotNil(t, summary)
	assert.Equal(t, int64(12_000_000_000), summary.TotalBurned)
}

// ── Test: Burn audit log ───────────────────────────────────────────────────────

func TestBurnAuditLog_SingleAccount(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	k.BeginBlock(firstBurn)

	// Query burn record by address and year.
	record := k.GetBurnRecord(testAddrs[0], 1)
	require.NotNil(t, record)
	assert.Equal(t, testAddrs[0], record.Address)
	assert.Equal(t, int64(12_000_000_000), record.BurnedAmount)
	assert.Equal(t, int64(12_000_000_000), record.RedistributedAmount)
	assert.Equal(t, firstBurn, record.YearHeight)
}

func TestBurnAuditLog_MultipleYears(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	// Register account with balance that persists across years.
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	// Simulate year 1, 2, 3 burns.
	for year := int64(1); year <= 3; year++ {
		burnHeight := 1 + year*burn.BlocksPerYear
		k.BeginBlock(burnHeight)
	}

	// Query records by address.
	records := k.GetBurnRecordsByAddress(testAddrs[0])
	require.Len(t, records, 3, "should have 3 burn records for 3 years")

	// Verify each record.
	for i, record := range records {
		expectedHeight := 1 + int64(i+1)*burn.BlocksPerYear
		assert.Equal(t, expectedHeight, record.YearHeight,
			"year %d burn height", i+1)
	}

	// Query specific year records.
	year2Record := k.GetBurnRecord(testAddrs[0], 2)
	require.NotNil(t, year2Record)
	assert.Equal(t, int64(2), year2Record.YearHeight/burn.BlocksPerYear,
		"year 2 record height is 2*BlocksPerYear from genesis")
}

func TestBurnAuditLog_QueryableByYear(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	k.BeginBlock(firstBurn)

	// Get all records for year 1.
	records := k.GetBurnRecordsByYear(1)
	assert.Len(t, records, 2, "should have 2 records for year 1")

	// Get non-existent year.
	records = k.GetBurnRecordsByYear(99)
	assert.Empty(t, records, "no records for year 99")
}

func TestBurnAuditLog_SummaryQueryable(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	// Year 1.
	year1Burn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(year1Burn)
	require.NotNil(t, summary)

	// Query summary by year index.
	s := k.GetBurnSummary(1)
	require.NotNil(t, s)
	assert.Equal(t, int64(18_000_000_000), s.TotalBurned)

	// Query summary by height.
	s = k.GetBurnSummaryByHeight(year1Burn)
	require.NotNil(t, s)
	assert.Equal(t, int64(18_000_000_000), s.TotalBurned)

	// Non-existent summary.
	s = k.GetBurnSummary(99)
	assert.Nil(t, s, "no summary for year 99")

	s = k.GetBurnSummaryByHeight(9999)
	assert.Nil(t, s, "no summary for non-burn height")
}

// ── Test: Burn only once per year ──────────────────────────────────────────────

func TestBeginBlock_BurnOnlyOncePerYear(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)
	require.NotNil(t, summary)

	// Calling BeginBlock again at same height should not trigger another burn.
	summary2 := k.BeginBlock(firstBurn)
	assert.Nil(t, summary2, "second call at same burn height should not trigger burn")
}

func TestBeginBlock_NotBurnBlockDoesNothing(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	// Mid-year block should not trigger burn.
	midYear := 1 + burn.BlocksPerYear/2
	summary := k.BeginBlock(midYear)
	assert.Nil(t, summary, "mid-year block should not trigger burn")

	// Balance should be unchanged.
	bal := mockIssuance.GetAccountBalance(testAddrs[0])
	assert.Equal(t, math.NewInt(12_000_000_000), bal)
}

// ── Test: Redistribution remainder goes to reserve ──────────────────────────────

func TestCalculateRedistribution_RemainderAccounting(t *testing.T) {
	// Verify the mathematical invariant: totalBurned = perHuman * activeHumans + remainder
	totalBurned := int64(10_000_000_001)
	activeHumans := int64(3)

	perHuman, remainder := burn.CalculateRedistribution(totalBurned, activeHumans)
	assert.Equal(t, totalBurned, perHuman*activeHumans+remainder,
		"total burned must equal sum of redistributed + remainder")
}

func TestCalculateRedistribution_LargeRemainder(t *testing.T) {
	// Large remainder test: 10,000,000,000 / 3 = 3,333,333,333 remainder 1
	perHuman, remainder := burn.CalculateRedistribution(10_000_000_000, 3)
	assert.Equal(t, int64(3_333_333_333), perHuman)
	assert.Equal(t, int64(1), remainder)
}

// ── Test: Year tracking ────────────────────────────────────────────────────────

func TestYearTracking_AdvancesCorrectly(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	assert.Equal(t, int64(1), k.GetCurrentYear(), "year starts at 1")

	// Advance to burn block.
	firstBurn := 1 + burn.BlocksPerYear
	k.BeginBlock(firstBurn)
	assert.Equal(t, int64(2), k.GetCurrentYear(), "year advances to 2 after first burn")

	// Advance to second burn.
	secondBurn := 1 + 2*burn.BlocksPerYear
	k.BeginBlock(secondBurn)
	assert.Equal(t, int64(3), k.GetCurrentYear(), "year advances to 3 after second burn")
}

// ── Test: Genesis state management ─────────────────────────────────────────────

func TestDefaultGenesisState(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	gs := k.DefaultGenesisState()
	require.NotNil(t, gs)
	assert.Empty(t, gs.BurnRecords)
}

func TestValidateGenesis_Valid(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	gs := k.DefaultGenesisState()
	err := k.ValidateGenesis(gs)
	assert.NoError(t, err)
}

func TestValidateGenesis_InvalidNilRecords(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	gs := &burn.GenesisState{BurnRecords: nil}
	err := k.ValidateGenesis(gs)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestExportGenesis(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	k.BeginBlock(firstBurn)

	gs := k.ExportGenesis()
	require.NotNil(t, gs)
	assert.Len(t, gs.BurnRecords, 1, "should have 1 burn record exported")
	assert.Equal(t, testAddrs[0], gs.BurnRecords[0].Address)
	assert.Equal(t, int64(12_000_000_000), gs.BurnRecords[0].BurnedAmount)
}

func TestInitializeGenesis(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	// The record stores with key "address/year"
	// Year is derived from YearHeight: (YearHeight - genesisHeight) / BlocksPerYear
	// For YearHeight = 1 + BlocksPerYear, the year key is 1
	burnHeight := 1 + burn.BlocksPerYear
	record := burn.BurnRecord{
		Address:             testAddrs[0],
		BurnedAmount:        12_000_000_000,
		RedistributedAmount: 12_000_000_000,
		YearHeight:          burnHeight,
	}
	gs := &burn.GenesisState{
		BurnRecords: []burn.BurnRecord{record},
	}

	err := k.InitializeGenesis(context.Background(), gs)
	require.NoError(t, err)

	// Verify record was loaded by querying with year=1.
	r := k.GetBurnRecord(testAddrs[0], 1)
	require.NotNil(t, r)
	assert.Equal(t, int64(12_000_000_000), r.BurnedAmount)
}

func TestInitializeGenesis_NilState(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	err := k.InitializeGenesis(context.Background(), nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

// ── Test: Integration with issuance module types ───────────────────────────────

func TestMaxBalanceConstant_MatchesIssuanceAnnualAllocation(t *testing.T) {
	// MaxBalanceUcredit = 3 × annual allocation
	expected := 3 * int64(12_000) * 1_000_000 // 36,000 credits in ucredit
	assert.Equal(t, expected, burn.MaxBalanceUcredit)
	assert.Equal(t, int64(36_000), burn.MaxBalanceCredits)
}

func TestBurnBlock_MatchesIssuanceBlocksPerYear(t *testing.T) {
	// Burn uses the same BlocksPerYear as issuance.
	assert.Equal(t, issuance.BlocksPerYear, burn.BlocksPerYear,
		"burn and issuance must use the same BlocksPerYear")
}

// ── Test: Zero humans edge case ────────────────────────────────────────────────

func TestCalculateRedistribution_ZeroHumansAllToReserve(t *testing.T) {
	perHuman, remainder := burn.CalculateRedistribution(1_000_000, 0)
	assert.Equal(t, int64(0), perHuman)
	assert.Equal(t, int64(1_000_000), remainder, "all goes to reserve with 0 humans")
}

// ── Test: BeginBlock at exact burn boundaries ───────────────────────────────────

func TestBeginBlock_ExactBurnBoundaries(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	// Block just before burn.
	justBefore := 1 + burn.BlocksPerYear - 1
	summary := k.BeginBlock(justBefore)
	assert.Nil(t, summary, "block before burn should not trigger burn")

	// Burn block.
	burnBlock := 1 + burn.BlocksPerYear
	summary = k.BeginBlock(burnBlock)
	require.NotNil(t, summary, "burn should trigger on exact burn block")
	assert.Equal(t, int64(12_000_000_000), summary.TotalBurned)

	// Next block.
	summary = k.BeginBlock(burnBlock + 1)
	assert.Nil(t, summary, "should not burn again next block")
}

// ── Test: GetBurnRecord non-existent ───────────────────────────────────────────

func TestGetBurnRecord_NonExistent(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	record := k.GetBurnRecord("nonexistent", 1)
	assert.Nil(t, record)
}

func TestGetBurnRecordsByAddress_NoRecords(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	records := k.GetBurnRecordsByAddress(testAddrs[0])
	assert.Empty(t, records)
}

// ── Test: Keeper state methods ─────────────────────────────────────────────────

func TestKeeper_GetSetBlockHeight(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	assert.Equal(t, int64(1), k.GetBlockHeight())
	k.SetBlockHeight(42)
	assert.Equal(t, int64(42), k.GetBlockHeight())
}

func TestKeeper_GetSetCurrentYear(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	assert.Equal(t, int64(1), k.GetCurrentYear())
	k.SetCurrentYear(5)
	assert.Equal(t, int64(5), k.GetCurrentYear())
}

func TestKeeper_GetSetLastBurnYear(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	assert.Equal(t, int64(0), k.GetLastBurnYear())
	k.SetLastBurnYear(2)
	assert.Equal(t, int64(2), k.GetLastBurnYear())
}

func TestKeeper_GenesisHeight(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(100)
	k := burn.NewKeeper(mockIssuance, 100, testReserveAddr)

	assert.Equal(t, int64(100), k.GetGenesisHeight())
}

// ── Test: Multiple years simulation ────────────────────────────────────────────

func TestBeginBlock_MultipleYearsRedistribution(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	// Year 1: burn.
	year1Burn := 1 + burn.BlocksPerYear
	summary1 := k.BeginBlock(year1Burn)
	require.NotNil(t, summary1)
	assert.Equal(t, int64(18_000_000_000), summary1.TotalBurned, "year 1 total burned")
	assert.Equal(t, int64(9_000_000_000), summary1.PerHumanAmount, "year 1 per human")

	// Both accounts now have 9 credits after redistribution.
	// Simulate new payouts throughout the year by setting new balances.
	mockIssuance.SetAccountBalance(testAddrs[0], math.NewInt(12_000_000_000))
	mockIssuance.SetAccountBalance(testAddrs[1], math.NewInt(6_000_000_000))

	// Year 2: burn.
	year2Burn := 1 + 2*burn.BlocksPerYear
	summary2 := k.BeginBlock(year2Burn)
	require.NotNil(t, summary2)
	assert.Equal(t, int64(18_000_000_000), summary2.TotalBurned, "year 2 total burned")
	assert.Equal(t, int64(9_000_000_000), summary2.PerHumanAmount, "year 2 per human")

	// Verify we have 2 summaries.
	summaries := k.GetAllBurnSummaries()
	assert.Len(t, summaries, 2)
}

// ── Test: Active humans count updates correctly ─────────────────────────────────

func TestActiveHumans_CountChanges(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 12_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)
	assert.Equal(t, int64(1), k.GetActiveHumans())

	// Register another account.
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)
	assert.Equal(t, int64(2), k.GetActiveHumans())
}

// ── Test: Burn with large numbers (edge case) ───────────────────────────────────

func TestBeginBlock_LargeBurnAmounts(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	// Set balance to MaxBalanceUcredit for multiple accounts.
	mockIssuance.RegisterAccount(testAddrs[0], burn.MaxBalanceUcredit)
	mockIssuance.RegisterAccount(testAddrs[1], burn.MaxBalanceUcredit)
	mockIssuance.RegisterAccount(testAddrs[2], burn.MaxBalanceUcredit)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)

	firstBurn := 1 + burn.BlocksPerYear
	summary := k.BeginBlock(firstBurn)

	require.NotNil(t, summary)

	// 3 accounts * 36,000 credits = 108,000 credits
	expectedTotal := 3 * burn.MaxBalanceUcredit
	assert.Equal(t, expectedTotal, summary.TotalBurned)
	assert.Equal(t, burn.MaxBalanceUcredit, summary.PerHumanAmount,
		"108,000,000,000 / 3 = 36,000,000,000 per human")
	assert.Equal(t, int64(0), summary.Remainder,
		"evenly divisible should have 0 remainder")
}

// ── Test: Event attributes completeness ─────────────────────────────────────────

func TestEventAnnualBurn_Attributes(t *testing.T) {
	burnedAmount := int64(12_000_000_000)
	event := burn.EventAnnualBurn{
		Address:             testAddrs[0],
		BurnedAmount:        burnedAmount,
		RedistributedAmount: 6_000_000_000,
		YearHeight:          1 + burn.BlocksPerYear,
		Year:                1,
	}

	attrs := event.Attributes()
	assert.Len(t, attrs, 10, "5 key-value pairs")
	assert.Contains(t, attrs, "address")
	assert.Contains(t, attrs, testAddrs[0])
	assert.Contains(t, attrs, "burned_amount")
	assert.Contains(t, attrs, fmt.Sprintf("%d", burnedAmount))
}

func TestEventAnnualBurnSummary_Attributes(t *testing.T) {
	event := burn.EventAnnualBurnSummary{
		YearHeight:        1 + burn.BlocksPerYear,
		Year:              1,
		TotalBurned:       18_000_000_000,
		TotalRedistributed: 18_000_000_000,
		ActiveHumans:      2,
		PerHumanAmount:    9_000_000_000,
		Remainder:         0,
		ReserveAddress:    testReserveAddr,
	}

	attrs := event.Attributes()
	assert.Len(t, attrs, 16, "8 key-value pairs")
	assert.Contains(t, attrs, "total_burned")
	assert.Contains(t, attrs, "reserve_address")
	assert.Contains(t, attrs, testReserveAddr)
}
// ── Test: remainder is conserved in the reserve ──────────────────────────────────

func TestBeginBlock_RemainderCreditedToReserve(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 10_000_000_001)
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)
	mockIssuance.RegisterAccount(testAddrs[2], 2_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)
	before := int64(10_000_000_001 + 6_000_000_000 + 2_000_000_000)

	summary := k.BeginBlock(1 + burn.BlocksPerYear)
	require.NotNil(t, summary)

	after := int64(0)
	for _, addr := range testAddrs {
		after += mockIssuance.GetAccountBalance(addr).Int64()
	}
	reserve := k.ReserveBalance().Int64()

	assert.Equal(t, int64(1), reserve, "the rounding remainder goes to the reserve")
	assert.Equal(t, before, after+reserve, "burned ucredit must equal redistributed plus reserve")
}

func TestBeginBlock_ReserveAccumulatesAcrossYears(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 10_000_000_001)
	mockIssuance.RegisterAccount(testAddrs[1], 6_000_000_000)
	mockIssuance.RegisterAccount(testAddrs[2], 2_000_000_000)

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)
	k.BeginBlock(1 + burn.BlocksPerYear)
	// Year 2: the three humans again hold 6,000,000,000 each; the total divides evenly.
	k.BeginBlock(1 + 2*burn.BlocksPerYear)

	assert.Equal(t, int64(1), k.ReserveBalance().Int64(), "year 2 has no remainder, so the reserve keeps year 1's")
}

// A balance that does not fit in int64 must halt the burn rather than wrap to a wrong amount.
func TestBeginBlock_OversizedBalanceHaltsInsteadOfTruncating(t *testing.T) {
	mockIssuance := NewMockIssuanceKeeper(1)
	mockIssuance.RegisterAccount(testAddrs[0], 1)
	mockIssuance.Accounts[testAddrs[0]].Balance = math.NewIntFromBigInt(new(big.Int).Lsh(big.NewInt(1), 70))

	k := burn.NewKeeper(mockIssuance, 1, testReserveAddr)
	assert.Panics(t, func() { k.BeginBlock(1 + burn.BlocksPerYear) })
}
