# TODO

Source: platform review 
Decisions: `packages/tpt-pay-core` is canonical (archive `backend/`); credit chain is relabeled as a simulator; scope is the full plan, executed in order, one PR per phase. Paths are relative to repo root; core = `packages/tpt-pay-core/`.

## 0. Status honesty (land first; no scope is removed)
Nothing below deletes a promised feature. Every promise stays and becomes a build item in §1–§9.
- [x] README "Status" table listing every promised capability (stable / alpha / prototype / in progress / simulator / planned), each row linking to its todo item; a row flips to "stable" only when its tests pass
- [x] Rows to include at minimum: NATS JetStream webhooks, Redis rate limiting, Vault/KMS encryption, `BankSponsorAdapter`, `DirectAcquiringAdapter`, Stripe compatibility, refunds, OpenAPI + generated SDKs, Cosmos SDK / CometBFT credit chain, wallets, explorer, `tpt` CLI, Helm, monitoring
- [x] Mark `docs/compliance/pci-dss-roc.md`, AML policy and legal memo as "template / draft, pending QSA or counsel" (expand them; do not delete)
- [x] Fix README package list to the real layout (`frontend/`, `packages/tpt-pay`, `packages/python`, `packages/tpt-credit-chain`, `packages/tpt-pay-core`) and reconcile the clone URL (`tpt-solutions/tpt-payments` per git remote)
- [x] Replace "1.0.0 / Production/Stable" labels (SDK package metadata, Helm `appVersion`, OpenAPI `info.version`) with an honest pre-release version such as `0.1.0` and a Development Status classifier
- [x] Reconcile LICENSE: `LICENSE-MIT` and `LICENSE-APACHE` are the dual-license files the README names; the tracked `LICENSE` file and package metadata must match
- [x] Add a short "Status" note to the top of `frontend/` and the credit-chain package README stating simulator / prototype status

## 1. Blocking bugs in core
- [x] 1. Confirm always 400s: add attach-payment-method / allow `requires_payment_method → processing` with a PM; add `processing → requires_capture` (`internal/payment/intent.go:52`, `service.go:90,154`)
- [x] 2. Capture fails on clearing-account FK: seed a system merchant + clearing account in a migration (`service.go:291`)
- [x] 3. Concurrency: do each transition in one txn with `SELECT … FOR UPDATE` / `WHERE status = expected` (`service.go:85,175,212,270-276`)
- [x] 4. Partial capture: validate `AmountToCapture` (>0, ≤ authorized), post the captured amount to the ledger
- [x] 5. Ledger idempotency: unique `(reference_id, type)`; write `ledger_txn_id` in the same txn
- [x] 6. Persist adapter charge ID with the status change; treat timeouts as unknown outcome (no rollback / double charge); use a detached ctx and check rollback errors
- [~] 7. Build out of the box: done: `go.sum`, Go 1.25 everywhere, migration 08 moved to `migrations_deferred/`, migrations apply cleanly to a fresh DB. Open: `atlas.sum` (needs the atlas CLI; CI no longer uses atlas, it applies migrations with psql and the tests apply their own)
- [x] 8. API keys: secret-only routes enforced (`RequireSecretKey`); keys minted in the caller's mode only; `cmd/bootstrap` creates the first merchant and keys; digest lookup replaces bcrypt-per-request (migration 11). Tests: `api/router_integration_test.go`.
  Policy choice to confirm: publishable keys may retrieve and confirm intents only.
- [x] 9. Idempotency: atomic claim (`INSERT … ON CONFLICT`), fingerprint covers method + path + body, 5xx and panics release the key, 1 MB limit rejects instead of truncating, responses stored byte-exact (migration 12), 24h expiry job started from `main`. Tests: concurrent duplicates create one intent; replay; body reuse rejected; path reuse rejected; oversize → 413.
  Not yet tested: the panic-release path (needs a fault-injecting handler).
- [x] 10. Webhooks: secrets encrypted with AES-256-GCM (`internal/secretbox`, key from `WEBHOOK_SECRET_KEY`, required at startup); URL validation (https in live, loopback-only in test, link-local always refused); event names validated; multiple `v1` accepted; future timestamps rejected; zero tolerance no longer disables replay checks; endpoints scoped by mode (migration 13).
  Open: Vault/KMS as the key store; hostnames are not re-checked at delivery time (the delivery engine is item §2).
- [x] 11. CI/release: production Dockerfile (distroless, nonroot); release pushes to GHCR with buildx/QEMU; changelog works on the first tag; core CI runs integration tests against a PostgreSQL service and no longer needs atlas; workflows trigger on `main` as well as `master`; `security.yml` audits real paths and pins govulncheck; Dependabot covers npm, pip, docker.
  The compatibility report was a hand-written table claiming 100% for endpoints the core service does not serve. Replaced by `packages/tpt-pay-core/cmd/compatreport`, which measures served routes: 9 of 27 reference endpoints (33%) today.
  Open: CodeQL, gitleaks, Trivy; the release workflow has not been run on a real tag; the image has not been built locally (no Docker here).
