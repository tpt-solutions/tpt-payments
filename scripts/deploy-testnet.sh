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

# Step 2: Prepare the node home. `init` writes the genesis file; --overwrite replaces the
# home from the previous run.
echo -e "${YELLOW}[2/6] Preparing node home...${NC}"
HOME_DIR="/tmp/.tptcredit-testnet"
rm -rf "$HOME_DIR"

"$CHAIN_DIR/bin/tptcreditd" init testnet-validator --chain-id "tpt-credit-testnet-1" --home "$HOME_DIR" --overwrite

# Copy testnet genesis.
cp "$CONFIG_DIR/testnet-genesis.json" "$HOME_DIR/config/genesis.json"
echo -e "${GREEN}  ✓ Node home prepared at $HOME_DIR${NC}"
echo ""

# Step 3: The faucet account. The chain CLI cannot create keys yet (todo §7: key management is
# not implemented), so the mnemonic must be supplied from outside and never generated here.
echo -e "${YELLOW}[3/6] Checking faucet mnemonic...${NC}"
if [ -z "${FAUCET_MNEMONIC:-}" ]; then
  echo -e "${RED}  ✗ FAUCET_MNEMONIC is not set. The faucet cannot start without it.${NC}" >&2
  echo "    Create the faucet account with an external wallet tool and export its mnemonic:" >&2
  echo "      export FAUCET_MNEMONIC=\"word1 word2 ... word24\"" >&2
  exit 1
fi
echo -e "${GREEN}  ✓ FAUCET_MNEMONIC is set${NC}"
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
echo -e "  Validator:     ${YELLOW}not generated (chain CLI has no key management yet)${NC}"
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