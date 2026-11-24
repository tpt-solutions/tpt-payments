package issuance

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
)

// EventBus is the interface for emitting events from the keeper.
// In a real Cosmos SDK integration this wraps sdk.Context.EventManager.
// In tests this can be a mock that collects emitted events for assertions.
type EventBus interface {
	// EmitHumanRegistered emits a human registration event.
	EmitHumanRegistered(event HumanRegisteredEvent)
	// EmitCreditsIssued emits a credits-issued event.
	EmitCreditsIssued(event CreditsIssuedEvent)
	// EmitFrequencyUpdated emits a frequency update event.
	EmitFrequencyUpdated(event FrequencyUpdatedEvent)
	// EmitCreditsTransferred emits a credits transfer event.
	EmitCreditsTransferred(event CreditsTransferredEvent)
	// EmitBatchSend emits a batch send event.
	EmitBatchSend(event BatchSendEvent)
}

// Keeper holds the module's state and business logic.
type Keeper struct {
	// storeService store.KVStoreService
	accounts []HumanAccount // in-memory store for testing
	events   EventBus       // event emitter (nil = no events emitted)
	// blockHeight is the current block height (set by BeginBlock).
	blockHeight int64
	// currentYear is the issuance year (1-based, computed from genesis).
	currentYear int64
	// genesisHeight is the block height at genesis.
	genesisHeight int64
	// transferRecords is the audit log of all transfers.
	transferRecords []TransferRecord
}

// NewKeeper creates a new Keeper.
func NewKeeper() *Keeper {
	return &Keeper{
		accounts:        make([]HumanAccount, 0),
		events:          nil,
		blockHeight:     0,
		currentYear:     1,
		genesisHeight:   1,
		transferRecords: make([]TransferRecord, 0),
	}
}

// NewKeeperWithEvents creates a new Keeper with an event bus.
func NewKeeperWithEvents(events EventBus, genesisHeight int64) *Keeper {
	return &Keeper{
		accounts:        make([]HumanAccount, 0),
		events:          events,
		blockHeight:     genesisHeight,
		currentYear:     1,
		genesisHeight:   genesisHeight,
		transferRecords: make([]TransferRecord, 0),
	}
}

// validateAccountFields checks the address and identity UUID format of an account.
func validateAccountFields(address, identityUUID string) error {
	if len(address) > MaxAddressLen || !bech32Regex.MatchString(address) {
		return fmt.Errorf("address %q is not a valid bech32 address", address)
	}
	if !hex64Regex.MatchString(identityUUID) {
		return fmt.Errorf("identity UUID must be a 64-character lowercase hex SHA-256 digest")
	}
	return nil
}

// validAccountRef rejects a malformed address before any lookup, so callers get a format error
// rather than a misleading "not found". A well-formed address that is not registered still fails
// as "not found".
func validAccountRef(role, addr string) error {
	if len(addr) > MaxAddressLen || !bech32Regex.MatchString(addr) {
		return fmt.Errorf("%s address %q is not a valid bech32 address", role, addr)
	}
	return nil
}

// validAmount rejects an amount that is unset, zero or negative. An unset math.Int has no value to
// compare, so it must be caught before any arithmetic touches it.
func validAmount(amount math.Int) error {
	if amount.IsNil() || amount.IsNegative() || amount.IsZero() {
		return fmt.Errorf("transfer amount must be positive")
	}
	return nil
}

// findAccountIndex returns the index of the account with the given address,
// or -1 if not found.
func (k *Keeper) findAccountIndex(address string) int {
	for i := range k.accounts {
		if k.accounts[i].Address == address {
			return i
		}
	}
	return -1
}

