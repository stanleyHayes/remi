# REMI retention and connection metric contract

Status: proposed operational contract for REMI-260  
Metric version: `retention-v1`  
Product owner: Membership and Assimilation ministry  
Data steward: Church operations lead  
Pastoral approver: designated pastoral-care lead  
Privacy approver: data protection supervisor  

This document defines aggregate ministry measures. It does not define spiritual health, faith, commitment, generosity, intent, risk, worth or pastoral need. Individual evidence may only enter the separately authorized human-review workflow in REMI-261/262.

## 1. Non-negotiable boundaries

1. Giving amounts, funds, pledges, payment regularity, donation absence and wealth proxies are prohibited inputs.
2. Anonymous headcount never becomes named attendance and never enters person-level cohorts.
3. A signal never changes membership stage, sends a message, opens a care case or labels a person automatically.
4. Only current, non-archived people and completed, non-cancelled occurrences are eligible unless a metric explicitly measures an inactive transition.
5. Corrections use the current attendance projection; effective-dated membership and roster events remain the lifecycle evidence.
6. Every result exposes metric version, branch scope, timezone, as-of time, source freshness, cohort maturity, privacy suppression and caveats.
7. Aggregate cells with fewer than five eligible people are suppressed. Suppressed numerator, denominator and derived rate must not be recoverable from adjacent totals.
8. Cross-branch reporting requires organization-wide permission. A person's home branch does not rewrite the branch attribution of an attendance, group or serving fact.

## 2. Shared terms

| Term | Definition |
|---|---|
| Qualifying visit | Current `present` named attendance at a completed, non-cancelled service occurrence. Duplicate person/occurrence facts count once. |
| First visit | Earliest qualifying visit in all available history, not merely the selected report range. |
| Return visit | A later qualifying visit at least one calendar day after the first visit. Multiple services on the same local date do not manufacture a return. |
| Cohort date | Local calendar date of the qualifying first visit in the occurrence branch timezone. |
| Eligible/mature | The as-of instant is on or after the end of the full observation window. Immature people are excluded from the denominator, not treated as non-returners. |
| Connected group | A group membership whose current status is `active` and whose `joinedAt` falls in the observation window. Waitlisted, invited, declined and ended entries do not qualify. |
| Connected serving | A volunteer assignment with status `accepted` or a completed assignment without a no-show state. Invitation alone does not qualify. |
| Inactive transition | An immutable membership event whose `toStage` is `inactive`, attributed by its effective date and branch scope. |
| Missing data | A source needed by a metric is unavailable, stale or has incomplete branch coverage. Missing never means false or zero. |

All local-calendar comparisons load the IANA timezone configured on the attributed branch. Storage and API instants remain UTC.

## 3. Metric dictionary

### 3.1 First-to-second visit conversion

- **Owner:** Membership and Assimilation ministry.
- **Question:** Of first-time visitors old enough to observe the configured return window, how many attended on a later local date?
- **Default window:** 30 days; configurable from 7 to 90 days.
- **Numerator:** distinct mature first visitors with at least one return visit after the first local date and no later than `firstVisit + window`.
- **Denominator:** distinct first visitors whose full window ended by `asOf`.
- **Unit/grain:** percentage; weekly first-visit cohort and selected branch.
- **Sources:** `chms_occurrences`, `chms_attendance` current projections.
- **Excluded:** same-day repeat services, anonymous headcount, cancelled/incomplete occurrences, absent/excused status, archived people, pre-cohort visitors.
- **Freshness target:** attendance approved within 24 hours; dashboard reports last recorded fact and occurrence coverage.
- **Reconciliation:** every numerator person must exist once in the denominator and have two distinct qualifying local dates.

### 3.2 30/60/90-day return

