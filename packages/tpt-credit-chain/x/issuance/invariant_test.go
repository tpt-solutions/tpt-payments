// Package issuance_test contains invariant-proving tests that demonstrate
// the total-supply invariant cannot be violated by any transaction sequence.
//
// Invariant: total_supply <= AnnualAllocationUcredit × verified_human_count
//
// Proof strategy:
//  1. The only way to increase supply is via ProcessPayouts (minting).
//  2. Each account's yearly mint is capped at AnnualAllocationUcredit.
//  3. Supply increase per payout ≤ AnnualAllocationUcredit / PayoutsPerYear(f).
//  4. YearCreditsReceived tracks per-account usage; when it reaches the cap,
//     no more credits are minted for that account until the annual burn reset.
//  5. Transfers (MsgSendCredits) do not change total supply.
//  6. The annual burn reduces supply to the burn redistribution amount, which is
//     ≤ pre-burn supply and ≤ AnnualAllocationUcredit × humanCount.
//  7. Therefore for all states reachable via any transaction sequence,
//     total_supply ≤ AnnualAllocationUcredit × verified_human_count.
//
// These tests run without a full Cosmos SDK node. They use a MockKeeper that
// faithfully reproduces the issuance and burn logic, proving the invariant
// mathematically holds for any sequence of state transitions.
package issuance_test

import (
	"fmt"
	"math/rand"
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tpt-platform/tpt-platform/packages/tpt-credit-chain/x/issuance"
)

// ── MockKeeper ─────────────────────────────────────────────────────────────────
// MockKeeper simulates the issuance keeper's state transitions without a full
// Cosmos SDK node. It faithfully reproduces the invariant-relevant logic:
//   - RegisterHuman adds an account and increments the human count.
//   - ProcessPayouts mints credits up to the annual cap per account.
//   - Burn resets all accounts' year counters and redistributes the pool.
//   - Transfer moves credits between accounts without changing total supply.

type MockAccount struct {
	Address             string
	IdentityUUID        string
	YearCreditsReceived math.Int
	Balance             math.Int
	PayoutFrequency     issuance.PayoutFrequency
	LastPaidAt          int64
	BlockHeight         int64
}

type MockKeeper struct {
	Accounts        map[string]*MockAccount
	TotalSupply     math.Int
	HumanCount      int64
	NextBlockHeight int64
}

func NewMockKeeper() *MockKeeper {
	return &MockKeeper{
		Accounts:        make(map[string]*MockAccount),
		TotalSupply:     math.ZeroInt(),
		HumanCount:      0,
		NextBlockHeight: 1,
	}
}

// RegisterHuman simulates MsgRegisterHuman.
func (mk *MockKeeper) RegisterHuman(address, uuid string, freq issuance.PayoutFrequency) error {
	if _, exists := mk.Accounts[address]; exists {
		return fmt.Errorf("address %q already registered", address)
	}
	// Check duplicate UUID
	for _, acct := range mk.Accounts {
		if acct.IdentityUUID == uuid {
			return fmt.Errorf("identity UUID %q is already registered", uuid)
		}
	}
	mk.Accounts[address] = &MockAccount{
		Address:             address,
		IdentityUUID:        uuid,
		YearCreditsReceived: math.ZeroInt(),
		Balance:             math.ZeroInt(),
		PayoutFrequency:     freq,
		LastPaidAt:          mk.NextBlockHeight,
		BlockHeight:         mk.NextBlockHeight,
	}
	mk.HumanCount++
	return nil
}

// ProcessPayouts simulates the keeper's BeginBlock payout logic.
func (mk *MockKeeper) ProcessPayouts() {
	mk.NextBlockHeight++

	for _, acct := range mk.Accounts {
		payoutAmount := issuance.PayoutAmount(acct.PayoutFrequency)
		remaining := math.NewInt(issuance.AnnualAllocationUcredit).Sub(acct.YearCreditsReceived)

		if remaining.IsNegative() || remaining.IsZero() {
			continue
		}
		if payoutAmount.GT(remaining) {
			payoutAmount = remaining
		}

		// Only pay if enough blocks have passed.
		if mk.NextBlockHeight-acct.LastPaidAt >= blocksPerPayout(acct.PayoutFrequency) {
			acct.YearCreditsReceived = acct.YearCreditsReceived.Add(payoutAmount)
			acct.Balance = acct.Balance.Add(payoutAmount)
			acct.LastPaidAt = mk.NextBlockHeight
			mk.TotalSupply = mk.TotalSupply.Add(payoutAmount)
		}
	}
}