// RegisterHuman registers a new verified human and schedules their first payout.
// Returns the HumanRegisteredEvent that was (or would be) emitted.
func (k *Keeper) RegisterHuman(ctx context.Context, address, identityUUID, tier string, freq PayoutFrequency) (HumanRegisteredEvent, error) {
	// The keeper enforces the same format rules as MsgRegisterHuman.ValidateBasic, so
	// callers that skip ValidateBasic cannot store a malformed address or identity.
	if err := validateAccountFields(address, identityUUID); err != nil {
		return HumanRegisteredEvent{}, err
	}

	// Validate identity tier.
	if !ValidTiers[tier] {
		return HumanRegisteredEvent{}, fmt.Errorf("identity_tier %q is not valid; must be one of: kyc, biometric, dna", tier)
	}

	// Validate payout frequency.
	if !ValidFrequencies[freq] {
		return HumanRegisteredEvent{}, fmt.Errorf(
			"payout_frequency %q is not valid; must be one of: weekly, biweekly, monthly, quarterly, annual", freq,
		)
	}

	// Ensure identity UUID is unique (duplicate check).
	for _, acct := range k.accounts {
		if acct.IdentityUUID == identityUUID {
			return HumanRegisteredEvent{}, fmt.Errorf("identity UUID %q is already registered to %s", identityUUID, acct.Address)
		}
	}
	// Ensure address is unique.
	for _, acct := range k.accounts {
		if acct.Address == address {
			return HumanRegisteredEvent{}, fmt.Errorf("address %q already registered", address)
		}
	}

	// Compute next due at: first payout is after BlocksPerPayout blocks.
	nextDueAt := k.blockHeight + BlocksPerPayout(freq)

	acct := HumanAccount{
		Address:             address,
		IdentityUUID:        identityUUID,
		IdentityTier:        tier,
		PayoutFrequency:     freq,
		YearCreditsReceived: math.ZeroInt(),
		LastPaidAt:          k.blockHeight,
		NextDueAt:           nextDueAt,
		RegisteredAt:        k.blockHeight,
		Balance:             math.ZeroInt(),
	}
	k.accounts = append(k.accounts, acct)

	// Create and emit the event.
	event := HumanRegisteredEvent{
		Address:         address,
		IdentityUUID:    identityUUID,
		IdentityTier:    tier,
		PayoutFrequency: freq,
		BlockHeight:     k.blockHeight,
	}
	if k.events != nil {
		k.events.EmitHumanRegistered(event)
	}

	return event, nil
}

// ProcessPayouts processes payouts for each account based on their frequency and block schedule.
// It returns a list of CreditsIssuedEvent for each successful payout.
// This is the BeginBlocker entry point.
// Implements missed payout recovery: if the node was down, payouts catch up
// on the next available block by checking if blocksSinceLastPaid >= blocksPerPayout.
func (k *Keeper) ProcessPayouts() []CreditsIssuedEvent {
	var issuedEvents []CreditsIssuedEvent
	year := k.currentYear

	for i := range k.accounts {
		acct := &k.accounts[i]

		blocksPerPayout := BlocksPerPayout(acct.PayoutFrequency)

		// The payout is due at NextDueAt. Accounts loaded from a genesis file that
		// carry no NextDueAt fall back to LastPaidAt + one interval.
		dueAt := acct.NextDueAt
		if dueAt == 0 {
			dueAt = acct.LastPaidAt + blocksPerPayout
		}

		// Only pay once the due height is reached. After an outage, the missed payout
		// is paid on the first block back, once per block, so there is no supply shock
		// from catching up several payouts in one block.
		if k.blockHeight < dueAt {
			continue
		}

		// Calculate payout amount, capped by the remaining annual allocation and by
		// the MaxBalanceUcredit cap on the account balance.
		payoutAmount := PayoutAmount(acct.PayoutFrequency)
		remaining := math.NewInt(AnnualAllocationUcredit).Sub(acct.YearCreditsReceived)
		if remaining.IsNegative() || remaining.IsZero() {
			continue
		}
		if payoutAmount.GT(remaining) {
			payoutAmount = remaining
		}
		balanceRoom := math.NewInt(MaxBalanceUcredit).Sub(acct.Balance)
		if balanceRoom.IsNegative() || balanceRoom.IsZero() {
			// The balance is at the cap. The payout stays due and is retried on later
			// blocks, so nothing is minted above the cap.
			continue
		}
		if payoutAmount.GT(balanceRoom) {
			payoutAmount = balanceRoom
		}

		// Apply the payout.
		acct.YearCreditsReceived = acct.YearCreditsReceived.Add(payoutAmount)
		acct.LastPaidAt = k.blockHeight
		acct.NextDueAt = k.blockHeight + blocksPerPayout
		acct.YearPayoutCount++
		acct.Balance = acct.Balance.Add(payoutAmount)

		// Build and emit event.
		event := CreditsIssuedEvent{
			Address:      acct.Address,
			Amount:       payoutAmount.String(),
			PayoutNumber: acct.YearPayoutCount,
			Year:         year,
			BlockHeight:  k.blockHeight,
		}
		issuedEvents = append(issuedEvents, event)

		if k.events != nil {
			k.events.EmitCreditsIssued(event)
		}
	}

	return issuedEvents
}

