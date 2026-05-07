#!/usr/bin/env bash
# deploy-testnet.sh — Deploy the TPT Credit testnet infrastructure.
# This script initializes the testnet genesis, builds the chain binary,
# and starts all testnet services via Docker Compose.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CHAIN_DIR="$PROJECT_DIR/packages/tpt-credit-chain"
DEPLOY_DIR="$PROJECT_DIR/deploy"
TESTNET_DIR="$DEPLOY_DIR/testnet"
CONFIG_DIR="$PROJECT_DIR/config"

# Color helpers.
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  TPT Credit Testnet Deployment Script  ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Step 1: Build the chain binary.
echo -e "${YELLOW}[1/6] Building chain binary...${NC}"
cd "$CHAIN_DIR"
go build -o "$CHAIN_DIR/bin/tptcreditd" ./cmd/tptcreditd
echo -e "${GREEN}  ✓ Built tptcreditd${NC}"

# Step 2: Generate validator key and gentx.
echo -e "${YELLOW}[2/6] Generating validator keys...${NC}"
HOME_DIR="/tmp/.tptcredit-testnet"
rm -rf "$HOME_DIR"

"$CHAIN_DIR/bin/tptcreditd" init testnet-validator --chain-id "tpt-credit-testnet-1" --home "$HOME_DIR" 2>/dev/null

# Copy testnet genesis.
cp "$CONFIG_DIR/testnet-genesis.json" "$HOME_DIR/config/genesis.json"

# Generate validator key.
VALIDATOR_MNEMONIC=$("$CHAIN_DIR/bin/tptcreditd" keys add validator \
  --keyring-backend test --home "$HOME_DIR" --output json 2>&1 | tee /tmp/validator-key.json | jq -r '.mnemonic' 2>/dev/null || true)

VALIDATOR_ADDR=$("$CHAIN_DIR/bin/tptcreditd" keys show validator -a --keyring-backend test --home "$HOME_DIR" 2>/dev/null || echo "unknown")

echo -e "${GREEN}  ✓ Validator address: $VALIDATOR_ADDR${NC}"
echo -e "${YELLOW}  ⚠ Save this mnemonic:${NC}"
echo ""
echo "  $VALIDATOR_MNEMONIC"
echo ""
echo -e "${YELLOW}  Save to FAUCET_MNEMONIC env var for faucet.${NC}"

# Step 3: Generate faucet key.
echo -e "${YELLOW}[3/6] Generating faucet key...${NC}"
FAUCET_MNEMONIC=$("$CHAIN_DIR/bin/tptcreditd" keys add faucet \
  --keyring-backend test --home "$HOME_DIR" --output json 2>&1 | tee /tmp/faucet-key.json | jq -r '.mnemonic' 2>/dev/null || true)

FAUCET_ADDR=$("$CHAIN_DIR/bin/tptcreditd" keys show faucet -a --keyring-backend test --home "$HOME_DIR" 2>/dev/null || echo "unknown")

echo -e "${GREEN}  ✓ Faucet address: $FAUCET_ADDR${NC}"
echo ""
echo -e "${YELLOW}  Faucet mnemonic (set as FAUCET_MNEMONIC):${NC}"
echo "  $FAUCET_MNEMONIC"
echo ""

# Step 4: Copy config files.
echo -e "${YELLOW}[4/6] Copying configuration files...${NC}"
cp "$CHAIN_DIR/config/app.toml" "$HOME_DIR/config/app.toml" 2>/dev/null || true
cp "$CHAIN_DIR/config/config.toml" "$HOME_DIR/config/config.toml" 2>/dev/null || true
echo -e "${GREEN}  ✓ Config files copied${NC}"

# Step 5: Start the testnet stack.
echo -e "${YELLOW}[5/6] Starting testnet Docker stack...${NC}"
cd "$PROJECT_DIR"

# Export needed variables for docker-compose.
export FAUCET_MNEMONIC="${FAUCET_MNEMONIC:-}"
export CHAIN_RPC_URL="http://validator-0:26657"
export CHAIN_API_URL="http://validator-0:1317"

docker compose -f "$TESTNET_DIR/docker-compose.testnet.yml" up --build -d
echo -e "${GREEN}  ✓ Docker stack started${NC}"

# Step 6: Wait for chain to be ready and print URLs.
echo -e "${YELLOW}[6/6] Waiting for chain to start...${NC}"
sleep 3

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  TPT Credit Testnet is Running!        ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "  Chain ID:      ${YELLOW}tpt-credit-testnet-1${NC}"
echo -e "  Validator:      ${YELLOW}$VALIDATOR_ADDR${NC}"
echo -e "  Faucet:        ${YELLOW}$FAUCET_ADDR${NC}"
echo ""
echo -e "  Services:"
echo -e "    Faucet:      ${YELLOW}http://localhost:3002${NC}"
echo -e "    Explorer:    ${YELLOW}http://localhost:3003${NC}"
echo -e "    Grafana:     ${YELLOW}http://localhost:3001${NC} (admin/tptcredit2026)"
echo -e "    Prometheus:  ${YELLOW}http://localhost:9091${NC}"
echo -e "    RPC:         ${YELLOW}http://localhost:26657${NC}"
echo -e "    API:         ${YELLOW}http://localhost:1317${NC}"
echo -e "    gRPC:        ${YELLOW}localhost:9090${NC}"
echo ""

echo -e "${YELLOW}IMPORTANT: Set the FAUCET_MNEMONIC environment variable before running this script.${NC}"
echo -e "${YELLOW}  export FAUCET_MNEMONIC=\"your faucet mnemonic here\"${NC}"
echo ""