- [x] 12. SDK packaging. Python (`packages/python`): fixed import; `pyproject.toml` replaces `setup.py`; typed errors; idempotency key only on writes, reused across retries; webhooks verify raw bytes and raise on failure; 23 tests pass. JS (`packages/tpt-pay`): tsconfig, jest (25 tests pass), eslint, tsup build for CJS and ESM with exports map, `crypto-js` removed (Node `crypto` in `@tpt/pay/webhooks`), lockfile committed; types match what core returns.
  Open: READMEs for both SDKs; the base URL is `api.tpt-payments.com` in both SDKs and needs a decision on the real domain.
- [x] Happy-path integration tests (`internal/payment/service_integration_test.go`, `api/router_integration_test.go`), run against PostgreSQL 18 with every migration applied: create → confirm → capture, decline/retry, partial capture, concurrent capture and confirm (20 workers, exactly one winner), cancel, ledger idempotency. Skipped unless `TEST_DATABASE_URL` is set. Not yet run under `-race` locally (no C compiler on this machine); CI runs it.

## 2. Missing features the README promises
- [~] Webhook delivery engine (outbox → NATS JetStream → HTTP delivery, retries, `webhook_deliveries` rows, 72h / 15-attempt policy). Done: Postgres outbox (`webhook_deliveries`, migration 14), worker, retry config. Open: NATS JetStream transport; 72h / 15-attempt policy not re-verified
- [ ] Redis rate limiting; Vault encryption for keys / webhook secrets
- [x] Refund endpoint (`POST /v1/refunds`, `GET /v1/refunds/{id}`): row-locked reservation, network call outside the transaction, ledger reversal, `charge.refunded`. Integration tests in `refund_integration_test.go` (partial and full refunds, over-refund, concurrency, tenant isolation)
- [x] `/readyz` (DB ping) and `/metrics` routes (Prometheus config has nothing to scrape)
- [x] Request logging; log causes of 500s; use `errors.As` instead of type assertions
- [x] Write to `audit_log`: every successful state-changing `/v1` call (`middleware/audit.go`). Known gap: the row is written after the response, not in the same transaction as the state change. Tests in `audit_integration_test.go`
- [x] `http.MaxBytesReader` on all handlers; reject trailing JSON; don't swallow decode errors in Capture/Cancel
- [x] ISO-4217 currency allowlist. Done: ISO-4217 check, per-currency max amount (zero-decimal aware), metadata limits, enum validation, zero-decimal currencies, max amount, metadata limits, enum validation for `capture_method` / `confirmation_method`
- [x] `livemode` from key mode; Stripe-style 400 for invalid params
- [x] `get()` should select `canceled_at` and `ledger_txn_id`
- [x] Production guards: refuse sandbox adapter, dev Vault token, default DB creds when `APP_ENV=production`; make sandbox state survive restarts or fail loudly
- [x] Replace bcrypt-per-request with HMAC / SHA-256 for high-entropy keys; longer lookup prefix; bounded `last_used_at` update
- [x] Config: return errors instead of panics; validate pool min/max; drop unused required `NATS_URL`
- [~] Trust `X-Forwarded-For` only from configured proxies. Done: no forwarded header is trusted, so client IPs come from the connection. Open: configured trusted-proxy support (`RealIP`)
- [x] Serve a real `/openapi.json` (currently 501); complete spec (cancel, api_keys, webhook_endpoints/{id}, refunds)
- [x] Ledger: DB-level balanced-entries constraint, TRUNCATE guard, currency-matches-account (migration 16), per-currency `Balance` / `MerchantBalances`, scheduled `VerifyIntegrity` (`LEDGER_INTEGRITY_INTERVAL`), TRUNCATE guard, per-currency `Balance`, currency match with account, scheduled `VerifyIntegrity`