// BeginBlock is called at the start of each block.
// Order:
//  1. Process payouts using the current year (before year rollover).
//  2. If the block starts a new year, reset every account's year counters and advance
//     currentYear. The reset does not depend on the burn running on this exact block:
//     if the node was down at the boundary, the year still rolls over here, and the burn
//     module catches up its own burn. Balances are managed by the burn module.
func (k *Keeper) BeginBlock(height int64) []CreditsIssuedEvent {
	k.blockHeight = height

	// Process payouts using the current (pre-advance) year.
	events := k.ProcessPayouts()

	if k.genesisHeight > 0 {
		elapsedBlocks := k.blockHeight - k.genesisHeight
		newYear := 1 + (elapsedBlocks / BlocksPerYear)
		if newYear > k.currentYear {
			for i := range k.accounts {
				k.accounts[i].YearCreditsReceived = math.ZeroInt()
				k.accounts[i].YearPayoutCount = 0
			}
			k.currentYear = newYear
		}
	}

	return events
}

// UpdatePayoutFrequency changes an account's payout frequency.
// The change applies starting from the next payout cycle (not retroactive).
// The NextDueAt is recomputed from the current block height so the new
// frequency takes effect after the next block interval.
func (k *Keeper) UpdatePayoutFrequency(address string, newFreq PayoutFrequency) (FrequencyUpdatedEvent, error) {
	if err := validAccountRef("account", address); err != nil {
		return FrequencyUpdatedEvent{}, err
	}
	idx := k.findAccountIndex(address)
	if idx == -1 {
		return FrequencyUpdatedEvent{}, fmt.Errorf("account %q not found", address)
	}

	if !ValidFrequencies[newFreq] {
		return FrequencyUpdatedEvent{}, fmt.Errorf(
			"payout_frequency %q is not valid; must be one of: weekly, biweekly, monthly, quarterly, annual",
			newFreq,
		)
	}

	acct := &k.accounts[idx]
	oldFreq := acct.PayoutFrequency

	// Update the frequency and recompute NextDueAt from current block.
	acct.PayoutFrequency = newFreq
	acct.NextDueAt = k.blockHeight + BlocksPerPayout(newFreq)

	event := FrequencyUpdatedEvent{
		Address:      address,
		OldFrequency: oldFreq,
		NewFrequency: newFreq,
		BlockHeight:  k.blockHeight,
		EffectiveAt:  acct.NextDueAt,
	}
	if k.events != nil {
		k.events.EmitFrequencyUpdated(event)
	}

	return event, nil
}

