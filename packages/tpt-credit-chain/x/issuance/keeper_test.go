package issuance_test

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-credit-chain/x/issuance"
)

// ── MockEventBus ───────────────────────────────────────────────────────────────

// MockEventBus collects emitted events for test assertions.
type MockEventBus struct {
	HumanRegisteredEvents    []issuance.HumanRegisteredEvent
	CreditsIssuedEvents      []issuance.CreditsIssuedEvent
	FrequencyUpdatedEvents   []issuance.FrequencyUpdatedEvent
	CreditsTransferredEvents []issuance.CreditsTransferredEvent
	BatchSendEvents          []issuance.BatchSendEvent
}

func NewMockEventBus() *MockEventBus {
	return &MockEventBus{
		HumanRegisteredEvents:    make([]issuance.HumanRegisteredEvent, 0),
		CreditsIssuedEvents:      make([]issuance.CreditsIssuedEvent, 0),
		FrequencyUpdatedEvents:   make([]issuance.FrequencyUpdatedEvent, 0),
		CreditsTransferredEvents: make([]issuance.CreditsTransferredEvent, 0),
		BatchSendEvents:          make([]issuance.BatchSendEvent, 0),
	}
}

func (m *MockEventBus) EmitHumanRegistered(event issuance.HumanRegisteredEvent) {
	m.HumanRegisteredEvents = append(m.HumanRegisteredEvents, event)
}

func (m *MockEventBus) EmitCreditsIssued(event issuance.CreditsIssuedEvent) {
	m.CreditsIssuedEvents = append(m.CreditsIssuedEvents, event)
}

func (m *MockEventBus) EmitFrequencyUpdated(event issuance.FrequencyUpdatedEvent) {
	m.FrequencyUpdatedEvents = append(m.FrequencyUpdatedEvents, event)
}

func (m *MockEventBus) EmitCreditsTransferred(event issuance.CreditsTransferredEvent) {
	m.CreditsTransferredEvents = append(m.CreditsTransferredEvents, event)
}

func (m *MockEventBus) EmitBatchSend(event issuance.BatchSendEvent) {
	m.BatchSendEvents = append(m.BatchSendEvents, event)
}

// ── Helpers ────────────────────────────────────────────────────────────────────

const testAddr1 = "cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9"
const testAddr2 = "cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqg7f8d8"
const testAddr3 = "cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqp8n9e7"

// unknownAddr is well formed but never registered in these tests.
const unknownAddr = "cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqzzzzzz"
const testUUID1 = "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b0"
const testUUID2 = "b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1bc00"
const testUUID3 = "c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1bcd000"

// ── Test: MsgRegisterHuman flow via keeper ─────────────────────────────────────

func TestKeeper_RegisterHuman_Success(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	event, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	assert.Equal(t, testAddr1, event.Address)
	assert.Equal(t, testUUID1, event.IdentityUUID)
	assert.Equal(t, "kyc", event.IdentityTier)
	assert.Equal(t, issuance.FrequencyMonthly, event.PayoutFrequency)

	// Verify event was emitted.
	require.Len(t, bus.HumanRegisteredEvents, 1)
	assert.Equal(t, testAddr1, bus.HumanRegisteredEvents[0].Address)

	// Verify account exists.
	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	assert.Equal(t, testAddr1, acct.Address)
	assert.Equal(t, testUUID1, acct.IdentityUUID)
	assert.Equal(t, "kyc", acct.IdentityTier)
	assert.Equal(t, issuance.FrequencyMonthly, acct.PayoutFrequency)
	assert.True(t, acct.YearCreditsReceived.IsZero())
	assert.True(t, acct.Balance.IsZero())
	assert.Equal(t, int64(1)+issuance.BlocksPerPayout(issuance.FrequencyMonthly), acct.NextDueAt,
		"next_due_at should be genesis + blocksPerPayout")
}

func TestKeeper_RegisterHuman_DuplicateUUID(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.RegisterHuman(ctx, testAddr2, testUUID1, "kyc", issuance.FrequencyMonthly)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestKeeper_RegisterHuman_DuplicateAddress(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.RegisterHuman(ctx, testAddr1, testUUID2, "kyc", issuance.FrequencyMonthly)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestKeeper_RegisterHuman_InvalidTier(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "superadmin", issuance.FrequencyMonthly)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
}

func TestKeeper_RegisterHuman_InvalidFrequency(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", "daily")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
}

func TestKeeper_RegisterHuman_AllFrequencies(t *testing.T) {
	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyWeekly,
		issuance.FrequencyBiweekly,
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}
	for _, freq := range freqs {
		t.Run(string(freq), func(t *testing.T) {
			k := issuance.NewKeeper()
			ctx := context.Background()
			event, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", freq)
			require.NoError(t, err)
			assert.Equal(t, freq, event.PayoutFrequency)

			acct := k.GetAccount(testAddr1)
			require.NotNil(t, acct)
			assert.Equal(t, freq, acct.PayoutFrequency)
			// NextDueAt = blockHeight (0) + BlocksPerPayout(freq)
			expectedNextDue := k.GetBlockHeight() + issuance.BlocksPerPayout(freq)
			assert.Equal(t, expectedNextDue, acct.NextDueAt,
				"next_due_at should be blockHeight + blocksPerPayout for %s", freq)
		})
	}
}

