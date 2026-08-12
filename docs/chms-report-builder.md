# Governed report builder

REMI's report studio is deliberately not a raw query tool. It composes canonical measures through the same domain services used by operational dashboards, so a report cannot bypass attendance semantics, finance reconciliation or pastoral assignment scope.

## Approved contract

- Scope: one explicit branch, a valid IANA timezone and a range no longer than 366 days.
- Dimensions: `metric`, with optional `domain`. Person, household, donor, contact and free-text dimensions are rejected.
- Measures: confirmed attendance, approved headcount, active group connections, serving fill rate, assigned open care cases, net posted giving and unexplained settlement variance.
- Authorization: every metric is checked against the current principal's resource, branch and field-class grants before execution and again before an artifact download.
- Sparse data: attendance and group connection cells below five are `suppressed`; value and denominator are cleared before JSON or CSV serialization.
- Unavailable data: missing denominators such as zero planned serving positions produce `unavailable`, not zero percent.

## Saved views

Saved views contain a name and the validated query only. They are scoped to organization plus owner, carry an optimistic version, and cannot be read or updated by another staff identity. Updating requires the current version; stale writes fail.

## Asynchronous exports

An export request executes and stores the authorized aggregate snapshot, records an audit event, and returns a `pending` run. The background worker claims the oldest pending run, generates a UTF-8 CSV, neutralizes spreadsheet-formula prefixes, stores the exact artifact SHA-256 and marks the run complete. Downloads require the original owner, reauthorization for the complete saved query, `private, no-store`, attachment disposition and a successful SHA-256 integrity check.

The snapshot contains aggregate cells only; no person, household, contact, care-note, donor or provider payload is exported.
