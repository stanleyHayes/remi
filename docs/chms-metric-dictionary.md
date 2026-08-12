# REMI metric dictionary

REMI publishes a number only when its meaning, source, ownership and safeguards are explicit. The canonical machine-readable definitions live in `apps/api/internal/chms/reporting/dictionary.go` and are exposed at `GET /api/chms/v1/reporting/metrics`.

## Contract

Every metric declares:

- a stable identifier and semantic version;
- a human owner and product domain;
- its unit, formula, source collections and reporting grain;
- organization, branch or ministry scope and the timezone used for boundaries;
- exclusions and a bounded freshness expectation;
- a reconciliation rule back to authoritative source records;
- a privacy rule and an internal resource/field classification.

The service validates the dictionary at startup. Empty, duplicate or incomplete definitions fail closed. API output is private and non-cacheable, contains no internal grant metadata, and includes only definitions the staff principal may read. Financial and sensitive-ministry definitions therefore do not leak into ordinary operational roles.

## Versioning and change control

Formula, grain, exclusion, timezone or source changes require a metric version change. Copy-only clarification may retain the version. Dashboards and exports must carry the metric identifier/version and their own source watermark so a displayed result can be reconstructed against the matching definition.

The current `reporting-dictionary-v1` catalog covers people, households, attendance, groups, serving, pastoral care, retention, giving, settlement reconciliation, communications and data quality. REMI-281 will bind role-specific live dashboard snapshots to these definitions; REMI-282 will constrain report-builder measures and dimensions to the same governed catalog.

## Acceptance checks

1. Dictionary validation rejects empty, incomplete and duplicate entries.
2. Entries are returned in stable identifier order.
3. Permission tests prove ordinary viewers cannot see finance, care or retention definitions and finance roles see only authorized financial measures.
4. HTTP tests prove private caching and absence of internal authorization fields.
5. The admin reporting library renders loading, restricted, error and populated states without native controls.