## 3. Repo hygiene
- [x] Archive `backend/` (no go.mod, doesn't build, unauthenticated, XSS, fake compliance code); exclude from CI. Moved to `archive/backend/` with a note; no CI or Dependabot reference existed
- [x] Removed `tools/compatibility-report` (replaced by `packages/tpt-pay-core/cmd/compatreport`). Stray `chain/go.mod`: not yet removed
- [~] Add `.gitignore` (done); `git rm` committed binaries (`packages/tpt-credit-chain/bin/tptcreditd.exe`, `tptcreditd.exe`) (not done: needs your go-ahead, they are tracked)
- [~] Add `.golangci.yml`, `CODEOWNERS`, `CHANGELOG.md`; tag a first release. Done: all three files; golangci-lint is clean. Open: first tag. CODEOWNERS points at the repo owner only
- [ ] Unify base URL (4 different today), Go module / org paths, version numbers
- [x] LICENSE: tracked `LICENSE` is MIT, but `LICENSE-APACHE` and `LICENSE-MIT` exist; reconcile with README / setup.py / package.json
- [ ] Tests: handlers, `payment.Service`, ledger DB ops, apikey, idempotency middleware, config; testcontainers integration tests; webhook Verify edge cases (multi-v1, malformed / future timestamps)

## 4. Adoption: examples, templates, DX
- [ ] `make quickstart`: copy `.env`, compose up, migrate in-container, seed test merchant + `sk_test_` key, print first-payment curl; cross-platform (no `SHELL := /bin/bash`)
- [ ] README "first payment in 5 minutes" (create → confirm → capture, webhook receive)
- [ ] Documented test cards / magic values, test vs live keys
- [ ] `examples/`: node-express-checkout, python-fastapi-webhooks, go-minimal, nextjs-checkout, subscription-billing, webhook-receiver (verification in each language); CI smoke job for each
- [ ] `templates/` + `npx create-tpt-pay-app` scaffolder
- [ ] Stripe migration guide + `base_url` shim; publish compatibility report
- [ ] `tpt` CLI: `login`, `listen --forward-to`, `trigger`, `logs`, `keys create`
- [ ] Generate SDKs from OpenAPI (JS, Python, Go, Java, Ruby, PHP, .NET); spec lint + breaking-change check (oasdiff); publish to npm / PyPI
- [ ] Docs site (Docusaurus / Mintlify) with integrator docs, OpenAPI reference, examples; link check in CI
- [ ] Production distroless multi-arch Docker image; `docker compose --profile monitoring`; publish Helm chart; one-click deploy buttons
- [ ] Devcontainer / Codespaces config; make `scripts/setup-dev.sh` discoverable from README

## 5. Automation / CI/CD
- [ ] CI for `frontend/`, both SDKs, Helm (`helm lint`), compose config, OpenAPI validation, examples; coverage reporting
- [ ] Always-run "required" gate job (path-filtered workflows can leave required checks stuck)
- [ ] `security.yml`: fix audited paths; add CodeQL, gitleaks, Trivy; pin govulncheck; remove `continue-on-error` on chain govulncheck
- [ ] Dependabot: npm (frontend, packages/tpt-pay, faucet), pip, docker, `/backend` (or archive), `/chain`, `/tools`; grouping + PR limits
- [~] Pin `arigaio/atlas:latest`, Helm `image.tag: latest`, other floating tags. Done: Helm tag now defaults to appVersion. Open: Atlas pin (CLI not installed here, so no verified version), other floating tags
- [ ] Release: release-please / conventional commits, GHCR push, cosign, SBOM, SLSA provenance, SDK + Helm publish, `-race`, fix `git describe` on first tag
- [ ] Fix `ci-credit-chain.yml` simulation step (no tests exist, `continue-on-error`)
- [ ] Stale / labeler / PR size bots; markdown lint

## 6. Security (outside core)
- [~] Credit chain identity bridge: remove hard-coded JWT fallback secret (`packages/tpt-credit-chain/services/identity/identity_bridge.go:165`); require `exp`, add replay protection; prefer asymmetric keys. Done: fails closed without a secret, requires `exp` and `jti`, rejects reused `jti` (`identity_bridge_test.go`). Open: asymmetric keys
- [~] Frontend auth: remove hard-coded user, scrypt salt, default JWT secret, hard-coded TOTP secret (`frontend/app/api/auth/*`); add `middleware.ts`; rate-limit login / 2FA; single-use pre-auth token. Done: credentials come from `DEMO_USER_*` env vars (no defaults; 503 when unset); per-user random salt in `scrypt$salt$key`; no default JWT key (`lib/auth.ts`); `middleware.ts` guards dashboard routes; login and 2FA rate-limited; pre-auth tokens single-use. Open: a real user store, account lockout, a shared rate-limit store. The prototype account still exists
- [~] Frontend: signing secret via `crypto.getRandomValues`, not `Math.random()`; bump `next` to ≥14.2.25. Done: API-key material uses `crypto.getRandomValues`; `next` is 14.2.35 (newest 14.2 patch). Open: `next` 14 declares a React 18 peer while the project uses React 19, so a clean `npm install` needs `--legacy-peer-deps`. Fix with a Next 15 migration or React 18 (decision needed)
- [~] Faucet: rate limit unit bug fixed (`RATE_LIMIT_PER_IP` is seconds; `formatWindow` builds the message). Open: lockfile for Docker build, stop leaking error text, real address validation, sequence-collision handling
- [ ] Helm: add `_helpers.tpl`, chart dependencies, Secret for credentials, `securityContext`, `sslmode`
- [ ] Remove committed dev credentials from compose / values (Vault token, Grafana password, DB passwords) or clearly gate them to dev
- [ ] `golang-jwt/jwt/v5` v5.2.1: check for fix in 5.2.2+ and bump; `aiohttp>=3.8.0` floor; Python 3.8 EOL

## 7. Credit chain (relabel as simulator)
- [ ] Update README, testnet compose, faucet, explorer, healthchecks to match reality (no CometBFT / ports 26657, 1317, 9090; metrics on 26660 only)
- [x] Remove nonexistent subcommands from `scripts/deploy-testnet.sh`, `devnet-reset.sh`; add `--home` / `--overwrite` or fix docs. Done: `init` and `start` accept `--home`, and `init` refuses to replace a genesis without `--overwrite` (tested). Keys and gentx calls were removed; the deploy script requires `FAUCET_MNEMONIC` instead of generating one. Key management is still open
- [ ] State persistence (currently lost on restart); make `tx` / `query` usable outside `start` process
- [ ] Fix genesis JSON mismatch (`config/devnet-genesis.json` vs `GenesisDoc`); validate genesis data
- [ ] Fix invalid bech32 reserve address (`app/app.go:51-53`); validate addresses / UUIDs in keeper
- [ ] Issuance: year reset when burn block is skipped; `NextDueAt` handling; enforce `MaxBalanceUcredit`; invariant should halt, not just print
- [ ] Burn: credit the remainder to reserve; no silent int64 truncation; deterministic ordering; handle skipped heights
- [ ] Keeper mutex / races; auth on `SendCredits` / `BatchSend` / `RegisterHuman`

## 8. SDKs and frontend (functional fixes)
- [x] Python SDK: fix import, retries / timeouts, parse API error bodies, drop empty `Idempotency-Key`, HMAC raw bytes in webhooks, handle malformed headers, add tests, `pyproject.toml`
- [x] JS SDK: tsconfig, jest, eslint, build with exports map, replace `crypto-js`, secret vs publishable surface enforced before any request, integer amounts and Unix `created` as core returns them, `TPTPayError` with code/param/requestId, idempotency + retry
  Open: README; `hosted_invoice_url` is not in the types because invoices are not served yet
- [ ] Align SDK / docs page: package names, import paths, signatures (`frontend/app/developers/sdks/page.tsx`)
- [~] Frontend: fix `@/*` alias (no `src/`), add missing `components/ui/*` and `globals.css`, declare `otpauth` / `@tpt/pay`, drop obsolete `experimental.serverActions`, `await cookies()`, replace mock dashboard data with API calls. Done: `otpauth` declared; `cookies()` awaited in auth routes. Open: alias, missing components and `globals.css` (`tsc` reports them), mock data
- [ ] Webhook header name consistency (`Stripe-Signature` vs `Column-Signature` vs `TPT-*`)
- [ ] Stripe-compat: event names (`chargeback.*` vs `charge.dispute.*`), 422 vs 400, wire compat suite into a module

## 9. Innovative additions
- [ ] Test clocks for subscriptions; time-travel sandbox
- [ ] Scenario-based sandbox: failure injection (latency, timeouts, issuer decline codes), `pm_test_*` tokens, chaos mode
- [ ] Webhook replay and inspector (persistent event log, redelivery, local tunnel in CLI)
- [ ] Ledger reconciliation job; Merkle-anchored checkpoints; exportable audit packs
- [ ] SDK idempotency and retry advisor (auto keys, backoff)
- [ ] OpenTelemetry tracing end to end
- [ ] MCP server for sandbox + docs; `llms.txt`
- [ ] Policy-as-code for routing / fraud rules (CEL / OPA) with dry-run
- [ ] Multi-adapter smart routing / failover (after `bank_sponsor` / `direct_acquiring` adapters exist)

## 10. Deferred: `backend/` features to port into core later
- [ ] Refunds, subscriptions, invoicing, disputes, payouts, connect, fraud, compliance (replace placeholder implementations: fake 1099-K totals, fake SAR filing, settlement hash / PDF placeholders, hard-coded `127.0.0.1` dispute evidence)
- [ ] Known bugs to avoid when porting: refund IDOR / double-refund race, ledger replay returning an error, ECB XML parsing, `%w` with string consts, reflected XSS in invoice page, no-op `APIKeyAuth`

## Suggested order
1. Section 0 + Section 1 items 1–8 + happy-path integration test
2. Section 1 items 11–12, Section 3 hygiene
3. Section 4 quickstart + first-payment docs + Node / Python / webhook examples
4. Webhook delivery engine + `tpt` CLI (`listen`, `trigger`)
5. OpenAPI completion → SDK generation → docs site → publishing
6. Sections 5, 6, 7, 8, then 9
