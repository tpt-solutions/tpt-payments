# tpt-credit-chain

> **Status: simulator / prototype.** Issuance and burn rules run in memory. There is no consensus, no networking and no persistence yet; state is lost on restart. The Cosmos SDK / CometBFT chain is planned ([todo §7](../../todo.md#7-credit-chain-relabel-as-simulator)).

Sovereign credit blockchain. Target design: Cosmos SDK v0.50 / CometBFT (planned).

**Chain ID:** `tpt-credit-1`  
**Denomination:** `ucredit` (1 credit = 1,000,000 ucredit)  
**Block time:** 2 seconds  
**Issuance:** 12,000 credits / verified human / year — immutable at genesis  
**Burn:** Unspent credits burn annually and redistribute to active accounts

---

## Prerequisites

- Go 1.23+
- [Ignite CLI v28+](https://docs.ignite.com/guide/install) (for scaffolding new modules)
- Docker (for devnet)

```bash
# Install Ignite CLI
curl https://get.ignite.com/cli | bash
```

## Running the devnet

```bash
# Build the chain binary
go build -o bin/tptcreditd ./cmd/tptcreditd

# Initialize a single-node devnet
./bin/tptcreditd init devnode --chain-id tpt-credit-devnet-1 --home ~/.tptcredit-devnet

# Add a validator key
./bin/tptcreditd keys add validator --keyring-backend test --home ~/.tptcredit-devnet

# Apply the devnet genesis
cp config/devnet-genesis.json ~/.tptcredit-devnet/config/genesis.json

# Start
./bin/tptcreditd start --home ~/.tptcredit-devnet
```

Or use the Docker Compose devnet (recommended):

```bash
cd ../../
make dev
```

## Module structure

```
x/
  issuance/    — credit issuance (immutable at genesis)
  burn/        — annual burn + pool redistribution (immutable at genesis)
```

Both modules are **immutable**: their core parameters (`annual_allocation`, `on_ramp_enabled`, `off_ramp_enabled`) are hardcoded in genesis and cannot be changed by governance. This is intentional — the entire value proposition of TPT Credit depends on these guarantees being unconditional.

## Adding a new module

```bash
ignite scaffold module <name> --module tpt-credit-chain
```

Only peripheral modules (payout scheduler, marketplace, CosmWasm) are upgradeable. Never add governance upgrade paths to `x/issuance` or `x/burn`.

## Invariants

The chain enforces the following invariant on every block:

```
total_supply == 12,000 * verified_human_count
```

A violation of this invariant triggers a critical alert and halts the chain. This is not a recoverable condition — it means something has gone fundamentally wrong with the issuance or burn logic.

## Key decisions

| Decision | Choice | Reason |
|----------|--------|--------|
| Issuance params | Hardcoded in genesis (not params store) | Prevents governance from changing allocation |
| On/off ramps | `false`, hardcoded | Prevents speculation and hoarding |
| Staking token | TBD — see Phase 2 PoS transition plan | Credits as stake creates hoarding incentives |
| Smart contracts | CosmWasm (Rust) | Upgradeable peripheral features without core changes |
