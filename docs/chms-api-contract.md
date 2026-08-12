# REMI ChMS API Contract v1

**Status:** P0 contract baseline  
**Owner:** ARCHITECT  
**Plan task:** REMI-204  
**Base path:** `/api/chms/v1`  
**Content type:** `application/json`

This contract governs ChMS endpoints only. Existing public/CMS `/api/*` routes remain backward compatible and isolated. Exact schemas will be mirrored in generated OpenAPI as implementation begins; this document defines behavior that generated code and tests must preserve.

## 1. Contract rules

### Authentication and authorization

- Staff routes require an access token with `aud=remi-chms-staff`; member routes require `aud=remi-member`.
- Every handler authorizes organization, role, action, branch/ministry/assignment scope and requested fields server-side.
- Resource existence is concealed with `404` when returning `403` would disclose an unauthorized person/case/financial record.
- Finance posting/approval, period reopen, restricted-care grants, exports and break glass require a recently MFA-verified session.
- Responses contain only fields authorized for that request. Omitted protected fields are not returned as `null` placeholders.

### Headers

| Header | Direction | Requirement |
|---|---|---|
| `Authorization: Bearer …` | request | Required except signed provider webhook and specifically public/member invitation flows |
| `X-Request-ID` | both | Client UUID accepted; server generates if absent and always returns one |
| `Idempotency-Key` | request | Required for create/post/transition, import, offline check-in and provider-affecting commands; 8–128 printable characters |
| `If-Match: "<version>"` | request | Required for mutable aggregate updates and state transitions |
| `ETag: "<version>"` | response | Returned for versioned aggregate resources |
| `X-Reason` | request | Required for protected overrides, reversal, reopen, merge and break-glass actions; body field is preferred where structured |

### Idempotency

- Scope key: `organizationId + principal/client identity + method + normalized route + Idempotency-Key`.
- The server stores request-body hash, response status/body/resource reference and expiry.
- Same key/hash returns the original result; same key/different hash returns `409 idempotency_key_reused`.
- Concurrent identical commands resolve to one committed result.
- Finance, webhook, import and offline attendance identities are retained for the approved domain retention period, not a short generic cache window.

### Optimistic concurrency

Mutable aggregate responses contain integer `version` and ETag. A stale/missing `If-Match` returns `428 precondition_required` or `412 version_conflict` with current version, never silently overwriting data. Append-only events use idempotency rather than update versions.

### Pagination

Lists return:

```json
{
  "items": [],
  "page": { "nextCursor": "opaque-or-null", "hasMore": false, "limit": 50 },
  "meta": { "requestId": "uuid", "asOf": "RFC3339" }
}
```

- `limit` defaults to 50, maximum 200 (exports use asynchronous jobs).
- `cursor` is opaque, signed/versioned and bound to organization, authorization scope, sort and normalized filters.
- Stable sorts include a unique tiebreaker. Unknown sort/filter fields return `400`; no arbitrary Mongo query syntax is accepted.
- Changes between pages may shift live data; `asOf`/snapshot query mode is used where deterministic report/export behavior is required.

### Filtering and search

Repeated query params represent OR within a field; distinct fields combine with AND unless endpoint documentation says otherwise. Date ranges are half-open `[from,to)` in RFC3339. Search strings are length-limited and authorized search indexes exclude restricted/financial content. Server returns applied normalized filters in `meta.filters` for reportable queries.

### Money and time

```json
{ "amountMinor": 25000, "currency": "GHS" }
```

Floating-point money is invalid. Timestamps are UTC RFC3339; local date boundaries require explicit IANA `timezone`. Dates without times use `YYYY-MM-DD` and never undergo timezone conversion.

### Errors

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Check the highlighted fields.",
    "fields": [{ "path": "contactPoints[0].value", "code": "invalid_phone", "message": "Enter a valid phone number." }],
    "details": {},
    "requestId": "uuid"
  }
}
```

Stable codes: `validation_failed`, `unauthenticated`, `forbidden`, `not_found`, `conflict`, `duplicate_candidate`, `precondition_required`, `version_conflict`, `idempotency_key_reused`, `invalid_transition`, `period_closed`, `reconciliation_required`, `rate_limited`, `provider_unavailable`, `dependency_unavailable`, `internal_error`.

Never place secrets, existence hints, raw provider bodies, restricted note content or stack traces in errors.

## 2. Common resource envelope

```json
{
  "id": "object-id",
  "organizationId": "object-id",
  "branchId": "object-id-or-null",
  "version": 3,
  "createdAt": "RFC3339",
  "createdBy": { "type": "staff|member|system|import", "id": "opaque-id" },
  "updatedAt": "RFC3339",
  "updatedBy": { "type": "staff|member|system|import", "id": "opaque-id" },
  "archivedAt": null
}
```

Audit actor summaries are identifiers/display labels appropriate to the reader. Authentication secrets and sensitive diffs are never embedded.

## 3. People and households

### Routes

```text
GET    /people
POST   /people
GET    /people/{personId}
PATCH  /people/{personId}
POST   /people/{personId}/archive
POST   /people/{personId}/restore
GET    /people/{personId}/timeline
GET    /people/{personId}/duplicate-candidates
POST   /people/merge

GET    /households
POST   /households
GET    /households/{householdId}
PATCH  /households/{householdId}
POST   /households/{householdId}/members
DELETE /households/{householdId}/members/{personId}
POST   /relationships
PATCH  /relationships/{relationshipId}
POST   /relationships/{relationshipId}/end

