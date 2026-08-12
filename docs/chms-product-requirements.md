# REMI Church Management System — Product Requirements

**Status:** P0 baseline for stakeholder review  
**Owner:** ARCHITECT  
**Plan task:** REMI-200  
**Last updated:** 2026-08-11

## 1. Product intent

REMI ChMS will be the trusted operational system for knowing and caring for people, running services and ministries, recording contribution activity, coordinating follow-up, and understanding church health across branches. It must reduce spreadsheet duplication without turning pastoral ministry into surveillance or finance into an editable CRUD table.

The public website/CMS remains independently operable. ChMS data is private by default and is never exposed through public content endpoints.

## 2. Outcomes and success measures

| Outcome | Baseline to capture in pilot | Release target |
|---|---:|---:|
| One trustworthy person record | Duplicate and incomplete-contact rates | ≥95% of active records pass required quality rules; reviewed duplicate queue has an owner |
| Visitor follow-up is closed-loop | First-visit records with an owner/outcome | ≥95% assigned within one working day; overdue work visible, never silently closed |
| Attendance is useful and timely | Time from service close to approved attendance | ≥95% approved within 24 hours; correction history retained |
| Giving reconciles | Difference between posted gifts, batches and provider/bank settlements | Zero unexplained variance at period close |
| Staff access is appropriate | Access-control test matrix and quarterly review | 100% protected operations deny unauthorized roles/scopes; stale privileged access removed |
| Retention leads to humane care | Explainable alerts with reviewed outcomes | Every individual alert shows evidence and supports suppress/snooze; no general user sees gift amounts as a retention signal |
| Members control their data | Profile/preference requests completed | Changes are attributable, consent withdrawals take effect, and data requests follow the approved SLA |

Targets are pilot hypotheses until church leadership assigns owners and baselines. They measure operating quality, not spiritual maturity.

## 3. People and roles

| Persona | Primary jobs | Important restrictions |
|---|---|---|
| Executive leadership | See aggregate health, branch trends and outstanding risks | No automatic right to restricted care notes or child incident detail |
| Senior pastor / assigned care pastor | Coordinate pastoral care and appropriate follow-up | Sees only assigned/authorized care cases; private prayer consent applies |
| Church administrator | Maintain people, households, membership stages and workflows | Cannot post/approve finance unless separately granted |
| Membership/assimilation team | Capture guests, run next-step workflows and resolve data quality | No gift amounts or restricted care notes |
| Check-in operator | Find households, check people in/out and add minimal guest data | Station/occurrence-scoped; no profile browsing or exports |
| Children's ministry lead | Manage authorized child check-in and pickup | Branch/event scoped; safeguarding records separated and tightly restricted |
| Group/ministry leader | Manage owned roster, attendance, communications and care handoffs | Only owned groups; private contact and household fields follow policy |
| Volunteer coordinator | Manage teams, eligibility, availability and schedules | No giving or pastoral records |
| Finance counter | Create and count an assigned batch | Cannot approve own batch or alter posted entries |
| Finance administrator | Match donors, manage funds, reconcile and report | Corrections are append-only; period close/reopen requires approval |
| Finance approver | Approve batches, reconciliation and period reopen | Cannot be the sole counter on the same batch |
| Auditor / board reviewer | Read approved reports and immutable evidence | Time-bounded access; no operational edits |
| Data protection supervisor | Review processing, requests, retention, incidents and access | Does not receive blanket ministry/finance access |
| Member / household delegate | Maintain allowed profile data and view own participation/giving | No staff-only notes, inferred signals, other adult data without delegation |
| Platform administrator | Configure system and identities | Technical access does not imply permission to view care or finance content |

One person may hold multiple roles, but permissions are additive only through explicit grants and scoped assignments. High-risk combinations are detected and reviewed.

## 4. Canonical terminology

- **Person:** one human identity, including guests and children. “Member” is a lifecycle state, not the base entity.
- **Household:** a practical grouping for shared contact, giving or check-in; it does not assert marriage, parentage or headship.
- **Relationship:** a typed, directional link between people, with effective dates where needed.
- **Branch:** a reporting and authorization boundary for a REMI location. A person may have a home branch and participate elsewhere.
- **Service definition / occurrence:** the recurring template versus one dated instance where attendance happens.
- **Attendance:** a person-to-occurrence fact with source and correction history. **Headcount** is an aggregate observation and must not create fake people.
- **Check-in:** arrival/departure workflow; not synonymous with final approved attendance.
- **Group:** ongoing community/class/ministry roster. **Team** is a serving roster with positions and schedules.
- **Workflow:** an owned sequence of stages/tasks. It does not change a person's status without an explicit authorized action.
- **Care case:** privacy-scoped pastoral need with consent, assignment and an accountable outcome.
- **Contribution:** an irrevocably posted gift fact. A correction is a linked reversal/adjustment, never an overwrite.
- **Fund/designation:** where a contribution is directed. This is distinct from a bank account or general-ledger account.
- **Batch:** a controlled group of offline contributions counted and deposited together.
- **Settlement:** the provider/bank movement used to reconcile online transactions or deposits.
- **Pledge:** a non-coercive intention toward a campaign/fund; not a receivable unless professional accounting policy says otherwise.
- **Engagement signal:** explainable evidence that a human may review. It is not a spiritual score or a prediction of belief.

