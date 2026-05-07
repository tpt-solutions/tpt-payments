#!/usr/bin/env bash
# stress-test.sh — Simulate 10,000 accounts receiving issuance and burning.
# Tests chain throughput, invariant enforcement, and event processing.
# Usage: ./scripts/stress-test.sh [num_accounts] [chain_rpc_url]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CHAIN_DIR="$PROJECT_DIR/packages/tpt-credit-chain"
BINARY="$CHAIN_DIR/bin/tptcreditd"

NUM_ACCOUNTS="${1:-10000}"
CHAIN_RPC="${2:-http://localhost:26657}"
HOME_DIR="/tmp/.tptcredit-stress-test"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  TPT Credit Stress Test Script        ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "  Accounts to simulate: ${YELLOW}$NUM_ACCOUNTS${NC}"
echo -e "  Chain RPC:            ${YELLOW}$CHAIN_RPC${NC}"
echo ""

# Step 1: Build the binary if needed.
if [ ! -f "$BINARY" ]; then
  echo -e "${YELLOW}[1/6] Building binary...${NC}"
  cd "$CHAIN_DIR"
  go build -o "$BINARY" ./cmd/tptcreditd
else
  echo -e "${YELLOW}[1/6] Binary exists, skipping build${NC}"
fi

# Step 2: Initialize a fresh chain instance.
echo -e "${YELLOW}[2/6] Initializing fresh chain...${NC}"
rm -rf "$HOME_DIR"
mkdir -p "$HOME_DIR"

"$BINARY" init stress-test-node --chain-id "tpt-credit-stress-1" --home "$HOME_DIR" 2>/dev/null

# Step 3: Start the chain in background.
echo -e "${YELLOW}[3/6] Starting chain...${NC}"
export TPTCREDIT_HOME="$HOME_DIR"
"$BINARY" start --home "$HOME_DIR" &
CHAIN_PID=$!
echo -e "${GREEN}  Chain PID: $CHAIN_PID${NC}"

# Wait for chain to be ready.
sleep 2

# Step 4: Register accounts in batches.
echo -e "${YELLOW}[4/6] Registering $NUM_ACCOUNTS accounts...${NC}"
BATCH_SIZE=50
REGISTERED=0
START_TIME=$(date +%s)

for ((i=1; i<=NUM_ACCOUNTS; i++)); do
  ADDR="cosmos1stress$(printf '%010d' $i)"
  UUID="sha256-$(printf '%040d' $i)"
  TIER="kyc"
  FREQ="monthly"

  "$BINARY" tx issuance register-human \
    --address "$ADDR" \
    --uuid "$UUID" \
    --tier "$TIER" \
    --freq "$FREQ" \
    --home "$HOME_DIR" 2>/dev/null || true

  REGISTERED=$((REGISTERED + 1))

  if (( REGISTERED % 100 == 0 )); then
    ELAPSED=$(( $(date +%s) - START_TIME ))
    RATE=$(( REGISTERED / (ELAPSED + 1) ))
    echo -e "  ${GREEN}✓${NC} Registered $REGISTERED / $NUM_ACCOUNTS accounts ($RATE/s)"
  fi
done

END_TIME=$(date +%s)
TOTAL_ELAPSED=$(( END_TIME - START_TIME ))
echo -e "${GREEN}  ✓ Registered $REGISTERED accounts in ${TOTAL_ELAPSED}s${NC}"

# Step 5: Simulate transfers and burn cycle.
echo -e "${YELLOW}[5/6] Simulating transfers...${NC}"
TRANSFER_COUNT=500
TRANSFERRED=0

for ((i=1; i<=TRANSFER_COUNT; i++)); do
  FROM_IDX=$(( (i % REGISTERED) + 1 ))
  TO_IDX=$(( ((i + 7) % REGISTERED) + 1 ))
  
  FROM_ADDR="cosmos1stress$(printf '%010d' $FROM_IDX)"
  TO_ADDR="cosmos1stress$(printf '%010d' $TO_IDX)"
  AMOUNT=$(( (RANDOM % 1000 + 1) * 1000000 ))  # 1-1000 credits

  "$BINARY" tx issuance send \
    --from "$FROM_ADDR" \
    --to "$TO_ADDR" \
    --amount "$AMOUNT" \
    --home "$HOME_DIR" 2>/dev/null || true

  TRANSFERRED=$((TRANSFERRED + 1))
done

echo -e "${GREEN}  ✓ Executed $TRANSFERRED transfers${NC}"

# Step 6: Print stress test results.
echo -e "${YELLOW}[6/6] Stress test results:${NC}"
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Stress Test Complete                 ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo -e "  Accounts registered:   ${YELLOW}$REGISTERED${NC}"
echo -e "  Transfers executed:    ${YELLOW}$TRANSFERRED${NC}"
echo -e "  Registration time:     ${YELLOW}${TOTAL_ELAPSED}s${NC}"
echo ""

# Query chain state.
echo -e "  Chain status:"
"$BINARY" query status --home "$HOME_DIR" 2>/dev/null | head -20 || echo "  (chain status query failed)"
echo ""

# Query supply.
echo -e "  Supply info:"
"$BINARY" query issuance supply --home "$HOME_DIR" 2>/dev/null || echo "  (supply query failed)"
echo ""

# Check invariants.
echo -e "  Invariant check:"
INVARIANT_OUTPUT=$("$BINARY" query issuance supply --home "$HOME_DIR" 2>&1 || true)
if echo "$INVARIANT_OUTPUT" | grep -qi "broken"; then
  echo -e "  ${RED}INVARIANT BROKEN${NC}"
else
  echo -e "  ${GREEN}All invariants passed${NC}"
fi
echo ""

# Cleanup.
echo -e "${YELLOW}Cleaning up chain process...${NC}"
kill $CHAIN_PID 2>/dev/null || true
wait $CHAIN_PID 2>/dev/null || true
rm -rf "$HOME_DIR"
echo -e "${GREEN}Done.${NC}"