GET    /membership-stages
POST   /people/{personId}/membership-transitions
```

`GET /people` filters: `q`, `branchId`, `membershipStage`, `tag`, `groupId`, `archived`, `qualityIssue`, `updatedFrom`, `updatedTo`; sorts: `name`, `createdAt`, `updatedAt`, `lastAttendanceAt`.

Create person request:

```json
{
  "names": { "given": "Ama", "family": "Mensah", "preferred": "Ama" },
  "photoAssetId": null,
  "dateOfBirth": { "value": "1992-04", "precision": "month" },
  "gender": null,
  "homeBranchId": "id",
  "contactPoints": [{ "type": "mobile", "value": "+233...", "primary": true }],
  "addresses": [],
  "membershipStage": "guest",
  "source": { "type": "staff-entry", "reference": null },
  "noticeVersion": "member-intake-v1"
}
```

Potential duplicate results do not auto-merge. The create may return `409 duplicate_candidate` with candidate IDs/safe comparison fields and an override token available only to authorized users.

Merge request requires `survivorPersonId`, `mergedPersonIds`, per-conflict selections, expected versions and reason. It returns a merge manifest and aliases; no historical source fact is deleted.

Timeline filters by event types/date and returns safe summaries. Each item has `sourceType/sourceId`; fetching source reauthorizes. Restricted care content and individual giving amounts are absent from the general timeline.

## 4. Services, attendance and check-in

```text
GET/POST /service-definitions
GET/PATCH /service-definitions/{id}
GET/POST /occurrences
GET/PATCH /occurrences/{id}
POST      /occurrences/{id}/cancel

GET       /occurrences/{id}/attendance
PUT       /occurrences/{id}/attendance/{personId}
POST      /occurrences/{id}/attendance-corrections
GET/POST  /occurrences/{id}/headcounts
POST      /occurrences/{id}/lock-attendance
GET       /attendance-analytics?branchId=&from=&to=

POST      /checkin/sessions
POST      /checkin/sessions/{id}/commands:sync
POST      /checkin/sessions/{id}/lock
POST      /checkin/{attendanceId}/pickup
```

Attendance analytics is branch-scoped and private/no-store. It returns completed-occurrence trends, distinct named people, named visit facts, the latest anonymous observation per occurrence/category, first/returning guest classification, privacy-thresholded 30/60/90-day cohorts, coverage/confidence/freshness metadata and metric definitions. Named attendance and anonymous headcount are never combined into one total.

Offline command batches are strictly sequence-ordered. A retryable dependency failure stops the batch without advancing the station checkpoint; the same command ID and payload can later be atomically reclaimed and applied. Concurrent delivery returns `retry`, completed identical delivery returns `duplicate`, changed-content ID reuse returns `rejected`, and stale optimistic versions return `conflict`.

Attendance upsert carries `status`, check-in/out times, source, station and idempotency key. It may create the one current fact but later locked-period corrections use `/attendance-corrections` with reason and expected version.

Offline sync request contains ordered commands with client UUID, local sequence, local occurrence/person references and captured time. Response classifies every command as `applied`, `duplicate`, `conflict`, `rejected` or `retry`; partial ambiguity is forbidden. Client retries only unresolved command IDs.

Pickup requires guardian/security proof, never returns the stored security code, and returns a minimal release receipt. Replays and unauthorized guardian attempts are audited and rejected.

## 5. Groups and volunteers

```text
GET/POST /groups
GET/PATCH /groups/{id}
GET/POST /groups/{id}/members
GET       /groups/{id}/members/{personId}/history
PATCH     /groups/{id}/members/{personId}
DELETE    /groups/{id}/members/{personId}
GET/POST /groups/{id}/meetings
PUT       /groups/{id}/meetings/{meetingId}/attendance/{personId}

GET/POST /teams
GET/PATCH /teams/{id}
GET/POST /teams/{id}/positions
PATCH     /teams/{id}/positions/{positionId}
GET/PUT   /people/{personId}/volunteer-profile
GET/POST  /people/{personId}/availability
GET/POST /service-plans
GET/PATCH /service-plans/{id}
POST      /service-plans/{id}/assignments
PATCH     /assignments/{id}/response
POST      /assignments/{id}/substitute
GET       /community-analytics?branchId={id}&from={RFC3339}&to={RFC3339}
```

Leader access is derived from current owned-group/team assignments and allowed actions. Passing another `groupId` never broadens scope.

Community analytics is branch-scoped, private/no-store and staff-authorized; owned-unit leadership alone does not grant the branch aggregate. The `community-ops-v1` response separates current group capacity from range-based meeting activity, calculates recorded attendance only from explicit marks, calculates serving fill from plan needs, exposes every denominator, freshness and caveat, and returns null when either observations or distinct people fall below the privacy threshold of five. It contains no names, contact data, leader notes, screening metadata, pastoral data or financial evidence.

Volunteer teams and positions are branch-scoped operational resources. Positions carry normalized required skills, optional membership/age constraints, and boolean/expiry policies for background checks and safeguarding training. A volunteer profile holds normalized skills and team/position preferences. Screening metadata is a separately authorized sensitive-ministry field containing only controlled status, checked/expiry dates and an external reference; reports, identity documents, findings and free-text adjudication are forbidden. Availability is stored as bounded `available|preferred|unavailable` windows with an idempotent source key so later schedulers can explain conflicts without inferring intent.

Group definitions carry a branch, optional owning `ministryId`, controlled type, leader person references, optional capacity and meeting pattern, independent joining privacy (`open|request|invite-only`), discoverability (`staff|members|public`) and lifecycle (`draft|active|paused|closed`). Staff group routes enforce the persisted branch/ministry pair and filter list results before serialization; legacy unassigned groups require a global or branch-wide administrator to classify them before ministry-scoped staff can access them. Invite-only groups cannot be public. Pausing/closing requires a reason, closed groups are terminal, and every update requires an expected version.

Group membership is a versioned person/group record with `applied|invited|waitlisted|active|declined|ended` state, controlled roster role, source, effective timestamps and an explicit per-person `directoryVisibility` choice (`hidden|members`, default `hidden`). A group-level discoverability setting never overrides that individual choice. Every create, rejoin and transition also appends an immutable membership-history record carrying the prior/current state, role, actor, request ID, reason and occurrence time; current-projection updates and history insertion share the same transaction. Active-seat reservation is atomic inside that transaction; approving into a full bounded group produces `waitlisted` instead of overbooking. Ending an active membership releases exactly one seat. Leader notes are staff-operational context, capped, deliberately excluded from lifecycle-history payloads and never included in member/public directory responses. Group meeting attendance is unique per meeting/person, limited to active roster members and reason-required when corrected after the meeting.

## 6. Workflows and pastoral care

```text
GET/POST /workflow-definitions
GET/PATCH /workflow-definitions/{id}
POST      /workflow-definitions/{id}/publish
GET/POST /workflow-instances
GET       /workflow-instances/{id}
POST      /workflow-instances/{id}/transitions
GET/POST /tasks
PATCH     /tasks/{id}
POST      /tasks/{id}/complete