func TestKeeper_RegisterHuman_AllTiers(t *testing.T) {
	tiers := []string{"kyc", "biometric", "dna"}
	for _, tier := range tiers {
		t.Run(tier, func(t *testing.T) {
			k := issuance.NewKeeper()
			ctx := context.Background()
			event, err := k.RegisterHuman(ctx, testAddr1, testUUID1, tier, issuance.FrequencyMonthly)
			require.NoError(t, err)
			assert.Equal(t, tier, event.IdentityTier)

			acct := k.GetAccount(testAddr1)
			require.NotNil(t, acct)
			assert.Equal(t, tier, acct.IdentityTier)
		})
	}
}

// ── Test: ProcessPayouts basic scheduling ───────────────────────────────────────

func TestProcessPayouts_Monthly_FullYear(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerMonth := blocksPerYear / 12

	events := make([]issuance.CreditsIssuedEvent, 0)

	// Simulate a full year: advance block height by blocksPerMonth each time.
	for month := int64(0); month < 12; month++ {
		blockHeight := int64(1) + (month+1)*blocksPerMonth
		k.SetBlockHeight(blockHeight)
		payouts := k.ProcessPayouts()
		events = append(events, payouts...)
	}

	// Should have received 12 payouts.
	require.Len(t, events, 12, "should have 12 monthly payouts")

	// Verify total received equals annual allocation.
	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	annual := issuance.AnnualAllocation()
	assert.True(t, acct.YearCreditsReceived.Equal(annual),
		"year credits %s should equal annual allocation %s after 12 monthly payouts",
		acct.YearCreditsReceived, annual)
	// Balance should equal year credits received (no transfers).
	assert.True(t, acct.Balance.Equal(annual),
		"balance %s should equal annual allocation %s",
		acct.Balance, annual)
}

func TestProcessPayouts_Weekly_FullYear(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyWeekly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerWeek := blocksPerYear / 52

	events := make([]issuance.CreditsIssuedEvent, 0)

	// Simulate a full year: process payouts at each weekly boundary.
	for week := int64(0); week < 52; week++ {
		blockHeight := 1 + week*blocksPerWeek
		k.SetBlockHeight(blockHeight)
		payouts := k.ProcessPayouts()
		events = append(events, payouts...)
	}

	// Verify total received equals annual allocation (within dust).
	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	annual := issuance.AnnualAllocation()
	assert.True(t, acct.YearCreditsReceived.LTE(annual),
		"year credits %s should not exceed annual allocation %s",
		acct.YearCreditsReceived, annual)

	// Balance should equal year credits.
	assert.True(t, acct.Balance.Equal(acct.YearCreditsReceived),
		"balance %s should equal year credits %s",
		acct.Balance, acct.YearCreditsReceived)
}

func TestProcessPayouts_Quarterly_FullYear(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyQuarterly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerQuarter := blocksPerYear / 4

	events := make([]issuance.CreditsIssuedEvent, 0)

	for quarter := int64(0); quarter < 4; quarter++ {
		blockHeight := (quarter + 1) * blocksPerQuarter
		k.SetBlockHeight(blockHeight)
		payouts := k.ProcessPayouts()
		events = append(events, payouts...)
	}

	require.Len(t, events, 4, "should have 4 quarterly payouts")

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	annual := issuance.AnnualAllocation()
	assert.True(t, acct.YearCreditsReceived.Equal(annual),
		"year credits %s should equal annual allocation %s after 4 quarterly payouts",
		acct.YearCreditsReceived, annual)
}

func TestProcessPayouts_Annual_FullYear(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyAnnual)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear

	// First payout at the end of the year.
	k.SetBlockHeight(1 + blocksPerYear)
	payouts := k.ProcessPayouts()
	require.Len(t, payouts, 1, "annual frequency: 1 payout per year")

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	annual := issuance.AnnualAllocation()
	assert.True(t, acct.YearCreditsReceived.Equal(annual),
		"year credits %s should equal annual allocation %s",
		acct.YearCreditsReceived, annual)
}

