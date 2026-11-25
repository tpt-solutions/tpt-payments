# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The project is pre-release
(`0.x`): no version is stable yet, and the status of each capability is in the README.

## [0.2.0](https://github.com/tpt-solutions/tpt-payments/compare/v0.1.0...v0.2.0) (2026-11-25)


### Features

* **api:** lint the OpenAPI spec and check it for breaking changes in CI ([50debf3](https://github.com/tpt-solutions/tpt-payments/commit/50debf3d74f1de9c5452055df8597c82e51c399f))
* **api:** serve /v1/subscriptions and run the billing loop ([ce9b428](https://github.com/tpt-solutions/tpt-payments/commit/ce9b4288127dff93585ae199e65551189ed0a9a9))
* **auth:** keep dashboard users, passwords and second factors in the core ([e18bd46](https://github.com/tpt-solutions/tpt-payments/commit/e18bd4623e31d86f8c83e670edae29fb6408e9cb))
* **cli:** add tpt listen, forwarding events without a tunnel ([7c70d8a](https://github.com/tpt-solutions/tpt-payments/commit/7c70d8a0fbd90022491a0fefae8133467ff32185))
* **cli:** tpt command for login, keys create, trigger and events ([f868c6b](https://github.com/tpt-solutions/tpt-payments/commit/f868c6b7ed4f1fba924340bb34a3907d46a89db6))
* **cli:** tpt logs and tpt replay over the delivery inspector ([16bb65b](https://github.com/tpt-solutions/tpt-payments/commit/16bb65bb4cf17163bb0d20756621f1eee8765cd8))
* **core:** add OpenTelemetry tracing end to end ([4f74ae0](https://github.com/tpt-solutions/tpt-payments/commit/4f74ae014c73e65094dd54aff249af93d4176561))
* **core:** database ledger invariants, scheduled integrity check, per-currency limits ([9448523](https://github.com/tpt-solutions/tpt-payments/commit/944852370b458bf3a680d6944acafa9b888a8bd0))
* **core:** list payment intents (GET /v1/payment_intents) ([1cf19cf](https://github.com/tpt-solutions/tpt-payments/commit/1cf19cfc3666c5a2bfa5761ce9f96611fa806c95))
* **core:** list webhook endpoints and API keys ([8648ff2](https://github.com/tpt-solutions/tpt-payments/commit/8648ff2ff4be49323c73b2085893d77eb192f1f2))
* **core:** merchant rules: flag and block payments, with a dry run ([089b334](https://github.com/tpt-solutions/tpt-payments/commit/089b33463a26cfc1f0f3a51af7cfe1857effc95e))
* **core:** NATS JetStream webhook transport ([7ab2544](https://github.com/tpt-solutions/tpt-payments/commit/7ab25449f708d4a28acadafaadec5d22b38dda3d))
* **core:** per-API-key rate limiting, Redis-backed with a memory backend for dev ([79f8294](https://github.com/tpt-solutions/tpt-payments/commit/79f829427525fefd09e1bd532f7320c90b2fad48))
* **core:** refunds with ledger reversal, and audit log ([006e0f3](https://github.com/tpt-solutions/tpt-payments/commit/006e0f38f812f2456264a5517ba13273fbc07f55))
* **core:** unwrap the webhook key with Vault Transit at startup ([354648b](https://github.com/tpt-solutions/tpt-payments/commit/354648bfeb016e0013eb0aa9e6918c9801726779))
* **core:** webhook delivery inspector: list deliveries and replay one ([ebdc5c1](https://github.com/tpt-solutions/tpt-payments/commit/ebdc5c10f1d8eef4373e65c93ba009d00627e2cf))
* **create-tpt-pay-app:** add Python, Go and Next.js checkout templates ([7bbb302](https://github.com/tpt-solutions/tpt-payments/commit/7bbb3021a5b75ae9a851d72566da8bb877bda9a4))
* **credit-chain:** answer queries from the saved state outside the node ([5b4c8ed](https://github.com/tpt-solutions/tpt-payments/commit/5b4c8ed79134b5a33587865ae7b48ec9be15bb8a))
* **credit-chain:** persist chain state across restarts ([4b0d716](https://github.com/tpt-solutions/tpt-payments/commit/4b0d716bb4bd3c762aaed965a7e5cc187ee9a4ff))
* **credit-chain:** require signed transfers on the transaction path ([e6f6be6](https://github.com/tpt-solutions/tpt-payments/commit/e6f6be6a660a1b2be0e707394a55887fcb9e102b))
* **credit-chain:** submit transactions to a running node from another shell ([bf42185](https://github.com/tpt-solutions/tpt-payments/commit/bf42185b2b003b183ccb1e92a0773973304e3d03))
* **deploy:** add a monitoring profile for the payments core ([e5e8790](https://github.com/tpt-solutions/tpt-payments/commit/e5e8790b3166b9b7d11a0738258988a134f37e10))
* **deploy:** harden the Helm chart and parameterize dev compose credentials ([a4dc326](https://github.com/tpt-solutions/tpt-payments/commit/a4dc326ec87dcdf1d41e3d52c651319cdccf59f0))
* **examples:** add a Next.js checkout example ([d9ee856](https://github.com/tpt-solutions/tpt-payments/commit/d9ee856e753d1a9541137880ff4114e2ed353f64))
* **examples:** add a Node Express checkout example ([018e09b](https://github.com/tpt-solutions/tpt-payments/commit/018e09b99f92767ee153efe2a52c71cf79251d80))
* **examples:** add a subscription billing example ([41bb26f](https://github.com/tpt-solutions/tpt-payments/commit/41bb26f37b8c2eaff99aea681259a1b489a5093d))
* **examples:** Node webhook receiver with signature verification tests ([7d6bea6](https://github.com/tpt-solutions/tpt-payments/commit/7d6bea6329a8b1dafe6f4247de01e0841c59e46f))
* **examples:** Python webhook receiver with signature verification tests ([146c113](https://github.com/tpt-solutions/tpt-payments/commit/146c1136d79ec77e0cecd29777f698f2aedc9657))
* **frontend:** let the API explorer send test requests ([3658fdc](https://github.com/tpt-solutions/tpt-payments/commit/3658fdc728324edb9aa39388362e392bf2fdd088))
* **frontend:** per-account lockout after repeated failed logins ([99b840c](https://github.com/tpt-solutions/tpt-payments/commit/99b840ca291156f47e197b8dcb074ff5a28020ac))
* **frontend:** read transactions and issue refunds through server routes ([10b28cc](https://github.com/tpt-solutions/tpt-payments/commit/10b28ccb10e3c6d270892743de4388f763ba163e))
* **frontend:** replay webhook deliveries from the delivery log ([d5a4695](https://github.com/tpt-solutions/tpt-payments/commit/d5a46953fa8c454befd77aaa55dee4becc4bfcbb))
* **frontend:** show webhooks, API keys and the dashboard from the API ([785e48d](https://github.com/tpt-solutions/tpt-payments/commit/785e48d22b7f70a768c6dc4a3b39d10e14102f86))
* **frontend:** sign in through the core's dashboard routes ([5a5b091](https://github.com/tpt-solutions/tpt-payments/commit/5a5b091ba2766445566b3ec4efcfe28463b2b162))
* **frontend:** UI kit, working alias and build, SDK docs match the real SDKs ([156fded](https://github.com/tpt-solutions/tpt-payments/commit/156fded42dd562402adaf4731fc7441ee341250a))
* **identity:** verify KYC attestations with a public key ([edd69ba](https://github.com/tpt-solutions/tpt-payments/commit/edd69bafe11a4e56999eff21a631beeeca93f92a))
* initial commit of TPT Payments platform ([8de6c8b](https://github.com/tpt-solutions/tpt-payments/commit/8de6c8b46874dfb08c7bee5ac0283da86f6f9ae5))
* **ledger:** persist audit checkpoints and store payment risk flags ([f1d44bd](https://github.com/tpt-solutions/tpt-payments/commit/f1d44bdf43386dc9e33d417b0f8333fafa287e14))
* **ledger:** reconcile the ledger and export audit packs ([95f56e8](https://github.com/tpt-solutions/tpt-payments/commit/95f56e8657c99ba94fc468a0475671f8ab1030d6))
* **mcp:** add a stdio Model Context Protocol server for the sandbox ([4d5a37b](https://github.com/tpt-solutions/tpt-payments/commit/4d5a37b5195282dceac8eda145180435dfe88a19))
* **migrations:** run schema migrations with 'server migrate' and a Helm hook job ([10662e4](https://github.com/tpt-solutions/tpt-payments/commit/10662e4f9360c7d041455135c7ce98ba219b08a7))
* **observability:** correlate logs with traces and trace webhook deliveries ([3d383ce](https://github.com/tpt-solutions/tpt-payments/commit/3d383ceec00c74fd3265234176d79e36abd670d7))
* **python:** honor Retry-After on retryable responses ([4dfb7b9](https://github.com/tpt-solutions/tpt-payments/commit/4dfb7b9381ca23d60f45fbe801ef2ba6bbf54e64))
* **quickstart:** make quickstart starts the stack and prints a first payment ([22ebdcf](https://github.com/tpt-solutions/tpt-payments/commit/22ebdcf53bf78f654f9e649f5d73a84c72560eac))
* **refunds:** GET /v1/refunds lists a merchant's refunds ([3f6cc2f](https://github.com/tpt-solutions/tpt-payments/commit/3f6cc2fc97ee1b768c5c062af7dab6141cd70921))
* **sandbox:** add a test clock and time-based outage windows ([e11ec5d](https://github.com/tpt-solutions/tpt-payments/commit/e11ec5dbd3fb9a948d0973c4028a9dcd97729d13))
* **sandbox:** chaos failure injection and pm_test_* scenario aliases ([c7b4b38](https://github.com/tpt-solutions/tpt-payments/commit/c7b4b380692da6f6ad38110da904cbd06c3fca86))
* **sandbox:** scripted failure scenarios for magic payment methods ([5e52d06](https://github.com/tpt-solutions/tpt-payments/commit/5e52d06521cf79181cb812be7f293b9b4852b70f))
* **scaffold:** create-tpt-pay-app with a Node checkout template ([fd0abeb](https://github.com/tpt-solutions/tpt-payments/commit/fd0abeba69268e9b89dff65a6770dbcebfade824))
* **sdk:** add a Go client, tpt-pay-go ([397b445](https://github.com/tpt-solutions/tpt-payments/commit/397b445585ef63a424e398de3dc135f8ab72c9aa))
* **sdk:** add a PHP client, tpt-solutions/tpt-pay ([3df37ce](https://github.com/tpt-solutions/tpt-payments/commit/3df37ced347eaef4bb9715ef6416b5166edb5530))
* **sdk:** add subscription methods to the JS and Python SDKs ([6aa751e](https://github.com/tpt-solutions/tpt-payments/commit/6aa751ee78ad83ea97f5097bcde3fc9fa4082e57))
* **sdk:** generate the JavaScript SDK's wire types from the OpenAPI spec ([11685fb](https://github.com/tpt-solutions/tpt-payments/commit/11685fb18ce9ffb41243466019d7a216ebc34dc2))
* **server:** believe X-Forwarded-For only from TRUSTED_PROXIES ([06f11f2](https://github.com/tpt-solutions/tpt-payments/commit/06f11f26ba6c4bb186eba4bfd0fa8781d664e3cc))
* **subscriptions:** bill fixed amounts on a schedule ([2e7de3e](https://github.com/tpt-solutions/tpt-payments/commit/2e7de3e2a8bc04986c5df299184a19967193ab07))


### Bug Fixes

* **api:** invalid parameters and idempotency misuse return 400, as Stripe does ([82656e6](https://github.com/tpt-solutions/tpt-payments/commit/82656e6d2d704c8ed531ac9f8eabc7446ce463e6))
* **burn:** credit the redistribution remainder to the reserve; halt on overflow ([2c88f33](https://github.com/tpt-solutions/tpt-payments/commit/2c88f336b5a1e248649a097e6cece17f74317dc7))
* **credit-chain:** CLI commands take the App lock while touching keepers ([2b37484](https://github.com/tpt-solutions/tpt-payments/commit/2b37484866d495e6580c34f34125ffb1990b939f))
* **credit-chain:** fail closed in the identity bridge; add replay protection ([0d12bd5](https://github.com/tpt-solutions/tpt-payments/commit/0d12bd5a20e2bdd59eabe62a48297b161bf7f9fc))
* **credit-chain:** init and start honor --home; init will not overwrite ([5675ced](https://github.com/tpt-solutions/tpt-payments/commit/5675ced35986841e744d1a78f574d3021f0c1fb3))
* **credit-chain:** payouts, year rollover, burn catch-up and invariant halt ([0031cd0](https://github.com/tpt-solutions/tpt-payments/commit/0031cd0df4607c400228dc1878c06211bdeb4fdc))
* **credit-chain:** shipped genesis files load; reserve address is valid bech32 ([50e6d8f](https://github.com/tpt-solutions/tpt-payments/commit/50e6d8fbec81016393b2c7dff494088a52e04f48))
* **credit-chain:** validate addresses and amounts before lookup; accept several JWT keys ([4f866a1](https://github.com/tpt-solutions/tpt-payments/commit/4f866a1aa367bb0c92306f22b0e6266c0b0cec20))
* **faucet:** send transfers one at a time ([1afb880](https://github.com/tpt-solutions/tpt-payments/commit/1afb8803a937f0d86cc44b80256f09b3f61fae27))
* **faucet:** validate bech32 checksums, stop leaking errors, add a lockfile ([123c51a](https://github.com/tpt-solutions/tpt-payments/commit/123c51a3e7f656155e578f346d010e7f0fe29002))
* **frontend:** escape quotes in developer pages and use fixed sample ids ([0585f85](https://github.com/tpt-solutions/tpt-payments/commit/0585f8546336a1a0075a227372d92679fbb0fd01))
* **frontend:** link the Select trigger to its listbox ([d347e52](https://github.com/tpt-solutions/tpt-payments/commit/d347e52fd68c9d8bd9a590cb96d77ab1935b112f))
* **frontend:** lock the account after repeated wrong 2FA codes ([8941951](https://github.com/tpt-solutions/tpt-payments/commit/894195137a1eeeeab3840bb525fda011b432e4f3))
* **frontend:** remove committed credentials, fail closed, harden sign-in ([f684b35](https://github.com/tpt-solutions/tpt-payments/commit/f684b35e1a891060000ef755d9d63586525484ee))
* **frontend:** webhook docs, tester and dashboard list only emitted events ([b9bc930](https://github.com/tpt-solutions/tpt-payments/commit/b9bc9306d054933c453c3aaf9d36618e5d7e1910))
* **go:** set go.work to go 1.26.0 to match module requirement ([ee0a535](https://github.com/tpt-solutions/tpt-payments/commit/ee0a5352972a5009fb0d56f0e725f560cbfeafbc))
* **ledger:** report payment and refund postings by their record id ([2cc833c](https://github.com/tpt-solutions/tpt-payments/commit/2cc833cd686416ffbdeb480835681fec7a12120d))
* **scripts:** remove calls to chain subcommands that do not exist ([9333e70](https://github.com/tpt-solutions/tpt-payments/commit/9333e7079bef74d3a5035a583a5ff14e5621509d))
* **testnet,helm:** faucet rate-limit unit, honest versions, Helm tag from appVersion ([da7324a](https://github.com/tpt-solutions/tpt-payments/commit/da7324a1cd4a31469e8549b8c2e1c749b7a1238a))
* **testnet:** match compose, healthcheck and docs to what the simulator serves ([44ae542](https://github.com/tpt-solutions/tpt-payments/commit/44ae542f0167f775e0f0a1ab3b5b69824326b1f5))
* **webhooks:** drop payment_intent.fraud_flagged from the event catalog ([21811e3](https://github.com/tpt-solutions/tpt-payments/commit/21811e3878f92c998fdfe67a10ced36bdca8c437))

## [Unreleased]

### Added

- Refunds: `POST /v1/refunds` and `GET /v1/refunds/{id}`, with partial refunds, a row-locked
  refundable balance, a ledger reversal per refund, and a `charge.refunded` webhook event.
- Audit log: every successful state-changing `/v1` call writes an `audit_log` row (actor,
  merchant, mode, resource, request ID, IP). Key material is never written.
- Database ledger invariants (migration 16): balanced transactions are enforced at commit,
  entry currency must match its account, and TRUNCATE is refused on ledger tables.
- Scheduled ledger integrity check (`LEDGER_INTEGRITY_INTERVAL`, default `1h`). A non-zero
  count is logged as an integrity failure.
- Per-currency balances: `Ledger.Balance` and `Ledger.MerchantBalances`.
- `.golangci.yml` for `packages/tpt-pay-core`, `CODEOWNERS`, and this changelog.

### Changed

- The maximum amount is defined per currency in major units, so zero-decimal currencies do
  not get a 100x larger ceiling. `MaxAmount` is replaced by `MaxAmountFor(code)`.
- `Ledger.Balance` returns the account's own currency and an error for an unknown account,
  instead of a summed value and a zero balance.
- The legacy `backend/` prototype moved to `archive/backend/`. It is not built or supported.
- Version labels corrected to `0.1.0` in the Helm chart, the frontend, and the faucet.

### Fixed

- Lint findings: unchecked response-body close, unchecked type assertion, error comparisons
  that fail on wrapped errors, and a deprecated time comparison in webhook verification.

### Security

- Compliance and legal documents are labelled as drafts pending QSA or counsel review.
