# TPT Platform

**TPT Pay** — Open-source Stripe alternative. MIT licensed. Self-hostable.  
**TPT Credit** — Sovereign credit blockchain. 12,000 credits/person/year. No on-ramps. No off-ramps.

---

## Why

Payment infrastructure is a moat controlled by a small number of companies. TPT Pay removes that moat by providing the full stack as open-source software that any operator can run. TPT Credit makes hoarding structurally impossible through an annual burn mechanism — giving every verified human an equal economic starting position.

---

## Repository structure

```
packages/
  tpt-pay-core/          Go — payment API, ledger, webhook engine
  tpt-pay-dashboard/     Next.js — merchant dashboard (Phase 1)
  tpt-pay-sdk-js/        TypeScript — JavaScript/browser SDK (Phase 1)
  tpt-pay-sdk-python/    Python — Python SDK (Phase 2)
  tpt-credit-chain/      Cosmos SDK — sovereign credit blockchain
  tpt-credit-wallet-pwa/ Next.js — PWA wallet (Phase 1)
  tpt-credit-wallet-native/ React Native — native wallet (Phase 1)
  tpt-credit-wallet-core/   Shared wallet logic
  tpt-credit-identity/   Go — identity bridge service
  tpt-credit-explorer/   Next.js — chain explorer (Phase 1)
```

---

## Quick start (local development)

### Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [Docker + Docker Compose](https://docs.docker.com/get-docker/)
- [Node.js 20+](https://nodejs.org/) (for JS packages)
- [golangci-lint](https://golangci-lint.run/usage/install/) (for linting)
- [Atlas](https://atlasgo.io/getting-started) (for database migrations)

### 1. Clone and set up

```bash
git clone https://github.com/tpt-platform/tpt-platform.git
cd tpt-platform
cp packages/tpt-pay-core/.env.example packages/tpt-pay-core/.env
```

### 2. Start the dev stack

```bash
make dev
```

This starts PostgreSQL, Redis, NATS JetStream, HashiCorp Vault (dev mode), and `tpt-pay-core` with hot reload.

### 3. Run migrations

```bash
make migrate
```

### 4. Run tests

```bash
make test
```

---

## Architecture

### TPT Pay

- **API**: REST, Stripe API v2024-06-20 compatible
- **Backend**: Go (Chi router, pgx/v5, NATS JetStream)
- **Database**: PostgreSQL with append-only double-entry ledger
- **Message queue**: NATS JetStream (webhook delivery, async jobs)
- **Secrets**: HashiCorp Vault (card tokenization, API key encryption)

The `CardNetworkAdapter` interface abstracts card network access. Ship with `SandboxAdapter` for testing; plug in a `BankSponsorAdapter` or `DirectAcquiringAdapter` for production.

### TPT Credit

- **Framework**: Cosmos SDK v0.50 / CometBFT
- **Chain ID**: `tpt-credit-1`
- **Block time**: 2 seconds
- **Denomination**: `ucredit` (1 credit = 1,000,000 ucredit)
- **Issuance**: 12,000 credits/person/year — immutable at genesis
- **Burn**: Unspent credits burn annually and redistribute to active accounts

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup instructions, PR process, and code style guide.

## Security

See [SECURITY.md](SECURITY.md) for the responsible disclosure policy.

## License

[MIT](LICENSE) — free to use, modify, and deploy.
