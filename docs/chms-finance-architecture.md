# REMI ChMS — Finance Architecture Decision Record

**Decision:** ADR-CHMS-001  
**Status:** Accepted engineering baseline; accountant and church-policy sign-off required  
**Owner:** ARCHITECT  
**Plan task:** REMI-203  
**Date:** 2026-08-11

## 1. Decision

REMI will implement an **immutable contribution and designated-fund subledger**, not a general ledger.

It records offerings, tithes and other contributions from receipt through fund allocation, batch approval, provider/bank settlement, reconciliation, statements and accounting export. It does not manage expenses, payroll, procurement, accounts payable/receivable, fixed assets, budgets or statutory financial statements.

The church's approved accounting system remains the book of record for the general ledger. REMI exports reconciled summaries and traceable transaction detail through configurable account mappings. A qualified Ghanaian accountant must approve the fiscal, receipt, retention and export policies before production.

## 2. Why this boundary

- The ChMS needs donor/member attribution, contribution history and ministry-facing fund/campaign reporting that a generic CMS does not provide.
- Building a full accounting system would materially expand regulatory, control and reconciliation risk beyond the requested giving scope.
- A dedicated subledger can enforce contribution-specific integrity while integrating with the accountant's chosen ledger.
- Paystack or another Bank of Ghana-authorized provider handles payment credentials and regulated payment processing. REMI stores references and verified events, never raw card/mobile-money credentials.

## 3. Ledger model

### 3.1 Posted contribution

A posted contribution contains:

- organization, branch and occurrence/service attribution where applicable;
- receipt number and received/posted timestamps;
- donor attribution: person, household, anonymous or unresolved;
- source: online, cash, cheque, mobile money, bank transfer, in-kind reference or imported history;
- payment method, provider/batch reference and currency;
- integer `totalAmountMinor` and one or more fund splits;
- fiscal period, posting state, source/provenance and adjustment chain.

The sum of active splits must equal the contribution total. Only `GHS` is enabled initially, but currency is explicit on every money object. Cross-currency conversion is out of scope.

### 3.2 States

```text
draft/imported/pending-provider -> posted -> reconciled
                              \-> failed
posted -> reversed (through a linked reversing contribution)
posted/reconciled -> adjusted (through linked reversing + replacement entries)
```

Draft/staged input may be corrected. Once posted, monetary, donor, fund, source, branch and date facts are immutable. A correction records a reason, approver and linked reversal/replacement; history remains queryable. “Delete contribution” is never an API or UI operation.

### 3.3 Receipt sequence

Receipt numbers are generated server-side from an organization-configured sequence such as `REMI-2026-000001`. Allocation is transactional and unique. Voided numbers remain in the register with a reason; numbers are never reused. Imported historical receipts retain their source number in a separate field.

## 4. Funds, campaigns and accounting mappings

- **Fund/designation:** donor-directed or internally classified destination such as Tithe, General Offering, Missions or Building Project.
- **Restriction type:** `unrestricted`, `temporarily_restricted`, `board_designated`, or another accountant-approved enumeration. Staff cannot invent free-text accounting behavior.
- **Campaign:** time-bound ministry goal that points to one or more funds; it does not replace the fund.
- **Pledge:** a donor's stated intention toward a fund/campaign. It is not automatically recognized as a receivable.
- **GL mapping:** versioned mapping from branch + fund + payment/fee category to external account/dimension codes. Changing a mapping affects future exports or an explicitly regenerated export version; it never rewrites posted gifts.

Funds have active dates. Deactivation blocks new splits but preserves history and statement/report labels. Fund mergers use an explicit successor mapping; they do not rewrite old splits.

## 5. Offline counting batches

### 5.1 Workflow

1. Finance administrator opens a batch for branch, received date/service and expected methods.
2. Two named counters are assigned when church policy requires dual control.
3. Counters enter contributions and denomination/tender totals; unresolved donors remain unresolved rather than guessed.
4. System proves `contribution totals = split totals = entered batch total` and compares declared physical total.
5. Counters complete the batch. A distinct finance approver reviews variance, evidence and donor/fund exceptions.
6. Approval posts contributions and locks monetary facts in one transaction.
7. Deposit evidence is attached privately; the batch moves to deposited and then reconciled against bank/import data.

