# REMI member app production readiness

Status: **engineering work in progress**  
Owner: Member + Backend + QA  
Ledger: REMI-330

This is the release evidence matrix for `apps/member`. A green production
build is necessary but not sufficient. Human legal, privacy, pastoral and UAT
decisions remain separate from automated engineering evidence.

| Gate | Required evidence | Current state |
| --- | --- | --- |
| Authorization | Self/household boundaries, cross-member denial, minors and restricted-field negatives through the real router | Automated domain and real-router negative coverage exists; member-wide release suite still being consolidated |
| End to end | Passwordless sign-in through every primary member destination and mutation, including expired session recovery | Seeded OTP production-mode navigation is automated across all primary destinations; mutation and expired-session journeys remain |
| Accessibility | Keyboard, focus, labels, landmark structure, reduced motion, 390 px, contrast and automated WCAG scan | Desktop and Pixel-sized WCAG A/AA scans pass for public recovery and all primary authenticated routes; branded year picker has pointer and keyboard regression coverage |
| Poor network | Explicit slow/offline state, retry guidance and duplicate-safe mutations | Connectivity feedback and safe retry guidance implemented; deployed throttling evidence remains |
| Offline | Safe full-navigation fallback with no authenticated HTML/API caching; no claim of offline writes | Production E2E proves generic fallback and that authenticated routes/API records are absent from Cache Storage |
| Installability | Valid manifest, icon, HTTPS production check, standalone behavior and install guidance | Manifest, icon, service worker and branded install guide implemented; deployed HTTPS/standalone evidence remains |
| Errors | Route loading, route error, global error and branded not-found surfaces | Implemented and included in automated public accessibility sweep |
| Observability | Privacy-minimized client failure/performance events with retention and volume bounds; server health and alert owner | Finite identity-free client event intake, 30-day TTL and authorization tests implemented; production alert owner remains |
| Privacy/legal | Accessible privacy summary, channel/consent controls, data-rights path, terms/notice ownership and approval state | Accessible engineering drafts and member controls implemented; accountable-owner approval remains explicitly required |
| Support | In-app help, correction/problem path, contact route, reference-safe error guidance and escalation owner | Branded support hub and routes implemented; named escalation owner remains |
| Security headers | No framing/sniffing, conservative referrer/permissions policy, service-worker cache policy | Implemented in member Next config and service-worker response policy |
| Performance | Production route budgets and Lighthouse evidence on deployed infrastructure | Missing |
| Rollout | Pilot branch, UAT representatives, support owner, known limitations, rollback criteria and 30-day hypercare | Requires REMI owner decisions |

## Non-negotiable implementation rules

- The service worker may cache only versioned public shell assets and the
  generic offline document. It must never cache authenticated pages, RSC
  payloads, BFF/API responses, receipts, statements or uploaded media.
- Offline writes are not queued. Member mutations remain server-authoritative,
  idempotent where their domain requires it, and show a clear retry state.
- Client telemetry accepts a finite event vocabulary and coarse route name;
  it excludes names, destinations, tokens, request bodies, free text, IDs and
  financial/care content.
- Legal/privacy copy is an engineering summary until REMI's accountable owner
  approves the official notice, processor inventory, retention schedule and
  support contacts. The UI must state that distinction truthfully.
- Automated checks cannot mark human UAT, legal approval or go-live authority
  complete.
