# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/). The project is pre-release
(`0.x`): no version is stable yet, and the status of each capability is in the README.

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
