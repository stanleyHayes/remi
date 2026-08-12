"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { api, ApiError, asList } from "@/lib/api";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";

type Branch = { id: string; name: string };
type Rule = {
  id: string;
  version: number;
  name: string;
  kind: string;
  description: string;
  timezone: string;
  windowDays: number;
  lookbackDays: number;
  expiresAfterDays: number;
  status: string;
  metricVersion: string;
  publishedAt?: string;
};
type Evidence = {
  sourceType: string;
  sourceId: string;
  occurredAt: string;
  fact: string;
};
type Signal = {
  id: string;
  version: number;
  ruleName: string;
  ruleKind: string;
  personId: string;
  observedFrom: string;
  observedThrough: string;
  evidence: Evidence[];
  caveats: string[];
  state: string;
  assigneeId?: string;
  snoozedUntil?: string;
  expiresAt: string;
};
type RulesResponse = {
  items: Rule[];
  activationEnabled: boolean;
  metricVersion: string;
};
const KINDS = [
  {
    value: "attendance-gap",
    label: "Attendance gap",
    copy: "Previously observed attendance, then no qualifying visit inside the configured gap.",
  },
  {
    value: "first-visit-no-return",
    label: "No return after first visit",
    copy: "A mature first-visit window with no later qualifying local date.",
  },
  {
    value: "first-visit-no-group",
    label: "No group connection",
    copy: "A mature first-visit window with no active group join.",
  },
  {
    value: "first-visit-no-serving",
    label: "No serving connection",
    copy: "A mature first-visit window with no accepted or completed assignment.",
  },
];
const EMPTY = {
  name: "",
  kind: "attendance-gap",
  description: "",
  windowDays: "30",
  lookbackDays: "120",
  expiresAfterDays: "14",
  reviewCadenceDays: "30",
};
export default function RetentionPage() {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branchId, setBranchId] = useState("");
  const [rules, setRules] = useState<Rule[]>([]);
  const [signals, setSignals] = useState<Signal[]>([]);
  const [enabled, setEnabled] = useState(false);
  const [metricVersion, setMetricVersion] = useState("retention-v1");
  const [form, setForm] = useState(EMPTY);
  const [showCreate, setShowCreate] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [notice, setNotice] = useState("");
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
    if (!branchId) {
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const [ruleData, signalData] = await Promise.all([
        api<RulesResponse>(
          `/api/chms/v1/engagement/rules?branchId=${encodeURIComponent(branchId)}`,
        ),
        api<{ items: Signal[] }>(
          `/api/chms/v1/engagement/signals?branchId=${encodeURIComponent(branchId)}&state=active`,
        ),
      ]);
      setRules(ruleData.items || []);
      setSignals(signalData.items || []);
      setEnabled(ruleData.activationEnabled);
      setMetricVersion(ruleData.metricVersion);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Retention policy workspace could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [branchId]);
  useEffect(() => {
    void load();
  }, [load]);
  async function create(event: FormEvent) {
    event.preventDefault();
    setBusy("create");
    setError(null);
    setNotice("");
    try {
      await api("/api/chms/v1/engagement/rules", {
        method: "POST",
        body: {
          branchId,
          name: form.name,
          kind: form.kind,
          description: form.description,
          timezone: "Africa/Accra",
          windowDays: Number(form.windowDays),
          lookbackDays: Number(form.lookbackDays),
          expiresAfterDays: Number(form.expiresAfterDays),
          approval: { reviewCadenceDays: Number(form.reviewCadenceDays) },
        },
      });
      setForm(EMPTY);
      setShowCreate(false);
      setNotice(
        "Draft rule created. It cannot produce signals until approvals and activation are complete.",
      );
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The draft rule could not be created.",
      );
    } finally {
      setBusy("");
    }
  }
  async function generate(rule: Rule) {
    setBusy(rule.id);
    setError(null);
    setNotice("");
    try {
      const value = await api<{
        created: number;
        existing: number;
        candidates: number;
      }>(
        `/api/chms/v1/engagement/rules/${encodeURIComponent(rule.id)}/generate`,
        { method: "POST", body: { asOf: new Date().toISOString() } },
      );
      setNotice(
        `${value.created} new evidence observations created; ${value.existing} already existed.`,
      );
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Signals could not be generated.",
      );
    } finally {
      setBusy("");
    }
  }
  const kind = KINDS.find((item) => item.value === form.kind)!;
  return (
    <div className="retention-workspace">
      <PageHeader
        title="Retention policy lab"
        subtitle="Prepare explainable participation evidence without scoring faith, worth, intent or generosity."
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
        <button
          type="button"
          className="admin-primary-button rounded-lg px-4 py-2.5 text-sm font-semibold"
          onClick={() => setShowCreate((value) => !value)}
        >
          {showCreate ? "Close draft" : "New draft rule"}
        </button>
      </PageHeader>
      <section className={`retention-gate ${enabled ? "is-enabled" : ""}`}>
        <span aria-hidden="true">{enabled ? "✓" : "○"}</span>
        <div>
          <b>
            {enabled
              ? "Approved activation is enabled"
              : "Individual signals are safely disabled"}
          </b>
          <p>
            {enabled
              ? "Published, fully approved rules may generate evidence observations."
              : "Draft policy work is available, but publishing and generation fail closed until product, pastoral and privacy approvals are recorded and the deployment flag is enabled."}
          </p>
        </div>
        <em>{metricVersion}</em>
      </section>
      {error && (
        <div className="mb-5">
          <ErrorBox message={error} onRetry={load} />
        </div>
      )}
      {notice && (
        <div className="retention-notice" role="status">
          {notice}
        </div>
      )}
      {showCreate && (
        <Card className="retention-builder">
          <header>
            <span>POLICY CONFIGURATION · DRAFT ONLY</span>
            <h2>Define an observable gap</h2>
            <p>
              Choose from approved evidence patterns. Rules cannot send, change
              membership or open care automatically.
            </p>
          </header>
          <form onSubmit={create}>
            <label className="person-field">
              <span>Rule name</span>
              <input
                required
                minLength={3}
                value={form.name}
                onChange={(event) =>
                  setForm((value) => ({ ...value, name: event.target.value }))
                }
                placeholder="First visit follow-up evidence"
              />
            </label>
            <label className="person-field">
              <span>Evidence pattern</span>
              <Select
                value={form.kind}
                onChange={(event) =>
                  setForm((value) => ({ ...value, kind: event.target.value }))
                }
              >
                {KINDS.map((item) => (
                  <option value={item.value} key={item.value}>
                    {item.label}
                  </option>
                ))}
              </Select>
              <small>{kind.copy}</small>
            </label>
            <label className="person-field md:col-span-2">
              <span>Reviewer-facing interpretation</span>
              <textarea
                required
                rows={3}
                maxLength={1000}
                value={form.description}
                onChange={(event) =>
                  setForm((value) => ({
                    ...value,
                    description: event.target.value,
                  }))
                }
                placeholder="Describe what this observed gap means operationally—and what it does not mean."
              />
            </label>
            <Preset
              label="Observation window"
              value={form.windowDays}
              values={["14", "30", "60", "90", "120"]}
              suffix="days"
              onChange={(windowDays) =>
                setForm((value) => ({
                  ...value,
                  windowDays,
                  lookbackDays:
                    Number(value.lookbackDays) < Number(windowDays)
                      ? windowDays
                      : value.lookbackDays,
                }))
              }
            />
            <Preset
              label="History lookback"
              value={form.lookbackDays}
              values={["90", "120", "180", "365", "730"]}
              suffix="days"
              onChange={(lookbackDays) =>
                setForm((value) => ({ ...value, lookbackDays }))
              }
            />
            <Preset
              label="Evidence expires"
              value={form.expiresAfterDays}
              values={["7", "14", "30", "60"]}
              suffix="days"
              onChange={(expiresAfterDays) =>
                setForm((value) => ({ ...value, expiresAfterDays }))
              }
            />
            <Preset
              label="Policy review cadence"
              value={form.reviewCadenceDays}
              values={["30", "60", "90", "180"]}
              suffix="days"
              onChange={(reviewCadenceDays) =>
                setForm((value) => ({ ...value, reviewCadenceDays }))
              }
            />
            <footer>
              <p>
                Permitted sources: named attendance, active group joins,
                accepted/completed serving assignments. Financial and anonymous
                data are structurally absent.
              </p>
              <button disabled={busy !== ""} className="admin-primary-button">
                {busy === "create" ? "Creating draft…" : "Create draft rule"}
              </button>
            </footer>
          </form>
        </Card>
      )}
      {loading ? (
        <div className="retention-skeleton">
          {[1, 2, 3].map((item) => (
            <i key={item} />
          ))}
        </div>
      ) : (
        <section className="retention-grid">
          <Card className="retention-list">
            <header>
              <div>
                <span>01 · POLICY</span>
                <h2>Evidence rules</h2>
              </div>
              <b>{rules.length}</b>
            </header>
            {rules.length ? (
              <div>
                {rules.map((rule) => (
                  <article key={rule.id}>
                    <span className={`retention-state ${rule.status}`}>
                      {rule.status}
                    </span>
                    <h3>{rule.name}</h3>
                    <p>
                      {KINDS.find((item) => item.value === rule.kind)?.label} ·{" "}
                      {rule.windowDays}-day window · {rule.lookbackDays}-day
                      history
                    </p>
                    <small>
                      {rule.description || "No interpretation note yet."}
                    </small>
                    <footer>
                      <em>
                        v{rule.version} · expires {rule.expiresAfterDays}d
                      </em>
                      {rule.status === "published" && (
                        <button
                          disabled={busy !== "" || !enabled}
                          onClick={() => void generate(rule)}
                        >
                          {busy === rule.id
                            ? "Evaluating…"
                            : "Generate evidence"}
                        </button>
                      )}
                    </footer>
                  </article>
                ))}
              </div>
            ) : (
              <EmptyState
                dense
                title="No evidence rules"
                hint="Create a draft from an approved observable pattern."
              />
            )}
          </Card>
          <Card className="retention-list signals">
            <header>
              <div>
                <span>02 · HUMAN REVIEW</span>
                <h2>Actionable observations</h2>
              </div>
              <b>{signals.length}</b>
            </header>
            {signals.length ? (
              <div>
                {signals.map((signal) => (
                  <article key={signal.id}>
                    <span className={`retention-state ${signal.state}`}>
                      {signal.state}
                    </span>
                    <h3>{signal.ruleName}</h3>
                    <p>Person reference {signal.personId}</p>
                    <small>
                      {signal.evidence[0]?.fact} ·{" "}
                      {signal.evidence[0]
                        ? new Date(
                            signal.evidence[0].occurredAt,
                          ).toLocaleDateString()
                        : "No source fact"}
                    </small>
                    <footer>
                      <em>
                        Expires{" "}
                        {new Date(signal.expiresAt).toLocaleDateString()}
                      </em>
                      <Link
                        href={`/retention/${encodeURIComponent(signal.id)}`}
                      >
                        Review evidence
                      </Link>
                    </footer>
                  </article>
                ))}
              </div>
            ) : (
              <EmptyState
                dense
                title="No actionable observations"
                hint={
                  enabled
                    ? "Published rules have not produced current evidence."
                    : "This queue remains empty while activation is disabled."
                }
              />
            )}
          </Card>
        </section>
      )}
      <Card className="retention-boundaries">
        <span>NON-NEGOTIABLE BOUNDARIES</span>
        <div>
          {[
            "No individual score",
            "No giving evidence",
            "No automatic contact",
            "No automatic care case",
            "No membership mutation",
            "Human context required",
          ].map((item, index) => (
            <p key={item}>
              <b>{String(index + 1).padStart(2, "0")}</b>
              {item}
            </p>
          ))}
        </div>
      </Card>
    </div>
  );
}
function Preset({
  label,
  value,
  values,
  suffix,
  onChange,
}: {
  label: string;
  value: string;
  values: string[];
  suffix: string;
  onChange: (value: string) => void;
}) {
  return (
    <fieldset className="retention-preset">
      <legend>{label}</legend>
      <div>
        {values.map((item) => (
          <button
            type="button"
            aria-pressed={value === item}
            onClick={() => onChange(item)}
            key={item}
          >
            {item}
            <small>{suffix}</small>
          </button>
        ))}
      </div>
    </fieldset>
  );
}
