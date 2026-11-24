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
archive/backend/         Archived legacy prototype (not built, not supported, not in CI); features are ported into tpt-pay-core per todo §10
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
| Rate limiting (Redis, per API key) | **Alpha** (unit-tested against miniredis; not yet run against a real Redis in CI) | todo §2 |
| Secret encryption (Vault / KMS) | **Planned** | todo §2 |
| Sandbox card adapter | **Alpha** | todo §9 |
| `BankSponsorAdapter` / `DirectAcquiringAdapter` | **Planned** | todo §9 |
| Refunds API | **Alpha** (sandbox adapter; tested incl. concurrency) | todo §2 |
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

### First payment in five minutes

You need [Docker with Compose](https://docs.docker.com/get-docker/) running. Then, from the
repository root:

```bash
git clone https://github.com/tpt-solutions/tpt-payments.git
cd tpt-payments
make quickstart
```

`make quickstart` (a Go program, so it works on Windows, macOS and Linux) does this:

1. Creates `packages/tpt-pay-core/.env` from the example if you do not have one. An existing file is
   never overwritten.
2. Starts PostgreSQL, Redis, NATS, Vault and `tpt-pay-core` with `docker compose up -d --build`.
3. Waits for `http://localhost:8080/health`.
4. Creates a sandbox merchant and prints a **test** secret key. It never prints a live key.
5. Prints the `curl` commands for a first payment.

Then, using the key it printed:

```bash
# 1. Create a payment intent (amounts are in minor units: 1000 = 10.00 USD)
curl -s -X POST http://localhost:8080/v1/payment_intents   -H "Authorization: Bearer sk_test_..." -H "Content-Type: application/json"   -d '{"amount":1000,"currency":"usd"}'

# 2. Confirm it with a sandbox card (use the id from step 1)
curl -s -X POST http://localhost:8080/v1/payment_intents/pi_.../confirm   -H "Authorization: Bearer sk_test_..." -H "Content-Type: application/json"   -d '{"payment_method":"pm_card_visa"}'
```

The status becomes `succeeded`. The OpenAPI document is served at `/openapi.json`.

To capture later instead, create the intent with `"capture_method":"manual"`. Confirming it with
`pm_card_visa` leaves it at `requires_capture`. Then capture it, in full or in part:

```bash
# 3. Capture (the body is optional; leave it out to capture everything)
curl -s -X POST http://localhost:8080/v1/payment_intents/pi_.../capture   -H "Authorization: Bearer sk_test_..." -H "Content-Type: application/json"   -d '{"amount_to_capture":1000}'
```

To receive the result as a webhook, register an endpoint. Test keys may use `http://localhost`.
The response includes the signing secret (`whsec_...`) once, so store it now:

```bash
# 4. Register an endpoint for successful payments
curl -s -X POST http://localhost:8080/v1/webhook_endpoints   -H "Authorization: Bearer sk_test_..." -H "Content-Type: application/json"   -d '{"url":"http://localhost:4000/webhooks","enabled_events":["payment_intent.succeeded"]}'

# 5. Send a signed test event to your receiver, using the CLI in packages/tpt-pay-core/cmd/tpt
tpt trigger payment_intent.succeeded --forward-to http://localhost:4000/webhooks --secret whsec_...
```

Receivers that verify the signature are in `examples/node-webhook-receiver` and
`examples/python-webhook-receiver`. `tpt listen`, which forwards live deliveries to a local
machine, is not available yet: it needs a tunnel service.

### Sandbox test cards

The sandbox adapter approves any payment method that is not a test card number. Pass one of these
as `payment_method` to get that outcome:

| `payment_method` | Result |
|---|---|
| `pm_card_visa` (or any other value) | Approved |
| `pm_card_declined` | Declined (`card_declined`) |
| `pm_card_insufficient_funds` | Declined (`insufficient_funds`) |
| `pm_card_expired` | Declined (`expired_card`) |
| `pm_card_cvc_incorrect` | Declined (`incorrect_cvc`) |
| `pm_card_network_error` | Network error: the outcome is unknown, and the intent is left for reconciliation |
| `pm_card_slow` | Approved after 2 seconds (exercises client timeouts) |

Each `pm_card_*` token above also works as `pm_test_*` with the same result (for example `pm_test_declined`). The `pm_test_*` form makes test tokens obvious and cannot be mistaken for an issued payment method.
| `4000000000000002` | Declined (`card_declined`) |
| `4000000000009995` | Declined (`insufficient_funds`) |
| `4000000000000069` | Declined (`expired_card`) |
| `4000000000000101` | Declined (`incorrect_cvc`) |
| `4000000000000119` | Network error: the outcome is unknown, and the intent is left in `processing` for reconciliation |

Test keys (`sk_test_` / `pk_test_`) only act on test-mode data. Live keys (`sk_live_`) are created
with `go run ./cmd/bootstrap --live` and must never be used in development.

For resilience tests, `ADAPTER_CHAOS_RATE` (a number from 0 to 1) makes that fraction of adapter calls
fail with a network error, so the intent is left for reconciliation as a real timeout would. Set
`ADAPTER_CHAOS_SEED` to make the same calls fail on every run. Chaos is off by default and applies only
to the sandbox adapter.

### Working on the code

Prerequisites: [Go 1.23+](https://go.dev/dl/), [Node.js 20+](https://nodejs.org/) for the JS packages,
[golangci-lint](https://golangci-lint.run/usage/install/) for linting, and
[Atlas](https://atlasgo.io/getting-started) for migrations. `scripts/setup-dev.sh` checks for and
installs most of these.

No local toolchain? Open the repository in a dev container (VS Code "Reopen in Container", or
GitHub Codespaces). `.devcontainer/devcontainer.json` installs Go, Node and Python, and runs
`scripts/setup-dev.sh` once the container is created.

```bash
make dev        # full stack with hot reload (Redis, NATS and Vault are provisioned, not yet used by core)
make migrate    # apply migrations with Atlas
make test       # unit and integration tests; integration tests need TEST_DATABASE_URL
make lint       # golangci-lint
```

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