// SendCredits transfers credits from one account to another.
// Validates sender balance and recipient existence.
// Enforces the MaxBalanceUcredit cap on the recipient.
func (k *Keeper) SendCredits(from, to string, amount math.Int) (CreditsTransferredEvent, error) {
	if err := validAccountRef("sender", from); err != nil {
		return CreditsTransferredEvent{}, err
	}
	if err := validAccountRef("recipient", to); err != nil {
		return CreditsTransferredEvent{}, err
	}
	fromIdx := k.findAccountIndex(from)
	if fromIdx == -1 {
		return CreditsTransferredEvent{}, fmt.Errorf("sender account %q not found", from)
	}

	toIdx := k.findAccountIndex(to)
	if toIdx == -1 {
		// Auto-create: allow sending to unregistered addresses?
		// Decision: require registration. Credits can only be sent
		// between registered humans. This prevents credits flowing to
		// unverified addresses which could be used for sybil attacks.
		return CreditsTransferredEvent{}, fmt.Errorf("recipient account %q not found; recipient must be registered", to)
	}

	if fromIdx == toIdx {
		return CreditsTransferredEvent{}, fmt.Errorf("cannot send credits to self")
	}

	if err := validAmount(amount); err != nil {
		return CreditsTransferredEvent{}, err
	}

	sender := &k.accounts[fromIdx]
	recipient := &k.accounts[toIdx]

	// Check sender balance.
	if sender.Balance.LT(amount) {
		return CreditsTransferredEvent{}, fmt.Errorf(
			"sender balance %s is insufficient for transfer amount %s",
			sender.Balance, amount,
		)
	}

	// Check recipient cap: recipient balance + amount must not exceed MaxBalanceUcredit.
	recipientNewBalance := recipient.Balance.Add(amount)
	if recipientNewBalance.GT(math.NewInt(MaxBalanceUcredit)) {
		return CreditsTransferredEvent{}, fmt.Errorf(
			"recipient would exceed max balance cap of %d ucredit (%d credits)",
			MaxBalanceUcredit, MaxBalanceCredits,
		)
	}

	senderBalanceBefore := sender.Balance
	recipientBalanceBefore := recipient.Balance

	// Execute transfer.
	sender.Balance = sender.Balance.Sub(amount)
	recipient.Balance = recipientNewBalance

	// Log transfer record.
	record := TransferRecord{
		From:        from,
		To:          to,
		Amount:      amount,
		BlockHeight: k.blockHeight,
	}
	k.transferRecords = append(k.transferRecords, record)

	// Build event.
	event := CreditsTransferredEvent{
		From:                   from,
		To:                     to,
		Amount:                 amount.String(),
		BlockHeight:            k.blockHeight,
		SenderBalanceBefore:    senderBalanceBefore.String(),
		RecipientBalanceBefore: recipientBalanceBefore.String(),
	}
	if k.events != nil {
		k.events.EmitCreditsTransferred(event)
	}

	return event, nil
}

// BatchSend sends credits to multiple recipients in one transaction.
// All transfers are atomic: if any fails, none are executed.
func (k *Keeper) BatchSend(from string, entries []BatchSendEntry) (BatchSendEvent, error) {
	if err := validAccountRef("sender", from); err != nil {
		return BatchSendEvent{}, err
	}
	fromIdx := k.findAccountIndex(from)
	if fromIdx == -1 {
		return BatchSendEvent{}, fmt.Errorf("sender account %q not found", from)
	}

	if len(entries) == 0 {
		return BatchSendEvent{}, fmt.Errorf("batch send must have at least one recipient")
	}

	if len(entries) > MaxBatchRecipients {
		return BatchSendEvent{}, fmt.Errorf("batch send recipients %d exceeds maximum %d", len(entries), MaxBatchRecipients)
	}

	// Validate all entries first (pre-commit validation).
	totalAmount := math.ZeroInt()
	type validatedEntry struct {
		recipientIdx int
		amount       math.Int
	}
	validated := make([]validatedEntry, 0, len(entries))
	seen := make(map[string]bool)

	for i, entry := range entries {
		if entry.To == "" {
			return BatchSendEvent{}, fmt.Errorf("entry %d: to address cannot be empty", i)
		}
		if err := validAccountRef("recipient", entry.To); err != nil {
			return BatchSendEvent{}, fmt.Errorf("entry %d: %w", i, err)
		}
		if entry.To == from {
			return BatchSendEvent{}, fmt.Errorf("entry %d: cannot send credits to self", i)
		}
		if seen[entry.To] {
			return BatchSendEvent{}, fmt.Errorf("entry %d: duplicate recipient %q", i, entry.To)
		}
		seen[entry.To] = true

		toIdx := k.findAccountIndex(entry.To)
		if toIdx == -1 {
			return BatchSendEvent{}, fmt.Errorf("entry %d: recipient account %q not found; must be registered", i, entry.To)
		}
		if err := validAmount(entry.Amount); err != nil {
			return BatchSendEvent{}, fmt.Errorf("entry %d: %w", i, err)
		}

		totalAmount = totalAmount.Add(entry.Amount)
		validated = append(validated, validatedEntry{recipientIdx: toIdx, amount: entry.Amount})
	}

	sender := &k.accounts[fromIdx]

	// Check sender balance.
	if sender.Balance.LT(totalAmount) {
		return BatchSendEvent{}, fmt.Errorf(
			"sender balance %s is insufficient for total batch amount %s",
			sender.Balance, totalAmount,
		)
	}

	// Check recipient caps.
	for _, ve := range validated {
		recipient := &k.accounts[ve.recipientIdx]
		recipientNewBalance := recipient.Balance.Add(ve.amount)
		if recipientNewBalance.GT(math.NewInt(MaxBalanceUcredit)) {
			return BatchSendEvent{}, fmt.Errorf(
				"recipient %q would exceed max balance cap of %d ucredit",
				recipient.Address, MaxBalanceUcredit,
			)
		}
	}

	// Execute all transfers.
	sender.Balance = sender.Balance.Sub(totalAmount)

	for _, ve := range validated {
		k.accounts[ve.recipientIdx].Balance = k.accounts[ve.recipientIdx].Balance.Add(ve.amount)

		// Log each transfer record.
		record := TransferRecord{
			From:        from,
			To:          k.accounts[ve.recipientIdx].Address,
			Amount:      ve.amount,
			BlockHeight: k.blockHeight,
		}
		k.transferRecords = append(k.transferRecords, record)
	}

	// Build event.
	event := BatchSendEvent{
		From:        from,
		Entries:     entries,
		TotalAmount: totalAmount.String(),
		BlockHeight: k.blockHeight,
	}
	if k.events != nil {
		k.events.EmitBatchSend(event)
	}

	return event, nil
}

