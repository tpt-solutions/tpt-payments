# PCI DSS Report on Compliance (ROC) Documentation Scaffolding

> **Status: template / draft, pending QSA review.** This is not an attestation of compliance. Operators must engage a Qualified Security Assessor (QSA) to produce and sign their own ROC.

## Overview
This document provides scaffolding for generating a PCI DSS (Payment Card Industry Data Security Standard) Report on Compliance (ROC). The ROC is required for merchants and service providers that process, store, or transmit cardholder data and meet certain volume thresholds.

## PCI DSS v4.0 Requirements Mapping

### Req 1: Install and maintain a firewall configuration to protect cardholder data
- [ ] Document firewall standards
- [ ] Network diagram with cardholder data flow
- [ ] Firewall configuration standards
- [ ] Quarterly firewall rule set review process

### Req 2: Do not use vendor-supplied defaults for system passwords and other security parameters
- [ ] Inventory of system components
- [ ] Password policy documentation
- [ ] Change default passwords/settings evidence
- [ ] Configuration standards for system components

### Req 3: Protect stored cardholder data
- [ ] Data retention and disposal policy
- [ ] Encryption key management process
- [ ] Cardholder data storage inventory
- [ ] PAN masking and truncation procedures

### Req 4: Encrypt transmission of cardholder data across open, public networks
- [ ] Encryption protocol standards (TLS 1.2+, SSH, etc.)
- [ ] Wireless network configuration
- [ ] VPN standards for remote access

### Req 5: Protect all systems and networks from malicious software
- [ ] Anti-malware policy
- [ ] Anti-virus/anti-malware deployment evidence
- [ ] Critical system patching process

### Req 6: Develop and maintain secure systems and software
- [ ] Secure development lifecycle documentation
- [ ] Code review processes
- [ ] Vulnerability management program
- [ ] Penetration testing methodology and results

### Req 7: Restrict access to cardholder data by business need to know
- [ ] Role-based access control matrix
- [ ] Access control policy
- [ ] Quarterly access review process

### Req 8: Identify and authenticate access to system components
- [ ] User ID standards
- [ ] Multi-factor authentication evidence
- [ ] Password requirements documentation
- [ ] Account review and termination process

### Req 9: Restrict physical access to cardholder data
- [ ] Physical security policy
- [ ] Visitor access logs
- [ ] Media storage and destruction procedures

### Req 10: Track and monitor all access to network resources and cardholder data
- [ ] Audit log standards
- [ ] Time synchronization policy (NTP)
- [ ] Log review procedures
- [ ] Critical security event alerting

### Req 11: Test security of systems and networks regularly
- [ ] Vulnerability scanning process (quarterly)
- [ ] Penetration testing (annual)
- [ ] Intrusion detection/prevention systems
- [ ] Wireless access point testing

### Req 12: Support information security with organizational policies and programs
- [ ] Information security policy
- [ ] Risk assessment process
- [ ] Incident response plan
- [ ] Security awareness training program
- [ ] Service provider monitoring (if applicable)

## TPT Pay Specific Controls

### API Security
- [ ] API authentication (API keys, JWT)
- [ ] Rate limiting and DDoS protection
- [ ] Input validation and sanitization
- [ ] Secure error handling (no PII in logs)

### Ledger Security
- [ ] Append-only ledger model
- [ ] Cryptographic integrity checks
- [ ] Audit trail for all financial transactions
- [ ] Separation of duties in ledger operations

### Webhook Security
- [ ] Webhook signature verification (HMAC-SHA256)
- [ ] Secure webhook endpoint configuration
- [ ] Webhook delivery retry and dead-letter queue

### Fraud Protection
- [ ] Rule-based fraud detection
- [ ] ML scoring engine (Phase 2)
- [ ] Velocity limiting and card testing detection
- [ ] BIN and IP blocklist management

### Data Protection
- [ ] Encryption at rest (AES-256 or equivalent)
- [ ] Encryption in transit (TLS 1.2+)
- [ ] Key management (rotation, storage)
- [ ] Data retention and purging policies

## ROC Generation Process

1. **Self-Assessment** (SAQ D for service providers)
   - Complete SAQ D v4.0 based on the above requirements
   - Document compensating controls where applicable

2. **External QSA Audit** (if required)
   - Engage a Qualified Security Assessor (QSA)
   - Provide evidence for each requirement
   - Remediate findings

3. **ROC Compilation**
   - Executive summary
   - Scope of assessment
   - Contact information
   - Assessment details (requirement-by-requirement)
   - Compensating controls
   - Findings and remediation

4. **Attestation of Compliance (AOC)**
   - Simplified version of ROC for merchants/partners
   - Signed by QSA (if applicable) or self-signed

## Evidence Repository Structure

```
/compliance/pci-dss/
  /evidence/
    /req1_firewall/
    /req2_defaults/
    /req3_data_protection/
    ...
  /policies/
    /standards/
    /diagrams/
    /audit_reports/
    /scan_results/
    /pen_test_reports/
  ROC_template.docx
  AOC_template.docx
```

## Automated Compliance Checks

TPT Pay includes automated checks for some PCI DSS requirements:

- **Req 8.2.3**: Password complexity (enforced via API key hashing)
- **Req 10.2**: Audit logging (all API requests logged)
- **Req 10.2.1-10.2.7**: Specific event logging (auth, admin, etc.)
- **Req 11.2**: Vulnerability scanning (CI/CD integration)
- **Req 12.11**: Service provider monitoring (for operators)

## Reporting for Operators

Operators deploying TPT Pay may need to provide PCI DSS compliance evidence to their acquirer or card networks. TPT Pay provides:

1. **Shared Responsibility Matrix**
   - TPT Pay (platform) responsibilities
   - Operator responsibilities (e.g., securing merchant dashboard access)

2. **Compliance Artifacts**
   - TPT Pay ROC (if applicable)
   - Subservice organization controls (SOC 2 Type II report)
   - Penetration test summaries

3. **Operator Guidance**
   - How to complete SAQ A, A-EP, B, C, or D based on integration model
   - Which requirements are handled by TPT Pay vs. operator

## Maintenance

- Review and update this scaffolding annually or upon PCI DSS version changes.
- Re-assess scope if architecture changes (e.g., new data centers, new services).
- Retain ROC and supporting evidence for at least 12 months (or as required by acquirer).

## References

- PCI DSS v4.0 Core Requirements: https://www.pcisecuritystandards.org/document_library
- SAQ Instructions and Guidelines: https://www.pcisecuritystandards.org/assessors-and-qualified-security-assessors/qualified-security-assessors
- NIST SP 800-53 (Security Controls): https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-53r5.pdf
- CIS Controls v8: https://www.cisecurity.org/controls/

---

**Note:** This document is scaffolding. Operators should engage a QSA to complete an actual ROC. TPT Pay provides this template to accelerate the process, but does not constitute legal or compliance advice.