- **Owner:** Membership and Assimilation ministry.
- **Question:** Did a first visitor return at least once within each fully matured observation window?
- **Numerator:** mature cohort people with a qualifying return after the first local date and on/before day 30, 60 or 90 respectively.
- **Denominator:** calculated independently per window; people enter only after that window matures.
- **Unit/grain:** three percentages; weekly first-visit cohort and selected branch.
- **Monotonicity invariant:** for an equally mature population, `returned30 <= returned60 <= returned90`. Denominators may differ because windows mature at different times.
- **Reconciliation:** recomputation from ordered named attendance must match the aggregate; immature cohorts display `pending`, never `0%`.

### 3.3 Group connection after first visit

- **Owner:** Groups ministry, jointly reviewed with Membership and Assimilation.
- **Question:** How many mature first visitors joined a group after visiting?
- **Default window:** 90 days; configurable from 30 to 180 days.
- **Numerator:** distinct mature first visitors with a connected-group `joinedAt` after the first visit and on/before the window end.
- **Denominator:** first visitors whose full connection window has matured.
- **Unit/grain:** percentage; weekly first-visit cohort and branch of first visit.
- **Sources:** qualifying visits plus `chms_group_memberships`; branch mismatch is retained as an evidence dimension rather than discarded.
- **Excluded:** applications, invitations, waitlists, declined/ended memberships, reactivation predating the first visit, leader notes and directory visibility.
- **Reconciliation:** numerator membership IDs must resolve to unique people in the cohort with qualifying status/timestamp.

### 3.4 Serving connection after first visit

- **Owner:** Volunteer/Serving ministry, jointly reviewed with Membership and Assimilation.
- **Question:** How many mature first visitors accepted or completed a serving assignment after visiting?
- **Default window:** 120 days; configurable from 30 to 180 days.
- **Numerator:** distinct mature first visitors with an accepted assignment or completed non-no-show assignment starting after the first visit and within the window.
- **Denominator:** first visitors whose full serving window has matured.
- **Unit/grain:** percentage; weekly first-visit cohort and branch of first visit.
- **Sources:** qualifying visits plus `chms_assignments`.
- **Excluded:** invitations without acceptance, declined/cancelled assignments, no-shows, reminders, response reasons and coordinator notes.
- **Reconciliation:** every numerator assignment must identify one cohort person and satisfy status and time predicates.

### 3.5 Combined community connection

- **Owner:** Membership and Assimilation ministry.
- **Question:** How many mature first visitors connected through either a group or serving opportunity?
- **Window:** maximum of the approved group and serving windows, evaluated with each signal's own deadline.
- **Numerator:** distinct people satisfying group connection, serving connection or both.
- **Denominator:** people mature for both component windows.
- **Unit/grain:** percentage and non-overlapping counts: group only, serving only, both, neither.
- **Reconciliation:** `groupOnly + servingOnly + both + neither = denominator`; overlap is never double-counted.

### 3.6 Inactive-transition rate

- **Owner:** Membership administration; pastoral-care lead reviews interpretation.
- **Question:** What proportion of the opening active membership population had an authorized transition to inactive during the period?
- **Numerator:** distinct people with an effective `toStage=inactive` event in the selected period, excluding reversed events and terminal transferred/deceased transitions.
- **Denominator:** distinct people in `member` or `regular-attendee` stage immediately before the period starts, plus people entering either stage during the period before an inactive transition.
- **Unit/grain:** percentage; calendar month and organization. Branch grain is unavailable in `retention-v1` until membership events snapshot an effective `branchId`; using the person's current home branch would rewrite history and is prohibited.
- **Sources:** immutable membership lifecycle events plus the person projection for archive state.
- **Excluded:** automated engagement signals, archive actions without a membership transition, transfers, deceased records and reversed transitions.
- **Reconciliation:** every numerator has an effective event and existed in the eligible population before that event; current-stage counts are not substituted for lifecycle history.

### 3.7 Reactivation after inactivity

