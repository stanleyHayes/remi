# REMI immutable audit governance

Status: REMI-291 implemented and production-verified locally on 2026-08-12.

## Purpose

The audit surface answers who performed an action, what controlled resource or subject it affected, when it happened, why it was performed, whether it succeeded, and which safe fields or versions changed. It is an assurance record, not a back door into pastoral, safeguarding, identity, contact or donor data.

## Safe projection

Every returned row is limited to immutable metadata already written to `chms_audit_events`:

- event ID, organization and optional branch;
- actor type and stable actor ID;
- action, outcome and occurrence time;
- resource type and stable resource ID;
- stable subject IDs;
- reason and request ID;
- changed field names plus optional before/after aggregate versions.

No before/after domain values are accepted or reconstructed. Restricted-note bodies, message bodies, contact destinations, payment data, passwords, MFA material, invitation/refresh tokens, provider payloads and encryption envelopes are forbidden from the schema, API, UI and exports.

## Authorization

- `super-admin` and `data-protection-supervisor` may review organization-wide safe metadata.
- `auditor` is read/export-only and must remain within assigned branches. An organization-wide request from a branch-scoped auditor is denied rather than silently widened.
- `finance-auditor` retains its existing finance assurance access; it does not inherit the cross-domain audit stream.
- every list, detail and export request reauthorizes current role and branch scope. Unknown roles fail closed.
- exports require recent MFA and a 10–300 character handling reason.

## Query and export controls

Queries use allowlisted exact filters for branch, actor, action, resource type/resource ID, subject ID, request ID, outcome and an RFC3339 time range. Results use stable descending keyset pagination and a maximum page size of 100. No regex, arbitrary field path, raw Mongo predicate or full-text search is accepted.

CSV exports repeat the same query and authorization, cap output, neutralize spreadsheet formula prefixes, include a SHA-256 digest, use attachment plus `private, no-store` and `nosniff` headers, and append an `audit.export` event containing only the filter field names, row count and artifact hash metadata. Reading audit history is itself recorded without recursively including the newly written read event in the response.

## Verification gates

- cross-organization and cross-branch denial;
- scoped query cannot omit its branch;
- unknown filters and invalid ranges fail validation;
- no forbidden key/value enters JSON, CSV, logs or audit-of-audit evidence;
- cursor scope cannot widen a query;
- formula-like text is neutralized in CSV;
- export requires recent MFA and a reason;
- artifact hash matches returned bytes;
- append-only storage has no update/delete route;
- desktop/mobile, keyboard, dark-theme and zero-native-control checks for the admin workspace.