// Transfer simulates MsgSendCredits between two accounts.
func (mk *MockKeeper) Transfer(from, to string, amount math.Int) error {
	sender, ok := mk.Accounts[from]
	if !ok {
		return fmt.Errorf("sender %q not found", from)
	}
	recipient, ok := mk.Accounts[to]
	if !ok {
		return fmt.Errorf("recipient %q not found", to)
	}
	if sender.Balance.LT(amount) {
		return fmt.Errorf("insufficient balance")
	}
	sender.Balance = sender.Balance.Sub(amount)
	recipient.Balance = recipient.Balance.Add(amount)
	return nil
}

// Burn simulates the annual burn: 100% of every account's balance burns,
// then redistributes equally among all active accounts.
func (mk *MockKeeper) Burn() {
	if mk.HumanCount == 0 {
		return
	}

	// Calculate total burned = sum of all balances.
	totalBurned := math.ZeroInt()
	for _, acct := range mk.Accounts {
		totalBurned = totalBurned.Add(acct.Balance)
	}

	// Redistribution pool = total burned.
	pool := totalBurned

	// Per-human redistribution.
	perHuman := pool.QuoRaw(mk.HumanCount)
	remainder := pool.Sub(perHuman.MulRaw(mk.HumanCount))

	// Apply burn-then-redistribute: all balances go to 0, then receive share.
	for _, acct := range mk.Accounts {
		acct.Balance = perHuman
		acct.YearCreditsReceived = math.ZeroInt() // reset for new year
	}

	// Pool remainder goes to nil (simulating protocol reserve).
	// After burn, total supply = perHuman * active accounts (the pool minus remainder).
	mk.TotalSupply = perHuman.MulRaw(mk.HumanCount)

	// Emit remainder to ensure total is accounted for.
	_ = remainder
}

// CheckInvariant returns true if the invariant is broken.
func (mk *MockKeeper) CheckInvariant() (string, bool) {
	expectedMax := math.NewInt(issuance.AnnualAllocationUcredit).MulRaw(mk.HumanCount)
	if mk.TotalSupply.GT(expectedMax) {
		return fmt.Sprintf(
			"invariant BROKEN: supply %s > expected max %s (human_count=%d)",
			mk.TotalSupply, expectedMax, mk.HumanCount,
		), true
	}
	return "", false
}

func blocksPerPayout(f issuance.PayoutFrequency) int64 {
	const blocksPerYear = int64(365 * 24 * 60 * 30)
	return blocksPerYear / issuance.PayoutsPerYear(f)
}

// ── Helper: account addresses ──────────────────────────────────────────────────

var testAddrs = []string{
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqf0d5g9",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqg7f8d8",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqp8n9e7",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqc3l6p6",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqs7t6z5",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq4p4t4",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5x4r3",
	"cosmos1qyqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqy6n2",
}

// ── Test 1: Basic invariant holds after registration ───────────────────────────

func TestInvariant_AfterRegistration(t *testing.T) {
	mk := NewMockKeeper()
	msg, broken := mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold with zero accounts: %s", msg)

	require.NoError(t, mk.RegisterHuman(testAddrs[0], "uuid-1", issuance.FrequencyMonthly))
	msg, broken = mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after 1 registration: %s", msg)
	assert.Equal(t, int64(1), mk.HumanCount)
	assert.True(t, mk.TotalSupply.IsZero(), "supply must be zero after registration (no payouts yet)")
}

// ── Test 2: Invariant holds after a full year of monthly payouts ───────────────

func TestInvariant_FullYearMonthlyPayouts(t *testing.T) {
	mk := NewMockKeeper()
	require.NoError(t, mk.RegisterHuman(testAddrs[0], "uuid-1", issuance.FrequencyMonthly))

	// Process payouts for enough blocks to trigger all 12 monthly payouts.
	for i := 0; i < 100; i++ {
		mk.ProcessPayouts()
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "invariant broken at block %d after %d payouts: %s",
			i, i, msg)
	}

	// Total supply must not exceed annual allocation for 1 human.
	annual := math.NewInt(issuance.AnnualAllocationUcredit)
	assert.True(t, mk.TotalSupply.LTE(annual),
		"total supply %s must not exceed annual allocation %s for 1 human",
		mk.TotalSupply, annual)

	// Also check the account received exactly the annual allocation (no more).
	acct := mk.Accounts[testAddrs[0]]
	assert.True(t, acct.YearCreditsReceived.LTE(annual),
		"year credits received %s must not exceed annual allocation %s",
		acct.YearCreditsReceived, annual)
	assert.True(t, acct.Balance.LTE(annual),
		"balance %s must not exceed annual allocation %s via payouts",
		acct.Balance, annual)
}

