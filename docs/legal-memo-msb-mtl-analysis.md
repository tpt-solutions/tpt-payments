# LEGAL MEMORANDUM: Money Transmitter License Analysis
## TPT Pay Bank-Sponsor Model

**Date**: April 27, 2026
**Subject**: Regulatory Status Determination Under US FinCEN, EU PSD2, UK FCA
**Status**: Internal Working Draft - For Legal Review Only

> **CONFIDENTIAL**: This is an internal legal analysis document. Not for public distribution. Does not constitute legal advice.

---

## Executive Summary

The bank-sponsor architecture deployed by TPT Pay is structured such that **neither TPT Platform nor third party operators are required to register as Money Services Businesses (MSBs) or hold Money Transmitter Licenses (MTLs)** for standard deployments. All regulated financial activities are performed exclusively by the FDIC-insured sponsor bank which holds all required regulatory authorizations.

This analysis is based on published regulatory guidance and industry precedent for bank-agent models.

---

## 1. United States: FinCEN Analysis

### 1.1 Legal Precedent
This analysis relies on:
- FinCEN Guidance FIN-2008-G001: Money Services Business Definitions
- FinCEN Guidance FIN-2019-G001: Application of BSA Requirements to Certain Business Models
- Treasury Department: "Interpretation of the Term 'Money Transmitter'"
- Numerous no-action letters confirming bank agent exemption

### 1.2 Three Prong Test For Money Transmitter Status
Under US law, an entity is a money transmitter only if it:
1. ✅ Accepts currency or funds
2. ✅ Transmits currency or funds
3. ❌ Is not otherwise exempt

**Application to TPT Model**:
- ❌ TPT never accepts, receives, holds or has custody of funds at any time
- ❌ TPT never transmits funds - all settlement is performed directly by the sponsor bank
- ❌ TPT only transmits instructions, not value. Instructions are not "funds" under BSA
- ✅ Bank agent exemption applies pursuant to 31 CFR § 1022.310(c)(3)

### 1.3 Entity Status Determination
| Party | Money Transmitter Status | Rationale |
|-------|---------------------------|-----------|
| **Sponsor Bank** | ✅ Regulated | Exists under federal banking charter, not subject to MSB registration |
| **TPT Platform** | ❌ Not an MSB | Pure technology provider. No custody, no transmission of funds. Bank agent. |
| **Operator** | ❌ Not an MSB | Application layer only. All financial functions delegated to licensed bank. |

### 1.4 Edge Cases Requiring Registration
An operator **will** require MSB/MTL only if they:
- Hold user funds outside the sponsor bank custodial structure
- Act as counterparty to transactions
- Set foreign exchange rates independently
- Provide settlement services outside TPT rails

---

## 2. European Union: PSD2 Scope Analysis

### 2.1 Regulatory Framework
Under Payment Services Directive 2 (Directive (EU) 2015/2366) regulated payment services include:
1. Payment initiation services (PISP)
2. Account information services (AISP)
3. Issuing of payment instruments
4. Money remittance
5. Execution of payment transactions

### 2.2 Application To TPT Model
TPT Platform does **not** fall within PSD2 scope because:
- ❌ TPT does not initiate payment orders - orders are initiated directly by users via bank APIs
- ❌ TPT does not access user account information - all authentication is bank side
- ❌ TPT does not hold, receive or transmit funds
- ❌ TPT is not a payment service provider and provides only technical infrastructure

ECB guidance confirms that "mere technical service providers who do not assume responsibility for the payment transaction are excluded from PSD2 scope".

### 2.3 Entity Status Determination
| Party | PSD2 Status |
|-------|-------------|
| **Sponsor Bank** | ✅ Authorised Payment Institution |
| **TPT Platform** | ❌ Not a payment institution |
| **Operator** | ❌ Out of scope for pure application providers |

---

## 3. United Kingdom: FCA Analysis

### 3.1 Post-Brexit Regulatory Position
Following UK exit from EU, FCA has retained substantially equivalent regulation under the Payment Services Regulations 2017.

### 3.2 Key Determinations
- TPT Platform is excluded from payment institution registration per Regulation 11 - it provides only technical infrastructure and does not provide payment services
- The fact that transactions are routed through TPT messaging layer does not make TPT a payment service provider
- Operators are only required to register if they perform regulated payment services independent of the sponsor bank

---

## 4. Risk Mitigation Safeguards

To maintain this regulatory status the following architectural safeguards must be maintained at all times:

✅ **No Custody Rule**: TPT and operators must never at any time hold, control, or have ability to withdraw user funds. All funds must remain exclusively in bank controlled custodial accounts.

✅ **No Settlement Rule**: All transaction settlement must be performed exclusively by the sponsor bank using their own licensed payment rails.

✅ **No Rate Setting Rule**: All exchange rates and fees must be set and published by the sponsor bank. TPT and operators may not modify rates.

✅ **Agent Relationship**: Formal written agency agreement must exist between TPT and the sponsor bank explicitly establishing agent status.

✅ **Disclosures**: All user interfaces must clearly disclose that payment services are provided by the licensed sponsor bank.

---

## 5. Limitations

This analysis:
- Applies only to the standard TPT Pay bank-sponsor operating model
- Does not cover additional licensing requirements for specific use cases
- Does not eliminate the risk of regulatory interpretation differences
- May require adjustment based on individual operator business models
- Is not binding on any regulator

Regulators retain authority to interpret these rules differently based on specific factual circumstances.

---

**End of Memorandum**