func TestProcessPayouts_Biweekly_FullYear(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyBiweekly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerBiweek := blocksPerYear / 26

	events := make([]issuance.CreditsIssuedEvent, 0)

	for period := int64(0); period < 26; period++ {
		blockHeight := 1 + period*blocksPerBiweek
		k.SetBlockHeight(blockHeight)
		payouts := k.ProcessPayouts()
		events = append(events, payouts...)
	}

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	annual := issuance.AnnualAllocation()
	assert.True(t, acct.YearCreditsReceived.LTE(annual),
		"year credits %s should not exceed annual allocation %s",
		acct.YearCreditsReceived, annual)
}

// ── Test: Cap enforcement ───────────────────────────────────────────────────────

func TestProcessPayouts_CapEnforcement_Weekly(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyWeekly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerWeek := blocksPerYear / 52

	annual := issuance.AnnualAllocation()

	for week := int64(0); week < 104; week++ {
		blockHeight := 1 + week*blocksPerWeek
		k.SetBlockHeight(blockHeight)
		k.ProcessPayouts()
	}

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)

	assert.True(t, acct.YearCreditsReceived.LTE(annual),
		"year credits %s must be <= annual allocation %s even after excessive payouts",
		acct.YearCreditsReceived, annual)

	msg, broken := k.CheckInvariant(ctx)
	assert.False(t, broken, "invariant must hold: %s", msg)
}

func TestProcessPayouts_CapEnforcement_Monthly(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerMonth := blocksPerYear / 12

	annual := issuance.AnnualAllocation()

	for month := int64(0); month < 100; month++ {
		blockHeight := 1 + month*blocksPerMonth
		k.SetBlockHeight(blockHeight)
		k.ProcessPayouts()
	}

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)

	assert.True(t, acct.YearCreditsReceived.LTE(annual),
		"year credits %s must be <= annual allocation %s",
		acct.YearCreditsReceived, annual)
}

// ── Test: Multiple accounts, mix of frequencies ─────────────────────────────────

func TestProcessPayouts_MultipleAccounts_MixedFrequencies(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyWeekly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr3, testUUID3, "kyc", issuance.FrequencyQuarterly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerMonth := blocksPerYear / 12

	annual := issuance.AnnualAllocation()

	for month := int64(0); month < 12; month++ {
		blockHeight := 1 + month*blocksPerMonth
		k.SetBlockHeight(blockHeight)
		k.ProcessPayouts()

		msg, broken := k.CheckInvariant(ctx)
		assert.False(t, broken, "invariant broken at block %d: %s", blockHeight, msg)
	}

	for _, addr := range []string{testAddr1, testAddr2, testAddr3} {
		acct := k.GetAccount(addr)
		require.NotNil(t, acct)
		assert.True(t, acct.YearCreditsReceived.LTE(annual),
			"account %s: year credits %s must not exceed %s",
			addr, acct.YearCreditsReceived, annual)
	}

	maxExpected := annual.MulRaw(3)
	totalSupply := k.GetSupply()
	assert.True(t, totalSupply.LTE(maxExpected),
		"total supply %s must not exceed %s for 3 humans",
		totalSupply, maxExpected)
}

// ── Test: Event emission during payouts ─────────────────────────────────────────

func TestProcessPayouts_EventEmission(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerMonth := blocksPerYear / 12

	for month := int64(0); month < 3; month++ {
		blockHeight := int64(1) + (month+1)*blocksPerMonth
		k.SetBlockHeight(blockHeight)
		k.ProcessPayouts()
	}

	require.Len(t, bus.CreditsIssuedEvents, 3)

	for i := 0; i < 3; i++ {
		event := bus.CreditsIssuedEvents[i]
		assert.Equal(t, testAddr1, event.Address)
		assert.Equal(t, int64(i+1), event.PayoutNumber, "payout number should increment")
		assert.Equal(t, int64(1), event.Year, "first year")
	}
}

func TestProcessPayouts_EventNoPaymentWhenCapped(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyAnnual)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear

	k.SetBlockHeight(1 + blocksPerYear - 1)
	k.ProcessPayouts()
	require.Len(t, bus.CreditsIssuedEvents, 0, "not enough blocks yet")

	k.SetBlockHeight(1 + blocksPerYear)
	payouts := k.ProcessPayouts()
	require.Len(t, payouts, 1, "should have 1 payout at year end")
	require.Len(t, bus.CreditsIssuedEvents, 1, "event should be emitted")

	k.SetBlockHeight(1 + blocksPerYear + blocksPerYear/2)
	payouts = k.ProcessPayouts()
	require.Len(t, payouts, 0, "no more payouts — cap reached")
	require.Len(t, bus.CreditsIssuedEvents, 1, "no additional event")
}

