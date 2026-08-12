"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { api, ApiError, asList } from "@/lib/api";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";

type Branch = { id: string; name: string };
type Policy = {
  id: string;
  version: number;
  name: string;
  kind: string;
  publishedAt?: string;
};
type Checklist = Record<ChecklistKey, boolean>;
type ChecklistKey =
  | "reasonUnderstood"
  | "caveatsVisible"
  | "noDiagnosis"
  | "noAutomaticAction"
  | "consentBoundaryClear"
  | "sparseDataProtected"
  | "noFinancialInference"
  | "languageIsPastoral";
type Review = {
  id: string;
  policyFingerprint: string;
  role: string;
  decision: string;
  findings: string;
  checklist: Checklist;
  actor: { id: string };
  reviewedAt: string;
};
type Readiness = {
  metricVersion: string;
  branchId: string;
  policyFingerprint: string;
  policies: Policy[];
  requiredRoles: string[];
  eligibleRoles: string[];
  roleStatus: Record<string, string>;
  releaseState: string;
  reviews: Review[];
  generatedAt: string;
};

const checks: Array<{ key: ChecklistKey; title: string; copy: string }> = [
  {
    key: "reasonUnderstood",
    title: "Observable reason is understandable",
    copy: "A reviewer can trace the statement to dated participation evidence.",
  },
  {
    key: "caveatsVisible",
    title: "Caveats stay beside the evidence",
    copy: "Missing coverage and limits are not hidden in secondary documentation.",
  },
  {
    key: "noDiagnosis",
    title: "No person is diagnosed or scored",
    copy: "The language avoids faith, intent, worth, risk and spiritual-health labels.",
  },
  {
    key: "noAutomaticAction",
    title: "No automatic ministry action",
    copy: "Signals cannot contact, change membership or create a care case.",
  },
  {
    key: "consentBoundaryClear",
    title: "Consent boundary is explicit",
    copy: "Outreach remains record-only and rechecks live channel consent.",
  },
  {
    key: "sparseDataProtected",
    title: "Sparse cohorts are protected",
    copy: "Numerator, denominator, rate and partitions disappear together below five.",
  },
  {
    key: "noFinancialInference",
    title: "Financial evidence is structurally absent",
    copy: "Giving, pledges, funds and wealth proxies cannot enter retention outputs.",
  },
  {
    key: "languageIsPastoral",
    title: "Language supports humane care",
    copy: "Copy invites context and dignity rather than pressure or judgment.",
  },
];
const emptyChecklist = Object.fromEntries(
  checks.map((check) => [check.key, false]),
) as Checklist;
const controls = [
  [
    "Source boundary",
    "Named attendance, active group joins and accepted/completed serving only",
    "Static dependency guard",
  ],
  [
    "Sparse data",
    "All values withheld together below five eligible people",
    "Serialization tests",
  ],
  [
    "Maturity",
    "Pending remains distinct from zero until the full window closes",
    "Window boundary tests",
  ],
  [
    "Parity",
    "Person identifiers and demographic attributes never enter cohort formulas",
    "Symmetry fixtures",
  ],
  [
    "Human action",
    "Assignment, consent and immutable outcome evidence gate outreach",
    "Lifecycle integration tests",
  ],
  [
    "Authorization",
    "Branch-sensitive staff grants; members and owned-unit leaders denied",
    "Authenticated HTTP tests",
  ],
];