GET/POST /care-cases
GET/PATCH /care-cases/{id}
POST      /care-cases/{id}/assign
POST      /care-cases/{id}/close
GET/POST /care-cases/{id}/notes
POST      /care-cases/{id}/contact-events
```

Care-case creation requires explicit `branchId`, `ministryId`, one person or household subject, granted consent evidence, a published pastoral workflow and at least one assignee. All subsequent case, contact and restricted-note routes reauthorize the persisted branch/ministry pair and current assignment. Restricted note bodies are never projected into case lists or case detail; they are decrypted only by the audited notes route for an assigned, sensitive-ministry-authorized caller.

Published workflow definition versions are immutable. Instances pin the version. Transitions require allowed state, expected version, permission and optional outcome fields.

Care-case list/detail responses depend on assignment and field class. Restricted note bodies use dedicated endpoints, are excluded from list/search/export and produce read audits. Linking a prayer submission to a person requires authorized consent evidence; anonymous/private requests cannot be silently identified.

## 7. Giving and finance

```text
GET/POST /finance/funds
GET/PATCH /finance/funds/{id}
POST      /finance/funds/{id}/deactivate
GET/POST /finance/campuses
GET/PATCH /finance/campuses/{id}
GET/POST /finance/payment-methods
GET/PATCH /finance/payment-methods/{id}
GET/POST /finance/periods
POST      /finance/periods/{id}/close
POST      /finance/periods/{id}/reopen
GET/POST /finance/receipt-sequences
GET/PATCH /finance/receipt-sequences/{id}
GET/POST /finance/account-mappings
GET/PATCH /finance/account-mappings/{id}

GET       /finance/reports/summary?branchId={id}&periodId={id}
GET/POST  /finance/export-runs
GET       /finance/export-runs/{id}
GET       /finance/export-runs/{id}/download

GET/POST /finance/contributions
GET       /finance/contributions/{id}
POST      /finance/contributions/{id}/adjustments
POST      /finance/contributions/{id}/attribution

GET/POST /finance/batches
GET       /finance/batches/{id}
POST      /finance/batches/{id}/entries
PATCH     /finance/batches/{id}/entries/{entryId}
POST      /finance/batches/{id}/confirmations
POST      /finance/batches/{id}/transitions
GET/POST /finance/settlements
GET       /finance/settlements/{id}
GET       /finance/settlements/{id}/items
GET       /finance/reconciliation-owners
POST      /finance/reconciliation-items/{id}/resolve
GET       /finance/period-control-requests?periodId={id}

GET/POST /finance/pledges
GET/PATCH /finance/pledges/{id}
GET/POST /finance/campaigns
PATCH     /finance/campaigns/{id}
POST      /finance/statement-runs
GET       /finance/statement-runs/{id}
POST      /finance/export-runs
GET       /finance/export-runs/{id}