// ── Test: BeginBlock with year transition ───────────────────────────────────────

func TestBeginBlock_YearTransition(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)

	blocksPerYear := issuance.BlocksPerYear

	ctx := context.Background()
	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	events := k.BeginBlock(100)
	assert.Equal(t, int64(1), k.GetCurrentYear(), "still year 1")
	assert.Len(t, events, 0, "no payout at block 100 (before first monthly boundary)")

	blocksPerMonth := blocksPerYear / 12
	for month := int64(0); month < 11; month++ {
		blockHeight := int64(1) + (month+1)*blocksPerMonth
		events := k.BeginBlock(blockHeight)
		assert.Equal(t, int64(1), k.GetCurrentYear(), "still year 1 at month %d", month)
		assert.Len(t, events, 1, "monthly payout at block %d", blockHeight)
	}

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	perPayout := issuance.PayoutAmount(issuance.FrequencyMonthly)
	expected11Months := perPayout.MulRaw(11)
	assert.True(t, acct.YearCreditsReceived.Equal(expected11Months),
		"year credits %s should equal 11 monthly payouts %s",
		acct.YearCreditsReceived, expected11Months)

	yearEndBlock := int64(1) + blocksPerYear
	events = k.BeginBlock(yearEndBlock)
	assert.Equal(t, int64(2), k.GetCurrentYear(), "should be year 2 after burn block")

	require.Len(t, events, 1, "12th monthly payout at year-end block")
	assert.Equal(t, int64(1), events[0].Year, "payout was in year 1")
	assert.Equal(t, int64(12), events[0].PayoutNumber, "12th payout")

	acct = k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	assert.True(t, acct.YearCreditsReceived.IsZero(), "year credits reset to 0 at year transition")
}

func TestBeginBlock_MultipleYears(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)

	blocksPerYear := issuance.BlocksPerYear

	ctx := context.Background()
	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyAnnual)
	require.NoError(t, err)

	year1End := 1 + blocksPerYear
	events := k.BeginBlock(year1End)
	require.Len(t, events, 1, "annual payout at year 1 end")
	assert.Equal(t, int64(1), events[0].Year)
	assert.Equal(t, int64(1), events[0].PayoutNumber)

	year2End := 1 + 2*blocksPerYear
	events = k.BeginBlock(year2End)
	require.Len(t, events, 1, "annual payout at year 2 end")
	assert.Equal(t, int64(2), events[0].Year)
	assert.Equal(t, int64(1), events[0].PayoutNumber, "payout number resets each year")

	year3End := 1 + 3*blocksPerYear
	events = k.BeginBlock(year3End)
	require.Len(t, events, 1, "annual payout at year 3 end")
	assert.Equal(t, int64(3), events[0].Year)
}

// ── Test: GetAccount and state queries ──────────────────────────────────────────

func TestGetAccount_NotFound(t *testing.T) {
	k := issuance.NewKeeper()
	acct := k.GetAccount("nonexistent")
	assert.Nil(t, acct)
}

func TestGetAccount_Found(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	assert.Equal(t, testAddr1, acct.Address)

	acct.Address = "modified"
	original := k.GetAccount(testAddr1)
	assert.Equal(t, testAddr1, original.Address, "original should be unmodified")
}

// ── Test: ResetYearCredits ──────────────────────────────────────────────────────

func TestResetYearCredits(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	blocksPerMonth := blocksPerYear / 12
	k.SetBlockHeight(1 + blocksPerMonth)
	k.ProcessPayouts()

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	assert.False(t, acct.YearCreditsReceived.IsZero(), "should have received credits")

	k.ResetYearCredits()
	acct = k.GetAccount(testAddr1)
	assert.True(t, acct.YearCreditsReceived.IsZero(), "year credits should be 0 after reset")
}

func TestResetYearCredits_MultipleAccounts(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	blocksPerYear := issuance.BlocksPerYear
	k.SetBlockHeight(1 + blocksPerYear/12)
	k.ProcessPayouts()

	assert.Equal(t, int64(2), k.GetHumanCount())
	assert.False(t, k.GetSupply().IsZero(), "supply should be non-zero")

	k.ResetYearCredits()
	assert.True(t, k.GetSupply().IsZero(), "supply should be 0 after reset")
}

// ── Test: Block height and year tracking ────────────────────────────────────────

func TestSetGetBlockHeight(t *testing.T) {
	k := issuance.NewKeeper()
	assert.Equal(t, int64(0), k.GetBlockHeight())
	k.SetBlockHeight(42)
	assert.Equal(t, int64(42), k.GetBlockHeight())
}