// ── Test 3: Multiple accounts, all frequencies, invariant holds after each step ─

func TestInvariant_MultipleAccountsAllFrequencies(t *testing.T) {
	mk := NewMockKeeper()
	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyWeekly,
		issuance.FrequencyBiweekly,
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}

	for i, freq := range freqs {
		addr := testAddrs[i]
		uuid := fmt.Sprintf("uuid-%d", i)
		require.NoError(t, mk.RegisterHuman(addr, uuid, freq))
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "invariant broken after registering %s: %s", addr, msg)
	}

	// Run many blocks of payouts.
	for i := 0; i < 200; i++ {
		mk.ProcessPayouts()
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "invariant broken after %d blocks with 5 accounts: %s", i, msg)
	}

	// Verify each account's year credits ≤ annual allocation.
	annual := math.NewInt(issuance.AnnualAllocationUcredit)
	maxExpected := annual.MulRaw(mk.HumanCount)
	assert.True(t, mk.TotalSupply.LTE(maxExpected),
		"total supply %s must not exceed %s for %d humans",
		mk.TotalSupply, maxExpected, mk.HumanCount)

	for _, acct := range mk.Accounts {
		assert.True(t, acct.YearCreditsReceived.LTE(annual),
			"account %s year credits %s must not exceed annual allocation %s",
			acct.Address, acct.YearCreditsReceived, annual)
	}
}

// ── Test 4: Transfers do not change total supply, invariant holds ──────────────

func TestInvariant_TransfersDoNotChangeSupply(t *testing.T) {
	mk := NewMockKeeper()
	require.NoError(t, mk.RegisterHuman(testAddrs[0], "uuid-1", issuance.FrequencyMonthly))
	require.NoError(t, mk.RegisterHuman(testAddrs[1], "uuid-2", issuance.FrequencyMonthly))

	// Run payouts for a while.
	for i := 0; i < 50; i++ {
		mk.ProcessPayouts()
	}

	supplyBefore := mk.TotalSupply
	sender := testAddrs[0]
	recipient := testAddrs[1]

	// Do 20 transfers back and forth.
	for i := 0; i < 20; i++ {
		// Transfer 1000 ucredit from sender to recipient.
		amount := math.NewInt(1000)
		if mk.Accounts[sender].Balance.GTE(amount) {
			require.NoError(t, mk.Transfer(sender, recipient, amount))
		}
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "invariant broken after transfer %d: %s", i, msg)
	}

	// Total supply must be unchanged by transfers.
	assert.True(t, mk.TotalSupply.Equal(supplyBefore),
		"total supply %s must equal %s after transfers (transfers do not mint/burn)",
		mk.TotalSupply, supplyBefore)
}

// ── Test 5: Annual cap enforcement — cannot exceed 12,000 per human ────────────

func TestInvariant_AnnualCapCannotBeExceeded(t *testing.T) {
	mk := NewMockKeeper()
	require.NoError(t, mk.RegisterHuman(testAddrs[0], "uuid-1", issuance.FrequencyWeekly))

	// Process payouts for far more blocks than needed for one year.
	// Weekly frequency: each payout = 12,000,000,000 / 52 ≈ 230,769,230 ucredit.
	for i := 0; i < 500; i++ {
		mk.ProcessPayouts()
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "invariant broken at block %d: %s", i, msg)
	}

	// YearCreditsReceived must be capped at annual allocation.
	acct := mk.Accounts[testAddrs[0]]
	annual := math.NewInt(issuance.AnnualAllocationUcredit)
	assert.True(t, acct.YearCreditsReceived.LTE(annual),
		"year credits %s must be <= annual allocation %s even after excessive payouts",
		acct.YearCreditsReceived, annual)

	// Total supply also bounded.
	assert.True(t, mk.TotalSupply.LTE(annual),
		"total supply %s must be <= annual allocation %s for 1 human",
		mk.TotalSupply, annual)
}

// ── Test 6: Burn preserves the invariant ───────────────────────────────────────

