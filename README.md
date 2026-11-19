# TPT Platform

**TPT Pay** — Open-source Stripe alternative. Dual licensed MIT / Apache-2.0. Self-hostable.  
**TPT Credit** — Sovereign credit blockchain. 12,000 credits/person/year. No on-ramps. No off-ramps.

> **Project status: pre-release.** Everything below is on the roadmap and being built; the
> [status table](#status) shows exactly what works today. It is updated as each capability
> lands, and every row links to its item in [todo.md](todo.md).

---

## Why

Payment infrastructure is a moat controlled by a small number of companies. TPT Pay removes that moat by providing the full stack as open-source software that any operator can run. TPT Credit makes hoarding structurally impossible through an annual burn mechanism — giving every verified human an equal economic starting position.

---

## Repository structure

```
packages/
  tpt-pay-core/          Go — payment API, double-entry ledger, webhooks (canonical backend)
  tpt-pay/               TypeScript — JavaScript/browser SDK (@tpt/pay)
  python/                Python — Python SDK (tpt-payments)
  tpt-credit-chain/      Go — TPT Credit chain (in-memory simulator today; see Status)
frontend/                Next.js — merchant dashboard + developer portal
deploy/                  Helm chart, monitoring (Prometheus/Grafana), testnet stack + faucet
docs/                    Operator, compliance and security documentation
tools/                   Developer tooling (Stripe compatibility report)
backend/                 Legacy prototype, being ported into tpt-pay-core (not built, not supported)
```

Planned (not yet in the repo): wallets (PWA, native, shared core), chain explorer, `tpt` CLI,
`examples/` and `templates/`.

---

## Status

| Capability | Status | Tracking |
|---|---|---|
| Payment intents: create / retrieve / confirm / capture / cancel | **In progress** (happy path being fixed) | todo §1 |
| Double-entry ledger (Postgres, append-only) | **Alpha** | todo §1, §2 |
| Idempotency keys | **Alpha** (known races) | todo §1 |
| API keys & auth | **Alpha** (scope enforcement pending) | todo §1 |
| Webhook signing (HMAC-SHA256, Stripe-style) | **Alpha** | todo §1 |
| Webhook delivery (NATS JetStream, retries) | **Planned** | todo §2 |
| Rate limiting (Redis) | **Planned** | todo §2 |
| Secret encryption (Vault / KMS) | **Planned** | todo §2 |
| Sandbox card adapter | **Alpha** | todo §9 |
| `BankSponsorAdapter` / `DirectAcquiringAdapter` | **Planned** | todo §9 |
| Refunds API | **Planned** | todo §2 |
| Stripe API compatibility (v2024-06-20) | **Planned** (compat report not yet green) | todo §8 |
| OpenAPI spec & generated SDKs | **Planned** | todo §4 |
| JS SDK (`@tpt/pay`) | **Alpha** (packaging pending) | todo §8 |
| Python SDK (`tpt-payments`) | **Alpha** (packaging pending) | todo §8 |
| Merchant dashboard | **Prototype** (mock data) | todo §8 |
| Helm chart / monitoring | **Prototype** | todo §5, §6 |
| TPT Credit chain | **Simulator** — in-memory issuance/burn logic, no consensus or networking yet | todo §7 |
| TPT Credit on Cosmos SDK / CometBFT | **Planned** | todo §7 |
| Wallets, explorer | **Planned** | todo §7 |

Legend: **Stable** (tested, supported) · **Alpha** (works, gaps known) · **Prototype** (demo-grade) ·
**In progress** · **Simulator** · **Planned** (designed, not built).

---

## Quick start (local development)

### Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [Docker + Docker Compose](https://docs.docker.com/get-docker/)
- [Node.js 20+](https://nodejs.org/) (for JS packages)
- [golangci-lint](https://golangci-lint.run/usage/install/) (for linting)
- [Atlas](https://atlasgo.io/getting-started) (for database migrations)

`scripts/setup-dev.sh` checks for and installs most of these.

### 1. Clone and set up

```bash
git clone https://github.com/tpt-solutions/tpt-payments.git
cd tpt-payments
cp packages/tpt-pay-core/.env.example packages/tpt-pay-core/.env
```

### 2. Start the dev stack

```bash
make dev
```

This starts PostgreSQL, Redis, NATS, HashiCorp Vault (dev mode), and `tpt-pay-core` with hot reload.
(Redis, NATS and Vault are provisioned but not yet used by the core service — see Status.)

### 3. Run migrations

```bash
make migrate
```

### 4. Run tests

```bash
make test
```

A one-command `make quickstart` that seeds a test merchant and API key and prints a first-payment
`curl` is in progress (todo §4).

---

## Architecture

### TPT Pay

- **API**: REST, aiming for Stripe API v2024-06-20 compatibility
- **Backend**: Go (Chi router, pgx/v5)
- **Database**: PostgreSQL with append-only double-entry ledger
- **Message queue**: NATS JetStream (webhook delivery, async jobs) — planned
- **Secrets**: HashiCorp Vault (card tokenization, API key encryption) — planned

The `CardNetworkAdapter` interface abstracts card network access. `SandboxAdapter` ships today
for testing; `BankSponsorAdapter` and `DirectAcquiringAdapter` are planned.

### TPT Credit

Target design:

- **Framework**: Cosmos SDK v0.50 / CometBFT
- **Chain ID**: `tpt-credit-1`
- **Block time**: 2 seconds
- **Denomination**: `ucredit` (1 credit = 1,000,000 ucredit)
- **Issuance**: 12,000 credits/person/year — immutable at genesis
- **Burn**: Unspent credits burn annually and redistribute to active accounts

Today `packages/tpt-credit-chain` implements the issuance and burn rules as an in-memory
simulator (no consensus, networking, or persistence). It remains the fast dev/test backend; the
Cosmos SDK / CometBFT implementation is on the roadmap (todo §7).

---

## Compliance documents

Documents under `docs/compliance/`, `docs/aml-policy-template.md` and
`docs/legal-memo-msb-mtl-analysis.md` are **templates and working drafts**, not attestations or
legal advice. Operators must obtain their own assessment (QSA for PCI DSS, counsel for licensing).

---

## Contributing

Issues only — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Dual licensed under [MIT](LICENSE-MIT) or [Apache-2.0](LICENSE-APACHE), at your option.
