# Scheduled reports and board packs

Status: implemented under `REMI-283` on 2026-08-12.

## Product boundary

The scheduler distributes governed report snapshots; it is not an arbitrary query runner or mailing list. A schedule references one personal saved view at an exact version. Weekly and monthly cadences can produce a formula-safe CSV or a ZIP containing `report.csv` and `manifest.json`.

Recipients are selected from accepted, active REMI staff accounts. The API accepts staff IDs only and resolves email addresses server-side, preventing delivery to an unapproved address. The current implementation is organization-wide because the legacy staff collection has no organization field; multi-tenant rollout must add that ownership field before enabling another organization.

## Lifecycle and controls

1. Creation revalidates the saved query and recipient accounts, persists the schedule, and appends `report.schedule.create` evidence.
2. The worker atomically leases one due schedule. It rebuilds the owner's principal from the current active role and pauses when the owner, saved-view version, or metric permission is no longer valid.
3. The authoritative report projections run again. The artifact stores the exact query result, source watermarks, dictionary version, saved-view version, SHA-256, row count, format and generation time.
4. Each approved recipient gets a separate seven-day delivery record and an authenticated admin link. Send success or failure is audited without placing the address in the audit record.
5. Opening or downloading resolves the recipient from the access token, reauthorizes every metric, verifies both delivery and artifact hashes, and records the first open. Expired links are denied and marked expired.

The scheduler moves the next due date by seven days or one calendar month from the intended run. A ten-minute lease prevents duplicate concurrent workers; a failed worker can be retried after the lease.

## Operator experience

The Report studio provides branded saved-view, cadence, format, custom date/time and approved-recipient controls. There are no native select or date inputs. Operators see upcoming schedule state, next delivery, recipient count, bound view version and safe pause reason. A delivery link opens an authenticated summary with hash prefix, expiry and download action.

## Verification contract

- Race tests cover active-recipient approval, current-role authorization, cross-recipient concealment, expiry, inactive-owner pause, ZIP contents and provenance.
- CSV protections inherited from the report builder remain active inside ZIP packs.
- Downloads return `private, no-store`, `nosniff` and attachment headers.
- Production readiness requires a configured email provider and worker process; send failure is retained and audited rather than reported as successful.
