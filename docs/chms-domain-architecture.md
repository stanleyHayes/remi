# REMI ChMS — Domain and Data Architecture

**Status:** P0 architecture baseline  
**Owner:** ARCHITECT  
**Plan task:** REMI-201  
**Last updated:** 2026-08-11

## 1. Architecture decision

Phase 2 begins as a **modular monolith** in the existing Go API and MongoDB deployment. Each ChMS bounded context owns its collections, commands, queries, policies and events under `apps/api/internal/chms/<context>`. Contexts communicate through explicit application interfaces and a transactional outbox—not direct cross-context collection writes.

This matches the present team and deployment scale, preserves atomic operations inside one MongoDB replica set, and keeps future service extraction possible without accepting distributed-system cost prematurely. Public CMS handlers remain separate adapters; no public endpoint can query ChMS collections directly.

## 2. Bounded contexts

| Context | Owns | May reference, not mutate |
|---|---|---|
| Identity & Access | staff/member identities, role grants, scopes, sessions, MFA | person ID, branch ID |
| People | people, contact points, addresses, households, relationships, lifecycle, tags/custom values, merge aliases | branch ID |
| Participation | service definitions/occurrences, attendance facts, headcounts, check-in sessions | person/household/branch IDs |
| Community | groups, rosters, meetings, volunteer teams/positions, availability, schedules/responses | person/branch/occurrence IDs |
| Care & Workflow | workflow definitions/instances, tasks, contact outcomes, care cases, restricted notes | person/household/assigned-user IDs |
| Giving | funds, contributions, splits, batches, pledges, provider transactions, settlements, reconciliation, periods | person/household/branch IDs |
| Communication & Consent | consent evidence, channel preferences, suppressions, audiences, campaigns, delivery events | person/household IDs |
| Engagement | derived aggregate facts, rule versions, explainable signals, reviewed outcomes | person and source-event IDs |
| Reporting | metric definitions, projections/materialized summaries, report jobs/snapshots | stable source IDs and event sequence |
| Governance | audit events, data requests, retention jobs, import runs, outbox/inbox, feature flags | all subject/resource IDs |

Ownership rule: the context that owns a fact is the only writer. For example, Giving cannot update a person after donor matching; it emits a match/quality event or calls a People command through an approved interface.

## 3. Identity, tenancy and branch strategy

- REMI is the single **organization/tenant** in Phase 2, but every ChMS record carries `organizationId`. This prevents global-query habits and leaves an intentional future boundary.
- `branchId` is nullable only for organization-wide records. Branch-scoped operations require an explicit branch; absence never means “all branches.”
- A person has `homeBranchId` but attendance, group, care and giving facts retain their own branch attribution.
- Authorization evaluates `organization + role + branch/ministry/assignment scope + action + field class`; database filters alone are not the policy engine.
- IDs are MongoDB ObjectIDs internally and opaque lowercase hex strings at the API. Imported/provider IDs live in a separate namespaced `externalIds` structure with unique partial indexes.
- IDs are stable through merge. A losing person ID becomes a `person_aliases` record resolving to the survivor; historical source facts are not rewritten in a risky fan-out transaction.

## 4. Common document envelope

Mutable aggregate roots use:

```text
_id, organizationId, branchId?, schemaVersion,
createdAt, createdBy, updatedAt, updatedBy,
version, archivedAt?, archivedBy?, archiveReason?
```

- `version` implements optimistic concurrency; update commands require the expected value.
- Time is stored as UTC BSON datetime. Local service/report boundaries use an IANA timezone from organization/branch settings.
- Money is `{amountMinor: int64, currency: "GHS"}`. Floating point is forbidden.
- Phone numbers are normalized to E.164 where possible while preserving the entered display value and verification state.
- Email is normalized for matching but the original display form is retained.
- Free text has explicit length limits and classification. Restricted notes never appear in general search or audit diffs.
- Archiving is not universal soft delete. Immutable facts (posted contributions, attendance corrections, audits) use lifecycle/status and linked reversals.

## 5. Core collection catalogue

The fields below are architecture-level contracts. REMI-204 will publish exact request/response schemas and validators.

### 5.1 People

#### `chms_people`

