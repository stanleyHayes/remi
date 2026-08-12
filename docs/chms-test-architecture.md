# REMI ChMS — Test Architecture and Release Evidence

**Status:** P0 baseline  
**Owner:** ARCHITECT  
**Plan task:** REMI-205  
**Date:** 2026-08-11

## 1. Test objective

The suite must prove domain integrity, authorization and recoverability—not merely that endpoints return 200. High-risk failures are cross-branch/member disclosure, restricted-note leakage, unsafe child pickup, duplicated/mutated gifts, unreconciled close, webhook replay, destructive imports and silent job loss.

## 2. Test layers

| Layer | Scope | Environment | Required on |
|---|---|---|---|
| Domain unit | value objects, state machines, policy predicates, transition rules | pure Go, deterministic clock/IDs | every PR |
| Property/invariant | money splits, adjustments, batch/settlement math, merge graphs, recurrence/timezones, idempotency | Go fuzz/property generators | every PR bounded; scheduled extended run |
| Repository integration | real Mongo validation/indexes/transactions/concurrency/migrations | MongoDB replica set, isolated database per test run | every PR/CI |
| HTTP contract | request validation, error/headers/schema, cursor, ETag, idempotency, auth | real router + replica set; fake clocks/providers | every PR |
| Authorization matrix | role × action × organization/branch/assignment × field class | generated cases against real HTTP handlers | every PR; release artifact |
| Provider contract | Paystack signature/raw body, init/verify adapter, Resend/asset adapters | recorded sanitized fixtures + local mock server; never live charge in CI | every PR; sandbox smoke before release |
| Journey E2E | critical staff/member browser/kiosk flows | production build + API + replica set, anonymized seed | merge/release |
| Migration/reconciliation | dry run, resume, rerun, rollback manifest, counts/checksums/finance totals | production-like sanitized dataset | release candidate |
| Accessibility/visual | keyboard, semantic names/states, contrast, reduced motion, 390px/kiosk | Playwright + axe plus human review | merge/release |
| Load/fault/recovery | Sunday check-in peak, reports, jobs, provider/network failure, restore | production-like staging | release candidate |
| Production smoke | read/create/reverse-safe synthetic journeys and observability | production with dedicated synthetic records | post-deploy |

Tests never call production payment/email recipients or use real member data.

## 3. Fixture system

`apps/api/internal/chms/testkit` will provide deterministic builders for:

- two organizations (isolation proof), three branches and organization-wide records;
- staff principals for every base role, mixed grants, stale/revoked sessions and member principals;
- flexible households: single adult, unrelated adults, guardian/child, child with multiple authorized guardians, no assumed “head”;
- guests/members/inactive people, shared/invalid contacts, duplicates and merge aliases;
- services across DST/timezone boundaries, attendance/headcounts and offline command queues;
- public/private groups, volunteer conflicts and owned/unowned leadership scopes;
- general/restricted care cases, private/anonymous prayer requests and consent states;
- funds, split gifts, anonymous/unresolved donors, batches, provider events, settlements, fees, refunds, chargebacks and closed periods;
- sparse/missing engagement history, suppression and false-positive review outcomes.

Builders generate synthetic names/contacts only. A seed manifest records fixture IDs and expected aggregate totals. Tests use injected clock, ID/random source and provider adapters; security-code tests still prove cryptographic/non-sequential generation through structural checks.

## 4. Authorization matrix

The permission catalogue is machine-readable and generates at minimum:

```text
role × action × own/other organization × allowed/other branch
× assigned/unassigned resource × operational/personal/restricted/child/finance/secret field
× normal/recent-MFA/break-glass session
```

For each case assert status, existence concealment, returned field set, mutation absence/presence, audit result and absence of protected values in logs/errors/outbox. Add explicit horizontal-ID substitution tests to every resource route and cursor/filter scope-binding tests.

Snapshots of allowed permission cases are reviewed; tests fail when new endpoints/actions lack matrix entries.

## 5. Domain invariant suites

### People and households

- person number/external identity uniqueness within correct scope;
- relationship graph accepts legitimate structures without cycles where prohibited;
- merge aliases resolve transitively without cycles and preserve source history;
- archive/status transitions retain finance/legal facts and authorized history;
- headcount can never generate a person or named attendance fact.

### Attendance/check-in

- one current attendance fact per occurrence/person;
- locked attendance only changes through reasoned correction event;
- duplicate/reordered offline commands apply once and return stable per-command results;
- pickup rejects guessed, replayed, expired and unauthorized proof;
- station lock/branch scope prevents later lookup/export.

### Finance

