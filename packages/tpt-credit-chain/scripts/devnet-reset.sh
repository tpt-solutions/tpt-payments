#!/usr/bin/env bash
# devnet-reset.sh — wipe and reinitialize the local devnet.
# Use this during development when you need a clean chain state.
set -euo pipefail

CHAIN_ID="tpt-credit-devnet-1"
HOME_DIR="${HOME}/.tptcredit-devnet"
BINARY="./bin/tptcreditd"

echo "==> Building tptcreditd..."
go build -o bin/tptcreditd ./cmd/tptcreditd

echo "==> Resetting devnet state at $HOME_DIR..."
rm -rf "$HOME_DIR"

echo "==> Initializing chain..."
"$BINARY" init devnode --chain-id "$CHAIN_ID" --home "$HOME_DIR" --overwrite

echo "==> Copying genesis..."
cp config/devnet-genesis.json "$HOME_DIR/config/genesis.json"

echo "==> Copying app.toml + config.toml..."
cp config/app.toml    "$HOME_DIR/config/app.toml"
cp config/config.toml "$HOME_DIR/config/config.toml"

echo "==> Adding dev validator key..."
"$BINARY" keys add validator --keyring-backend test --home "$HOME_DIR" --output json 2>&1 | tee /tmp/validator-key.json

echo "==> Adding validator to genesis..."
VALIDATOR_ADDR=$("$BINARY" keys show validator -a --keyring-backend test --home "$HOME_DIR")
"$BINARY" genesis add-genesis-account "$VALIDATOR_ADDR" 1000000ucredit \
    --keyring-backend test --home "$HOME_DIR"
"$BINARY" genesis gentx validator 500000ucredit \
    --chain-id "$CHAIN_ID" --keyring-backend test --home "$HOME_DIR"
"$BINARY" genesis collect-gentxs --home "$HOME_DIR"

echo ""
echo "==> Devnet initialized. Run to start:"
echo "    $BINARY start --home $HOME_DIR"
