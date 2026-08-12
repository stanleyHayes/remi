# Reporting hardening and operating budgets

Status: verified for `REMI-284` on 2026-08-12.

## Runtime budgets

- Interactive governed queries have an eight-second server deadline. Cancellation is propagated to every authoritative domain projection and database operation.
- The production service objective is p95 below two seconds for a report containing up to twelve measures over at most 366 days. Local seeded verification recorded 6 ms p50 and 47 ms p95 over five uncached HTTP runs.
- Scheduled and CSV exports snapshot the interactive result before artifact work, so a worker never obtains a longer-lived or broader database capability.
- Queries are bounded to one branch, one time range, twelve metrics and two fixed dimensions. The source repositories use organization/branch/time or organization/assignment/state compound indexes; there is no raw-field or unbounded aggregation API.

The eight-second limit is a safety ceiling, not a target. Production alerting should warn when five-minute p95 exceeds two seconds or the timeout rate exceeds one percent. Capacity tests in `REMI-312` remain responsible for validating these thresholds at agreed congregation volume.

## Reconciliation invariants

The report service now rejects a projection when it:

- omits a requested metric, emits an unrequested metric, or duplicates a metric;
- changes the canonical domain or unit;
- lacks generation/as-of timestamps;
- emits an unknown availability state;
- reports NaN, infinity, a negative/fractional count, an out-of-range percentage, or a non-positive denominator.

Suppressed and unavailable cells have values and denominators removed before serialization. Available cells must contain a valid value. Live verification compared the governed attendance cells with the same-window attendance projection: the named count was correctly suppressed at two, and the headcount value matched its source at zero.

## Time and empty-data behavior

The request retains its IANA timezone and exact RFC3339 instants as provenance. `Africa/Accra` and an exact 366-day range are accepted; an unknown zone, inverted interval, or any duration over 366 days fails before source execution. Domain repositories compare UTC instants while occurrence and branch timezone remains part of the source record.

Empty source data is not silently recast as success. A meaningful aggregate zero may be available, a missing denominator is unavailable, and a small identifiable cohort is suppressed. These states are tested independently and remain intact through CSV and ZIP generation.

## Authorization abuse cases

- Saved views, schedules and export runs are owner-scoped.
- Export retrieval reauthorizes every captured metric; owner identity alone is insufficient after a role change.
- Scheduled runs rebuild the owner principal from the current role.
- Deliveries require the exact signed-in recipient, current metric permission, a future expiry and matching SHA-256 values on both delivery and artifact.
- Cross-recipient and cross-owner requests are concealed as unavailable; arbitrary recipient email input is unsupported.

Race tests exercise these cases, worker leasing and lifecycle state transitions. The full API suite and Go vet are required release evidence.