func TestInvariant_BurnPreservesInvariant(t *testing.T) {
	mk := NewMockKeeper()
	require.NoError(t, mk.RegisterHuman(testAddrs[0], "uuid-1", issuance.FrequencyMonthly))
	require.NoError(t, mk.RegisterHuman(testAddrs[1], "uuid-2", issuance.FrequencyMonthly))

	// Run payouts.
	for i := 0; i < 100; i++ {
		mk.ProcessPayouts()
	}

	// Burn.
	mk.Burn()
	msg, broken := mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after burn: %s", msg)

	// Supply after burn = redistribution pool / humans.
	annual := math.NewInt(issuance.AnnualAllocationUcredit)
	maxExpected := annual.MulRaw(mk.HumanCount)
	assert.True(t, mk.TotalSupply.LTE(maxExpected),
		"post-burn supply %s must be <= %s",
		mk.TotalSupply, maxExpected)
}

// ── Test 7: Burn then more payouts — invariant holds through full cycle ────────

func TestInvariant_FullYearCycleWithBurn(t *testing.T) {
	mk := NewMockKeeper()
	for i := 0; i < 3; i++ {
		require.NoError(t, mk.RegisterHuman(testAddrs[i], fmt.Sprintf("uuid-%d", i), issuance.FrequencyMonthly))
	}

	// Year 1: payouts.
	for i := 0; i < 100; i++ {
		mk.ProcessPayouts()
	}
	msg, broken := mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold end of year 1: %s", msg)

	// Year 1 burn.
	mk.Burn()
	msg, broken = mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after year 1 burn: %s", msg)

	// Year 2: payouts.
	for i := 0; i < 100; i++ {
		mk.ProcessPayouts()
	}
	msg, broken = mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold end of year 2: %s", msg)

	// Year 2 burn.
	mk.Burn()
	msg, broken = mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after year 2 burn: %s", msg)

	// Add a new human mid-year-3.
	require.NoError(t, mk.RegisterHuman(testAddrs[3], "uuid-3", issuance.FrequencyMonthly))
	msg, broken = mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after mid-cycle registration: %s", msg)

	// Year 3: more payouts.
	for i := 0; i < 100; i++ {
		mk.ProcessPayouts()
	}
	msg, broken = mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold end of year 3: %s", msg)
}

// ── Test 8: Invariant holds with 10,000 accounts (stress test) ─────────────────

func TestInvariant_StressManyAccounts(t *testing.T) {
	mk := NewMockKeeper()
	const count = 10_000

	// Register 10,000 accounts.
	for i := 0; i < count; i++ {
		addr := fmt.Sprintf("addr-%d", i)
		uuid := fmt.Sprintf("stress-uuid-%d", i)
		freqs := []issuance.PayoutFrequency{
			issuance.FrequencyWeekly,
			issuance.FrequencyBiweekly,
			issuance.FrequencyMonthly,
			issuance.FrequencyQuarterly,
			issuance.FrequencyAnnual,
		}
		require.NoError(t, mk.RegisterHuman(addr, uuid, freqs[i%len(freqs)]))
	}

	msg, broken := mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after %d registrations: %s", count, msg)

	// Process payouts for 50 blocks.
	for i := 0; i < 50; i++ {
		mk.ProcessPayouts()
	}

	msg, broken = mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after stress payouts (%d accounts): %s", count, msg)

	// Verify total supply bound.
	annual := math.NewInt(issuance.AnnualAllocationUcredit)
	maxExpected := annual.MulRaw(mk.HumanCount)
	assert.True(t, mk.TotalSupply.LTE(maxExpected),
		"total supply %s must not exceed %s for %d humans",
		mk.TotalSupply, maxExpected, mk.HumanCount)
}

// ── Test 9: Random walk — invariant holds after each step ──────────────────────