POST      /payments/paystack/intents
POST      /webhooks/paystack
GET       /fundraising/campaigns
GET       /fundraising/campaigns/{slug}
```

Fundraising campaigns are time-bound public appeals mapped to an active finance fund. Draft and paused campaigns never appear in the public API. Public responses include the goal, current raised amount and gift count; those totals are derived from immutable posted contributions and their provider refunds/chargebacks rather than editable counters. Giving to a campaign sends `campaignId` with the payment intent. The API resolves the campaign's configured fund and persists that campaign attribution through the verified Paystack webhook into the contribution ledger.

The local development bootstrap may explicitly enable a `/give/demo-success` redirect when Paystack is not configured. Production validates `PAYSTACK_SECRET_KEY` at startup and the finance service independently fails closed with `503 provider_unavailable`; a live deployment can never silently present a simulated successful checkout.

Contribution create/post uses one command:

```json
{
  "receivedAt": "RFC3339",
  "branchId": "id",
  "donor": { "type": "person|household|anonymous|unresolved", "id": "id-or-null" },
  "source": "cash|cheque|mobile-money|bank-transfer|online|import",
  "paymentMethodId": "configured-method-id",
  "paymentMethodReference": null,
  "total": { "amountMinor": 10000, "currency": "GHS" },
  "splits": [{ "fundId": "id", "amount": { "amountMinor": 10000, "currency": "GHS" } }],
  "batchId": "id-or-null",
  "postingAction": "stage|post"
}
```

Posting requires a recent MFA-authenticated finance grant and an
`Idempotency-Key`. The branch, donor record, active payment method, active
funds, balanced GHS splits, open fiscal period and receipt sequence are checked
again inside the posting transaction. Responses include immutable `state` and
derived `effectiveState`; an original remains unchanged while a linked reversal
or correction makes its effective state `reversed` or `corrected`.

Adjustment commands create an exact negative reversal and, for a correction,
an optional balanced positive replacement. Both entries receive unique receipt
register numbers and link to the immutable chain root. The original contribution
has no update or delete route.

Only permitted direct sources may post outside a batch. Posted response is immutable and includes receipt/period. Adjustment request contains `type`, reason, reversal/replacement splits/attribution and approver workflow; it never PATCHes the original.

The finance summary report returns fund movement, timezone-bucketed giving trends,
deposit reconciliation and aggregate pledge progress with `metricVersion`, `asOf`,
`sourceWatermark`, applied branch/period scope and caveats. Totals are derived from
the same posted contribution, settlement and pledge records used by the operator
workspaces. Creating an `accounting-csv` or `audit-package` export requires an
explicit export grant and recent MFA. Runs persist the exact report snapshot,
artifact SHA-256, row count and reconciled total; downloads are reauthorized and
served `private, no-store`. CSV cells beginning with spreadsheet formula characters
are escaped. Audit packages contain `manifest.json`, `report.json` and the exact
accounting CSV.

Batch transitions currently implemented by the counting slice are
`start-counting`, `reset-count`, `approve`, and `post`. Settlement work adds
`mark-deposited`, `reconcile`, and controlled exception transitions later.
Assigned counters enter contributions and independently submit tender totals,
cash denominations, and private `remi/finance/...` evidence references. The
first confirmation freezes entry editing. A second counter must submit the same
tender totals before a dual-control batch becomes `counted`; mismatches do not
advance state. Any entered-versus-declared variance requires an approver reason,
and no assigned counter may approve or post the batch. Posting requires recent
MFA and transactionally creates all immutable ledger contributions, receipt
register entries, audits, and outbox records before locking the batch as
`posted`.

Paystack webhook is unauthenticated by JWT but requires a valid provider signature over the bounded raw body. It responds quickly after durable inbox recording. Unknown reference/amount/currency/state mismatches become finance exceptions and never post a gift automatically.

### Settlement import and reconciliation

Settlement imports accept one to 5,000 stable source rows from `paystack`,
`bank` or `deposit` evidence. The evidence file is uploaded using authenticated
Cloudinary delivery into `remi/finance/settlements`; the API stores its private
asset ID and a lowercase SHA-256 digest, never a public URL. Totals and every
row use GHS minor units. Row amounts must sum exactly to gross, row fees must
sum exactly to fees, and `net = gross - fees - otherDeduction`; integer overflow
is rejected. The settlement date and every eventual match are constrained to
the selected branch and open fiscal period. Closed periods must first pass the
controlled reopen workflow.

```json
{
  "branchId": "accra",
  "sourceType": "paystack",
  "sourceName": "Paystack Ghana",
  "fileName": "settlement-2026-08-10.csv",
  "fileHash": "64-lowercase-hex-characters",
  "privateAssetId": "remi/finance/settlements/private-id",
  "periodId": "period-id",
  "reference": "PST-2026-08-10",
  "settledAt": "RFC3339",
  "currency": "GHS",
  "grossAmountMinor": 25000,
  "feeAmountMinor": 300,
  "otherDeductionMinor": 0,
  "netAmountMinor": 24700,
  "rows": [{
    "sourceRowId": "provider-stable-row-id",
    "reference": "provider-transaction-reference",
    "occurredAt": "RFC3339",
    "amountMinor": 25000,
    "feeAmountMinor": 300,
    "description": "optional bounded source narration"
  }]
}
```

`sourceType + fileHash` is a unique replay identity. Serial or concurrent
identical imports return the original decorated settlement; they never create
duplicate rows or audits. Paystack suggestions require the same branch,
provider reference, bounded date proximity and effective contribution amount
after refund/chargeback adjustments. Deposit suggestions require the posted or
deposited batch reference and amount. Suggestions expose evidence and
confidence but never resolve a row automatically.

Resolution actions are `match`, `approve-variance` and `assign-exception`.
Exact matches require an authorized finance reconciler. A non-zero variance
requires a detailed reason plus a distinct `finance-ledger:approve` grant and a
recently MFA-confirmed session. An exception requires a detailed reason and an
active staff owner. Targets outside the settlement branch/period and targets
already consumed by another resolved row are rejected. Resolution, target
state, settlement projection and audit evidence commit atomically. A settlement
is `reconciled` only when no unresolved or owned-exception rows remain; its
matched and variance totals are derived from those immutable row decisions.

### Period close and controlled reopen

Both close and reopen are two commands against the same route: first
`{"action":"request","reason":"..."}`, then an independent approver sends
`{"action":"approve","requestId":"...","reason":"...","expectedVersion":3}`.
The requester cannot approve their own request. Approval requires
`finance-ledger:approve` and recent MFA; the fiscal-period version provides the
optimistic concurrency boundary. Request creation and its audit commit in one
transaction. Approval atomically changes the period, approves the request,
appends a control event/audit and queues its domain event.

Close is refused while any settlement, counting batch, provider intent,
provider exception, reconciliation row or posted online contribution remains
unreconciled. The request and approval retain the source-watermarked blocker
snapshot. Once closed, all contribution, batch and settlement posting paths
reject backdated mutation until an independently approved reopen is complete.

## 8. Consent, communications and member access

```text
GET       /people/{personId}/consents
POST      /people/{personId}/consent-events
GET/POST /segments
POST      /segments/{id}/preview
GET/POST /campaigns
POST      /campaigns/{id}/approve
POST      /campaigns/{id}/schedule