export default function RetentionSafetyPage() {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branchId, setBranchId] = useState("");
  const [readiness, setReadiness] = useState<Readiness | null>(null);
  const [role, setRole] = useState("");
  const [decision, setDecision] = useState("approved");
  const [findings, setFindings] = useState("");
  const [checklist, setChecklist] = useState<Checklist>(emptyChecklist);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api("/api/branches")
      .then((value) => {
        const items = asList<Branch>(value);
        setBranches(items);
        setBranchId(items[0]?.id || "");
      })
      .catch(() => setError("Branches could not be loaded."));
  }, []);
  const load = useCallback(async () => {
    if (!branchId) return;
    setLoading(true);
    setError(null);
    try {
      const value = await api<Readiness>(
        `/api/chms/v1/engagement/safety-readiness?branchId=${encodeURIComponent(branchId)}`,
      );
      setReadiness(value);
      setRole((current) =>
        value.eligibleRoles.includes(current)
          ? current
          : value.eligibleRoles[0] || "",
      );
    } catch (cause) {
      setReadiness(null);
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Safety readiness could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [branchId]);
  useEffect(() => {
    void load();
  }, [load]);
  const complete = useMemo(
    () => checks.every((check) => checklist[check.key]),
    [checklist],
  );
  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      const value = await api<Readiness>(
        "/api/chms/v1/engagement/safety-reviews",
        {
          method: "POST",
          body: { branchId, role, decision, findings, checklist },
        },
      );
      setReadiness(value);
      setChecklist(emptyChecklist);
      setFindings("");
      setNotice(
        `${role[0].toUpperCase()}${role.slice(1)} decision recorded against this exact policy fingerprint.`,
      );
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The review decision could not be recorded.",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="retention-safety">
      <PageHeader
        title="Safety & pastoral UAT"
        subtitle="A version-bound acceptance room for humane language, explainable evidence, privacy protection and independent human sign-off."
      >
        <Select
          aria-label="Branch"
          value={branchId}
          onChange={(event) => setBranchId(event.target.value)}
        >
          <option value="">Choose branch</option>
          {branches.map((branch) => (
            <option value={branch.id} key={branch.id}>
              {branch.name}
            </option>
          ))}
        </Select>
        <Link className="admin-secondary-button" href="/retention/insights">
          Connection insights
        </Link>
      </PageHeader>
      {error && <ErrorBox message={error} onRetry={load} />}
      {notice && (
        <div className="retention-notice" role="status">
          {notice}
        </div>
      )}
      {loading ? (
        <div className="retention-safety-skeleton">
          <i />
          <i />
          <i />
        </div>
      ) : readiness ? (
        <>
          <section
            className={`retention-release-state is-${readiness.releaseState}`}
          >
            <span>
              {readiness.releaseState === "approved"
                ? "✓"
                : readiness.releaseState === "changes-required"
                  ? "!"
                  : "○"}
            </span>
            <div>
              <small>RELEASE DECISION · {readiness.metricVersion}</small>
              <h2>
                {readiness.releaseState === "approved"
                  ? "Independent review complete"
                  : readiness.releaseState === "changes-required"
                    ? "Changes are required"
                    : "Awaiting named human reviewers"}
              </h2>
              <p>
                {readiness.releaseState === "approved"
                  ? "Product, pastoral and privacy reviewers approved this exact policy set."
                  : "Engineering evidence is available, but the system will not convert it into human approval."}
              </p>
            </div>
            <code>{readiness.policyFingerprint.slice(0, 12)}</code>
          </section>
          <section className="retention-safety-grid">
            <Card className="retention-review-roles">
              <header>
                <span>01 · INDEPENDENT REVIEW</span>
                <h2>Three accountable perspectives</h2>
              </header>
              <div>
                {readiness.requiredRoles.map((requiredRole, index) => (
                  <article
                    className={`is-${readiness.roleStatus[requiredRole]}`}
                    key={requiredRole}
                  >
                    <b>{String(index + 1).padStart(2, "0")}</b>
                    <div>
                      <strong>{requiredRole}</strong>
                      <small>{readiness.roleStatus[requiredRole]}</small>
                    </div>
                    <em>
                      {readiness.roleStatus[requiredRole] === "approved"
                        ? "✓"
                        : readiness.roleStatus[requiredRole] ===
                            "changes-required"
                          ? "!"
                          : "○"}
                    </em>
                  </article>
                ))}
              </div>
              <footer>
                {readiness.eligibleRoles.length
                  ? `You may submit: ${readiness.eligibleRoles.join(", ")}.`
                  : "Your account is not a named reviewer for every published policy. This workspace is read-only."}
              </footer>
            </Card>
            <Card className="retention-policy-set">
              <header>
                <span>02 · POLICY SET</span>
                <h2>What this decision covers</h2>
              </header>
              <div>
                {readiness.policies.map((policy) => (
                  <article key={policy.id}>
                    <b>{policy.name}</b>
                    <span>{policy.kind}</span>
                    <small>
                      Version {policy.version} ·{" "}
                      {policy.publishedAt
                        ? new Date(policy.publishedAt).toLocaleDateString(
                            "en-GH",
                          )
                        : "publication pending"}
                    </small>
                  </article>
                ))}
              </div>
            </Card>
          </section>
          <Card className="retention-control-matrix">
            <header>
              <span>03 · ENGINEERING EVIDENCE</span>
              <h2>Controls reviewers should challenge</h2>
              <p>
                These are enforced contracts, not a substitute for pastoral
                judgment.
              </p>
            </header>
            <div>
              {controls.map(([title, copy, evidence], index) => (
                <article key={title}>
                  <b>{String(index + 1).padStart(2, "0")}</b>
                  <div>
                    <strong>{title}</strong>
                    <p>{copy}</p>
                  </div>
                  <em>{evidence}</em>
                </article>
              ))}
            </div>
          </Card>
          {readiness.eligibleRoles.length ? (
            <Card className="retention-uat-form">
              <header>
                <span>04 · HUMAN DECISION</span>
                <h2>Review the exact experience</h2>
                <p>
                  Approval is immutable and becomes stale if any published
                  policy version changes.
                </p>
              </header>
              <form onSubmit={submit}>
                <div className="retention-uat-fields">
                  <label className="person-field">
                    <span>Reviewer perspective</span>
                    <Select
                      value={role}
                      onChange={(event) => setRole(event.target.value)}
                    >
                      {readiness.eligibleRoles.map((value) => (
                        <option value={value} key={value}>
                          {value[0].toUpperCase() + value.slice(1)}
                        </option>
                      ))}
                    </Select>
                  </label>
                  <label className="person-field">
                    <span>Decision</span>
                    <Select
                      value={decision}
                      onChange={(event) => setDecision(event.target.value)}
                    >
                      <option value="approved">Approve this policy set</option>
                      <option value="changes-required">Require changes</option>
                    </Select>
                  </label>
                </div>
                <fieldset>
                  <legend>
                    Confirm each statement from the rendered queue, detail and
                    cohort dashboard
                  </legend>
                  <div className="retention-uat-checks">
                    {checks.map((check) => (
                      <label
                        className={checklist[check.key] ? "is-checked" : ""}
                        key={check.key}
                      >
                        <input
                          className="sr-only"
                          type="checkbox"
                          checked={checklist[check.key]}
                          onChange={(event) =>
                            setChecklist((current) => ({
                              ...current,
                              [check.key]: event.target.checked,
                            }))
                          }
                        />
                        <i>{checklist[check.key] ? "✓" : ""}</i>
                        <span>
                          <strong>{check.title}</strong>
                          <small>{check.copy}</small>
                        </span>
                      </label>
                    ))}
                  </div>
                </fieldset>
                <label className="person-field">
                  <span>Findings and pastoral context</span>
                  <textarea
                    required
                    minLength={20}
                    maxLength={2000}
                    rows={5}
                    value={findings}
                    onChange={(event) => setFindings(event.target.value)}
                    placeholder="Record what was reviewed, concerns raised, language changes requested, and why this decision is appropriate…"
                  />
                </label>
                <footer>
                  <p>
                    {decision === "approved" && !complete
                      ? "Confirm all eight safety statements before approval."
                      : "The decision will bind to the displayed fingerprint and reviewer identity."}
                  </p>
                  <button
                    className="admin-primary-button"
                    disabled={
                      busy ||
                      !role ||
                      findings.trim().length < 20 ||
                      (decision === "approved" && !complete)
                    }
                  >
                    {busy
                      ? "Recording decision…"
                      : decision === "approved"
                        ? "Record approval"
                        : "Record required changes"}
                  </button>
                </footer>
              </form>
            </Card>
          ) : (
            <Card className="retention-uat-locked">
              <span>04 · HUMAN DECISION</span>
              <EmptyState
                dense
                title="Named reviewer required"
                hint="Ask the product, pastoral or privacy reviewer recorded on every published policy to sign in and complete this version-bound checklist."
              />
            </Card>
          )}
          <Card className="retention-review-history">
            <header>
              <span>05 · IMMUTABLE HISTORY</span>
              <h2>Decisions remain attributable</h2>
            </header>
            {readiness.reviews.length ? (
              <div>
                {readiness.reviews.map((review) => (
                  <article
                    className={
                      review.policyFingerprint === readiness.policyFingerprint
                        ? "is-current"
                        : "is-stale"
                    }
                    key={review.id}
                  >
                    <b>{review.role}</b>
                    <strong>{review.decision}</strong>
                    <p>{review.findings}</p>
                    <small>
                      {new Date(review.reviewedAt).toLocaleString("en-GH")} ·
                      reviewer {review.actor.id} ·{" "}
                      {review.policyFingerprint === readiness.policyFingerprint
                        ? "current policy set"
                        : "superseded policy set"}
                    </small>
                  </article>
                ))}
              </div>
            ) : (
              <EmptyState
                dense
                title="No human decisions yet"
                hint="Automated checks do not create approval records."
              />
            )}
          </Card>
        </>
      ) : null}
    </div>
  );
}