## 5. Critical journeys

### 5.1 First-time guest to connected participant

1. Capture minimal guest information through visit form, assisted check-in or import with notice/consent.
2. Match or create a person without silently merging uncertain identities.
3. Record visit and start the configured first-visit workflow.
4. Assign an owner and due date; honor channel consent and contact preferences.
5. Record contact attempt/outcome and offer an appropriate next step.
6. Track return attendance, group/next-step participation and membership transitions as separate facts.
7. Close or suppress the workflow with a reason; never mark someone inactive solely because an automation fired.

### 5.2 Household service check-in and secure child pickup

1. Authorized station loads only the current branch/occurrence and locks after inactivity.
2. Operator or household delegate finds the household without exposing a full directory.
3. Select attendees; unknown guests can be created with minimal data and later reviewed.
4. For children, validate authorized guardian and issue a random, non-sequential security code/label.
5. Queue actions safely during connectivity loss and display sync state.
6. Pickup validates authorization/code and logs the releasing operator/time.
7. Attendance is reviewed and locked; later corrections require a reason and remain auditable.

### 5.3 Offering/tithe from receipt to reconciliation

1. Finance opens a branch/date batch and assigns two counters where policy requires.
2. Counters enter cash/cheque/mobile-money contributions and fund splits; totals must balance.
3. Unknown donors remain unresolved or anonymous—never guessed.
4. A separate approver reviews variance/evidence and posts the batch.
5. Posted entries become immutable; fixes create linked adjustments.
6. Deposit or licensed-provider settlement is imported and matched.
7. Finance resolves exceptions, closes the period and produces reconciled fund/deposit reports.
8. Members receive receipts/statements only through authorized, verified channels.

### 5.4 Possible disengagement to pastoral care

1. A configurable rule observes a meaningful change using permitted attendance/group/serving evidence.
2. The queue explains the exact dates and signals, confidence/data gaps, and previous suppressions.
3. Authorized staff reviews rather than automatically contacting or changing status.
4. Staff suppresses, snoozes or assigns a consent-aware care task.
5. Contact and outcome are recorded minimally; unrelated private detail is not copied into the general timeline.
6. Aggregate retention reporting uses thresholds and cohorts, not public individual rankings.

## 6. Functional capability boundary

### Included

- People, households, relationships, membership lifecycle, tags, custom fields, search, segmentation, import, merge and archive.
- Branch/service occurrences, named attendance, headcounts, kiosk/offline check-in and secure child check-in/out.
- Groups, ministry rosters, volunteers, positions, availability, schedules, responses and attendance.
- Visitor assimilation, pastoral cases, prayer linkage, tasks, ownership, due dates, outcomes and notifications.
- Contributions for offering, tithe and configurable funds; cash/cheque/mobile-money/online sources; batches; Paystack event matching; funds, pledges, receipts/statements, settlements, reconciliation, period close and accounting export.
- Explainable engagement/retention cohorts and human-reviewed care queues.
- Consent-aware communication orchestration, member identity and self-service.
- Operational/leadership/finance dashboards, authorized exports, migration tools, audit and data-subject workflows.

### Explicitly not in the initial ChMS release

- Payroll, procurement, expenses, accounts payable/receivable, asset register, budgeting or a full general ledger.
- Storing card numbers, CVVs, mobile-money PINs, provider secrets in member records, or operating as a payment service provider.
- Clinical/medical case management, legal safeguarding adjudication or background-check screening itself.
- Biometric/facial recognition attendance, location tracking, covert communication monitoring or public member directories by default.
- Automated spiritual-worth, generosity, discipline or “faithfulness” scores; automated adverse membership or pastoral decisions.
- Native mobile apps, worship song licensing/planning, facility maintenance and inventory in P0–P7. Integration-ready boundaries are retained.

## 7. Data and privacy classes

| Class | Examples | Default handling |
|---|---|---|
| Operational | service definitions, group names, aggregate counts | Authorized staff; may be broadly visible inside admin |
| Personal | identity, phone/email/address, household links, attendance | Purpose-limited role/scope access; exports audited |
| Sensitive ministry | prayer linkage, care category/notes, membership reasons | Separate authorization, assignment scope, tighter retention and no bulk export by default |
| Child/safeguarding | minor identity, guardian/pickup authorization, incident reference | Strict branch/event roles, minimal display, no general search/export |
| Financial | contribution amounts, donor mapping, batches, settlement evidence | Finance roles only; append-only posting and separation of duties |
| Authentication/secrets | hashes, MFA secrets, recovery tokens, provider keys | Never returned/logged; encrypted/hashed and rotated as appropriate |