func TestSetGetCurrentYear(t *testing.T) {
	k := issuance.NewKeeper()
	assert.Equal(t, int64(1), k.GetCurrentYear())
	k.SetCurrentYear(5)
	assert.Equal(t, int64(5), k.GetCurrentYear())
}

// ── Test: Edge case — no accounts ───────────────────────────────────────────────

func TestProcessPayouts_NoAccounts(t *testing.T) {
	k := issuance.NewKeeper()
	events := k.ProcessPayouts()
	assert.Empty(t, events)
	assert.Equal(t, int64(0), k.GetHumanCount())
	assert.True(t, k.GetSupply().IsZero())
}

func TestBeginBlock_NoAccounts(t *testing.T) {
	k := issuance.NewKeeperWithEvents(NewMockEventBus(), 1)
	events := k.BeginBlock(1000)
	assert.Empty(t, events)
}

// ── Test: Payout amount correctness ─────────────────────────────────────────────

func TestPayoutAmount_Frequencies(t *testing.T) {
	tests := []struct {
		freq     issuance.PayoutFrequency
		expected int64
	}{
		{issuance.FrequencyWeekly, 12_000_000_000 / 52},
		{issuance.FrequencyBiweekly, 12_000_000_000 / 26},
		{issuance.FrequencyMonthly, 12_000_000_000 / 12},
		{issuance.FrequencyQuarterly, 12_000_000_000 / 4},
		{issuance.FrequencyAnnual, 12_000_000_000},
	}
	for _, tc := range tests {
		t.Run(string(tc.freq), func(t *testing.T) {
			amount := issuance.PayoutAmount(tc.freq)
			assert.Equal(t, math.NewInt(tc.expected), amount)
		})
	}
}

func TestBlocksPerPayout(t *testing.T) {
	blocksPerYear := issuance.BlocksPerYear
	tests := []struct {
		freq     issuance.PayoutFrequency
		expected int64
	}{
		{issuance.FrequencyWeekly, blocksPerYear / 52},
		{issuance.FrequencyBiweekly, blocksPerYear / 26},
		{issuance.FrequencyMonthly, blocksPerYear / 12},
		{issuance.FrequencyQuarterly, blocksPerYear / 4},
		{issuance.FrequencyAnnual, blocksPerYear / 1},
	}
	for _, tc := range tests {
		t.Run(string(tc.freq), func(t *testing.T) {
			bpp := issuance.BlocksPerPayout(tc.freq)
			assert.Equal(t, tc.expected, bpp)
		})
	}
}

// ── NEW TESTS: Payout Scheduler ─────────────────────────────────────────────────

// TestPayoutFrequency_Queryable verifies per-account frequency is stored and queryable.
func TestPayoutFrequency_Queryable(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyWeekly)
	require.NoError(t, err)

	freq := k.GetPayoutFrequency(testAddr1)
	assert.Equal(t, issuance.FrequencyWeekly, freq, "frequency should be queryable")
}

// TestUpdatePayoutFrequency verifies frequency change applies next cycle.
func TestUpdatePayoutFrequency_Success(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	// Change to weekly.
	event, err := k.UpdatePayoutFrequency(testAddr1, issuance.FrequencyWeekly)
	require.NoError(t, err)
	assert.Equal(t, testAddr1, event.Address)
	assert.Equal(t, issuance.FrequencyMonthly, event.OldFrequency)
	assert.Equal(t, issuance.FrequencyWeekly, event.NewFrequency)
	assert.Equal(t, int64(1), event.BlockHeight)

	// Verify frequency changed.
	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	assert.Equal(t, issuance.FrequencyWeekly, acct.PayoutFrequency,
		"frequency should be updated")

	// Verify NextDueAt was recomputed from current block.
	expectedNextDue := int64(1) + issuance.BlocksPerPayout(issuance.FrequencyWeekly)
	assert.Equal(t, expectedNextDue, acct.NextDueAt,
		"next_due_at should be recomputed for new frequency")
}

