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

# The chain CLI has no key management or gentx support yet (todo §7), so this devnet has no
# validator keys. The simulator does not need them to run.

echo ""
echo "==> Devnet initialized (no validator keys: key management is not implemented yet). Run to start:"
echo "    $BINARY start --home $HOME_DIR"
