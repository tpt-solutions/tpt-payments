# TPT Pay Anti-Money Laundering (AML) Policy Template

> **Status: template / draft, pending counsel review.** Not adopted policy. Operators must have it reviewed by counsel before use.

> **DISCLAIMER**: This is a template only and does not constitute legal advice. Operators must adapt this policy to their specific jurisdiction, business model, and risk profile, and have it reviewed and approved by qualified legal counsel and compliance professionals before implementation.

---

## Policy Overview
This template establishes minimum standards for Anti-Money Laundering (AML), Counter-Terrorist Financing (CTF), and sanctions compliance for operators deploying TPT Pay.

This policy aligns with international standards including FATF Recommendations, FinCEN guidance, EU 5AMLD, and UK Money Laundering Regulations.

---

## 1. Governance & Accountability

### 1.1 Designated Responsibilities
All operators must appoint:
| Role | Responsibility | Minimum Qualifications |
|------|----------------|-------------------------|
| MLRO (Money Laundering Reporting Officer) | Overall compliance oversight, SAR filing authority, regulatory liaison | Certified AML professional, minimum 3 years experience |
| Deputy MLRO | Backup authority | Appropriate compliance training |
| Transaction Monitoring Officer | Day to day alert review | Completed recognised AML training |

### 1.2 Training Requirements
✅ Mandatory initial training for all staff with customer or transaction access
✅ Annual refreshers training
✅ Documented training records for all personnel
✅ Role specific training for compliance staff

---

## 2. Transaction Monitoring Thresholds

These are minimum recommended thresholds. Operators must adjust based on their own risk assessment.

### 2.1 Automated Alert Rules
| Trigger Condition | Priority | Required Action |
|-------------------|----------|-----------------|
| Single transaction > $10,000 USD | HIGH | Manual review within 1 business day |
| Aggregate 24hr volume > $15,000 USD | HIGH | Manual review within 1 business day |
| Transaction to/from high risk jurisdiction | HIGH | Immediate review, enhanced due diligence |
| Structured transactions (<$10k to avoid reporting) | CRITICAL | Freeze account, file SAR immediately |
| Multiple failed identity verification attempts | MEDIUM | Additional checks, temporary account limit |
| Rapid in/out transaction pattern | HIGH | Review for layering activity |
| Transaction value > user declared income profile | MEDIUM | Account activity review |

### 2.2 Transaction Retention
✅ All transaction records must be retained for **minimum 5 years**
✅ All identity verification documents must be retained for **minimum 5 years after account closure**
✅ All alert review decisions and audit trails must be permanently retained

---

## 3. Suspicious Activity Reporting (SAR) Obligations

### 3.1 Mandatory Reporting Triggers
File a Suspicious Activity Report when:
- There is knowledge or suspicion that funds are proceeds of crime
- Transaction appears to be structured to avoid reporting thresholds
- Customer refuses to provide required information
- Transaction has no apparent lawful purpose
- Customer provides false or forged documentation
- Transaction involves individuals or entities on sanctions lists

### 3.2 Filing Requirements By Jurisdiction
| Jurisdiction | Reporting Authority | Filing Deadline |
|--------------|---------------------|-----------------|
| United States | FinCEN | 30 calendar days from detection |
| European Union | National FIU | Within 2 business days |
| United Kingdom | NCA | Immediately, no later than 3 working days |

> **Important**: TPT Platform automatically files required reports at network level. Operators remain responsible for filing reports based on activity specific to their platform and user base.

### 3.3 Safe Harbour Protection
✓ All jurisdictions provide statutory safe harbour for good faith SAR filings
✓ **PROHIBITED**: Do NOT disclose to the customer that a SAR has been filed
✓ Do NOT tip off the subject of any suspicious activity investigation

---

## 4. Sanctions Screening Requirements

### 4.1 Mandatory Screening Lists
All users and transaction counterparties must be screened against:
- OFAC SDN List (US)
- EU Consolidated Sanctions List
- UK OFSI Sanctions List
- UN Security Council Sanctions List
- FATF High Risk and Monitored Jurisdictions

### 4.2 Screening Frequency
✅ On account creation (real time)
✅ Before every outbound transaction
✅ Periodic rescreening of all active accounts (minimum monthly)
✅ Immediately upon new sanctions list updates

### 4.3 Prohibited Actions
❌ No transaction may be processed for any sanctioned individual or entity
❌ No funds may be held or received from sanctioned jurisdictions
❌ No indirect transactions through third parties to circumvent sanctions

---

## 5. Customer Due Diligence (CDD) Requirements

### 5.1 Standard Verification Levels
| User Tier | Verification Required | Transaction Limits |
|-----------|-----------------------|--------------------|
| Tier 0 | Email only | $0 - NO TRANSACTIONS |
| Tier 1 | Full name + verified phone | < $1,000 per month |
| Tier 2 | Government issued ID + address verification | < $10,000 per month |
| Tier 3 | Enhanced due diligence, source of funds | Unlimited |

### 5.2 Enhanced Due Diligence (EDD) Required For:
- Politically Exposed Persons (PEPs)
- High risk jurisdiction residents
- Transactions exceeding $50,000 USD
- Cash intensive businesses
- Transactions involving shell companies

---

## 6. Internal Controls & Audit

### 6.1 Independent Audit
✅ Annual independent AML compliance audit
✅ Quarterly internal compliance reviews
✅ Bi-annual risk assessment update
✅ Documented evidence of all review findings and corrective actions

### 6.2 Record Keeping Requirements
Maintain complete auditable records for:
- All user identity verification
- All transaction activity
- All alert reviews and decisions
- All SAR filings
- All staff training
- All audit findings and remediation

---

## 7. TPT Platform Provided Controls

The TPT Platform provides at network level:
✅ Real time OFAC / sanctions screening for all transactions
✅ KYC / identity verification services
✅ Transaction monitoring and alerting
✅ Regulatory reporting to relevant authorities
✅ Immutable audit log for all activity

> Operators remain fully responsible for:
> - Implementing additional controls appropriate to their risk profile
> - Reviewing and acting on all alerts generated
> - Filing jurisdiction specific SARs as required
> - Maintaining their own compliance program and records

---

## 8. Regulatory References

| Jurisdiction | Relevant Regulation |
|--------------|---------------------|
| Global | FATF Recommendations 2023 |
| United States | Bank Secrecy Act (BSA), FinCEN Guidance FIN-2019-G001 |
| European Union | Directive (EU) 2018/843 (5AMLD), Regulation (EU) 2015/847 |
| United Kingdom | Money Laundering, Terrorist Financing and Transfer of Funds (Information on the Payer) Regulations 2017 |

---

## Implementation Checklist

- [ ] Appoint MLRO and establish compliance team
- [ ] Adapt policy to specific business model and jurisdictions
- [ ] Obtain legal review and sign off
- [ ] Implement transaction monitoring thresholds
- [ ] Complete staff training
- [ ] Conduct initial risk assessment
- [ ] Establish audit schedule
- [ ] File required registrations with financial intelligence units

---

*Last updated: April 2026 | Template Version 1.0*