- integer overflow/invalid currency/negative-state rejection;
- split sum equals header total atomically;
- posted contribution fields cannot update or delete;
- reversal/adjustment chains are acyclic and net correctly;
- duplicate/concurrent webhook/import requests post at most once;
- stale/pending events cannot downgrade a verified successful payment;
- batch counter/approver separation and valid transition graph;
- settlement gross/net/fees and offline deposits reconcile;
- closed periods reject writes; reopen requires separate authorized approver;
- statements/exports deterministically trace to posted ledger and adjustment version.

### Care, consent and retention

- restricted note content absent from general list/search/timeline/audit/log/export;
- assignment/consent gates hold after reassignment/withdrawal/session staleness;
- campaign execution rechecks current suppression after audience snapshot;
- retention proposal honors holds and never removes required finance provenance;
- engagement signal always exposes evidence/rule version and excludes individual gift amount.

## 6. Idempotency, concurrency and failure tests

For every command endpoint:

1. same key/body sequentially returns same status/body/resource;
2. same key/body concurrently commits once;
3. same key/different body returns conflict;
4. crash after domain commit/before response can retry safely;
5. outbox delivery duplicates are consumed once;
6. stale `If-Match` cannot overwrite; missing precondition is rejected;
7. provider timeout occurs outside transaction and leaves recoverable intent/job state.

Use failpoints around transaction commit, outbox claim, provider response and worker acknowledgment. Test bounded retries, dead-letter visibility and manual replay authorization.

## 7. Critical E2E journeys

1. Guest intake → reviewed duplicate → person → attendance → assigned first-visit workflow → consent-aware outcome.
2. Household kiosk lookup → child check-in label/security proof → offline sync → authorized pickup → attendance lock/correction.
3. Group leader sees owned roster only, records meeting attendance and cannot enumerate another group.
4. Volunteer schedule conflict → invite → accept/decline → substitute → completion analytics.
5. Private care case assignment → restricted note → reassignment → access revocation/read audit → closure/retention proposal.
6. Cash offering batch → dual count → variance resolution → separate approval/post → deposit reconciliation → period close → statement/export.
7. Paystack intent → signed duplicate/out-of-order events → one contribution → settlement/fee match → refund adjustment.
8. Explainable retention signal → human review/snooze/suppress → care workflow, with no finance detail exposed.
9. Member invitation → MFA/recovery where applicable → profile/consent update → household delegation → own statement → data request.
10. Import dry run → row correction → resumable commit → duplicate merge → count/finance reconciliation → rollback rehearsal.

## 8. Load and service-level test profiles

Exact capacities follow REMI-200 decision data. Until confirmed, tests are parameterized rather than claiming arbitrary production scale:

- `CHECKIN_PEAK_CONCURRENCY`, `CHECKIN_COMMANDS_PER_MINUTE`, offline backlog size/sync burst;
- active people/households, historical attendance/contribution years and branch count;
- report/export row count and concurrent job count;
- provider webhook burst/replay backlog.

Measure p50/p95/p99 latency, error/timeout rate, database operations/explain plans, worker lag, queue depth and recovery time. Pass budgets are recorded before P6 and cannot be relaxed after results without an approved exception.

## 9. Migration and recovery evidence

Every rehearsal produces source/accepted/rejected/created/updated/merged counts; input and manifest hashes; unresolved rows; per-branch/person/attendance/giving totals; duplicate report; elapsed time; resume checkpoint; rollback scope; and stakeholder sample sign-off.

Backup restore drills prove encrypted backup availability, measured RPO/RTO, restored indexes/transactions, audit continuity and finance/report reconciliation. A successful backup job without a restore is not release evidence.

## 10. CI and release gates

### Pull request

- formatting/static analysis, unit/property bounded tests;
- real replica-set repository/HTTP/authorization/contract tests;
- frontend type/build/component tests and impacted Playwright journeys;
- secret/personal-data fixture scan, dependency/container scan and `git diff --check`.

### Release candidate

- full E2E matrix, extended property/fuzz run, provider sandbox, migration rehearsal;
- accessibility/manual screen-reader, responsive/kiosk and performance/load/fault tests;
- restore/rollback exercise, OWASP ASVS review, permission matrix artifact;
- finance zero-variance pilot evidence and role-based UAT.

### Post-deploy

- version/health/readiness, organization isolation synthetic check;
- synthetic guest/check-in and reversible finance-safe smoke where approved;
- worker/webhook/queue/audit metrics, error budget and deployment annotations;
- daily finance/migration reconciliation during hypercare.

No test may mark ChMS complete while a required external policy/UAT/reconciliation gate is missing. Skips fail CI when `REQUIRE_INFRA=1`; release evidence includes command, revision, environment, result and artifact location.