// TestUpdatePayoutFrequency_AccountNotFound verifies error on unknown account.
func TestUpdatePayoutFrequency_AccountNotFound(t *testing.T) {
	k := issuance.NewKeeper()
	_, err := k.UpdatePayoutFrequency(unknownAddr, issuance.FrequencyMonthly)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestUpdatePayoutFrequency_InvalidFrequency verifies error on invalid frequency.
func TestUpdatePayoutFrequency_InvalidFrequency(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.UpdatePayoutFrequency(testAddr1, "daily")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not valid")
}

// TestUpdatePayoutFrequency_EventEmitted verifies frequency update event is emitted.
func TestUpdatePayoutFrequency_EventEmitted(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.UpdatePayoutFrequency(testAddr1, issuance.FrequencyQuarterly)
	require.NoError(t, err)

	require.Len(t, bus.FrequencyUpdatedEvents, 1)
	event := bus.FrequencyUpdatedEvents[0]
	assert.Equal(t, testAddr1, event.Address)
	assert.Equal(t, issuance.FrequencyMonthly, event.OldFrequency)
	assert.Equal(t, issuance.FrequencyQuarterly, event.NewFrequency)
	assert.Equal(t, int64(1), event.BlockHeight)
	assert.Equal(t, int64(1)+issuance.BlocksPerPayout(issuance.FrequencyQuarterly), event.EffectiveAt)
}

// TestUpdatePayoutFrequency_AllFrequencyVariants verifies all frequency changes work.
func TestUpdatePayoutFrequency_AllFrequencyVariants(t *testing.T) {
	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyWeekly,
		issuance.FrequencyBiweekly,
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}

	for _, oldFreq := range freqs {
		for _, newFreq := range freqs {
			if oldFreq == newFreq {
				continue
			}
			t.Run(string(oldFreq)+"_to_"+string(newFreq), func(t *testing.T) {
				k := issuance.NewKeeper()
				ctx := context.Background()

				_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", oldFreq)
				require.NoError(t, err)

				event, err := k.UpdatePayoutFrequency(testAddr1, newFreq)
				require.NoError(t, err)
				assert.Equal(t, oldFreq, event.OldFrequency)
				assert.Equal(t, newFreq, event.NewFrequency)

				acct := k.GetAccount(testAddr1)
				require.NotNil(t, acct)
				assert.Equal(t, newFreq, acct.PayoutFrequency)
			})
		}
	}
}

// TestSchedulerStateMachine_NextDueAt verifies NextDueAt tracking.
func TestSchedulerStateMachine_NextDueAt(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	blocksPerMonth := issuance.BlocksPerYear / 12

	// NextDueAt = blockHeight (0) + BlocksPerPayout (monthly).
	assert.Equal(t, blocksPerMonth, acct.NextDueAt,
		"next_due_at should be blockHeight + blocksPerPayout")

	// Process a payout at the due block.
	k.SetBlockHeight(blocksPerMonth)
	payouts := k.ProcessPayouts()
	require.Len(t, payouts, 1, "should have 1 payout at due block")

	acct = k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	assert.Equal(t, blocksPerMonth, acct.LastPaidAt,
		"last_paid_at should be the payout block")
	assert.True(t, acct.LastPaidAt+blocksPerMonth >= acct.NextDueAt,
		"next_due_at should advance after payout")

	// Verify GetNextDueAt query.
	nextDue := k.GetNextDueAt(testAddr1)
	assert.Greater(t, nextDue, int64(0), "next_due_at should be > 0")
}

// TestMissedPayoutRecovery verifies catch-up on next available block.
func TestMissedPayoutRecovery(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	blocksPerMonth := issuance.BlocksPerYear / 12

	// Simulate the node being down for 3 months.
	// Jump to block after 3 months have passed.
	k.SetBlockHeight(1 + 3*blocksPerMonth)

	// Process payouts — should catch up 1 missed payout (the one due at month 3).
	payouts := k.ProcessPayouts()
	require.Len(t, payouts, 1,
		"should catch up 1 missed payout, not 3 (no supply shock)")

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	perPayout := issuance.PayoutAmount(issuance.FrequencyMonthly)
	assert.True(t, acct.YearCreditsReceived.Equal(perPayout),
		"should have received 1 missed payout: %s", acct.YearCreditsReceived)
}

// TestMissedPayoutRecovery_LongDowntime verifies longer recovery.
func TestMissedPayoutRecovery_LongDowntime(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyWeekly)
	require.NoError(t, err)

	blocksPerWeek := issuance.BlocksPerYear / 52

	// Node was down for 10 weeks. Jump ahead.
	k.SetBlockHeight(1 + 10*blocksPerWeek)

	// Should catch up 1 missed payout (no supply shock).
	payouts := k.ProcessPayouts()
	require.Len(t, payouts, 1,
		"should catch up 1 missed payout after 10 weeks of downtime")

	acct := k.GetAccount(testAddr1)
	require.NotNil(t, acct)
	assert.True(t, acct.YearCreditsReceived.GT(math.ZeroInt()),
		"should have received a payout")
}

// ── NEW TESTS: Transaction Layer ────────────────────────────────────────────────