// SetBlockHeight updates the keeper's block height (used in testing).
func (k *Keeper) SetBlockHeight(height int64) {
	k.blockHeight = height
}

// SetCurrentYear updates the keeper's current year (used in testing).
func (k *Keeper) SetCurrentYear(year int64) {
	k.currentYear = year
}

// GetCurrentYear returns the current issuance year.
func (k *Keeper) GetCurrentYear() int64 {
	return k.currentYear
}

// GetBlockHeight returns the current block height.
func (k *Keeper) GetBlockHeight() int64 {
	return k.blockHeight
}

// GetAccount returns a copy of the account at the given address.
// Returns nil if the address is not registered.
func (k *Keeper) GetAccount(address string) *HumanAccount {
	for i := range k.accounts {
		if k.accounts[i].Address == address {
			acct := k.accounts[i]
			return &acct
		}
	}
	return nil
}

// GetPayoutFrequency returns the payout frequency for an account.
// Returns an empty string if the account is not found.
func (k *Keeper) GetPayoutFrequency(address string) PayoutFrequency {
	acct := k.GetAccount(address)
	if acct == nil {
		return ""
	}
	return acct.PayoutFrequency
}

// GetNextDueAt returns the block height at which the next payout is due.
// Returns 0 if the account is not found.
func (k *Keeper) GetNextDueAt(address string) int64 {
	acct := k.GetAccount(address)
	if acct == nil {
		return 0
	}
	return acct.NextDueAt
}

// GetBalance returns the spendable balance of an account.
// Returns 0 if the account is not found.
func (k *Keeper) GetBalance(address string) math.Int {
	acct := k.GetAccount(address)
	if acct == nil {
		return math.ZeroInt()
	}
	return acct.Balance
}

// GetAccounts returns all registered human accounts.
func (k *Keeper) GetAccounts() []HumanAccount {
	result := make([]HumanAccount, len(k.accounts))
	copy(result, k.accounts)
	return result
}

// CheckInvariant verifies that the chain's total supply equals
// AnnualAllocationUcredit × verified_human_count.
func (k *Keeper) CheckInvariant(ctx context.Context) (string, bool) {
	humanCount := len(k.accounts)
	expectedMax := math.NewInt(AnnualAllocationUcredit).MulRaw(int64(humanCount))

	// Calculate current supply from all account balances (year credits).
	totalSupply := math.ZeroInt()
	for _, acct := range k.accounts {
		totalSupply = totalSupply.Add(acct.YearCreditsReceived)
	}

	if totalSupply.GT(expectedMax) {
		return fmt.Sprintf(
			"issuance invariant BROKEN: supply %s > expected max %s (human_count=%d)",
			totalSupply, expectedMax, humanCount,
		), true
	}
	return "", false
}