GET/PATCH /member/profile
GET/PATCH /member/household
GET       /member/attendance
GET       /member/groups
GET       /member/serving
GET       /member/giving
GET       /member/statements
POST      /member/data-requests
```

Consent is append-only with purpose, channel, state, notice version, source and occurred time. Send execution rechecks current consent/suppression; an old audience snapshot cannot override a later withdrawal.

Saved audience routes are `GET/POST /communication-audiences`, `PUT /communication-audiences/{audienceId}`, `GET /communication-audiences/{audienceId}/preview`, and `GET/POST /communication-audiences/{audienceId}/exports`. An audience stores only its purpose, channel, authorized saved-segment IDs and explicit exclusions—not a resolved contact snapshot. Preview resolves current segment membership and exposes masked destinations plus exclusion counts for minors, missing verified contacts, no grant, withdrawal, suppression and duplicates. Export requires explicit `export` permission and a 10–500 character handling purpose, resolves again immediately, writes immutable export/audit evidence, and returns `X-Export-Audit-ID` with `Cache-Control: private, no-store`. Export history never repeats the destinations. REMI-272 send execution must repeat the same live checks; neither a preview nor an export is sending authority.

Campaign orchestration uses `GET/POST /communication-templates`, `PUT /communication-templates/{id}`, `POST /communication-templates/{id}/publish`, `GET/POST /communication-campaigns`, `GET /communication-campaigns/{id}` and its `/submit`, `/approve`, and `/schedule` commands. The finite lifecycle is draft → pending approval → independently approved → scheduled → sending → dispatched/partially-failed; a creator cannot approve their own campaign. Approval pins exact audience and published-template versions. Dispatch resolves the saved audience again, applies every current eligibility rule, and inserts one unique campaign/person delivery before invoking Resend, Arkesel or Meta WhatsApp. Scheduling fails closed if that real provider is not configured.

`POST /api/webhooks/communications/{resend|arkesel|meta-whatsapp}` accepts a canonical gateway event `{providerEventId,reference,type,occurredAt}` with `X-REMI-Communication-Signature: sha256=<hex HMAC-SHA256(raw body)>`. Event IDs are unique and replay-safe. Bounce, complaint and provider opt-out events create current suppressions. `POST /api/communications/opt-outs` consumes the signed token from the public `/unsubscribe` experience, reveals no person identifier, and creates a purpose/channel-specific suppression immediately. `COMMUNICATION_WEBHOOK_SECRET` is shared only with the trusted provider-event adapter; `PUBLIC_WEB_URL` supplies the signed preference-link origin.

Member household access is explicit delegation with expiry/revocation and per-field permissions. An adult never gains another adult's finance/profile access merely because both share a household.

Authenticated member generosity uses `GET /api/member/chms/finance/campaigns`, `POST /finance/giving-intents`, `GET|DELETE /finance/payment-methods[/{id}]`, and `GET|POST|PATCH /finance/recurring-instructions[/{id}]`. The server derives person, branch, email and eligible household scope from the session; client-supplied identity is never trusted. Saved-method responses expose masked brand/channel/last-four/expiry metadata only. Provider authorization codes and their original email are encrypted at rest, captured only after signed-event verification, and never returned to a browser.

Member care and content uses `GET /api/member/care-content`, `POST /prayer-requests`, `POST /care-requests`, `POST /pastoral-appointments`, and `PUT /saved-content/{type}/{id}`. The read model includes published announcements/sermons/live state plus only the authenticated member's identified requests and saved items. Request history deliberately reduces staff workflow to `received`, `acknowledged`, or `completed`; it excludes assignments, internal outcomes and all restricted note content. Anonymous prayer/care documents omit person, name and destination fields and therefore cannot provide tracking. Member private content notes are AES-GCM encrypted with organization/person/content-bound associated data and never enter staff content records.

### Member identity, recovery, MFA and delegation

- `POST /api/member-auth/otp/request` starts the same privacy-preserving passwordless flow for sign-in and account recovery. Responses do not reveal whether an identifier exists.
- `POST /api/member-auth/otp/verify` consumes the first one-time code. When optional member MFA is enabled it returns `202` with a second challenge and **does not issue tokens**.
- `POST /api/member-auth/mfa/verify` consumes the second-channel challenge and issues the member session. Challenges expire after ten minutes, lock after five failed attempts and are one-use.
- `GET /api/member/mfa` returns only masked channel readiness and enabled state. `POST /mfa/setup` + `POST /mfa/confirm` enable protection; `POST /mfa/disable/request` + `DELETE /mfa` require fresh verified proof before disabling it. Enable/disable actions append immutable audit evidence.
- `GET /api/member/household-delegations` returns the caller's active grants/receipts plus only eligible adult household accounts and a controlled access-area vocabulary. `POST` creates a one-hour-to-one-year field-scoped grant; only its grantor may `DELETE /api/member/household-delegations/{id}`. Shared household membership alone never grants access.

### Member participation self-service

- `GET /api/member/participation` is a purpose-built projection for the authenticated member. It returns their registrations, active-household registration candidates, currently available public events and form questions, their own group memberships, branch-scoped discoverable groups, current next-step requests, guardian-eligible child pre-check choices, and confirmed-present service/group history.
- The attendance policy is versioned as `member-attendance-v1`. Only confirmed presence is visible. Absences, correction history, sources, confidence, operator reasons, wider rosters, retention signals and pastoral/safeguarding data are not queried or serialized. Members cannot modify attendance.
- `POST /api/member/registrations` accepts one public upcoming event, validated allowlisted text/choice/boolean answers, and one-to-ten people selected from the caller's active household context. Capacity is reserved atomically with person-linked registrations and immutable audits. A full waitlist-enabled event places the complete request on the waitlist. `DELETE /api/member/registrations/{id}` is limited to the registered person or household member who created it and promotes the oldest waitlisted place or releases capacity exactly once.
- Paid events create a twenty-minute `pending-payment` seat hold. `POST /api/member/registrations/{registrationId}/payment` creates or replays one member-owned checkout for the complete booking; `POST /api/member/registration-payments/{paymentId}/confirm` verifies provider success, reference, amount and currency before transactionally confirming every held place. Expired holds restore capacity. A paid-event waitlist promotion creates a fresh payment hold rather than a free confirmation. Event fees remain separate from charitable contributions, receipts and giving statements. Provider-less confirmation is permitted only outside production.
- `GET /api/member/registrations/{registrationId}/calendar.ics` returns a private no-store calendar file only for a confirmed registration owned by or created by the member.
- `POST /api/member/pathway-requests` creates or idempotently replays an active baptism, membership-class or new-believer conversation request. It never changes membership stage automatically.
- `POST /api/member/child-prechecks` requires an active household guardian/adult/parent relationship and active child/dependent/minor relationship, returns a short handoff code once, stores only its hash, expires automatically and never records attendance or bypasses staffed safeguarding/check-in.
- `POST /api/member/groups/{groupId}/membership` permits only active, member/public-discoverable groups in the member's branch. Open groups reserve capacity atomically or waitlist; request groups create an application; invite-only, staff-only and cross-branch groups are concealed. `DELETE` ends only the caller's membership, releases an active seat and appends lifecycle/audit evidence.

### Member community self-service

- `GET /api/member/groups/{groupId}/community` requires an active membership and returns safe group details, upcoming/recent meetings, the member's own planning responses, opted-in name/photo-only roster entries and leader display names. Contact points, leader notes, attendance facts, confidence, corrections, pastoral data and hidden members are excluded.
- `PUT /api/member/groups/{groupId}/meetings/{meetingId}/response` records version-checked `going`, `maybe` or `not-going` planning intent for a future meeting. It never creates or changes factual attendance.
- `PUT /api/member/groups/{groupId}/directory-visibility` lets a member explicitly choose `hidden` or `members` for that group and uses optimistic versioning.
- `POST /api/member/groups/{groupId}/leader-messages` creates an audited `pending-consent-review` handoff to the current leader person IDs. The handoff contains no contact destinations; delivery resolves current consent, suppression and verified destinations later.

### Member serving self-service

- `GET /api/member/serving-workspace` returns the authenticated member's interests, active branch teams/positions, future availability, own assignments, reminder state, self-check-in state and explainable overlaps. Screening, safeguarding, coordinator notes and every other volunteer are excluded.
- `PUT /api/member/serving-workspace/preferences` updates only skills, preferred team/position IDs and active/paused interest with optimistic versioning. Staff-owned eligibility metadata is preserved and never serialized.
- `POST /api/member/serving-workspace/availability` creates a future member-owned planning window no longer than 90 days; `DELETE /availability/{id}` cancels only the caller's window without erasing it.
- Existing `/api/member/chms/serving/assignments/{id}/response` remains the version-checked invitation accept/decline command. `POST /assignments/{id}/substitute-request` records a reasoned coordinator request without selecting or assigning another person.
- `POST /assignments/{id}/check-in` is idempotent and limited to the caller's accepted/substitute-requested assignment from two hours before through four hours after its start. It records arrival only and does not mark attendance, completion or eligibility.

### Member directory and communications

- `GET /api/member/communication-workspace` returns only accepted/delivered campaign records for the authenticated person, immutable member-safe subject/body snapshots, self-owned read state and active group choices. Inbox read/archive state never changes provider delivery evidence.
- `PATCH /api/member/inbox/{deliveryId}` accepts `read`, `unread` or `archive` only after rechecking delivery ownership. Archive is a private projection state and does not erase the delivery.
- `GET /api/member/directory?scope=branch|group[&groupId=][&q=]` is adult-only and capped. Branch discovery requires the target's explicit branch visibility; group discovery requires active shared membership plus target group visibility. Minors and people with child/minor household roles are excluded. Email/mobile appear independently only when the target selected those fields. There is no church-wide scope or export route.
- `GET|POST /api/member/groups/{groupId}/messages` requires active adult membership. Posting additionally requires the author to be visible in that group. The API never triggers an external channel; inactive, hidden or minor authors are relabelled rather than disclosed.
- `PUT /api/member/communication-preferences` uses optimistic versioning for an IANA timezone, finite half-open quiet window, daily non-essential campaign cap of 1–5 and independently shared directory fields. Dispatch applies these controls after canonical consent/suppression resolution and before provider invocation. Quiet-hour and cap exclusions become explicit suppressed deliveries. Essential security and transactional notices are outside campaign policy.
- Global profile directory visibility accepts private, shared groups or home branch. Per-group visibility remains independently controllable. Channel and purpose consent for push, email, SMS, WhatsApp and phone remains canonical in the consent centre; directory and fatigue controls cannot grant withdrawn consent or release a suppression.

## 9. Engagement and retention

Operational branch references use the stable public branch slug (for example, `accra-headquarters`) as `branchId`. `GET /api/branches` exposes that value as both `id` and `operationalId`; the CMS document identifier remains available separately as `contentId`. Admin clients must not send the CMS document identifier to CHMS endpoints.

```text
GET/POST /engagement/rules
GET/PATCH /engagement/rules/{id}
POST      /engagement/rules/{id}/publish
POST      /engagement/rules/{id}/generate
GET       /engagement/signals
GET       /engagement/signals/{id}
GET       /engagement/signals/{id}/review
POST      /engagement/signals/{id}/assign
POST      /engagement/signals/{id}/snooze
POST      /engagement/signals/{id}/suppress
POST      /engagement/signals/{id}/contact-events
POST      /engagement/signals/{id}/resolve
GET       /engagement/cohorts
```

Signal review detail returns the permitted person label, rule name/version, exact source evidence, observation window, missing-data caveats, expiry, authorized reviewer choices and immutable review history. Assignment establishes human ownership. Only the assigned reviewer may snooze, suppress, record outreach or resolve; every mutation requires the expected version and a reason/outcome. Outreach is record-only—it never sends—and re-evaluates current `pastoral-care` consent and active suppressions for the selected channel immediately before persistence. Snoozes must end before evidence expiry, resolved/suppressed observations cannot be reopened, and false-positive feedback is retained for policy review. No action automatically creates a care case, changes membership or contacts a person. The API provides no individual engagement score and excludes all gift, pledge and fund evidence.

`GET /engagement/cohorts` requires `branchId`, RFC3339 `from` and `to`, a published approved branch retention policy, and branch-scoped sensitive-ministry read access. The `retention-v1` response computes weekly first-visit cohorts from current named attendance, independently matures 30/60/90-day return windows, reports active group and accepted/completed serving connection windows, and provides a non-overlapping group-only/serving-only/both/neither partition. Every metric cell is explicitly `available`, `pending` or `suppressed`. Below five eligible people, numerator, denominator and rate are all absent; immature cells are pending rather than zero. The response exposes timezone, configuration, source watermark, caveats and generation time but contains no person IDs, signals, contacts or financial evidence. Adjacent totals that could reveal a suppressed cell by subtraction are intentionally omitted.

`GET /engagement/safety-readiness?branchId=` returns the exact published-policy fingerprint, required product/pastoral/privacy decision states, current actor eligibility and immutable review history. `POST /engagement/safety-reviews` accepts a role, `approved` or `changes-required` decision, findings and the complete safety checklist. Only the reviewer ID already named for that role on every published policy may submit; a general administrator cannot impersonate a reviewer. Approval requires all checklist statements. Decisions bind to the SHA-256 policy-set fingerprint, become stale automatically when any published rule/version changes, append audit/outbox evidence and are never overwritten. The release state becomes approved only when all three current-fingerprint roles independently approve.

Rules remain draft-capable while individual generation is disabled. Publishing and generation both fail with `feature_disabled` unless `CHMS_RETENTION_SIGNALS_ENABLED=true`; publishing additionally requires recorded product-owner, pastoral and privacy approver IDs, an approval time and review cadence. Production defaults the flag to false. `retention-v1` rules are immutable after publication, and signal generation is idempotent per rule version and person. Permitted source queries are restricted to active people, completed non-cancelled occurrences, current named attendance, active group joins and accepted/completed serving assignments; a static test rejects finance collection or giving-domain dependencies.

## 10. Staff access governance

```text
GET       /api/admin/users
POST      /api/admin/users
PATCH     /api/admin/users/{id}/scopes
```

Operational invitations accept role plus current `branchIds` and optional `ministryIds`; each ID must resolve to an existing configured record and at least one branch is mandatory for operational roles. Arbitrary wildcard input is rejected. Super administrators remain organization-wide.

Scope changes accept `branchIds`, `ministryIds`, `expectedAccessVersion` and a reason of 8–300 characters. They are optimistic, audited and increment `accessVersion`, immediately invalidating every previously issued staff token. Staff authentication checks current account state, role and access version on each protected request. The response never contains assignment details beyond safe scope IDs/counts.

## 11. Reporting, exports and governance

```text
GET       /metrics/dictionary
GET       /dashboards/{dashboardKey}
GET/POST /reports
POST      /reports/{id}/runs

