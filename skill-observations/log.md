# Skill Observation Log

Observations captured during task-oriented work.

**Status key:** OPEN = not yet actioned | ACTIONED (YYYY-MM-DD) = skill
updated/created | DECLINED (YYYY-MM-DD) = user decided not to pursue —
resolved statuses always carry their resolution date

---

## 2026-08-11

### Observation 1: Provider webhooks need an invariant-first implementation workflow

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Implementing a signed payment-provider lifecycle that posts into an immutable subledger.
**Skill:** New skill candidate: provider-webhook-lifecycle
**Type:** open-source
**Phase/Area:** Architecture, implementation, and adversarial verification

**Issue:** A provider redirect integration can look complete while omitting the durable local intent, exact raw-body signature check, monotonic event state, retryable inbox, amount and currency verification, immutable financial side effects, and duplicate or reordered delivery proofs. These gaps are coupled, so adding only an endpoint or signature check leaves unsafe partial behavior.

**Suggested improvement:** Capture a reusable workflow that begins with provider and ledger invariants, models intent/inbox/exception state before handlers, separates durable receipt from processing, preserves backward-compatible initiation routes, and requires duplicate, concurrency, reordering, mismatch, refund, dispute, and dependency-failure tests before production verification.

**Principle:** External payment events become trustworthy business state only after durable receipt, cryptographic origin validation, semantic verification, monotonic transition control, and idempotent atomic side effects are all proven together.

### Observation 2: Recovery-sensitive integration suites need layered evidence

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Production-verifying a financial lifecycle while a local database recovered from an unclean shutdown and exhibited highly variable index latency.
**Skill:** New skill candidate: production-integration-verification
**Type:** open-source
**Phase/Area:** Test infrastructure, diagnosis, and release evidence

**Issue:** A broad integration run can report failure solely because shared infrastructure exceeds a short package-wide deadline, even while the same application invariants pass in focused real-database runs. Treating every red command as an application defect wastes effort; treating it as harmless without stronger evidence is equally unsafe.

**Suggested improvement:** Define a layered verification workflow that first classifies failures by application assertion versus dependency deadline, checks dependency recovery evidence, reruns the affected invariant in isolation, uses bounded but realistic cold-start budgets, then completes compile, vet, build, runtime, and diff checks before making a scoped release claim. Keep broad-suite instability explicit until the infrastructure is healthy enough to rerun it.

**Principle:** Release evidence should distinguish product behavior from test-infrastructure health, but only layered independent proofs justify narrowing an infrastructure-caused failure.

### Observation 3: Pledge workflows must separate intention, fulfillment, and communication consent

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Building a non-coercive church pledge lifecycle across staff, member, ledger, refund, and reminder surfaces.
**Skill:** New skill candidate: ethical-pledge-lifecycle
**Type:** open-source
**Phase/Area:** Domain modelling, financial integrity, privacy, and communication orchestration

**Issue:** Treating a pledge as a mutable balance or receivable lets administrative edits drift from posted and refunded gifts. Treating reminder opt-in as permanent permission also bypasses later consent withdrawals, suppressions, and fatigue limits. A UI-only disclaimer cannot repair those domain mistakes.

**Suggested improvement:** Model a pledge as a voluntary intention with an explicit schedule and lifecycle, derive fulfillment exclusively from immutable contribution and adjustment entries, validate donor and fund attribution at payment time, and evaluate current purpose-and-channel consent immediately before every reminder. Persist both queued and suppressed reminder decisions and prohibit giving ranks or comparisons.

**Principle:** Ethical pledge systems keep intention non-coercive, calculate fulfillment from the financial ledger, and treat communication permission as a current decision rather than a stored assumption.

### Observation 4: Reconciliation workflows need concurrency, authority and close-state proofs

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Hardening settlement import, row resolution and fiscal-period controls for a financial subledger.
**Skill:** New skill candidate: settlement-reconciliation-lifecycle
**Type:** open-source
**Phase/Area:** Import integrity, separation of duties, exception ownership and period control

**Issue:** A settlement feature can appear complete with file upload, match suggestions and a close button while still accepting integer overflow, surfacing duplicate-key races, importing into closed periods, letting ordinary reconcilers approve variances, assigning exceptions to inactive users, or persisting an approval request without its audit. These failures cross storage, policy and UI boundaries and are easy to miss in happy-path tests.

