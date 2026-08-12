# Staff permission scopes

Status: REMI-290 implemented and production-verified locally on 2026-08-12.

## Effective policy input

Every staff decision combines organization, role-derived actions/resources, persisted branch IDs, optional ministry IDs, assigned resource IDs, requested field classes and session assurance. The shared authorizer denies by default.

- A grant containing explicit branch IDs cannot authorize a request that omits `branchId`.
- A grant containing explicit ministry IDs cannot authorize a request that omits `ministryId`.
- Assignment-required decisions require a concrete `resourceId` present in the principal's current assignment list.
- `secret` fields are never authorized. Personal, sensitive-ministry, child-safeguarding and financial fields remain distinct.
- Organization mismatch, unknown role, empty operational scope, resource/action mismatch and stale MFA all deny.

## Session and background behavior

Operational invitations require at least one branch. The invitation stores only normalized IDs; active staff tokens carry those scopes as signed claims. Super administrators receive explicit wildcard branch/ministry scope. Non-global roles with no branch assignment receive no operational access rather than an implicit wildcard.

Profile updates and MFA completion reissue the token with the scopes currently stored on the user. Scheduled reporting does not trust an old token: it reloads role, branch, ministry and assignment scopes from the active staff record before each run.

Every newly issued staff token is bound to the user's `accessVersion`. `PATCH /api/admin/users/{id}/scopes` uses optimistic concurrency, requires a reason, validates every branch/ministry against current records, increments that version and writes immutable audit evidence in the same transaction. Staff middleware compares the signed version, current role and active account state to the database on every staff request. Consequently all prior tokens fail immediately with `401` after a scope change; the user must sign in again. Unversioned tokens exist only as a temporary migration bridge and expire naturally.

Role mutations use the same endpoint and optimistic version. They additionally require MFA verified within the previous ten minutes, cannot target the caller's own role, and cannot demote the final active super administrator. A successful role mutation records `staff.access-role.update`, increments `accessVersion` and revokes all existing target sessions immediately.

## Resource-bound ministry enforcement

Groups persist an optional `ministryId` alongside their branch. Create, detail, update, roster and meeting paths authorize against the stored branch/ministry pair; list results are filtered before serialization, and cross-ministry detail is concealed as not found. A grant with explicit ministry IDs must receive and match a resource ministry. A role intentionally unrestricted by ministry can operate on tagged resources, while an explicitly scoped role cannot see legacy unassigned records.

Existing unassigned groups remain readable to global and branch-wide administrators so they can be classified in the branded Groups workspace. They are deliberately absent from ministry-scoped operator results until an authorized administrator assigns an owning ministry; the system never guesses ownership during startup or silently broadens a scoped operator.

Restricted care cases require a persisted `ministryId` at creation. Detail, reassignment, closure, contact-event and restricted-note paths require both a matching sensitive-ministry grant and current assignment through the case's `assignedUserIds` or the principal's assigned-resource claim. Reassignment therefore removes the former assignee immediately. List queries load only the caller's assigned cases and apply the same ministry check before serialization. Note bodies remain encrypted at rest, absent from case/list responses and available only through the audited restricted-note route.

## UI contract

The invitation workspace uses branded toggle cards for branch and ministry selection. Operational invitations cannot be submitted until a branch is selected. The user list exposes a safe scope count without revealing sensitive assignments. Super administrators can reopen a non-global account's scope, choose current branches/ministries, provide the audit reason and save with a clear session-revocation warning.

## Verification

Tests cover omitted-scope denial, cross-organization, cross-branch, cross-ministry, cross-assignment, field-class and recent-MFA decisions; scoped JWT round trips; operational invitation validation and persistence; stale scope update conflict; immediate old-token rejection; reasoned audit creation; and current-scope background-principal reconstruction. Browser verification covers custom controls, disabled-state behavior, dark-theme contrast and zero horizontal overflow at desktop and 390 px.
