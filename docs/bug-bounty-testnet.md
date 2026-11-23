# TPT Credit Testnet — Bug Bounty Program

**Status:** Active (testnet phase)  
**Rewards:** Up to $5,000 USD (or equivalent in credit-pool contributions)  
**Scope:** TPT Credit chain core modules (issuance, burn, scheduler, transfer logic)  

## Introduction

The TPT Credit testnet is now live, and we invite security researchers, developers,
and community members to audit the chain's core modules before mainnet launch.

The two most critical modules — **issuance** and **burn** — are designed to be
immutable at genesis. This means any bugs discovered and fixed during testnet
are permanently prevented from being reintroduced.

## Scope (In-Scope)

| Module | Description | Criticality |
|--------|-------------|-------------|
| `x/issuance` | Human registration, payout scheduling, transfer logic | Critical |
| `x/burn` | Annual full-burn, redistribution pool calculation | Critical |
| `x/issuance/simulation` | Invariant enforcement, edge cases | High |
| Identity bridge | UUID uniqueness, tier validation | High |
| Chain CLI | Command parsing, state queries | Medium |

## Out of Scope

- Infrastructure (Docker, Prometheus, Grafana configurations)
- Faucet web application
- Ping.pub explorer frontend
- Third-party dependencies (Cosmos SDK, CometBFT)
- Any code not in the `packages/tpt-credit-chain/` directory

## Reward Structure

| Severity | Description | Reward |
|----------|-------------|--------|
| **Critical** | Invariant violation (supply != humans * 12,000), balance cap bypass, unauthorized issuance | $5,000 |
| **High** | Double-spend, unauthorized transfer, payout timing manipulation | $2,500 |
| **Medium** | Event emission failure, incorrect burn calculation edge case, state corruption | $1,000 |
| **Low** | UI/CLI display bugs, non-critical log errors, typos | $250 |

## How to Report

1. **DO NOT** file public GitHub issues for vulnerabilities.
2. Send details to **[security@tptcredit.com](mailto:security@tptcredit.com)** with:
   - Subject: `[Bug Bounty] <module> - <brief description>`
   - Full description of the vulnerability
   - Steps to reproduce (including testnet RPC endpoints if needed)
   - Proof of concept code (if applicable)
   - Impact assessment
3. You will receive an acknowledgement within 48 hours.
4. We will validate, assign severity, and determine reward.

## Rules

- First reporter of each unique vulnerability receives the reward.
- Reports must include sufficient detail to reproduce.
- You must not exploit the vulnerability beyond what is needed to demonstrate it.
- You must not access, modify, or delete data you do not own.
- You must not perform attacks that degrade testnet service (DoS, spam).
- Rewards are paid after the bug is fixed and verified on testnet.

## Testnet Access

- Faucet: https://testnet-faucet.tptcredit.com
- Explorer: https://testnet-explorer.tptcredit.com
- RPC: https://testnet-rpc.tptcredit.com
- API: https://testnet-api.tptcredit.com
- Chain ID: `tpt-credit-testnet-1`

## Timeline

| Phase | Date | Description |
|-------|------|-------------|
| Testnet Launch | June 2026 | Public testnet goes live |
| Bug Bounty Start | June 2026 | Bug bounty opens |
| Bug Bounty End | 4 weeks after testnet stable | Bounty closes for mainnet prep |
| Mainnet Launch | 2 weeks after bounty closes | Genesis ceremony |

## Previous Audits

- Internal audit: March 2026 (all findings addressed)
- Simulation tests: April 2026 (1000-account stress passed)
- Invariant tests: April 2026 (all invariants enforced every block)

## Contact

- Security: security@tptcredit.com
- Testnet issues: testnet@tptcredit.com
- GitHub: https://github.com/tpt-solutions/tpt-payments
