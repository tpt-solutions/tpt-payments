# Contributing to TPT Platform

Thank you for your interest in contributing. This document covers how to set up your development environment, submit changes, and what to expect during review.

---

## Table of contents

1. [Code of Conduct](#code-of-conduct)
2. [Prerequisites](#prerequisites)
3. [Development setup](#development-setup)
4. [Making changes](#making-changes)
5. [Pull request process](#pull-request-process)
6. [Code style](#code-style)
7. [Testing requirements](#testing-requirements)
8. [Commit messages](#commit-messages)
9. [Contributor License Agreement](#contributor-license-agreement)

---

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md). By participating you agree to abide by its terms.

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.23+ | Payment backend, identity service |
| Docker + Compose | Latest | Local dev stack |
| Node.js | 20 LTS | Dashboard and JS SDK |
| Atlas | Latest | Database migrations |
| golangci-lint | 1.58+ | Go linting |
| Ignite CLI | 28+ | Cosmos SDK chain development |

---

## Development setup

```bash
# Clone the repo
git clone https://github.com/tpt-platform/tpt-platform.git
cd tpt-platform

# Copy environment template
cp packages/tpt-pay-core/.env.example packages/tpt-pay-core/.env

# Start the full dev stack (PostgreSQL, Redis, NATS, Vault, tpt-pay-core)
make dev

# Apply database migrations
make migrate

# Verify everything is working
make test
```

### Running a subset

```bash
# Only tpt-pay-core tests
make test-pay

# Integration tests (needs docker stack running)
make test-integration
```

---

## Making changes

1. **Open an issue first** for anything beyond a typo fix or obvious bug. This avoids wasted work if a direction isn't a good fit.
2. **Fork** the repo and create a branch from `main`: `git checkout -b feat/your-feature-name`
3. **Write tests** for all new behavior. See [Testing requirements](#testing-requirements).
4. **Run `make lint` and `make test`** before pushing. CI will reject lint failures and test failures.
5. **Create a PR** against `main`. Fill out the pull request template.

---

## Pull request process

- PRs require at least one approving review from a maintainer.
- CI must pass: lint, tests (unit + integration), and build.
- Keep PRs focused. A PR that mixes a bug fix with a refactor is harder to review and slower to merge.
- If your PR implements something from the TODO list, reference the specific item in the PR description.
- Maintainers may request changes. Address feedback and push new commits (don't force-push during review).

---

## Code style

### Go

- Follow standard Go conventions (`gofmt`, `go vet`).
- Use `slog` for structured logging — no `fmt.Println` in production paths.
- Money amounts are always `int64` in smallest currency units. Never use `float64` for money.
- Error types live in `internal/apierrors/`. Return `*apierrors.APIError` from handlers; let the error middleware render it.
- No global state. Dependencies are explicit constructor parameters.
- Table-driven tests with `t.Run()` subtests.

### SQL / Migrations

- All schema changes go in a migration file (`make migration name=your_change`).
- No raw DDL executed outside migration files.
- Migrations must be idempotent where possible.
- The ledger tables (`ledger_entries`) must never have UPDATE or DELETE statements. If you think you need one, open an issue first.

### TypeScript / JavaScript

- ESLint + Prettier config is in each package. Run `npm run lint` before committing.
- Full type safety — no `any` without a comment explaining why.

---

## Testing requirements

| Layer | Requirement |
|-------|-------------|
| Unit tests | Required for all business logic (ledger invariants, state machines, signing) |
| Integration tests | Required for anything touching PostgreSQL or NATS |
| Compatibility tests | Required for any changes to the Stripe-compatible API surface |

Do not mock the database in integration tests. Tests run against a real PostgreSQL instance via Docker Compose (`docker-compose.test.yml`). This is intentional: mock/real divergence has caused prod incidents before.

---

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(pay-core): add velocity limiter for card testing detection
fix(ledger): correct rounding in multi-currency balance query
docs: update migration runbook with rollback steps
test(adapter): add sandbox latency injection tests
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `ci`

---

## Contributor License Agreement

By submitting a pull request you agree that your contributions are licensed under the MIT License, and you have the right to make that grant. A formal CLA process will be added before Phase 1 launch.
