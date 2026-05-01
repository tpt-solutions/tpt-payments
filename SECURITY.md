# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| `main` branch | Yes |
| Tagged releases | Latest minor only |

## Reporting a vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Email: **security@tptplatform.org** (PGP key will be published here before Phase 1 launch)

Please include:

1. Description of the vulnerability
2. Steps to reproduce
3. Affected component(s) and version(s)
4. Potential impact assessment
5. Any suggested mitigations you have identified

## What to expect

| Timeline | Action |
|----------|--------|
| 48 hours | Initial acknowledgement |
| 7 days | Severity assessment and remediation timeline shared with reporter |
| 90 days | Public disclosure (coordinated with reporter) |

We will credit reporters in the security advisory unless they request anonymity.

## Scope

In scope:
- `tpt-pay-core` API (authentication bypass, authorization failures, injection attacks)
- Double-entry ledger integrity violations
- Webhook signing bypass
- API key exposure or privilege escalation
- `tpt-credit-chain` consensus or invariant violations
- Identity bridge sybil resistance bypasses

Out of scope:
- Denial of service via excessive API requests (rate limiting is a separate concern)
- Issues in third-party dependencies (report to the upstream project, then notify us)
- Social engineering attacks

## Security design principles

- **PCI DSS scope minimization**: raw card data never touches TPT Pay servers. Card tokenization is handled via the `CardNetworkAdapter` (sponsor bank or card network) before any data reaches the ledger.
- **Append-only ledger**: `ledger_entries` rows are never updated or deleted. Any attempt to modify ledger history is a critical bug.
- **Signed webhooks**: all webhook payloads are HMAC-SHA256 signed. Verify signatures before processing.
- **No PII on-chain**: the TPT Credit chain stores only UUID attestations and identity tier flags. No name, email, date of birth, or biometric template is stored on-chain.