// CheckBalanceInvariant verifies that no account balance exceeds MaxBalanceUcredit.
func (k *Keeper) CheckBalanceInvariant(ctx context.Context) (string, bool) {
	for _, acct := range k.accounts {
		if acct.Balance.GT(math.NewInt(MaxBalanceUcredit)) {
			return fmt.Sprintf(
				"balance invariant BROKEN: account %s balance %s exceeds max %d",
				acct.Address, acct.Balance, MaxBalanceUcredit,
			), true
		}
	}
	return "", false
}

// GetHumanCount returns the number of registered humans.
func (k *Keeper) GetHumanCount() int64 {
	return int64(len(k.accounts))
}

// GetSupply returns the total supply (sum of all year credits).
func (k *Keeper) GetSupply() math.Int {
	total := math.ZeroInt()
	for _, acct := range k.accounts {
		total = total.Add(acct.YearCreditsReceived)
	}
	return total
}

// GetTotalBalance returns the sum of all spendable balances.
func (k *Keeper) GetTotalBalance() math.Int {
	total := math.ZeroInt()
	for _, acct := range k.accounts {
		total = total.Add(acct.Balance)
	}
	return total
}

// ResetYearCredits resets all year credits to zero (simulates annual burn).
func (k *Keeper) ResetYearCredits() {
	for i := range k.accounts {
		k.accounts[i].YearCreditsReceived = math.ZeroInt()
	}
}

// SetAccountBalance sets the balance of an account (used by burn module).
func (k *Keeper) SetAccountBalance(address string, balance math.Int) {
	for i := range k.accounts {
		if k.accounts[i].Address == address {
			k.accounts[i].Balance = balance
			return
		}
	}
}

// GetAccountBalance returns the balance of an account (used by burn module).
func (k *Keeper) GetAccountBalance(address string) math.Int {
	acct := k.GetAccount(address)
	if acct == nil {
		return math.ZeroInt()
	}
	return acct.Balance
}

// GetGenesisHeight returns the genesis block height.
func (k *Keeper) GetGenesisHeight() int64 {
	return k.genesisHeight
}

// GetTransferRecords returns all transfer records.
func (k *Keeper) GetTransferRecords() []TransferRecord {
	result := make([]TransferRecord, len(k.transferRecords))
	copy(result, k.transferRecords)
	return result
}

// GetTransferCount returns the total number of transfers.
func (k *Keeper) GetTransferCount() int {
	return len(k.transferRecords)
}

// InitializeGenesis initializes the keeper from a genesis state.
func (k *Keeper) InitializeGenesis(ctx context.Context, state *GenesisState) error {
	if state == nil {
		return fmt.Errorf("genesis state cannot be nil")
	}

	// Validate every genesis account before loading any of them.
	seenAddr := make(map[string]bool, len(state.HumanAccounts))
	seenUUID := make(map[string]bool, len(state.HumanAccounts))
	for i, acct := range state.HumanAccounts {
		if err := validateAccountFields(acct.Address, acct.IdentityUUID); err != nil {
			return fmt.Errorf("genesis account %d: %w", i, err)
		}
		if seenAddr[acct.Address] {
			return fmt.Errorf("genesis account %d: duplicate address %q", i, acct.Address)
		}
		if seenUUID[acct.IdentityUUID] {
			return fmt.Errorf("genesis account %d: duplicate identity UUID", i)
		}
		seenAddr[acct.Address] = true
		seenUUID[acct.IdentityUUID] = true
	}

	// Initialize accounts from genesis state.
	accounts := make([]HumanAccount, len(state.HumanAccounts))
	for i, acct := range state.HumanAccounts {
		accounts[i] = HumanAccount{
			Address:             acct.Address,
			IdentityUUID:        acct.IdentityUUID,
			IdentityTier:        acct.IdentityTier,
			PayoutFrequency:     acct.PayoutFrequency,
			YearCreditsReceived: acct.YearCreditsReceived,
			YearPayoutCount:     acct.YearPayoutCount,
			LastPaidAt:          acct.LastPaidAt,
			NextDueAt:           acct.NextDueAt,
			RegisteredAt:        acct.RegisteredAt,
			Balance:             acct.Balance,
		}
	}
	k.accounts = accounts

	return nil
}
