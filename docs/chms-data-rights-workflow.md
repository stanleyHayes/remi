# REMI data-rights request workflow

Status: REMI-292 staff lifecycle and member withdrawal implemented; controlled artifact generation and destructive-action execution remain open, 2026-08-12.

## Boundary

The workflow begins with a verified member-session request for access, correction, portability, deletion review or objection. It manages accountable review and evidence; it does not automatically delete records, override finance/safeguarding/legal holds, expose unrelated people or let engineers invent legal exceptions.

## Request envelope

Each request stores a stable ID, organization, subject person, home branch at intake, request type, member-provided details, verification method/time, status, priority, due time, assigned reviewer, optimistic version, created/updated times and immutable transition IDs. Staff responses expose only the subject’s safe display name and request metadata. Contact destinations, credentials, pastoral content and underlying record values do not enter the request envelope.

The current operational SLA is configurable; the initial engineering default is 30 days and must not be represented as approved statutory advice until REMI’s privacy owner confirms the production policy.

## Lifecycle

```text
received
  -> triaged
      -> in-fulfilment
          -> awaiting-approval
              -> completed
              -> partially-completed
              -> declined
  -> withdrawn
```

- `received`: authenticated intake exists; no fulfilment conclusion has been made.
- `triaged`: an approved privacy reviewer has confirmed scope, identity proof sufficiency, due date and accountable assignee.
- `in-fulfilment`: evidence collection or a correction plan is underway.
- `awaiting-approval`: a second eligible reviewer must approve disclosure, portability, deletion/anonymisation or a limitation/decline.
- terminal decisions require a controlled outcome code, plain-language member response, internal evidence references and reason. The member response cannot contain restricted internal rationale.
- `withdrawn` records the requester’s decision to stop the workflow; it does not erase immutable processing evidence.

No transition overwrites history. Every command uses `expectedVersion`, appends a transition record, updates the current projection and writes safe audit/outbox evidence transactionally.

## Fulfilment rules

- access and portability artifacts are generated from an allowlisted subject-data manifest, reviewed before release, hashed, private/no-store, recipient-bound and expiring;
- correction produces a field-level plan routed to each authoritative domain. It never rewrites immutable finance, attendance or audit facts; those domains use their established correction/reversal controls;
- objection or consent withdrawal updates only the specified purpose after current consent/suppression evaluation;
- deletion review first classifies every requested data category as eligible for anonymisation, retained under an approved hold/policy, or requiring accountable review;
- deletion/anonymisation execution is a separately approved, resumable job with a manifest. The request workflow cannot issue collection-wide delete commands;
- household and relationship data is disclosed only to the extent the requester is entitled to it; another person’s independent data is redacted.

## Roles and scope

- `data-protection-supervisor` can triage, assign and approve organization-wide requests.
- a branch-scoped `auditor` may read safe request metadata for assigned branches but cannot fulfil or approve.
- `super-admin` can operate the workflow but cannot bypass second-review or recent-MFA gates.
- final artifact release, deletion/anonymisation approval and decline/limitation require recent MFA and a distinct approver from the fulfiller.
- members can read only their own safe status history and withdraw a non-terminal request.

## Verification gates

- cross-organization, branch and subject isolation;
- valid transition graph, optimistic conflicts and one-winner assignment/approval;
- distinct-approver and recent-MFA enforcement;
- safe member projection excludes assignees, internal evidence and hold rationale;
- access/portability manifest excludes other subjects and restricted note bodies;
- correction delegates to immutable-domain correction commands;
- deletion review never performs implicit deletion and respects approved holds;
- artifact hash, expiry and recipient reauthorization;
- safe audit/outbox evidence for every transition and artifact open;
- branded staff queue/detail plus member status/withdrawal E2E.