Implemented governed reporting routes under the staff-only ChMS boundary:

```text
GET       /reporting/metrics
POST      /reporting/query
GET/POST  /reporting/saved-views
PUT       /reporting/saved-views/{viewId}
GET/POST  /reporting/export-runs
GET       /reporting/export-runs/{exportId}
GET       /reporting/export-runs/{exportId}/download
GET/POST  /reporting/schedules
GET       /reporting/deliveries/{deliveryId}
GET       /reporting/deliveries/{deliveryId}/download
```

`POST /reporting/query` accepts an explicit branch, IANA timezone, RFC3339 range up to 366 days, one to twelve approved metric IDs and only the `metric`/`domain` dimensions. It never accepts collection names, field paths, filters over people, or ad hoc formulas. Each measure is reauthorized against its canonical resource and field class before the authoritative domain projection runs. Sparse cells remove values and denominators before serialization. Saved views are owner-scoped and optimistic-versioned. Export requests capture the exact governed snapshot and return `202`; the worker creates formula-safe UTF-8 CSV, stores its SHA-256, and downloads reauthorize the owner and every selected metric before checking artifact integrity.

Interactive execution has an eight-second hard deadline and an operating objective of p95 below two seconds. Results must contain each requested metric exactly once and no others, match the canonical domain/unit, carry generation and as-of timestamps, and use only `available`, `suppressed` or `unavailable`. Invalid counts, percentages, denominators, non-finite values and incomplete projections fail closed. See `docs/chms-reporting-hardening.md` for budgets and release evidence.