P0 must assign every field a purpose, collection source, consent/lawful basis as advised, access roles, retention trigger, export behavior and deletion/anonymisation rule. REMI must register/maintain status with Ghana's Data Protection Commission, name a data protection supervisor, publish notices and validate international processor transfers before ChMS production use.

## 8. Quality attributes

- **Security:** deny-by-default server authorization; branch/assignment and field policies; MFA for privileged/finance roles; audit access without logging sensitive values; rate limiting, webhook signatures, idempotency and OWASP ASVS review.
- **Finance integrity:** all money stored as integer minor units plus ISO currency; totals/splits reconcile; posted rows immutable; correction chains and period locks; dual control; deterministic reports trace to ledger entries.
- **Reliability:** transactional outbox for side effects, retry-safe jobs/webhooks, observable dead letters, point-in-time backups and rehearsed restores.
- **Performance:** product targets are specified against measured congregation size in P0; check-in remains responsive during Sunday peak and tolerates temporary network loss.
- **Accessibility:** WCAG 2.2 AA critical journeys, keyboard/assistive-tech usable, reduced motion, visible state beyond color and kiosk touch targets.
- **Mobile usability:** all high-frequency operator flows work at 390px; kiosk layouts support touch and sunlight-appropriate contrast.
- **Data quality:** provenance and external IDs; dry-run imports; reviewed merges; no destructive bulk action without preview and outcome manifest.
- **Observability:** structured request IDs, policy denials, job/webhook lag, check-in sync, reconciliation exceptions and SLO alerts without personal data leakage.

## 9. Reporting rules

Every KPI needs a dictionary entry containing owner, source facts, formula, unit/grain, timezone, included/excluded statuses, freshness, privacy threshold and reconciliation test. Dashboards display the as-of time and caveats. Attendance headcount and named attendance are separate measures. Contribution reporting uses posted entries and linked adjustments, never current editable form values.

## 10. Decisions required before dependent implementation

These are gates, not blockers to P0 documentation and platform work:

1. Confirm REMI's legal entity, DPC registration/status, data protection supervisor and approved privacy/retention policy.
2. Confirm branches/campuses, expected active people, peak concurrent check-ins, finance users and historical import volumes.
3. Approve membership stages/reasons, household terminology, child check-in policy and safeguarding escalation owner.
4. Approve the contribution-subledger boundary, fiscal year, funds, receipt policy, counting/approval matrix, bank settlement format and accounting export target with the church's accountant.
5. Confirm whether member giving statements are informational or must meet a specific tax/receipt format.
6. Confirm communication providers and consent language for email, SMS and WhatsApp.
7. Approve retention windows/signals, pastoral owners, privacy exclusions and review cadence.
8. Name pilot branch, parallel-run period, UAT representatives and final go-live authority.

## 11. Release gates

No ChMS production launch occurs until:

- DPC/compliance pack and processor inventory are approved by REMI's accountable owner.
- Role/branch/field authorization matrix tests are green, including finance, minors and pastoral-care negative cases.
- Full migration rehearsal reconciles counts and finance totals with sampled stakeholder sign-off.
- Finance property/invariant tests pass and pilot closes with zero unexplained variance.
- Backup restore, provider outage, webhook replay, offline check-in and rollback exercises pass.
- WCAG, responsive, performance and security gates pass in production-like infrastructure.
- Every operational persona completes UAT; SOPs/training/support ownership are accepted.
- Signed go-live decision records known limitations, rollback criteria and 30-day hypercare owners.

## 12. Research notes

- [Planning Center's rollout guidance](https://support.planningcenteronline.com/hc/en-us/articles/1260804626129-Rolling-Out-Planning-Center-Products) reinforces People as the hub, staged rollout, funds/giving, check-in, groups and staff training.
- [Planning Center's product support map](https://support.planningcenteronline.com/hc/en-us) shows the mature boundary among people, secure check-in, giving/statements, groups, registrations, schedules and member-facing access.
- [Pushpay's ChMS overview](https://pushpay.com/product/chms-software) validates unified profiles, follow-up, attendance, volunteers, communication and reporting as a coherent operational set.
- [Ghana DPC organizational guidance](https://dataprotection.org.gh/for-organisations/) explicitly calls for registration, a trained data protection supervisor, privacy notices, data quality, purpose limitation, security and data-subject participation.
- The official [Data Protection Act, 2012 (Act 843)](https://dataprotection.org.gh/wp-content/uploads/2025/05/Data-Protection-Act-2012-Act-843.pdf) is the legal baseline to map with qualified Ghanaian advice.
- [Bank of Ghana Act 987 materials](https://www.bog.gov.gh/fintech_publications/payment-systems-and-services-act-2019-act-987/) support using an authorized payment provider rather than making REMI a payment processor.
- [OWASP ASVS](https://devguide.owasp.org/en/11-security-gap-analysis/01-guides/02-asvs/) supplies verification categories for access control, logging, data protection, communications, business logic and APIs.