// TestSendCredits_Success verifies a basic credits transfer.
func TestSendCredits_Success(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	// Give sender some balance by processing a payout.
	blocksPerMonth := issuance.BlocksPerYear / 12
	k.SetBlockHeight(1 + blocksPerMonth)
	k.ProcessPayouts()

	senderBefore := k.GetBalance(testAddr1)
	recipientBefore := k.GetBalance(testAddr2)

	transferAmount := math.NewInt(100_000_000) // 100 credits
	event, err := k.SendCredits(testAddr1, testAddr2, transferAmount)
	require.NoError(t, err)
	assert.Equal(t, testAddr1, event.From)
	assert.Equal(t, testAddr2, event.To)
	assert.Equal(t, transferAmount.String(), event.Amount)

	// Verify balances.
	senderAfter := k.GetBalance(testAddr1)
	recipientAfter := k.GetBalance(testAddr2)

	assert.True(t, senderAfter.Equal(senderBefore.Sub(transferAmount)),
		"sender balance should decrease by transfer amount")
	assert.True(t, recipientAfter.Equal(recipientBefore.Add(transferAmount)),
		"recipient balance should increase by transfer amount")

	// Verify event was emitted.
	require.Len(t, bus.CreditsTransferredEvents, 1)
	assert.Equal(t, testAddr1, bus.CreditsTransferredEvents[0].From)
	assert.Equal(t, testAddr2, bus.CreditsTransferredEvents[0].To)

	// Verify transfer records.
	assert.Equal(t, 1, k.GetTransferCount())
}

