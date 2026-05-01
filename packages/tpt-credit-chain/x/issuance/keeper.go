package issuance

import (
	"context"
	"fmt"

	"cosmossdk.io/core/store"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
)

// Keeper holds the module's state and business logic.
type Keeper struct {
	storeService store.KVStoreService
	bankKeeper   bankkeeper.Keeper
	// identityKeeper provides access to on-chain identity records.
	// Set via SetIdentityKeeper after construction (avoids import cycle).
	identityKeeper IdentityKeeper
}

// IdentityKeeper is the subset of the identity module's keeper that issuance needs.
type IdentityKeeper interface {
	GetHumanCount(ctx context.Context) (int64, error)
	IsVerifiedHuman(ctx context.Context, address string) (bool, error)
}

// NewKeeper creates a new Keeper.
func NewKeeper(ss store.KVStoreService, bk bankkeeper.Keeper) *Keeper {
	return &Keeper{storeService: ss, bankKeeper: bk}
}

// SetIdentityKeeper wires in the identity keeper (called during app wiring).
func (k *Keeper) SetIdentityKeeper(ik IdentityKeeper) {
	k.identityKeeper = ik
}

// RegisterHuman registers a new verified human and schedules their first payout.
// Called by MsgRegisterHuman handler.
func (k *Keeper) RegisterHuman(ctx sdk.Context, address, identityUUID, tier string, freq PayoutFrequency) error {
	// Ensure identity UUID is unique (duplicate check).
	existing, err := k.getByUUID(ctx, identityUUID)
	if err == nil && existing != nil {
		return fmt.Errorf("identity UUID %q is already registered to %s", identityUUID, existing.Address)
	}

	acct := HumanAccount{
		Address:             address,
		IdentityUUID:        identityUUID,
		IdentityTier:        tier,
		PayoutFrequency:     freq,
		YearCreditsReceived: math.ZeroInt(),
		LastPaidAt:          ctx.BlockHeight(),
		RegisteredAt:        ctx.BlockHeight(),
	}
	return k.setHumanAccount(ctx, acct)
}

// ProcessPayouts mints credits for all accounts whose next payout is due.
// Called from BeginBlock.
func (k *Keeper) ProcessPayouts(ctx sdk.Context) error {
	accounts, err := k.getAllAccounts(ctx)
	if err != nil {
		return fmt.Errorf("get accounts: %w", err)
	}

	for _, acct := range accounts {
		if !k.isPayoutDue(ctx, acct) {
			continue
		}

		amount := PayoutAmount(acct.PayoutFrequency)

		// Enforce the annual cap: do not issue more than AnnualAllocationUcredit per year.
		remaining := math.NewInt(AnnualAllocationUcredit).Sub(acct.YearCreditsReceived)
		if remaining.IsNegative() || remaining.IsZero() {
			continue
		}
		if amount.GT(remaining) {
			amount = remaining
		}

		coins := sdk.NewCoins(sdk.NewCoin(Denom, amount))
		addr, err := sdk.AccAddressFromBech32(acct.Address)
		if err != nil {
			return fmt.Errorf("invalid address %s: %w", acct.Address, err)
		}

		if err := k.bankKeeper.MintCoins(ctx, ModuleName, coins); err != nil {
			return fmt.Errorf("mint for %s: %w", acct.Address, err)
		}
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, ModuleName, addr, coins); err != nil {
			return fmt.Errorf("send to %s: %w", acct.Address, err)
		}

		acct.YearCreditsReceived = acct.YearCreditsReceived.Add(amount)
		acct.LastPaidAt = ctx.BlockHeight()
		if err := k.setHumanAccount(ctx, acct); err != nil {
			return fmt.Errorf("update account %s: %w", acct.Address, err)
		}

		ctx.EventManager().EmitEvent(sdk.NewEvent(
			"credits_issued",
			sdk.NewAttribute("address", acct.Address),
			sdk.NewAttribute("amount", amount.String()),
			sdk.NewAttribute("frequency", string(acct.PayoutFrequency)),
		))
	}
	return nil
}

// CheckInvariant verifies that the chain's total supply equals
// AnnualAllocationUcredit × verified_human_count.
// This is called by the chain's invariant checker on every block.
func (k *Keeper) CheckInvariant(ctx sdk.Context) (string, bool) {
	if k.identityKeeper == nil {
		return "", false
	}
	humanCount, err := k.identityKeeper.GetHumanCount(ctx)
	if err != nil {
		return fmt.Sprintf("issuance invariant: could not get human count: %v", err), true
	}

	supply := k.bankKeeper.GetSupply(ctx, Denom)
	expectedMax := math.NewInt(AnnualAllocationUcredit).MulRaw(humanCount)

	// Supply must never exceed maximum possible issuance.
	if supply.Amount.GT(expectedMax) {
		return fmt.Sprintf(
			"issuance invariant BROKEN: supply %s > expected max %s (human_count=%d)",
			supply.Amount, expectedMax, humanCount,
		), true
	}
	return "", false
}

// ── Storage helpers ───────────────────────────────────────────────────────────

func (k *Keeper) isPayoutDue(ctx sdk.Context, acct HumanAccount) bool {
	blocksPerPayout := blocksPerPayout(acct.PayoutFrequency)
	return ctx.BlockHeight()-acct.LastPaidAt >= blocksPerPayout
}

// blocksPerPayout returns how many blocks between payouts.
// Assumes 2-second block time and 365-day year.
func blocksPerPayout(f PayoutFrequency) int64 {
	const blocksPerYear = int64(365 * 24 * 60 * 30) // 2s blocks ≈ 15,768,000/year
	return blocksPerYear / PayoutsPerYear(f)
}

func (k *Keeper) getByUUID(ctx sdk.Context, uuid string) (*HumanAccount, error) {
	// TODO: implement secondary index by UUID in Phase 1
	// For Phase 0 scaffold this is a linear scan.
	accounts, err := k.getAllAccounts(ctx)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		if accounts[i].IdentityUUID == uuid {
			return &accounts[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}

func (k *Keeper) getAllAccounts(ctx sdk.Context) ([]HumanAccount, error) {
	// TODO: implement proper KV store iteration in Phase 1.
	// This stub returns empty for the scaffold.
	return nil, nil
}

func (k *Keeper) setHumanAccount(ctx sdk.Context, acct HumanAccount) error {
	// TODO: implement KV store persistence in Phase 1.
	return nil
}