- **Owner:** Membership administration.
- **Question:** How many inactive transitions were followed by an authorized return to returning guest, regular attendee or member?
- **Default window:** 180 days; configurable from 30 to 365 days.
- **Numerator:** mature inactive episodes followed by a valid effective reactivation transition within the window.
- **Denominator:** inactive episodes whose full window matured; a person may contribute multiple non-overlapping episodes, reported separately from distinct-person counts.
- **Unit/grain:** episode rate and distinct-person count; month of inactive transition and organization. Branch breakdown has the same event-snapshot gate as inactive-transition rate.
- **Reconciliation:** transition sequence must be valid and ordered; reversed inactive events contribute neither numerator nor denominator.

## 4. Configuration contract

Configuration is versioned and effective-dated. A published version contains:

```json
{
  "version": "retention-v1",
  "timezoneSource": "branch",
  "privacyThreshold": 5,
  "firstToSecondDays": 30,
  "returnWindowsDays": [30, 60, 90],
  "groupConnectionDays": 90,
  "servingConnectionDays": 120,
  "reactivationDays": 180,
  "attendanceFreshnessHours": 24,
  "owners": {
    "retention": "membership-assimilation",
    "groups": "groups-ministry",
    "serving": "volunteer-ministry",
    "inactive": "membership-administration"
  }
}
```

Changes require a reason, effective date and approver. Historical reports retain the configuration version used; new configuration never rewrites past snapshots.

## 5. Missing-data and caveat rules

- If no completed occurrence exists, return an empty report with coverage—not a zero-retention conclusion.
- If completed occurrences lack named attendance, mark named coverage incomplete and withhold affected rates when reliable eligibility cannot be established.
- If group or serving stores are unavailable, the component metric is `unavailable`; combined connection is also unavailable.
- If a branch timezone is invalid or absent, fail validation rather than silently using server time.
- If a requested grain lacks historical attribution in the source event (currently membership-transition branch), return `unavailable` with the schema caveat rather than joining to a mutable current profile.
- Corrections after report generation change the source watermark and create a new snapshot; old exported snapshots remain identifiable.
- Sparse cohorts are suppressed before pagination/export. Organization totals must not reveal a suppressed branch by subtraction.

## 6. Authorization and presentation

- Aggregate retention dashboard: roles with branch-scoped `read` on retention analytics.
- Individual evidence: only the later human-review permission; never included in aggregate downloads.
- Group leaders see only owned-group operational facts, not organization retention cohorts.
- Finance roles gain no retention access merely through finance permissions.
- Member-facing applications expose none of these internal metrics or signals.
- Charts always show the denominator, maturity, suppression state, as-of time, timezone, metric version and caveats alongside the rate.

## 7. Acceptance and reconciliation tests

1. Same-day attendance twice is not a second visit; next-day attendance is.
2. A 29-day return counts in 30/60/90; a 45-day return counts only in 60/90; a 91-day return counts in none.
3. Immature people are excluded from the relevant denominator and labelled pending.
4. Duplicate/current corrected attendance counts once and follows the current status.
5. Anonymous headcount cannot change any named cohort.
6. Waitlisted group and invited serving records do not count; active join and accepted/completed assignment do.
7. One person connected to both group and serving contributes once to combined connection and once to the `both` partition.
8. Reversed inactive transitions do not count; repeated non-overlapping inactive episodes remain auditable.
9. Branch-local dates around UTC midnight and daylight-saving boundaries produce deterministic cohort dates.
10. Cohorts under five are suppressed in UI, API and export; subtraction attacks are blocked for rollups.
11. Missing source coverage yields unavailable/caveat states, never false zeroes.
12. Static and runtime guards prove no finance collection, pledge, fund or contribution field enters retention queries or payloads.

## 8. Activation gate

`retention-v1` remains proposed until the four named owners above approve the windows, branch attribution, privacy threshold, interpretation copy and review cadence. Engineering may implement behind a feature flag and test with synthetic data, but production activation and any individual care queue require that recorded approval.