Canonical identity: display/preferred/legal name parts; aliases; photo asset reference; optional DOB with precision (`unknown|year|month|day`); optional gender; contact points; addresses; `homeBranchId`; lifecycle status; tags; custom field values; communication preference summary; data-quality state; archive metadata.

Indexes:

- unique `{organizationId, personNumber}`
- partial unique `{organizationId, externalIds.namespace, externalIds.value}`
- `{organizationId, normalizedEmails.value}`, `{organizationId, normalizedPhones.value}`
- `{organizationId, homeBranchId, lifecycle.status, archivedAt}`
- Atlas Search index for authorized name/contact lookup; sensitive fields excluded

#### `chms_households`

Name/label, shared contacts/addresses, branch, member summary, contribution statement preference and archive metadata. Household membership is modeled separately to support history and multiple household contexts.

#### `chms_relationships`

`fromPersonId`, `toPersonId`, directional `type`, optional household ID, effective dates, source and visibility. Reciprocal display is derived from configuration, not duplicated as an uncontrolled second fact.

Unique active relationship index: `{organizationId, fromPersonId, toPersonId, type, endedAt}`.

#### `chms_membership_events`

Append-only transition facts: person, from/to status, effective time, reason code, note reference, actor and reversal link. Current status on the person is a transactional projection for fast reads.

#### `chms_person_aliases`

Losing person ID → survivor ID, merge decision/run, actor and timestamp. Unique on losing ID and cycle checked.

### 5.2 Participation

#### `chms_service_definitions` / `chms_occurrences`

Definition owns recurrence intent; occurrence owns one start/end, branch, rooms, capacity, status and immutable occurrence key. Recurrence changes never rewrite historical occurrences.

Occurrence indexes: `{organizationId, branchId, startsAt}`, unique `{organizationId, occurrenceKey}`.

#### `chms_attendance`

One person/occurrence fact: checked-in/out times, attendance status, source (`kiosk|operator|roster|import`), station/operator, guest flag, sync command ID and correction chain.

Unique `{organizationId, occurrenceId, personId}`. Corrections append to `chms_attendance_events` and update the current projection transactionally.

#### `chms_headcounts`

Occurrence, category/room, count, source, observed time and correction chain. Headcounts are never expanded into person attendance.

#### `chms_checkin_sessions`

Short-lived station/operator session, authorized branch/occurrences, device label, last activity and lock/revocation. Offline commands use client-generated UUID idempotency keys and monotonic local sequence.

#### `chms_guardian_authorizations` / `chms_pickup_events`

Guardian-child permissions with effective dates and source; pickup events hold check-in reference, releasing operator, authorized person and hashed/random security-code verification result. Plain security codes are not retained after the operational window.

### 5.3 Community and serving

- `chms_groups`: type, branch, privacy/discoverability, capacity, meeting pattern, leaders and lifecycle.
- `chms_group_memberships`: group/person, role, status, requested/joined/ended timestamps and source; one active membership per pair.
- `chms_group_meetings`: dated occurrence linked to group with attendance summary.
- `chms_teams`, `chms_positions`: serving structures, eligibility policy and branch/ministry scope.
- `chms_availability`: person, window, preference/unavailable state and source.
- `chms_service_plans`: occurrence, team needs, status and lock version.
- `chms_assignments`: plan/person/position, response, reminder state and substitution chain; unique active assignment for a position slot/person.

### 5.4 Care and workflow

- `chms_workflow_definitions`: versioned stages, transitions, task templates, timers and allowed scopes. Published versions are immutable.
- `chms_workflow_instances`: subject type/ID, definition/version, stage, owner, due time, state and outcome.
- `chms_tasks`: instance/case, assignee, due time, status, outcome and escalation metadata.
- `chms_contact_events`: channel, purpose, outcome, actor and minimal summary; content/body is not copied from external providers by default.
- `chms_care_cases`: person/household, category, urgency, consent, assignment scope, state and closure.
- `chms_restricted_notes`: encrypted content envelope, case ID, classification, authorized group, author and retention trigger. General audit records only “restricted note created/read/changed,” never the body.
- `chms_safeguarding_refs`: minimal pointer/status/owner to an approved external safeguarding process; no attempt to become a legal case-management system.

### 5.5 Giving

#### `chms_funds`

Name/code, restriction/designation category, branch availability, active dates and accounting mapping. Fund deactivation cannot orphan historical splits.