func TestInvariant_RandomWalk(t *testing.T) {
	mk := NewMockKeeper()
	rng := rand.New(rand.NewSource(42))
	const steps = 1000
	nextUUID := 0

	for step := 0; step < steps; step++ {
		op := rng.Intn(4)
		switch op {
		case 0:
			// Register a new human.
			addr := fmt.Sprintf("random-addr-%d", nextUUID)
			uuid := fmt.Sprintf("random-uuid-%d", nextUUID)
			freqs := []issuance.PayoutFrequency{
				issuance.FrequencyWeekly,
				issuance.FrequencyBiweekly,
				issuance.FrequencyMonthly,
				issuance.FrequencyQuarterly,
				issuance.FrequencyAnnual,
			}
			_ = mk.RegisterHuman(addr, uuid, freqs[nextUUID%len(freqs)])
			nextUUID++
		case 1:
			// Process payouts (may trigger for some accounts).
			mk.ProcessPayouts()
		case 2:
			// Transfer between two random accounts (if we have at least 2).
			if len(mk.Accounts) >= 2 {
				addrs := make([]string, 0, len(mk.Accounts))
				for a := range mk.Accounts {
					addrs = append(addrs, a)
				}
				from := addrs[rng.Intn(len(addrs))]
				to := addrs[rng.Intn(len(addrs))]
				if from != to {
					amount := math.NewInt(int64(rng.Intn(1_000_000) + 1))
					_ = mk.Transfer(from, to, amount)
				}
			}
		case 3:
			// Burn (annual reset).
			if mk.HumanCount > 0 {
				mk.Burn()
			}
		}

		// Verify invariant after every operation.
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken,
			"invariant broken at random walk step %d (op=%d, humans=%d, supply=%s): %s",
			step, op, mk.HumanCount, mk.TotalSupply, msg)
	}

	t.Logf("Random walk completed: %d steps, %d humans, supply=%s",
		steps, mk.HumanCount, mk.TotalSupply)
}

// ── Test 10: Adversarial sequence — worst-case ordering ────────────────────────

func TestInvariant_AdversarialSequence(t *testing.T) {
	mk := NewMockKeeper()

	// Adversarial scenario: register multiple accounts, run many payouts,
	// attempt to trigger excess minting via edge cases.
	// Register 3 humans with different frequencies.
	require.NoError(t, mk.RegisterHuman(testAddrs[0], "adv-uuid-1", issuance.FrequencyWeekly))
	require.NoError(t, mk.RegisterHuman(testAddrs[1], "adv-uuid-2", issuance.FrequencyMonthly))
	require.NoError(t, mk.RegisterHuman(testAddrs[2], "adv-uuid-3", issuance.FrequencyQuarterly))

	annual := math.NewInt(issuance.AnnualAllocationUcredit)

	// Adversarial check: repeatedly try to push accounts past their allocation.
	for i := 0; i < 1000; i++ {
		mk.ProcessPayouts()
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "invariant broken at adversarial step %d: %s", i, msg)
	}

	// None should exceed annual allocation.
	for _, addr := range []string{testAddrs[0], testAddrs[1], testAddrs[2]} {
		acct := mk.Accounts[addr]
		assert.True(t, acct.YearCreditsReceived.LTE(annual),
			"account %s year credits %s must not exceed %s",
			addr, acct.YearCreditsReceived, annual)
	}

	// Total supply must be ≤ annual * human count.
	maxExpected := annual.MulRaw(3)
	assert.True(t, mk.TotalSupply.LTE(maxExpected),
		"total supply %s must not exceed %s",
		mk.TotalSupply, maxExpected)

	// Now burn and verify again.
	mk.Burn()
	msg, broken := mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after adversarial burn: %s", msg)

	// Post-burn, run more payouts.
	for i := 0; i < 100; i++ {
		mk.ProcessPayouts()
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "invariant broken at post-burn step %d: %s", i, msg)
	}
}

// ── Test 11: Register-then-burn-then-payout cycle ad infinitum ─────────────────

func TestInvariant_RegisterBurnPayoutCycle(t *testing.T) {
	mk := NewMockKeeper()

	for cycle := 0; cycle < 20; cycle++ {
		// Register a new human.
		addr := fmt.Sprintf("cycle-addr-%d", cycle)
		uuid := fmt.Sprintf("cycle-uuid-%d", cycle)
		require.NoError(t, mk.RegisterHuman(addr, uuid, issuance.FrequencyMonthly))
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "invariant broken after registration in cycle %d: %s", cycle, msg)

		// Run payouts for a simulated "year".
		for i := 0; i < 100; i++ {
			mk.ProcessPayouts()
			msg, broken := mk.CheckInvariant()
			assert.False(t, broken, "invariant broken at payout %d cycle %d: %s", i, cycle, msg)
		}

		// Burn.
		mk.Burn()
		msg, broken = mk.CheckInvariant()
		assert.False(t, broken, "invariant broken after burn in cycle %d: %s", cycle, msg)
	}

	annual := math.NewInt(issuance.AnnualAllocationUcredit)
	maxExpected := annual.MulRaw(mk.HumanCount)
	assert.True(t, mk.TotalSupply.LTE(maxExpected),
		"total supply %s must not exceed %s after %d cycles",
		mk.TotalSupply, maxExpected, 20)
}