**Suggested improvement:** Define a reusable invariant-first workflow: checked minor-unit arithmetic; file-hash and stable-row replay identities with concurrent tests; private evidence retention; deterministic but non-automatic suggestions; branch/period target scope; distinct exact-match, variance-approval and owned-exception permissions; atomic request/audit and approval/event transitions; and a source-watermarked close snapshot that proves every financial intake path is reconciled.

**Principle:** Financial reconciliation is complete only when source evidence, human authority, exception accountability, replay safety and period immutability are proven as one lifecycle.

### Observation 5: Liveness-only health checks can conceal total application unavailability

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Running browser verification after builds and integration tests passed while an old API process still answered its health route.
**Skill:** New skill candidate: production-runtime-readiness
**Type:** open-source
**Phase/Area:** Local orchestration, dependency readiness and smoke testing

**Issue:** The API returned a successful health response while its database container was stopped and every database-backed login request hung. A listening process plus liveness endpoint therefore looked healthy even though the application was unusable; restarting the API revealed the dependency failure immediately.

**Suggested improvement:** Define separate liveness and readiness probes, make readiness exercise bounded critical dependencies, and require at least one authenticated database-backed smoke path after process restart. When a browser action stalls, compare liveness, readiness and direct domain requests before attributing the failure to frontend state.

**Principle:** A process is alive when it can answer, but an application is ready only when its critical dependencies and one representative business path complete within a bounded time.

### Observation 6: Segmented OTP redesigns must preserve non-OTP recovery paths

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Standardizing one-time-code entry across member sign-in, admin MFA login, and MFA enrollment.
**Skill:** Existing skill improvement: frontend-design
**Type:** open-source
**Phase/Area:** Authentication UX, accessibility, and input behavior

**Issue:** Replacing a single authentication field with six visual slots can silently remove recovery-code support or produce six boxes that fail on paste, autofill, deletion, keyboard navigation, mobile keyboards, dark themes, or reduced motion. Visual segmentation alone is therefore not a complete OTP interaction.

**Suggested improvement:** Audit every code-entry surface and its accepted credential types, preserve a distinct recovery-code path, and verify the segmented control as a behavior contract: digit filtering, paste distribution, forward focus, backward deletion, arrow navigation, per-slot accessible names, one-time-code autofill, numeric input mode, theme contrast, and reduced-motion handling.

**Principle:** Authentication input redesigns must improve the common OTP path without narrowing legitimate fallback access or weakening keyboard and assistive-technology behavior.

### Observation 7: Sensitive workflow autocomplete needs its own minimum-safe projection

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Building staff statement issuance for finance roles that should find recipients without receiving the full people-directory permission set.
**Skill:** Existing skill improvement: frontend-design
**Type:** open-source
**Phase/Area:** Privacy-aware search, role boundaries, and workflow design

**Issue:** Reusing a broad people-directory endpoint for a narrow financial recipient picker either overexposes personal fields to finance operators or makes the workflow unusable when those operators correctly lack directory permissions. A visually polished autocomplete can therefore conceal an authorization mismatch.

**Suggested improvement:** Define a workflow-specific search contract that returns only the minimum stable identity needed for the action, applies the destination workflow's branch and field-class authorization, requires a bounded query, and excludes contact, care, attendance, and other unrelated details. Keep final subject authorization in the domain service even after search filtering.

**Principle:** Autocomplete is an authorization surface; sensitive workflows should search purpose-built projections rather than inherit the reach of a general directory.

### Observation 8: Feature routes must inherit the authenticated product shell by construction

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Integrating member giving, pledges, receipts and statements into an established member dashboard.
**Skill:** Existing skill improvement: frontend-design
**Type:** open-source
**Phase/Area:** Application routing, information architecture and responsive navigation

**Issue:** A feature page can be complete in isolation yet feel like it exits the product when it recreates its own full-screen root instead of inheriting the authenticated shell. This breaks navigation continuity, active-route context, profile access and mobile wayfinding even when every feature control works.

**Suggested improvement:** Put authenticated routes beneath a shared layout or shell component before building feature content. Make the shell responsible for identity context, desktop and mobile navigation, active-page semantics and workspace sizing; verify every new route for one shell instance, correct active state and zero overflow.