#### `chms_contributions`

Append-only posted header: receipt number, contribution time, donor attribution (`person|household|anonymous|unresolved`), source/method, branch, batch/provider transaction reference, currency, total, posting period, state and adjustment/reversal link.

#### `chms_contribution_splits`

Contribution, fund, amount minor and campaign/pledge reference. Invariant: active split sum equals contribution total. Indexed by fund/posted date and contribution ID.

#### `chms_batches`

Branch/date, assigned counters, declared/entered totals, denomination summary, variance/reason, evidence assets, status (`open|counted|approved|posted|deposited|reconciled|voided`), approver and lock version. State machine enforces separation of duties.

#### `chms_provider_transactions` / `chms_webhook_inbox`

Provider reference, normalized payment state, amount/currency, payer hint, contribution link, settlement reference and raw-payload encrypted/retention-limited pointer. Inbox is unique on provider event ID/hash, stores signature verification and processing attempts.

#### `chms_settlements` / `chms_reconciliation_items`

Provider/bank settlement totals and fees, source import, matched contribution/batch/deposit items, exceptions, resolutions and close approval. Reconciliation never changes contribution amount.

#### `chms_pledges`, `chms_finance_periods`, `chms_statement_runs`

Pledge schedules/progress; open/closed periods with reopen approval; deterministic statement runs holding filter, generation version, document hash and delivery audit.

Finance indexes include unique receipt/provider/event identifiers, `{organizationId, postedAt, state}`, `{organizationId, donor.type, donor.id, postedAt}`, and `{organizationId, splits.fundId, postedAt}`. Compound indexes must be validated against actual aggregation explain plans.

### 5.6 Consent, communications and engagement

- `chms_consent_events`: append-only grant/withdraw evidence by person, purpose, channel, notice version, source and time; current consent is a projection.
- `chms_suppressions`: normalized destination, channel, reason and provenance; checked at send execution time.
- `chms_segments`: authorized versioned query definition; materialization records the rule version and as-of time.
- `chms_campaigns`, `chms_deliveries`: approved content/template, audience snapshot, schedule, provider result and opt-out events.
- `chms_engagement_rules`: versioned, human-readable inputs/windows/exclusions and owner.
- `chms_engagement_signals`: person, rule version, evidence source IDs/times, generated/expiry time, state and reviewer outcome. No general-purpose hidden numeric faith score.

### 5.7 Governance and platform

- `chms_audit_events`: append-only actor identity/type, action, resource/subject IDs, branch, reason, request ID, safe changed-field names and outcome. Bodies/secrets/restricted note text excluded.
- `chms_outbox`: aggregate/event IDs, type/version, payload, occurred time, delivery attempts and state. Unique event ID.
- `chms_inbox`: consumer/event ID, received/processed state. Unique per consumer/event for idempotency.
- `chms_import_runs`, `chms_import_rows`: source/hash, mapping version, validation state, proposed/resolved IDs, row errors, commit/rollback manifest and actor.
- `chms_data_requests`: requester verification, request type/scope, due dates, tasks, outcome and evidence.
- `chms_retention_jobs`: policy/version, candidate query, review/approval, processed counts and manifest.
- `chms_report_snapshots`: report/metric versions, as-of time, authorized scope, source watermark, artifact hash and expiry.

## 6. Event and timeline model

The member profile timeline is a **read projection**, not a writable mega-collection. Contexts emit versioned facts such as:

- `people.person.created.v1`, `people.membership.changed.v1`, `people.household.joined.v1`
- `participation.attendance.recorded.v1`, `community.group.joined.v1`, `community.assignment.completed.v1`
- `care.workflow.stage_changed.v1`, `giving.contribution.posted.v1`

The timeline stores safe summary metadata plus source context/ID. Authorization is re-evaluated when reading the underlying record; projection presence never grants access. Giving amount and restricted-care text are not copied into the general timeline.

Outbox dispatch is at-least-once, so all consumers are idempotent. Events have `eventId`, `eventType`, `eventVersion`, `aggregateType/Id/version`, organization/branch, occurred/recorded times, actor/request IDs and the minimum payload.

## 7. Consistency and transaction boundaries

MongoDB transactions are required for:

- aggregate change + audit event + outbox event;
- person merge alias/current-state update and merge manifest;
- attendance correction and current projection;
- contribution header + splits + posting/audit/outbox;
- batch state transition and posting;
- consent event and current preference projection.

External calls never execute inside a database transaction. Commands persist intent/outbox first; workers call providers and record idempotent outcomes. Every public mutation accepts/derives a request ID; payment/webhook/import/offline mutations require a stable idempotency key.

## 8. Query, search and reporting strategy

- Operational reads use context-owned repositories and explicit projections/DTOs—not raw `bson.M` across the ChMS boundary.
- Cursor pagination uses a stable compound sort (`createdAt/_id` or domain equivalent). Offset pagination is not used for growing ledgers/timelines.
- Atlas Search indexes only approved person lookup fields and filters by organization/authorization scope. Restricted notes, giving and child incident content are excluded.
- Reporting consumes immutable events/source collections into daily/materialized projections. Each projection records a source watermark and rebuild version.
- Executive aggregates enforce small-cohort privacy thresholds where applicable. Finance reports reconcile from posted contributions/splits and adjustments, never from engagement projections.

## 9. Retention architecture

Exact periods require REMI/DPC/accounting approval in REMI-202/203. Architecture supports policy-driven triggers rather than hard-coded dates:

| Data category | Trigger/model |
|---|---|
| Active person/household | Retain while ministry relationship/purpose is active; periodically verify quality and consent |
| Archived/inactive person | Policy review after inactivity; preserve legally/operationally necessary finance facts while anonymising or restricting unrelated profile data when approved |
| Attendance | Time-limited identifiable operational history; longer aggregate trends can be retained without identity |
| Child check-in security code/session | Expire promptly after operational/safeguarding window; retain minimal pickup audit under approved policy |
| Care/prayer/restricted notes | Short, category-specific retention from closure; reviewer approval before destruction; no indefinite “just in case” storage |
| Contributions/statements/reconciliation | Accountant/legal-policy period; immutable and access-restricted; profile erasure does not corrupt required finance records |
| Consent and suppression evidence | Retain enough evidence to honor grant/withdrawal and prevent unwanted contact |
| Audit/security logs | Purpose-specific operational/security period; safe metadata only |
| Imports/provider payloads | Raw staging/payload expires quickly after reconciliation; provenance/hash/manifests remain as approved |

Retention jobs operate as reviewable proposals, support legal/incident holds, emit manifests and are tested on backups/restores. Hard deletion is never the default UI action.

## 10. Data evolution and migrations

- Every document has `schemaVersion`; readers tolerate supported older versions during rolling deploys.
- Migrations are versioned Go commands with dry-run, count/checksum report, bounded batches, resume token and explicit confirmation in production.
- Expand/migrate/contract: add compatible shape, backfill/reconcile, switch reads/writes, then remove obsolete fields in a later release.
- Unique indexes are built only after a duplicate report and resolution rehearsal.
- Import runs write staging rows first. Commit creates canonical records with provenance; rollback manifest identifies created IDs and reversible changes.
- Production migration never runs automatically on API boot.

## 11. Package shape

```text
apps/api/internal/chms/
  platform/        ids, money, clocks, transactions, outbox, policy contracts
  people/          domain, application commands/queries, mongo adapters, http adapter
  participation/
  community/
  care/
  giving/
  communication/
  engagement/
  reporting/
  governance/
```

Each context exposes domain types and application interfaces. HTTP request types, Mongo documents and provider payloads stay in adapters. Tests live beside each layer; cross-context journeys use server integration tests.

## 12. Architecture acceptance checks

- No context writes another context's collection.
- Every mutation transactionally emits safe audit/outbox records.
- Every protected query takes an authorization principal/scope; repositories cannot accidentally perform an unscoped “all organizations” query.
- Person merges preserve aliases and historical facts; household changes preserve effective history.
- Headcount cannot create person attendance.
- Posted contribution totals/splits cannot mutate; all fixes link adjustments/reversals.
- General timeline/search/retention signals contain no restricted note text or individual giving amount.
- Import, webhook and offline commands are retry-safe and idempotent.
- Reporting states metric version, as-of/source watermark and reconciliation rule.
- Schema migrations are dry-runnable, resumable, reconciled and never implicit at server startup.
