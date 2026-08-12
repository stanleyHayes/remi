# REMI ChMS — Security, Privacy and Access-Control Baseline

**Status:** P0 baseline; organizational/legal approval required  
**Owner:** ARCHITECT  
**Plan task:** REMI-202  
**Last updated:** 2026-08-11

This document is an engineering control plan, not legal advice. REMI must validate it with its Ghana data protection supervisor and professional advisers before production.

## 1. Control objectives

1. Collect only data needed for a stated ministry/operational purpose and tell people how it is used.
2. Make access depend on the user's role, branch/ministry/assignment scope, action and field class—not merely possession of an admin login.
3. Keep pastoral care, children/safeguarding and financial data separated from the general member profile.
4. Produce safe, immutable evidence of access and changes without copying sensitive content into logs.
5. Support access, correction, consent withdrawal/objection and approved retention/anonymisation workflows.
6. Prevent payment credentials from entering REMI systems and reconcile provider events without trusting browser redirects.

## 2. Ghana Act 843 engineering map

The [Data Protection Commission's organizational guidance](https://dataprotection.org.gh/for-organisations/) calls out registration, a data protection supervisor, privacy policy/notices, data quality, purpose specification, openness, security safeguards and data-subject participation. Engineering evidence is mapped as follows:

| Principle/obligation | REMI evidence/control | Production owner |
|---|---|---|
| Accountability / registration | DPC registration record, named supervisor, processing register, quarterly control review | REMI leadership + data protection supervisor |
| Lawfulness and collection limitation | Field-level processing inventory; form notice/version; source and consent/lawful-basis record as advised | Product owner + supervisor |
| Purpose specification | Purpose IDs on consent, campaigns, exports and care workflows; prevent incompatible reuse | Data steward |
| Data quality | Member correction/self-service, source provenance, duplicate queue, review dates | Membership administrator |
| Openness | Layered public/member notices listing categories, purposes, recipients/processors, transfers, retention and rights | Supervisor |
| Security safeguards | Access policy engine, MFA, encryption, audit, backups, incident response, provider due diligence | Security/technical owner |
| Data-subject participation | Verified request workflow for access/correction/portability/withdrawal and documented exceptions | Supervisor |
| Processor/international transfer governance | Cloudinary, Resend, MongoDB Atlas, Vercel, Render and Paystack inventory with contract/location/safeguard review | REMI leadership |

No field ships until the processing inventory records: owner, subject category, purpose, source, required/optional, lawful/consent basis as approved, readers, processors/transfers, retention trigger and rights behavior.

## 3. Authorization model

Policy input:

```text
principal { userId, organizationId, roles[], grants[], sessionAssurance }
resource  { type, organizationId, branchId?, ministryId?, assignees[], fieldClass }
request   { action, selectedFields[], purpose?, requestId }
context   { currentBranch?, breakGlass?, device/station?, time }
```

Decision is deny by default. A role grants candidate actions; scope must intersect; field policy may remove fields or deny the command; high-risk actions require session assurance and sometimes separation of duties.

Core roles are `platform-admin`, `executive`, `pastor`, `membership-admin`, `assimilation-worker`, `checkin-operator`, `children-lead`, `group-leader`, `volunteer-coordinator`, `finance-counter`, `finance-admin`, `finance-approver`, `auditor`, `data-protection-supervisor`, and `member`. Custom roles compose named permissions but cannot bypass immutable prohibitions.

### Field classes

- `operational`: broad authorized staff visibility.
- `personal`: explicit person/household read permission and scope.
- `sensitive-ministry`: assigned pastor/care team only; no bulk export/search.
- `child-safeguarding`: occurrence/branch and safeguarding-grant scoped.
- `financial`: finance grant; amounts masked/absent outside finance.
- `secret`: never returned; managed through dedicated rotation flows.

### Hard prohibitions

- Check-in operators cannot browse people after the station session or export rosters.
- Group leaders see owned-group fields only and never giving/care data.
- General executive access yields aggregates, not restricted case notes or child incidents.
- Technical platform administration does not imply pastoral or finance read access.
- A finance counter cannot approve their own batch; reopening a closed period needs a distinct approver and reason.
- Members cannot see another adult's data without explicit, revocable household delegation.
- Break-glass access is time-limited, reasoned, MFA-protected, alerted and reviewed; it cannot export finance or restricted notes by default.

## 4. Threat model

| Threat | Principal controls | Required verification |
|---|---|---|
| Stolen staff session | MFA for privilege, short idle/absolute limits, rotation/revocation, device/session list | Reuse/revocation/session fixation tests |
| IDOR/cross-branch access | Server policy on every object/query, organization+scope repository guards | Full role × action × branch negative matrix |
| Insider bulk exfiltration | Field masking, export permission/preview, row limits, asynchronous artifact expiry, audit/alerts | Unauthorized export and unusual-volume tests |
| Pastoral note leakage | Separate encrypted store, assignment access, excluded search/log/timeline, read audit | Search/export/log snapshot assertions |
| Child pickup misuse | Random code, authorized guardian, station scope, operator lock, minimal display | Guessing/replay/offline/conflict tests |
| Finance tampering | Append-only ledger, transactions, dual control, period lock, reconciliation, adjustment chains | Invariant/property and separation tests |
| Forged/replayed payment webhook | Provider signature, raw-body verification, unique event ID, amount/currency/reference match | Invalid signature/reorder/replay tests |
| CSV/formula/malware injection | Staged parse, MIME/size scan, field validation, CSV formula escaping, signed asset access | Malformed/bomb/formula tests |
| Log/backup disclosure | Data-minimized structured logs, encrypted backup, access/restore audit, no secrets | Log fixture scan and restore drill |
| Automated retention harm | Explainable evidence, authorized human review, suppression, no gift amount/general faith score | Sparse/bias/privacy and permission tests |
| Provider outage or duplicate jobs | Transactional outbox/inbox, idempotency, bounded retry, dead letters | Fault/retry/recovery tests |

OWASP ASVS categories for access control, validation, cryptography, logging, data protection, communications, business logic and APIs form the security verification checklist.

## 5. Authentication and session requirements

- Staff invitations are single-use, hashed and expire. Privileged and finance roles require MFA enrollment before first protected action.
- Passwords are rate-limited and stored with a modern adaptive password hash. Recovery revokes prior sessions and is enumeration-resistant.
- Member and staff audiences are distinct; a member token cannot be upgraded to a staff principal.
- Access tokens are short-lived; refresh/session records are revocable, rotated and device-labelled. Role/scope changes invalidate affected sessions.
- Sensitive actions require recent authentication: MFA changes, exports, finance posting/approval, period reopen, restricted-care access grants and break glass.
- Service/job identities receive minimum audience/scopes, rotated secrets and no interactive login.

## 6. Data protection

- TLS everywhere; HSTS and secure cookie/header policy at frontends/API.
- Platform storage/backups encrypted; restricted-note contents use application-envelope encryption with versioned keys and separately controlled key access.
- Provider credentials remain only in deployment secret stores. Never persist card numbers, CVV, mobile-money PINs, password/MFA secrets or invitation tokens in logs/audit.
- Production data is forbidden in local/test fixtures. Support uses anonymized exports or audited, time-boxed access.
- Downloads use short-lived signed URLs, content disposition, safe MIME handling and expiry. Report artifacts are encrypted/private and access-checked at download time.
- Search indexes and analytics omit restricted notes, child incidents and individual financial amounts unless a separate finance-only projection is approved.

## 7. Audit and monitoring

Audit events include actor/identity type, organization/branch, action, resource/subject ID, safe field-name diff, outcome/denial reason, purpose/reason where required, request/session ID and timestamp. They exclude secret values, full request bodies, care-note text and unnecessary contact data.

Alert on repeated authorization denial, privilege changes, MFA removal, break glass, large/export bursts, batch/period reopen, reconciliation overrides, webhook signature failure, excessive donor rematching, retention jobs and audit pipeline failure. Audit storage is append-only with integrity checks and access restricted to approved reviewers.

## 8. Rights, consent and retention procedures

### Verified data request

1. Record request without exposing whether an unrelated person exists.
2. Verify identity through an approved channel; record method, not reusable secrets.
3. Locate data by canonical/merge alias/external ID across contexts.
4. Route legal/finance/safeguarding exceptions to the supervisor; engineers do not invent exceptions.
5. Generate a reviewed, scoped export or correction plan; log disclosure/correction.
6. Close with evidence and due-date metrics.

### Consent withdrawal

Append a withdrawal event, update the current projection and suppression list atomically, stop future matching communications at execution time, and retain only approved evidence necessary to honor the withdrawal. Withdrawal does not erase lawful finance records or unrelated purposes automatically.

### Retention/anonymisation

Policy jobs first produce candidate counts and samples for an accountable approver. Holds and finance/legal policy are evaluated. Execution is batched, resumable and emits a manifest; aggregates are rebuilt and restore behavior tested. Restricted-care and raw import/provider data use shorter category-specific periods. Exact durations require REMI approval.

## 9. Incident and breach runbook

1. Detect and open an incident with severity, systems/data categories and incident commander.
2. Contain: revoke sessions/keys, disable affected integration/feature flag, preserve safe evidence; do not destroy logs.
3. Assess affected subjects, records, branches, processors, encryption and likely impact with the data protection supervisor.
4. Engage provider/security/legal leadership and follow Ghana DPC reporting/notification requirements as professionally advised.
5. Recover from known-good state, rotate, reconcile finance/audit completeness and increase monitoring.
6. Document decisions/timeline, communicate approved notices, complete subject support and test corrective actions.

The engineering team must not promise a statutory notification window in code/docs until REMI's adviser confirms the current legal requirement and responsible authority.

## 10. Release evidence

- DPC registration/status, named supervisor and approved processing/processor/transfer inventory.
- Versioned privacy/member/guest/check-in/giving/care notices and consent copy.
- Machine-readable permission matrix with route/action/field/branch tests.
- Threat-model review and OWASP ASVS verification record.
- MFA/session/key/backup/restore evidence and incident tabletop.
- Authorized export, retention, request, break-glass and audit-review runbooks.
- Finance separation/invariant results; child pickup/offline tests; restricted-note leakage scans.
- Signed exceptions with owner, expiry and compensating control—no permanent undocumented waiver.

