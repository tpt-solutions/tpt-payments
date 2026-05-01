#!/usr/bin/env bash
set -euo pipefail

echo "==> TPT Platform — dev environment setup"

# ── Go ──────────────────────────────────────────────────────────────────────
echo ""
echo "── Checking Go..."
if ! command -v go &>/dev/null; then
  echo "ERROR: Go not found. Install from https://go.dev/dl/ (need 1.23+)"
  exit 1
fi
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED_GO="1.23"
if [[ "$(printf '%s\n' "$REQUIRED_GO" "$GO_VERSION" | sort -V | head -n1)" != "$REQUIRED_GO" ]]; then
  echo "ERROR: Go $REQUIRED_GO+ required, found $GO_VERSION"
  exit 1
fi
echo "   Go $GO_VERSION — OK"

# ── Node ─────────────────────────────────────────────────────────────────────
echo ""
echo "── Checking Node.js..."
if ! command -v node &>/dev/null; then
  echo "WARNING: Node.js not found. Required for tpt-pay-dashboard and JS SDK."
  echo "         Install from https://nodejs.org/ (need 20 LTS)"
else
  NODE_VERSION=$(node --version | sed 's/v//')
  echo "   Node $NODE_VERSION — OK"
fi

# ── Docker ───────────────────────────────────────────────────────────────────
echo ""
echo "── Checking Docker..."
if ! command -v docker &>/dev/null; then
  echo "ERROR: Docker not found. Install from https://docs.docker.com/get-docker/"
  exit 1
fi
echo "   Docker $(docker --version | awk '{print $3}' | tr -d ',') — OK"

# ── Atlas ────────────────────────────────────────────────────────────────────
echo ""
echo "── Checking Atlas (migrations)..."
if ! command -v atlas &>/dev/null; then
  echo "   Atlas not found — installing..."
  curl -sSf https://atlasgo.sh | sh
else
  echo "   Atlas $(atlas version 2>/dev/null | head -1) — OK"
fi

# ── golangci-lint ─────────────────────────────────────────────────────────────
echo ""
echo "── Checking golangci-lint..."
if ! command -v golangci-lint &>/dev/null; then
  echo "   golangci-lint not found — installing..."
  go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
else
  echo "   golangci-lint $(golangci-lint version 2>/dev/null | head -1) — OK"
fi

# ── Environment files ─────────────────────────────────────────────────────────
echo ""
echo "── Setting up environment files..."
if [ ! -f packages/tpt-pay-core/.env ]; then
  cp packages/tpt-pay-core/.env.example packages/tpt-pay-core/.env
  echo "   Created packages/tpt-pay-core/.env from .env.example"
else
  echo "   packages/tpt-pay-core/.env already exists — skipped"
fi

# ── Generate dev TLS certs ────────────────────────────────────────────────────
echo ""
echo "── Generating dev TLS certificates..."
CERT_DIR="packages/tpt-pay-core/certs"
mkdir -p "$CERT_DIR"
if [ ! -f "$CERT_DIR/dev.crt" ]; then
  openssl req -x509 -newkey rsa:4096 -keyout "$CERT_DIR/dev.key" \
    -out "$CERT_DIR/dev.crt" -days 365 -nodes \
    -subj "/CN=localhost" \
    -addext "subjectAltName=DNS:localhost,IP:127.0.0.1" 2>/dev/null
  echo "   Generated dev TLS certs in $CERT_DIR/"
else
  echo "   Dev TLS certs already exist — skipped"
fi

echo ""
echo "==> Setup complete."
echo ""
echo "Next steps:"
echo "  1. Review packages/tpt-pay-core/.env and update any values"
echo "  2. Run: make dev"
echo "  3. Run: make migrate"
echo "  4. Run: make test"