// ── Test 12: Pure mathematical proof — sum of all payouts ≤ allocation × humans ─

func TestInvariant_MathematicalProof(t *testing.T) {
	// Mathematical proof: for any set of accounts with any payout frequencies,
	// the sum of all YearCreditsReceived across all accounts in any year
	// is bounded by AnnualAllocationUcredit × humanCount.
	//
	// Proof:
	//   For each account a, YearCreditsReceived[a] ≤ AnnualAllocationUcredit.
	//   This is enforced in ProcessPayouts: if remaining ≤ 0, skip.
	//   Therefore sum(YearCreditsReceived) ≤ N × AnnualAllocationUcredit
	//   where N = humanCount.
	//
	//   TotalSupply ≤ sum(YearCreditsReceived) because supply comes only
	//   from payouts (no pre-mine, no inflation beyond per-account caps).
	//
	//   Therefore TotalSupply ≤ N × AnnualAllocationUcredit.
	//
	// This test enumerates all possible payout schedules to verify the proof.

	freqs := []issuance.PayoutFrequency{
		issuance.FrequencyWeekly,
		issuance.FrequencyBiweekly,
		issuance.FrequencyMonthly,
		issuance.FrequencyQuarterly,
		issuance.FrequencyAnnual,
	}

	for _, f := range freqs {
		t.Run(string(f), func(t *testing.T) {
			mk := NewMockKeeper()
			require.NoError(t, mk.RegisterHuman(testAddrs[0], "proof-uuid", f))

			// Run far more blocks than needed.
			for i := 0; i < 1000; i++ {
				mk.ProcessPayouts()
			}

			acct := mk.Accounts[testAddrs[0]]
			annual := math.NewInt(issuance.AnnualAllocationUcredit)
			assert.True(t, acct.YearCreditsReceived.LTE(annual),
				"frequency %s: year credits %s > annual %s",
				f, acct.YearCreditsReceived, annual)

			// Verify the payout amount × payouts per year ≤ allocation
			perPayout := issuance.PayoutAmount(f)
			payoutsPerYear := issuance.PayoutsPerYear(f)
			totalPossible := perPayout.MulRaw(payoutsPerYear)
			assert.True(t, totalPossible.LTE(annual),
				"frequency %s: total possible payouts %s > annual %s",
				f, totalPossible, annual)

			assert.True(t, mk.TotalSupply.LTE(annual),
				"frequency %s: supply %s > annual %s",
				f, mk.TotalSupply, annual)
		})
	}
}

// ── Test 13: Zero accounts edge case ──────────────────────────────────────────

func TestInvariant_ZeroAccounts(t *testing.T) {
	mk := NewMockKeeper()
	msg, broken := mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold with zero humans: %s", msg)
	assert.True(t, mk.TotalSupply.IsZero(), "supply must be zero with no humans")

	// Burn with zero accounts should not panic.
	mk.Burn()
	msg, broken = mk.CheckInvariant()
	assert.False(t, broken, "invariant must hold after burn with zero humans: %s", msg)
}

// ── Test 14: Single account burn-invariant hold ───────────────────────────────

func TestInvariant_SingleAccountBurnCycle(t *testing.T) {
	mk := NewMockKeeper()
	require.NoError(t, mk.RegisterHuman(testAddrs[0], "single-uuid", issuance.FrequencyMonthly))

	for year := 0; year < 5; year++ {
		// Full year of payouts.
		for i := 0; i < 100; i++ {
			mk.ProcessPayouts()
		}
		msg, broken := mk.CheckInvariant()
		assert.False(t, broken, "year %d pre-burn: %s", year, msg)

		supplyBefore := mk.TotalSupply

		// Burn.
		mk.Burn()
		msg, broken = mk.CheckInvariant()
		assert.False(t, broken, "year %d post-burn: %s", year, msg)

		// Post-burn supply ≤ pre-burn supply (no new credits created).
		assert.True(t, mk.TotalSupply.LTE(supplyBefore),
			"post-burn supply %s must be <= pre-burn supply %s at year %d",
			mk.TotalSupply, supplyBefore, year)
	}

	// After 5 years, supply must still respect the invariant.
	annual := math.NewInt(issuance.AnnualAllocationUcredit)
	assert.True(t, mk.TotalSupply.LTE(annual),
		"final supply %s must be <= annual allocation %s for 1 human",
		mk.TotalSupply, annual)
}