**Principle:** Product navigation continuity is a routing invariant, not page-level decoration; authenticated features should inherit their shell by construction.

### Observation 9: Native controls need a cross-app regression gate

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Removing operating-system select popovers from member, admin and public REMI workflows after new finance surfaces reintroduced native controls.
**Skill:** Existing skill improvement: frontend-design
**Type:** open-source
**Phase/Area:** Design-system conformance, forms and cross-application QA

**Issue:** A product can already own a branded select primitive while new feature slices quietly return to native selects. Builds and static screenshots still pass, but the open operating-system menu breaks the visual system, behaves differently across platforms and often exposes unreviewed dark-theme contrast.

**Suggested improvement:** Give every frontend an accessible shared select primitive, include open-menu light/dark review in feature QA, and add a repository scan that fails when native select tags appear in application TSX. Verify both trigger and portal/menu states because the closed control alone cannot prove the user-facing result.

**Principle:** Design-system conformance includes browser and operating-system popovers; branded form controls need both a reusable primitive and an enforceable regression check.

### Observation 10: Financial configuration UIs must preserve domain control workflows

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Building the finance configuration surface for fiscal periods, receipt sequences, fund restrictions and external account mappings.
**Skill:** Existing skill improvement: frontend-design
**Type:** open-source
**Phase/Area:** High-integrity administration and control design

**Issue:** A conventional settings screen encourages direct toggles and save buttons, but financial states such as fiscal close, reopen, fund retirement and receipt allocation carry audit, separation-of-duties and non-reuse rules that cannot be represented as ordinary preferences.

**Suggested improvement:** Derive the interface from domain commands before designing the form. Surface optimistic versions, immutable counters, effective dates, mandatory reasons, successor records, pending control requests and independent approval as first-class states; never collapse a multi-operator command into a cosmetic toggle.

**Principle:** High-integrity configuration is an operational workflow, not a settings form; the UI must make domain controls visible and harder to bypass.

### Observation 11: Financial charts need source provenance and export parity

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Building fund, trend, deposit and pledge reporting plus accounting and audit exports from the immutable finance ledger.
**Skill:** Existing skill improvement: frontend-design
**Type:** open-source
**Phase/Area:** Reporting integrity, visualizations and export workflows

**Issue:** A polished dashboard can calculate totals from a convenient client-side list while an accounting export uses a different query, producing attractive but irreconcilable numbers. A timestamp alone also does not identify which source versions produced a report.

**Suggested improvement:** Build one server-side report snapshot that owns metrics, scope, timezone, caveats and a deterministic source watermark. Persist that exact snapshot with export row count, reconciled total and artifact hash; show the provenance in the UI and prove report-to-ledger plus export-to-report equality in real-database tests.

**Principle:** Financial visualization is evidence presentation, not decoration; charts and exports must be two views of the same versioned source snapshot.

### Observation 12: Value parameters can still share mutable request memory

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Running the full race-enabled finance invariant suite after adding generated ledger and report properties.
**Skill:** Existing skill improvement: task-observer
**Type:** open-source
**Phase/Area:** Go service boundaries, idempotent concurrency and mutation ownership

**Issue:** Passing a request struct by value looks isolated, but slice and map fields still share their backing memory. In-place normalization of nested rows raced when concurrent idempotent settlement imports reused the same request value, even though the outer struct was copied.

**Suggested improvement:** Treat service inputs as caller-owned. Before normalization or sorting, clone every mutable nested slice/map that the service will alter; add race-enabled replay tests that deliberately reuse one request object across goroutines instead of only decoding independent HTTP bodies.

**Principle:** Request ownership is transitive; a copied struct does not own the mutable memory referenced by its slices, maps or pointers.

### Observation 13: Native date inputs puncture an otherwise branded form system

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Replacing the member profile date-of-birth picker after its operating-system year menu visually escaped the REMI interface.
**Skill:** Existing skill improvement: redesign-existing-projects
**Type:** open-source
**Phase/Area:** Branded form controls and cross-platform UI consistency

**Issue:** Styling the closed state of a native date input does not control its calendar, month or year popovers. Those surfaces vary by browser and operating system, can introduce stark off-brand colors and geometry, and are especially conspicuous beside custom listboxes.

