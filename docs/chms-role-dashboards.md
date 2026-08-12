# REMI role dashboard contract

The admin overview is a permission-aware composition of existing governed domain projections. It does not maintain a second analytics truth or infer access from the visible interface alone: each source request is independently authorized by the API.

## Perspectives

| Responsibility | Default dashboard emphasis | Sensitive boundaries |
|---|---|---|
| Executive / super administrator | Attendance, care assignments, group connection, serving and aggregate finance | Full access remains auditable; individual pastoral and donor records are not placed on the overview |
| Pastor | Assigned open care, attendance and privacy-safe retention | No finance field class |
| Branch administrator | Attendance, groups and serving for the selected branch | No pastoral-sensitive or finance field class |
| Membership administrator | Named attendance and membership-journey operations | No care notes, engagement signals or finance |
| Groups administrator | Active connections, group capacity and meeting outcomes | No people-directory export, care or finance |
| Volunteer coordinator | Planned versus filled serving positions | No screening detail on the overview; no finance |
| Finance operator | Posted giving and unexplained settlement variance | No attendance, care, retention or group requests |

The interface also supports content editor and general viewer fallbacks. These receive only source projections their API grants permit.

## Display rules

- The selected branch and a rolling 90-day window are explicit.
- Cards use canonical metric identifiers from `reporting-dictionary-v1`; definitions are one click away.
- A rejected or unavailable domain request removes that domain rather than substituting zero.
- Zero is displayed only when an authorized source explicitly returns zero.
- Retention rates are displayed only for a mature, publishable cohort; protected cells remain labelled `Protected`.
- Finance values come from the immutable finance report and settlement reconciliation projection.
- Care is the signed-in pastor's assigned open queue, not an organization-wide count.
- Loading, restricted and partial-access states are first-class and do not expose authorization failures from another domain.

Branch assignment scopes are part of REMI-290. Until user records carry explicit branch grants, operational roles use the current organization-wide branch wildcard and each dashboard request still requires an explicit branch. This limitation must not be described as branch-level least privilege in release evidence.
