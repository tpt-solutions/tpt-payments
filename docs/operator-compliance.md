# TPT Pay Operator Compliance Guide

> **IMPORTANT LEGAL DISCLAIMER**: This document is for informational purposes only and does NOT constitute legal advice, financial advice, or professional counsel. Regulatory requirements vary significantly by jurisdiction, business model, and transaction type. All operators must retain qualified legal counsel licensed in their operating jurisdictions before deploying TPT Pay. This guidance represents general analysis only.

---

## Overview

TPT Pay utilizes a **bank-sponsor model** which shifts primary regulatory liability to licensed financial institutions. This document explains how this model impacts regulatory obligations for the TPT Platform and individual operators.

---

## Regulatory Analysis By Jurisdiction

### United States (FinCEN)

#### Bank-Sponsor Model Structure

Under the TPT model:
1. All fiat holdings remain at all times within custodial accounts held by the FDIC-insured sponsor bank
2. Payment rails and settlement are operated exclusively by the sponsor bank
3. TPT Platform only provides transaction messaging and order routing
4. Operators act as application providers, not financial intermediaries

#### MSB / MTL Status Determination

| Entity | Money Transmitter Status | Rationale |
|--------|--------------------------|-----------|
| **Sponsor Bank** | ✅ Licensed | Already holds federal and state banking charters, fully regulated |
| **TPT Platform** | ❌ Not required | TPT never holds, transmits, or controls funds. Only facilitates instruction routing between authorized parties. No beneficial interest in transactions. |
| **Operator** | ⚠️ Fact specific | *May* require registration if operator: <br>• Exercises custody over user funds<br>• Sets exchange rates independently<br>• Provides settlement services<br>• Acts as counterparty to transactions |

#### Operator US Requirements

✅ Standard minimum for most operators:
- FinCEN MSB registration is **NOT** required when running on TPT Pay bank-sponsor model
- State MTL licensing is **NOT** required for pure application layer operators

⚠️ Additional requirements when:
- Handling user funds directly: requires full MSB + 50 state MTLs
- Operating as a liquidity provider: requires additional licensing

---

### European Union (PSD2)

#### Scope Analysis

Under EU Payment Services Directive 2:

| Entity | Regulatory Classification | Obligations |
|--------|----------------------------|-------------|
| **Sponsor Bank** | Authorised Payment Institution (API) | Full PSD2 compliance, transaction reporting, capital requirements |
| **TPT Platform** | ❌ Not a PISP/AISP/EMI | TPT does not: <br>• Initiate payment orders<br>• Access account information<br>• Issue e-money<br>• Hold funds |
| **Operator** | ⚠️ Case by case | PSD2 applies only if operator performs regulated payment activities. Pure frontend application providers are out of scope. |

#### Operator EU Requirements

✅ Standard minimum:
- No payment institution license required for TPT integrated operators
- No e-money institution authorization required
- Must comply with general GDPR, consumer protection, and commercial law

⚠️ Additional licensing required if:
- Operator holds user balances outside the bank sponsor system
- Operator provides payment services to third parties not integrated via TPT

---

### United Kingdom (FCA)

#### Post-Brexit Regulatory Status

| Entity | FCA Classification |
|--------|---------------------|
| **Sponsor Bank** | PRA Authorised Bank |
| **TPT Platform** | ❌ Not regulated payment entity |
| **Operator** | ⚠️ May require PI/EMI authorisation only if performing regulated activities outside the sponsored model |

#### Operator UK Requirements

✅ Standard minimum:
- No Payment Institution (PI) license required
- No E-Money Institution (EMI) license required
- Compliance with UK GDPR, Consumer Rights Act, and FCA conduct rules where applicable

---

## How The Bank Sponsor Model Shifts Regulatory Burden

| Risk / Obligation | Handled By | Operator Responsibility |
|-------------------|------------|-------------------------|
| Funds custody | Sponsor Bank | None |
| Settlement finality | Sponsor Bank | None |
| Transaction reporting | Sponsor Bank / TPT | None |
| OFAC / Sanctions screening | Sponsor Bank / TPT | None |
| KYC / Identity verification | TPT Platform | None (operators may implement additional checks) |
| Consumer complaints resolution | Sponsor Bank / TPT | Frontend user support only |
| Regulatory capital requirements | Sponsor Bank | None |
| Reserve holding requirements | Sponsor Bank | None |
| Audit and examination | Sponsor Bank / TPT | None |
| Privacy compliance | Shared | Operator must comply for user data they collect |
| Terms of service | Shared | Operator responsible for their own user agreements |

---

## What This Platform Does NOT Provide

1. **Legal representation or advice** - This is technical infrastructure only
2. **Regulatory compliance outsourcing** - Operators remain responsible for their own business activities
3. **Guarantee against regulatory action** - Jurisdictions may interpret rules differently
4. **Insurance coverage** - Operators must obtain their own business insurance
5. **Tax advisory services** - All tax reporting and remittance remains operator responsibility

---

## Recommended Next Steps For Operators

1. Retain legal counsel specialising in financial services regulation in your target jurisdictions
2. Conduct formal legal review of your specific business model and use case
3. Obtain written legal opinion regarding your licensing obligations
4. Register for all applicable business and tax registrations
5. Maintain complete transaction records for audit purposes
6. Implement required internal policies (see AML Policy Template)

---

*Last updated: April 2026 | For TPT Pay v1.0*