// TestSendCredits_SenderNotFound verifies error on unknown sender.
func TestSendCredits_SenderNotFound(t *testing.T) {
	k := issuance.NewKeeper()

	_, err := k.SendCredits(unknownAddr, testAddr2, math.NewInt(100))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestSendCredits_RecipientNotFound verifies error on unknown recipient.
func TestSendCredits_RecipientNotFound(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.SendCredits(testAddr1, unknownAddr, math.NewInt(100))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be registered")
}

// TestSendCredits_SelfTransfer verifies error on self-transfer.
func TestSendCredits_SelfTransfer(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.SendCredits(testAddr1, testAddr1, math.NewInt(100))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot send credits to self")
}

// TestSendCredits_InsufficientBalance verifies error on insufficient balance.
func TestSendCredits_InsufficientBalance(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	// Sender has 0 balance.
	_, err = k.SendCredits(testAddr1, testAddr2, math.NewInt(1_000_000))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient")
}

// TestSendCredits_RecipientCapEnforcement verifies MaxBalanceUcredit cap.
func TestSendCredits_RecipientCapEnforcement(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	// Manually set sender balance to a large amount.
	k.SetAccountBalance(testAddr1, math.NewInt(36_000_000_000))

	// Set recipient balance near the cap (35,500,000,000 out of 36,000,000,000 max).
	k.SetAccountBalance(testAddr2, math.NewInt(35_500_000_000))

	// Sending 1,000,000,000 ucredit (1 credit) would push recipient to
	// 36,500,000,000 which exceeds the 36,000,000,000 cap.
	_, err = k.SendCredits(testAddr1, testAddr2, math.NewInt(1_000_000_000))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max balance cap")
}

// TestBatchSend_Success verifies a batch send to multiple recipients.
func TestBatchSend_Success(t *testing.T) {
	bus := NewMockEventBus()
	k := issuance.NewKeeperWithEvents(bus, 1)
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr3, testUUID3, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	// Give sender balance.
	blocksPerMonth := issuance.BlocksPerYear / 12
	k.SetBlockHeight(1 + blocksPerMonth)
	k.ProcessPayouts()

	senderBefore := k.GetBalance(testAddr1)

	entries := []issuance.BatchSendEntry{
		{To: testAddr2, Amount: math.NewInt(50_000_000)},
		{To: testAddr3, Amount: math.NewInt(30_000_000)},
	}
	totalAmount := math.NewInt(80_000_000)

	event, err := k.BatchSend(testAddr1, entries)
	require.NoError(t, err)
	assert.Equal(t, testAddr1, event.From)
	assert.Equal(t, totalAmount.String(), event.TotalAmount)
	assert.Len(t, event.Entries, 2)

	// Verify sender balance decreased.
	senderAfter := k.GetBalance(testAddr1)
	assert.True(t, senderAfter.Equal(senderBefore.Sub(totalAmount)),
		"sender balance should decrease by total")

	// Verify recipient balances increased.
	recipient2After := k.GetBalance(testAddr2)
	recipient3After := k.GetBalance(testAddr3)
	assert.True(t, recipient2After.GT(math.ZeroInt()), "recipient 2 should have balance")
	assert.True(t, recipient3After.GT(math.ZeroInt()), "recipient 3 should have balance")

	// Verify event was emitted.
	require.Len(t, bus.BatchSendEvents, 1)
	assert.Equal(t, testAddr1, bus.BatchSendEvents[0].From)

	// Verify transfer records.
	assert.Equal(t, 2, k.GetTransferCount(),
		"batch send should create 2 transfer records")
}

// TestBatchSend_EmptyEntries verifies error on empty batch.
func TestBatchSend_EmptyEntries(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.BatchSend(testAddr1, []issuance.BatchSendEntry{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at least one recipient")
}

// TestBatchSend_DuplicateRecipient verifies duplicate recipient rejection.
func TestBatchSend_DuplicateRecipient(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	// Give sender balance.
	blocksPerMonth := issuance.BlocksPerYear / 12
	k.SetBlockHeight(1 + blocksPerMonth)
	k.ProcessPayouts()

	entries := []issuance.BatchSendEntry{
		{To: testAddr2, Amount: math.NewInt(10_000_000)},
		{To: testAddr2, Amount: math.NewInt(10_000_000)},
	}
	_, err = k.BatchSend(testAddr1, entries)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate recipient")
}

// TestBatchSend_ExceedsMaxRecipients verifies max batch size enforcement.
func TestBatchSend_ExceedsMaxRecipients(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	entries := make([]issuance.BatchSendEntry, issuance.MaxBatchRecipients+1)
	for i := range entries {
		entries[i] = issuance.BatchSendEntry{
			To:     testAddr2,
			Amount: math.NewInt(1),
		}
	}
	_, err = k.BatchSend(testAddr1, entries)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum")
}

// TestSendCredits_ZeroAmount verifies zero amount is rejected.
func TestSendCredits_ZeroAmount(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.SendCredits(testAddr1, testAddr2, math.ZeroInt())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}

// TestSendCredits_NegativeAmount verifies negative amount is rejected.
func TestSendCredits_NegativeAmount(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.SendCredits(testAddr1, testAddr2, math.NewInt(-100))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}

// TestBalanceInvariant verifies no account exceeds MaxBalanceUcredit.
func TestBalanceInvariant(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	msg, broken := k.CheckBalanceInvariant(ctx)
	assert.False(t, broken, "balance invariant should hold: %s", msg)
}

// TestGetTotalBalance verifies total balance tracking.
func TestGetTotalBalance(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()

	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	blocksPerMonth := issuance.BlocksPerYear / 12
	k.SetBlockHeight(1 + blocksPerMonth)
	k.ProcessPayouts()

	totalBal := k.GetTotalBalance()
	assert.True(t, totalBal.GT(math.ZeroInt()), "total balance should be > 0")
	assert.Equal(t, k.GetBalance(testAddr1), totalBal, "total balance should equal single account")
}

// TestFeePolicy_Zero verifies fee policy is documented as Option A.
func TestFeePolicy_Zero(t *testing.T) {
	// The FeePolicy constant is compile-time and immutable.
	assert.Equal(t, issuance.FeePolicy("zero"), issuance.FeeZero,
		"fee policy must be zero (Option A: pure credit system)")
}
// Malformed addresses are refused by format before any lookup, so the error says what is wrong.
func TestSendCredits_MalformedAddressesAreRefusedByFormat(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()
	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	_, err = k.SendCredits("nonexistent", testAddr2, math.NewInt(100))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid bech32 address")

	_, err = k.SendCredits(testAddr1, "not-an-address", math.NewInt(100))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid bech32 address")

	_, err = k.BatchSend("nonexistent", []issuance.BatchSendEntry{{To: testAddr2, Amount: math.NewInt(100)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid bech32 address")

	_, err = k.BatchSend(testAddr1, []issuance.BatchSendEntry{{To: "bad", Amount: math.NewInt(100)}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entry 0")
	assert.Contains(t, err.Error(), "not a valid bech32 address")

	_, err = k.UpdatePayoutFrequency("bad", issuance.FrequencyWeekly)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a valid bech32 address")
}

// An unset amount is refused, not dereferenced.
func TestSendCredits_UnsetAmountIsRefused(t *testing.T) {
	k := issuance.NewKeeper()
	ctx := context.Background()
	_, err := k.RegisterHuman(ctx, testAddr1, testUUID1, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)
	_, err = k.RegisterHuman(ctx, testAddr2, testUUID2, "kyc", issuance.FrequencyMonthly)
	require.NoError(t, err)

	var unset math.Int
	assert.NotPanics(t, func() {
		_, err = k.SendCredits(testAddr1, testAddr2, unset)
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")

	assert.NotPanics(t, func() {
		_, err = k.BatchSend(testAddr1, []issuance.BatchSendEntry{{To: testAddr2, Amount: unset}})
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be positive")
}