### 5.2 Batch states

`open -> counting -> counted -> approved -> posted -> deposited -> reconciled`

Exceptional transitions:

- `open|counting|counted -> voided` with reason and authorization;
- `counted -> counting` by approver with reason;
- posted batches cannot return to editable states;
- reconciliation corrections create exception/resolution records or ledger adjustments.

### 5.3 Separation of duties

- A sole counter cannot approve the same batch.
- The creator may count if policy permits, but cannot be the only approver.
- Posting and period reopen require recent MFA.
- Approval/override actions require a reason and are alerted/audited.
- Role-combination reports surface users holding counter + approver grants for periodic review.

## 6. Online giving and Paystack

### 6.1 Initiation

The server creates a local payment intent with a random internal reference, expected amount/currency/fund splits and optional donor hints, then initializes Paystack. The browser receives only the authorization URL/reference. Client redirects are never proof of payment.

### 6.2 Webhook processing

1. Read bounded raw request body.
2. Verify the Paystack signature using the configured secret before parsing into trusted state.
3. Store an inbox record unique by provider event identity/hash with verification status.
4. Acknowledge duplicates safely.
5. Match provider reference to the local intent and verify amount, currency and supported event transition.
6. Transactionally post/update normalized provider state, contribution/audit/outbox records.
7. Retry transient internal failures; permanent mismatches enter a finance exception queue.

Events may arrive late, duplicated or out of order. State transition rules prevent success from being downgraded by an older pending event. The system may verify a transaction through Paystack's server API during exception handling, but polling does not replace webhook signature validation.

### 6.3 Refunds and chargebacks

Provider refunds/chargebacks create linked negative adjustment entries and reconciliation exceptions. They never mutate or erase the original contribution. Fees are settlement-level accounting export lines unless accountant policy requires a different mapping; donor statements show gross contribution and approved corrections, not processor-net amounts.

## 7. Settlement and reconciliation

### 7.1 Sources

- Paystack settlement/export or verified API data;
- bank statement CSV in a versioned, staged import format;
- physical deposit record linked to offline batches.

Raw import files are private, hashed and retention-limited. Parsed rows retain provenance and stable source row identity for idempotency.

### 7.2 Matching

Deterministic matches use provider reference, deposit reference, amount/currency and date window. Suggested matches display evidence and confidence but require human resolution when not exact. A person/donor match is separate from a bank settlement match.

### 7.3 Invariants

- Reconciled settlement gross = matched posted online contributions plus recognized adjustments for the settlement.
- Settlement net = gross - provider fees - approved other deductions.
- Offline deposit total = matched posted batch totals plus approved variance resolution.
- Every difference is either zero or represented by an open/approved exception with owner and reason.
- A closed finance period has zero unexplained reconciliation variance.

Reconciliation status is append-only evidence. “Unreconcile” creates an approved reopen event and new reconciliation version.

## 8. Fiscal periods and close

Periods are organization-configured, timezone-aware and non-overlapping. Closing a period:

1. proves all included batches are posted and required settlements/deposits reconciled;
2. lists unresolved donors, open exceptions, pending/failed provider intents and statement-impacting adjustments;
3. snapshots report metric/mapping versions and source watermark;
4. requires finance approver confirmation and recent MFA;
5. blocks backdated posting/adjustment into that period.

Reopening is exceptional: authorized approver distinct from requester, documented reason, alert, audit event and regenerated export/statement impact analysis.

## 9. Statements and receipts

- Receipts are generated only for posted contributions and reference the immutable receipt number.
- Statements are deterministic snapshots for a person/household, date range, branch scope and statement version.
- Household attribution follows the contribution's recorded attribution at posting time; later household changes do not silently move historical gifts.
- Anonymous/unresolved gifts do not appear on a member statement until an authorized finance match creates a documented adjustment/attribution event.
- Corrections regenerate a new statement version and preserve prior artifact hash/delivery audit.
- Delivery uses a verified member session or time-limited private link; email attachments depend on approved privacy policy.
- Receipt/tax wording and required fields remain configuration-gated until the church's accountant confirms Ghana requirements.

