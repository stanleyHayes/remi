# REMI retention safety and pastoral UAT

Status: engineering-ready; organizational sign-off pending  
Metric contract: `retention-v1`  
Workspace: `/retention/safety`

This is the acceptance record for REMI-264. Automated evidence can prove source boundaries, invariants, authorization and serialization behavior. It cannot decide whether the language and intervention model are pastorally appropriate for REMI. Product, pastoral and privacy reviewers must complete the version-bound workflow themselves; seed fixtures and engineering test accounts are not approval.

## Acceptance matrix

| Risk | Required behavior | Authoritative evidence | Current engineering state |
|---|---|---|---|
| Opaque or diagnostic language | Every individual observation shows dated evidence, caveats, expiry and a human-review warning; no score or inferred faith, intent, worth or need | engagement evaluator/unit tests, review-detail HTTP test, browser review-detail QA | Implemented and verified |
| Unequal treatment | Identical participation histories produce identical classifications regardless of person identifier; demographics are absent from inputs | symmetry and source-boundary unit tests | Implemented; included in the release suite |
| Immature data read as failure | A cohort enters each denominator only after its entire local-calendar window closes; pending is never 0% | 29/45/91-day, same-local-date and maturity tests | Implemented and verified |
| Sparse cohort disclosure | Below five eligible people, cohort size, numerator, denominator, rate and combined partitions are absent in API and DOM | sparse serialization tests, live one-protected-cohort fixture, browser DOM audit | Implemented and verified |
| Subtraction disclosure | No aggregate total is returned alongside weekly suppressed cells | response-schema inspection and dashboard DOM audit | Implemented and verified |
| Giving-based inference | Contributions, pledges, funds, donation behavior and wealth proxies are absent from repositories, formulas and payloads | static source guards plus API/DOM inspection | Implemented and verified |
| Unauthorized access | Branch-sensitive retention reads/actions require staff grants; member and owned-unit leadership access is denied | service authorization and authenticated HTTP tests | Implemented and verified |
| Automatic harmful action | No signal sends, changes membership or creates a care case; outreach recording requires assignment and live purpose/channel consent | review lifecycle, wrong-reviewer, denied-consent and immutable-event integration tests | Implemented and verified |
| Stale approval | Human decisions bind to the complete published-policy fingerprint and stop counting when it changes | safety-review integration test | Implemented and verified |
| Impersonated approval | Only the reviewer ID already named for the role on every published policy may submit | wrong-reviewer service test and authenticated HTTP test | Implemented and verified |

## Required human walkthrough

Each named reviewer signs into the admin and opens `/retention/safety` for each branch.

1. Open at least one actionable observation and trace every statement to source evidence.
2. Confirm the caveats prevent the observation from being read as a diagnosis or spiritual score.
3. Confirm assignment, consent denial, snooze, suppression and false-positive feedback copy is humane and operationally clear.
4. Open Connection Pathways and inspect an available, pending and privacy-protected cohort.
5. Confirm no hidden value can be inferred from totals or neighboring cells.
6. Confirm the interface never suggests that attendance, group or serving activity proves faith, worth, intent or generosity.
7. Record concerns in the findings field and choose `changes-required` whenever wording, windows or ministry practice needs revision.
8. Approve only after all eight checklist statements are true for the displayed policy fingerprint.

## Release rule

The retention release gate remains `awaiting-human-review` until current-fingerprint decisions from the named product, pastoral and privacy reviewers are all `approved`. Any published policy change creates a new fingerprint and returns all three roles to pending. Organizational approval must not be inferred from a green CI run, local seed, super-admin role or this document.

## Engineering verification commands

```sh
cd apps/api
go test -race ./...
go vet ./...

cd ../admin
pnpm run build

cd ../..
rg -n '<select|type="(date|time|datetime-local|month|week)"' apps/admin --glob '*.tsx' --glob '*.ts'
git diff --check
```

The source scan must return no matches. Browser acceptance is performed in authenticated dark theme at desktop and 390×844, checking active navigation, native controls, page overflow, application-origin errors, hidden identifiers and the honest pending-human-review state.