**Suggested improvement:** During form-system audits, inventory semantic input types as well as literal `select` tags. For dates that need strong visual consistency, use accessible branded day/month/year listboxes while preserving a single ISO value at the API boundary, including leap-year clamping, keyboard navigation and reduced-motion behavior.

**Principle:** A branded control system is only as coherent as the browser-owned surface it leaves exposed.

### Observation 14: Resource ownership needs an explicit identity domain

**Status:** OPEN
**Date:** 2026-08-11
**Session context:** Adding a leader workspace scoped by group and team `leaderPersonIds` while staff JWTs identify user accounts and member JWTs identify people.
**Skill:** Existing skill improvement: task-observer
**Type:** open-source
**Phase/Area:** Authorization, identity linkage and ministry ownership

**Issue:** Comparing a generic actor ID to a domain person ID can silently grant nothing or grant the wrong resource when staff accounts and member profiles use different identifier namespaces. A branch-wide staff grant is not evidence of owned-unit leadership.

**Suggested improvement:** Make ownership checks require an identity whose type and identifier domain are explicit. For REMI, member-linked leadership uses `ActorMember` plus person ID; staff leadership needs a separately modeled account-to-person link before it can inherit owned-unit access. Test both owned and unowned records, and reject unlinked staff rather than guessing by ID equality.

**Principle:** Authorization identifiers are typed data; identical-looking strings from different identity domains are not interchangeable.

### Observation 15: Shared reference data needs one explicit operational identifier

**Status:** OPEN
**Date:** 2026-08-12
**Session context:** Verifying a cross-domain review queue that filtered operational records using branch choices sourced from a public CMS endpoint
**Skill:** New skill candidate: cross-domain contract audit
**Type:** open-source
**Phase/Area:** Integration verification and reference-data identity

**Issue:** A UI can render the correct branch name while sending the CMS document ID into operational APIs whose records use a stable branch code or slug. Each layer appears healthy in isolation, but filtered lists return empty and make real data look absent.

**Suggested improvement:** During cross-domain implementation, trace every shared reference from storage through API serialization, selection controls, and domain queries. Expose a canonical operational ID explicitly, preserve the content-document ID separately, and add an integration test proving a selected reference retrieves seeded operational data.

**Principle:** Display equality does not prove identifier equality; shared reference data needs a canonical cross-domain identity contract verified end to end.

### Observation 16: Privacy thresholds must protect the whole analytical equation

**Status:** OPEN
**Date:** 2026-08-12
**Session context:** Implementing a cohort dashboard with weekly detail, maturity states and aggregate pathway visualizations
**Skill:** New skill candidate: privacy-safe product analytics
**Type:** open-source
**Phase/Area:** Metric contract, API projection and visualization verification

**Issue:** Hiding only a sparse rate still leaks its numerator or denominator, while a visible aggregate can reveal a suppressed subgroup by subtraction. A visually polished dashboard can therefore violate its privacy promise even when each chart appears anonymized.

**Suggested improvement:** Model availability as an explicit cell state and suppress numerator, denominator and derived rate together below threshold. Audit totals, adjacent cohorts, exports and chart labels for subtraction paths; keep immature cells distinct from zero and sparse cells distinct from missing data. Verify the serialized API and rendered DOM contain no hidden person identifiers or withheld values.

**Principle:** Privacy suppression applies to the complete inferential surface, not merely to the number most prominently displayed.

### Observation 17: Human approval must be structurally non-derivable

**Status:** OPEN
**Date:** 2026-08-12
**Session context:** Building a safety review gate after automated privacy, authorization and explainability checks passed
**Skill:** New skill candidate: trustworthy human release gates
**Type:** open-source
**Phase/Area:** Approval identity, version binding and release-state truthfulness

**Issue:** A system can accidentally convert a passing test suite, seed fixture, broad administrator role or old sign-off into apparent human approval. That makes the release dashboard look complete while bypassing the accountable person and exact artifact that required judgment.

**Suggested improvement:** Store human decisions as immutable records bound to a deterministic fingerprint of the reviewed policy/configuration set. Require the authenticated actor to match the preconfigured reviewer identity for that role, keep independent roles separate, invalidate prior decisions when the fingerprint changes, and render automated readiness separately from human approval.

**Principle:** If a decision requires human judgment, no technical success state should be able to manufacture, inherit or imply that decision.

