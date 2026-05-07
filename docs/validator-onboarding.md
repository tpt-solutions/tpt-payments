# TPT Credit Testnet — Validator Onboarding Guide

This guide walks external validators through joining the TPT Credit testnet.
The testnet uses Proof-of-Authority (PoA) consensus with a maximum of 7 validators.

## Prerequisites

- Linux server (Ubuntu 22.04+ or Debian 12+ recommended)
- Go 1.23+ installed
- Minimum 4 CPU cores, 8 GB RAM, 100 GB SSD
- Ports 26656 (P2P) and 26660 (metrics) open

## Step 1: Build the Chain Binary

```bash
git clone https://github.com/tpt-platform/tpt-platform.git
cd tpt-platform/packages/tpt-credit-chain
go build -o /usr/local/bin/tptcreditd ./cmd/tptcreditd
```

Verify the binary works:
```bash
tptcreditd version
# Expected: tptcreditd v0.1.0
```

## Step 2: Initialize the Node

```bash
tptcreditd init <your-validator-moniker> --chain-id tpt-credit-testnet-1
```

This creates `~/.tptcredit/config/` with:
- `genesis.json` — chain genesis state
- `node_key.json` — node identity
- `priv_validator_key.json` — validator signing key

## Step 3: Download the Testnet Genesis

```bash
# Download the testnet genesis file.
curl -o ~/.tptcredit/config/genesis.json \
  https://raw.githubusercontent.com/tpt-platform/tpt-platform/main/config/testnet-genesis.json

# Verify the genesis hash.
sha256sum ~/.tptcredit/config/genesis.json
# Expected hash: <hash to be published on testnet launch>
```

## Step 4: Configure the Node

Edit `~/.tptcredit/config/config.toml`:

```toml
[base]
moniker = "<your-moniker>"
log_level = "info"
log_format = "json"

[p2p]
# Replace with the boot node peer ID and address.
persistent_peers = "<boot-node-id>@<boot-node-ip>:26656"

[consensus]
timeout_commit = "1000ms"
```

Edit `~/.tptcredit/config/app.toml`:

```toml
minimum-gas-prices = "0ucredit"

[telemetry]
enabled = true
service-name = "tpt-credit"
```

## Step 5: Generate a Validator Key

```bash
# Generate a new key for signing transactions.
tptcreditd keys add validator --keyring-backend test

# Show the address.
tptcreditd keys show validator -a --keyring-backend test
```

**IMPORTANT**: Save the mnemonic phrase securely. This controls your validator.

## Step 6: Generate and Submit Gentx

```bash
# Create a gentx for the initial validator set.
tptcreditd genesis gentx validator 1000000ucredit \
  --chain-id tpt-credit-testnet-1 \
  --keyring-backend test \
  --moniker "<your-moniker>" \
  --details "TPT Credit testnet validator" \
  --website "https://your-website.com" \
  --security-contact "your-email@example.com"
```

The gentx is created at `~/.tptcredit/config/gentx/`. Submit it to the testnet
administrators via:

1. Email to [testnet@tptcredit.com](mailto:testnet@tptcredit.com)
2. Or open a PR adding your gentx file to the repository at:
   `config/testnet-gentxs/<your-moniker>-gentx.json`

## Step 7: Start the Node

```bash
tptcreditd start
```

Expected output:
```
Chain started at block height 1 (chain ID: tpt-credit-testnet-1)
Block interval: 2s
Metrics endpoint: http://localhost:26660/metrics
```

## Step 8: Verify Node Status

```bash
# Check node status via RPC.
curl http://localhost:26657/status

# Check validator signing.
curl http://localhost:26657/validators

# View metrics.
curl http://localhost:26660/metrics
```

## Monitoring Setup

Each validator should run the Prometheus node exporter:

```bash
docker run -d --name node-exporter \
  -p 9100:9100 \
  -v /proc:/host/proc:ro \
  -v /sys:/host/sys:ro \
  -v /:/rootfs:ro \
  prom/node-exporter:v1.8.0 \
  --path.procfs=/host/proc \
  --path.sysfs=/host/sys \
  --path.rootfs=/rootfs
```

## Troubleshooting

| Symptom | Solution |
|---------|----------|
| `connection refused` on P2P | Verify port 26656 is open in firewall |
| `genesis file mismatch` | Re-download the genesis file and verify hash |
| `insufficient peers` | Check `persistent_peers` config is correct |
| `validator not signing` | Verify `priv_validator_key.json` exists |
| `out of sync` | Check system clock (NTP enabled) |

## Security Checklist

- [ ] Firewall: only ports 26656 and 26660 (metrics) exposed
- [ ] SSH: key-based authentication only, fail2ban enabled
- [ ] System: automatic security updates enabled
- [ ] Monitoring: node exporter running, alerts configured
- [ ] Backups: `priv_validator_key.json` backed up offline
- [ ] NTP: time synchronization enabled

## Getting Help

- Testnet administrators: [testnet@tptcredit.com](mailto:testnet@tptcredit.com)
- GitHub Issues: https://github.com/tpt-platform/tpt-platform/issues
- Validator chat: [Telegram link to be provided]

## Useful Commands

```bash
# Check your validator's balance.
tptcreditd query bank balances <validator-address>

# Check recent blocks.
curl http://localhost:26657/block?height=<height>

# Pause signing (if needed).
tptcreditd tx staking unbond <validator-address> 1000000ucredit \
  --from validator --chain-id tpt-credit-testnet-1

# Restart node after maintenance.
tptcreditd start