# Security Audit Scope — TPT Pay Platform

## Purpose
This document defines the scope for an external security audit of the TPT Pay platform. It is intended to be used by operators when engaging a security audit firm (e.g., NCC Group, Trail of Bits, Cure53, etc.) prior to launching the platform.

## Audit Objectives
1. **API Layer Security** — Assess the security of RESTful APIs, authentication, authorization, input validation, rate limiting, and error handling.
2. **Ledger Integrity** — Verify the append-only ledger model, cryptographic integrity checks, and concurrency safety. 
3. **Authentication & Authorization** — Review API key management, JWT/session handling, multi-factor authentication, and role-based access control.
4. **Webhook Signing** — Validate HMAC-SHA256 signature generation, verification, and replay protection.
5. **Fraud Protection Rules** — Review rule engine, velocity limiting, card testing detection, blocklist management, and ML scoring integration points.

## Scope Boundaries

### In Scope
- `tpt-payments/archive/backend/api/` — API handlers, middleware, authentication, rate limiting.
- `tpt-payments/archive/backend/internal/ledger/` — Ledger implementation, double-entry invariants, integrity checks. 
- `tpt-payments/archive/backend/internal/auth/` — API key hashing, JWT generation, session management.
- `tpt-payments/archive/backend/internal/webhooks/` — Webhook signing, delivery, retry logic, dead-letter queue.
- `tpt-payments/archive/backend/internal/fraud/` — Rule engine, velocity limiter, card testing detector, blocklist management. 
- `tpt-payments/archive/backend/internal/compliance/` — GDPR erasure handler, AML screening, SAR filing hooks. 
- `tpt-payments/frontend/` — Merchant dashboard authentication, session management, CSRF protection, XSS prevention. 

### Out of Scope
- Infrastructure configuration (Kubernetes, Docker) — unless provided by operator. 
- Third-party services (PostgreSQL, Redis, NATS) — audit of these services is responsibility of operator or vendor. 
- Sponsor bank integrations — audited separately by bank's security team. 
- Card network direct acquiring (Phase 3) — audited as part of PCI DSS ROC. 

## Audit Methodology
1. **White-Box Review** — Source code review of Go backend and TypeScript frontend. 
2. **Black-Box Testing** — Penetration testing of API endpoints, dashboard, webhooks. 
3. **Architecture Review** — Review of system design, data flow diagrams, trust boundaries. 
4. **Configuration Review** — Review of default configurations, secrets management, TLS settings. 
5. **Compliance Mapping** — Map findings to OWASP Top 10, PCI DSS requirements, and SOC 2 criteria. 

## Deliverables
- **Executive Summary** — High-level findings and risk ratings. 
- **Detailed Findings** — Per-component vulnerabilities with CVSS scores, proof-of-concept, and remediation steps. 
- **Remediation Plan** — Prioritized list of fixes, estimated effort, and responsible teams. 
- **Re-Testing Report** — Verification that critical/high findings are resolved. 
- **Attestation Letter** — Signed statement of compliance for transparency publication. 

## Timeline (Typical)
- **Week 1–2**: Scoping call, documentation review, environment setup. 
- **Week 3–6**: White-box code review + black-box penetration testing. 
- **Week 7**: Findings presentation, remediation planning. 
- **Week 8–10**: Remediation window (operator fixes critical/high findings). 
- **Week 11**: Re-testing of remediated issues. 
- **Week 12**: Final report delivery + attestation letter. 

## Operator Responsibilities
- Provide test environment with production-like configuration. 
- Grant auditors read access to source code (private repository). 
- Provide API keys, test credentials, and sandbox access. 
- Remediate all critical and high findings before launch. 
- Publish audit report (or summary) for transparency. 

## TPT Pay Platform Pre-Audit Checklist
- [ ] All API endpoints require authentication (except health checks). 
- [ ] API keys are hashed (bcrypt) and never stored in plaintext. 
- [ ] Ledger entries are append-only; no UPDATE/DELETE operations. 
- [ ] Webhook signatures use HMAC-SHA256 with secret rotation. 
- [ ] Rate limiting is enforced per API key (Redis-backed). 
- [ ] Fraud rules are configurable and tested. 
- [ ] Error responses never expose stack traces or PII. 
- [ ] TLS 1.2+ enforced for all connections. 
- [ ] Dependency scan (Snyk, OWASP DC) shows no critical vulnerabilities. 
- [ ] Penetration testing tools (OWASP ZAP, Burp Suite) used internally. 

## References
- OWASP Top 10 (2021): https://owasp.org/www-project-top-ten/ 
- PCI DSS v4.0: https://www.pcisecuritystandards.org/document-library 
- CWE Top 25: https://cwe.mitre.org/top25/ 
- NIST SP 800-53: https://nvlpubs.nist.gov/nistpubs/SpecialPublications/NIST.SP.800-53r5.pdf 
- SOC 2 Criteria: https://www.aicpa.org/interestareas/frc/assuranceadvisoryservices/pages/aicpa-soc-2.aspx 

---

**Note:** This is a template. Operators should customize based on their deployment model, jurisdiction, and risk appetite. TPT Pay provides this scaffolding but does not constitute security advice.