- 2026-08-12 checkpoint after three completed REMI-271 work items: no additional reusable skill observation; the live-consent boundary is already covered by existing observations.
- 2026-08-12 checkpoint after completing the REMI-272 backend, provider, UI and browser-verification batch: no additional reusable skill observation; independent approval and replay-safe external event handling are already represented by existing observations.
- 2026-08-12 checkpoint after the branded-control source audit, keyboard fix and production/browser verification: no additional reusable skill observation; accessible custom-control implementation is project-specific and the general verification lesson is already represented by existing observations.
- 2026-08-12 checkpoint after completing the REMI-273 delegation, optional MFA, UI and verification batch: no additional reusable skill observation; session withholding and explicit delegation boundaries are already covered by the security and authorization principles in the existing observation set.
- 2026-08-12 checkpoint after the first REMI-274 API, branded portal and adversarial authorization-test batch: no additional reusable skill observation; present-only projection and lifecycle re-entry checks reinforce existing privacy and audit observations.
- 2026-08-12 checkpoint after REMI-324 community API, branded workspace, privacy tests and authenticated responsive QA: no additional reusable skill observation; typed BSON projections and member-intent versus attendance separation reinforce existing serialization and privacy observations.
- 2026-08-12 checkpoint after REMI-325 serving API, branded workspace, lifecycle tests and responsive QA: no additional reusable skill observation; preserving staff-owned eligibility and separating arrival check-in from attendance reinforce existing field-ownership and privacy observations.
- 2026-08-12 checkpoint after completing the six-step REMI-326 event, payment, safety and verification plan: no additional reusable skill observation; paid waitlist promotion and provider-bound confirmation reinforce existing lifecycle and audit principles.

### Observation 18: Compatibility fallbacks need an explicit publication boundary

**Status:** OPEN
**Date:** 2026-08-12
**Session context:** A concurrent idempotent settlement import intermittently replayed the winning document before its reconciliation rows were visible on standalone development MongoDB
**Skill:** New skill candidate: transactional compatibility and idempotent publication
**Type:** open-source
**Phase/Area:** Multi-document writes, standalone-database fallback and concurrent replay

**Issue:** A transaction abstraction may fall back to sequential writes on a development database that lacks transaction support. A unique aggregate document written first can then become visible to an idempotent racing caller before its children, audit and outbox records exist. The replay appears successful but returns a structurally incomplete aggregate.

**Suggested improvement:** Persist new aggregates as explicitly not ready, write every child and evidence record, and make the readiness flag the final write. All lookup/replay paths must exclude not-ready records and boundedly wait after a duplicate-key race for the winner to publish. Preserve backward compatibility by treating legacy documents without the marker as ready, and stress the invariant with repeated race tests.

**Principle:** Atomic intent is not atomic visibility; any non-transactional compatibility path needs a final publication boundary that every reader honors.

### Observation 19: Extensible-map keys need storage-aware query tests

**Status:** OPEN
**Date:** 2026-08-12
**Session context:** Implementing member directory visibility from an existing custom-fields map
**Skill:** Improvement candidate: backend schema and query design
**Type:** open-source
**Phase/Area:** Document databases, extensible fields and authorization filters

**Issue:** Application code stored a namespaced key such as `member.directoryVisibility` as one literal map key, while a MongoDB dotted-path query interpreted the same text as nested documents. The write and in-memory read looked correct, but the authorization query could never match the intended records.

**Suggested improvement:** When extensible maps permit punctuation or namespaced keys, document their physical BSON shape and test the real database query against a persisted example. Prefer typed nested fields for security-sensitive filters; if compatibility requires literal dotted keys, fetch a bounded authorized candidate set or use an explicit `$getField` expression rather than assuming dot-path equivalence.

**Principle:** A field name that looks like a path is not proof that its persisted value is queryable as that path.

### Observation 20: Route allowlists must validate domain identifiers, not storage folklore

**Status:** OPEN
**Date:** 2026-08-12
**Session context:** Authenticated runtime verification of a member group-conversation BFF
**Skill:** Improvement candidate: secure BFF route design
**Type:** open-source
**Phase/Area:** Route allowlists, identifier contracts and end-to-end verification