## 10. Accounting export

Each export run records:

- period, branches, mapping version, source watermark and creator/approver;
- contribution/fund totals, payment method/deposit/settlement groupings, fees and adjustments;
- external account/dimension codes;
- row count, totals, generated artifact hash and prior/replacement export link.

An export is immutable. Corrections produce a delta/reversal or replacement export according to the target accounting system's approved import method. The finance dashboard must reconcile export totals back to the same posted ledger query.

Initial output is a documented UTF-8 CSV with CSV-injection protection. Provider-specific integrations are adapters added after the target accounting package is confirmed.

## 11. Permissions

| Capability | Counter | Finance admin | Approver | Auditor | General staff/member |
|---|---:|---:|---:|---:|---:|
| Enter assigned batch | Yes | Yes | Optional | No | No |
| View donor gift detail | Assigned batch only | Yes | Yes | Read by mandate | Own only / No |
| Manage funds/mappings | No | Yes | Approve policy | Read | No |
| Post batch | No | Request | Yes | No | No |
| Reconcile | No | Yes | Approve overrides | Read | No |
| Close/reopen period | No | Request | Yes | Read | No |
| Export finance data | No | By explicit grant | Yes | Time-bounded | No |
| Issue statement | No | Yes | Policy dependent | No | Own statement only |

Amounts and donor identity are absent from general engagement/care views. Aggregate leadership dashboards apply authorized branch scope and do not become a route to donor-level drill-down.

## 12. Required invariants and tests

1. Money uses signed 64-bit minor units with overflow and positive/negative state validation.
2. Contribution split sum equals header total in the posting transaction.
3. Posted contribution monetary/classification fields never update.
4. Adjustment/reversal links form an acyclic, traceable chain and net correctly.
5. Receipt, provider event/reference and import source IDs are unique in organization/provider scope.
6. Webhook processing is idempotent under duplicates, retries, reorder and concurrent workers.
7. Counter cannot approve own controlled batch; closed-period writes fail server-side.
8. Batch, settlement, fund, statement and export totals reconcile to the posted ledger.
9. Unauthorized roles cannot infer amounts through responses, counts, errors, exports, search or audit diffs.
10. Report/statement regeneration with the same inputs/version yields the same rows/totals and records artifact hash.
11. Failed external calls cannot leave partially posted contributions.
12. Migration/replay never doubles historical gifts or provider events.

Property tests generate split/adjustment/batch/settlement combinations and prove these invariants. Integration tests use a MongoDB replica set to exercise real transactions; HTTP tests cover permission and idempotency semantics.

## 13. Operational metrics and alerts

- webhook verification failures, age of unprocessed inbox/outbox events and payment intents stuck pending;
- open batch age, variance amount/count and approval SLA;
- unmatched provider transactions, deposits and unresolved donors;
- settlement/period unexplained variance;
- adjustment/refund/chargeback volume;
- period reopen, export replacement and large finance export events;
- statement generation/delivery failures.

Metrics use IDs/counts/amount aggregates appropriate to finance roles and do not place donor names/contact information in logs or alerts.

## 14. Decisions REMI must approve

1. Fiscal year/period cadence and GHS-only assumption.
2. Official fund list, restriction categories and external chart-of-account/dimension mappings.
3. Dual-counter/approval thresholds, variance tolerance and who may reopen periods.
4. Receipt numbering, wording, signature and Ghana tax/retention requirements.
5. Household versus individual statement rules and anonymous/unresolved attribution policy.
6. Supported offline tenders and actual bank/Paystack settlement formats.
7. Target accounting package and import/reversal conventions.
8. Pledge treatment, reminder consent and campaign ownership.
9. Required retention for finance records and statement artifacts.

These gates affect configuration and production sign-off. They do not justify weakening append-only posting, reconciliation, audit or separation-of-duties controls.