Schedules bind to an owner-scoped saved-view ID and exact optimistic version. They accept weekly or monthly cadence, CSV or ZIP board-pack output, a first run within one year and one to twenty active staff account IDs; arbitrary email addresses are never accepted. Every due run resolves the owner's current role and reruns authorization. Deactivated owners, changed views or revoked metric permissions pause future delivery. ZIP manifests retain the query, saved-view version, dictionary version, generation time and row count. Recipient links identify a delivery but are not bearer credentials: the signed-in staff identity must match, current metric permissions are rechecked, SHA-256 integrity must match and access expires after seven days. Creation, run, send/failure, first open and expiry append audit evidence.
GET       /report-runs/{id}
GET       /report-runs/{id}/download

GET       /audit-events
POST      /audit-events/export
POST      /exports
GET       /exports/{id}
GET       /exports/{id}/download
GET/POST /imports
POST      /imports/{id}/mapping
POST      /imports/{id}/validate
POST      /imports/{id}/commit
POST      /imports/{id}/rollback
GET/POST /data-requests
POST      /data-requests/{id}/transitions
```

Audit queries require an explicit RFC3339 range of no more than 366 days and accept only exact allowlisted branch, actor, action, resource, subject, request and outcome filters. Branch-scoped auditors must provide one of their assigned branches; super administrators and data-protection supervisors may review organization-wide safe metadata. Responses expose actor/action/resource/subject/reason/request/outcome, changed field names and optional aggregate versions—never domain before/after values or restricted content. CSV export repeats current authorization, requires recent MFA plus a 10–300 character handling reason, caps rows, neutralizes spreadsheet formulas, returns SHA-256/private-download headers and appends safe `audit.export` evidence. See `docs/chms-audit-governance.md`.

Data-rights staff routes use `GET /data-requests?branchId=&status=`, `GET /data-requests/{id}` and `POST /data-requests/{id}/transitions`. They enforce organization/branch scope, return only the safe request envelope and immutable transition history, and require `expectedVersion` plus a valid next state and accountable reason. Triage additionally requires priority and assignee. Final decisions require recent MFA, an approver distinct from the fulfiller, an allowlisted outcome and a plain-language member response. `POST /api/member/data-requests/{id}/withdraw` permits an authenticated member to withdraw only their own newly received request at its current version; history and audit evidence remain.

Reports/dashboards return `metricVersion`, `asOf`, `sourceWatermark`, timezone, applied scope/filters and caveats. Finance report totals expose reconciliation evidence to finance roles.

Exports/imports are asynchronous. Download URLs are short-lived and access is reauthorized. Exported CSV values that begin with spreadsheet formula characters are escaped. Imports require staged upload, mapping, validation/dry run, commit manifest and provenance; repeating a source hash cannot silently duplicate records.

Audit query permits approved filters only and returns safe field names/outcomes, not secret/restricted values. Audit records cannot be edited/deleted via the API.

## 12. Asynchronous jobs

Job response:

```json
{
  "job": {
    "id": "id",
    "type": "statement-run",
    "status": "queued|running|completed|failed|cancelled",
    "progress": { "processed": 0, "total": 0 },
    "createdAt": "RFC3339",
    "completedAt": null,
    "error": null
  }
}
```

Clients poll `GET /jobs/{id}` with backoff; future server-sent notifications may complement polling. Job retries are idempotent, progress excludes sensitive row data, and cancellation is available only where domain-safe. Completed artifacts have hash, retention expiry and authorized download endpoint.

## 13. Versioning and compatibility

- Major API version is path-based. Additive optional response fields and new enum values are allowed within v1; clients must ignore unknown fields and handle unknown enum values as `unknown` for display.
- Removing/renaming fields, changing meaning/type/default, narrowing accepted inputs or changing authorization side effects requires v2 or a documented migration window.
- Event payloads have independent `eventType/eventVersion`; consumers explicitly register supported versions.
- Deprecations return `Deprecation`/`Sunset` headers and appear in release notes/contract tests for at least the approved support window.
- Database `schemaVersion` is not exposed as API version.

## 14. Contract acceptance tests

Every endpoint must prove:

1. authentication audience and positive/negative role/scope/field cases;
2. organization/branch isolation and existence concealment;
3. validation boundaries and safe stable error shape;
4. cursor stability/filter binding and maximum limits;
5. stale/missing version behavior for mutations;
6. idempotent duplicate, concurrent duplicate and key/body mismatch behavior;
7. audit/outbox emission without sensitive content;
8. no partial commit on dependency/provider failure;
9. domain invariants and valid/invalid state transitions;
10. OpenAPI examples/schemas match real handler responses.

Finance, minors/check-in, restricted care, imports/exports and member delegation require dedicated abuse/authorization suites before release.