**Issue:** A secure BFF allowlist accepted only 24-character hexadecimal IDs because some records used Mongo identifiers, while the domain contract also used stable slug IDs. Compilation, production builds and direct API tests passed, but the real browser-facing route returned 404 for a valid seeded group.

**Suggested improvement:** Define an explicit identifier grammar at the domain boundary and reuse it in API validation, BFF allowlists, fixtures and contract tests. Keep route matching narrow, but test one persisted identifier of every supported form through the full browser-to-BFF-to-service path.

**Principle:** Security allowlists should constrain the documented domain, not accidentally constrain it to one current storage representation.

### Observation 21: Offline capability for private apps starts with a cache prohibition

**Status:** OPEN
**Date:** 2026-08-12
**Session context:** Adding installability and offline recovery to an authenticated member portal
**Skill:** Improvement candidate: privacy-safe PWA delivery
**Type:** open-source
**Phase/Area:** Service workers, offline UX and sensitive application data

**Issue:** Generic PWA recipes encourage caching pages and API responses for a seamless offline experience. In a portal containing care, giving, household and identity data, that default can persist private authenticated content beyond the server session and make shared-device exposure much harder to control.

**Suggested improvement:** Start the offline design with an explicit cache classification. Cache only a generic offline document and versioned public assets; use network-only handling for authenticated navigations, RSC payloads, APIs, files and media. State clearly that offline mutations are not queued, and test cache contents as a privacy artifact rather than only testing whether a page loads without a network.

**Principle:** For sensitive apps, a trustworthy offline fallback is often less data, not more caching.

- 2026-08-12 checkpoint after REMI-330 installability, privacy-safe offline, client observability and desktop/mobile accessibility/E2E work: no additional reusable skill observation; hydration-aware interaction checks and custom combobox semantics reinforce existing browser-verification and accessible-control guidance.
- 2026-08-12 checkpoint after REMI-280 reporting governance: `frontend-design` helped turn dense metric metadata into a calm, brand-consistent progressive-disclosure library instead of a generic data table. Reusable lesson: metric definitions need authorization too; filter the catalog with the same resource and field-class grants as source data, omit grant internals from JSON, and fail closed when catalog validation or authorization is absent.
- 2026-08-12 checkpoint after REMI-281 role dashboards: the useful pattern is permission-aware composition rather than a universal dashboard payload. Each domain remains authoritative and independently denies access; the client omits unavailable domains instead of coercing failures to zero, and every displayed card retains its canonical metric ID. This kept the executive surface rich without weakening pastoral or finance isolation.
- 2026-08-12 checkpoint after REMI-282 report builder: a safe report builder is a constrained compiler, not an SQL-shaped UI. Validate the measure/dimension grammar against the permission-filtered dictionary, delegate evaluation to authoritative domain services, strip sparse values before serialization, snapshot before asynchronous artifact generation, and reauthorize plus hash-check at download time.
- 2026-08-12 checkpoint after REMI-283 scheduled reports: no new frontend-design issue. The reusable security pattern is that an emailed delivery URL should identify a record, not authorize it; bind access to the signed-in recipient, recheck current metric grants, enforce expiry and validate the stored artifact hash before returning bytes.
- 2026-08-12 checkpoint after REMI-284 reporting hardening: no additional skill issue. A reporting adapter must validate its output as strictly as its input: exact requested metric coverage, canonical unit/domain, explicit availability state, finite/ranged values and timestamps. A server deadline then turns the documented latency budget into an enforceable failure boundary.
- 2026-08-12 checkpoint during REMI-290 permission scopes: a signed token is only a snapshot, so narrowing staff access requires a server-side access version checked on every protected request. Optimistic scope mutation, transactional audit and immediate version mismatch rejection provide a simple revocation boundary without logging sensitive assignments.
- 2026-08-12 checkpoint after REMI-290 closure: route-level scope proof must bind authorization to the persisted resource attributes, not caller-supplied filters alone. Real-router tests covering list filtering, concealed detail, current assignment and sensitive-field endpoints reinforce the existing authorization-matrix guidance; no separate new observation added.
- 2026-08-12 checkpoint after REMI-291 audit assurance: an audit viewer must be treated as a controlled data product, not raw collection access. Safe projections, query-bound cursors, branch authorization, audit-of-audit evidence and a separately gated export path reinforce existing privacy-safe reporting guidance; no separate new observation added.
