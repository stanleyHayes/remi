"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { api, ApiError, asList } from "@/lib/api";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";

type Branch = { id: string; name: string };
type Metric = {
  state: "available" | "pending" | "suppressed";
  eligible?: number;
  achieved?: number;
  ratePercent?: number;
};
type Combined = Metric & {
  connected?: number;
  groupOnly?: number;
  servingOnly?: number;
  both?: number;
  neither?: number;
};
type Cohort = {
  cohortWeek: string;
  cohortSize?: number;
  cohortState: string;
  return30: Metric;
  return60: Metric;
  return90: Metric;
  groupConnection: Metric;
  servingConnection: Metric;
  combinedConnection: Combined;
};
type Dashboard = {
  metricVersion: string;
  range: { branchId: string; from: string; to: string; timezone: string };
  configuration: {
    privacyThreshold: number;
    returnWindowsDays: number[];
    groupConnectionDays: number;
    servingConnectionDays: number;
  };
  cohorts: Cohort[];
  quality: { sourceWatermark?: string; caveats: string[] };
  generatedAt: string;
};

const ranges = [
  { value: "180", label: "6 months" },
  { value: "270", label: "9 months" },
  { value: "365", label: "12 months" },
];
const metricDefinitions = [
  {
    key: "return30" as const,
    label: "30-day return",
    color: "var(--retention-coral)",
  },
  {
    key: "return60" as const,
    label: "60-day return",
    color: "var(--retention-gold)",
  },
  {
    key: "return90" as const,
    label: "90-day return",
    color: "var(--retention-sage)",
  },
  {
    key: "groupConnection" as const,
    label: "Group connection",
    color: "var(--retention-blue)",
  },
  {
    key: "servingConnection" as const,
    label: "Serving connection",
    color: "var(--retention-violet)",
  },
];

export default function RetentionInsightsPage() {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branchId, setBranchId] = useState("");
  const [days, setDays] = useState("270");
  const [dashboard, setDashboard] = useState<Dashboard | null>(null);
  const [loading, setLoading] = useState(true);
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
    const to = new Date();
    const from = new Date(to);
    from.setUTCDate(from.getUTCDate() - Number(days));
    try {
      const value = await api<Dashboard>(
        `/api/chms/v1/engagement/cohorts?branchId=${encodeURIComponent(branchId)}&from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`,
      );
      setDashboard(value);
    } catch (cause) {
      setDashboard(null);
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Retention insights could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [branchId, days]);

  useEffect(() => {
    void load();
  }, [load]);
  const latest = useMemo(
    () =>
      dashboard?.cohorts
        .slice()
        .reverse()
        .find((cohort) =>
          metricDefinitions.some(
            (metric) => cohort[metric.key].state === "available",
          ),
        ),
    [dashboard],
  );
  const publishable =
    dashboard?.cohorts.filter((cohort) => cohort.cohortState === "available")
      .length || 0;
  const protectedCount = (dashboard?.cohorts.length || 0) - publishable;

  return (
    <div className="retention-insights">
      <PageHeader
        title="Connection pathways"
        subtitle="Privacy-safe weekly cohorts showing observed return, group and serving connections—not faith, intent, worth or generosity."
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
        <Select
          aria-label="Reporting range"
          value={days}
          onChange={(event) => setDays(event.target.value)}
        >
          {ranges.map((range) => (
            <option value={range.value} key={range.value}>
              {range.label}
            </option>
          ))}
        </Select>
        <Link className="admin-secondary-button" href="/retention">
          Policy &amp; review
        </Link>
        <Link className="admin-secondary-button" href="/retention/safety">
          Safety &amp; UAT
        </Link>
      </PageHeader>
      {error && <ErrorBox message={error} onRetry={load} />}
      {loading ? (
        <InsightsSkeleton />
      ) : dashboard ? (
        <>
          <section className="retention-insight-meta">
            <div>
              <span>METRIC CONTRACT</span>
              <strong>{dashboard.metricVersion}</strong>
              <small>{dashboard.range.timezone}</small>
            </div>
            <div>
              <span>PUBLISHABLE WEEKS</span>
              <strong>{String(publishable).padStart(2, "0")}</strong>
              <small>Threshold met</small>
            </div>
            <div>
              <span>PRIVACY PROTECTED</span>
              <strong>{String(protectedCount).padStart(2, "0")}</strong>
              <small>Counts withheld</small>
            </div>
            <div>
              <span>FRESH THROUGH</span>
              <strong>
                {dashboard.quality.sourceWatermark
                  ? new Date(
                      dashboard.quality.sourceWatermark,
                    ).toLocaleDateString("en-GH", {
                      day: "2-digit",
                      month: "short",
                    })
                  : "—"}
              </strong>
              <small>
                {dashboard.quality.sourceWatermark
                  ? new Date(
                      dashboard.quality.sourceWatermark,
                    ).toLocaleTimeString("en-GH", {
                      hour: "2-digit",
                      minute: "2-digit",
                    })
                  : "No named facts"}
              </small>
            </div>
          </section>
          <section className="retention-insight-grid">
            <Card className="retention-path-chart">
              <header>
                <div>
                  <span>01 · COHORT PULSE</span>
                  <h2>Return and connection over time</h2>
                </div>
                <p>
                  Each column is a first-visit week. A dot replaces any value
                  that is immature or below the privacy threshold.
                </p>
              </header>
              {dashboard.cohorts.length ? (
                <div className="retention-chart-scroll">
                  <div
                    className="retention-chart"
                    style={
                      {
                        "--cohorts": Math.max(dashboard.cohorts.length, 1),
                      } as React.CSSProperties
                    }
                  >
                    {metricDefinitions.map((definition) => (
                      <div className="retention-chart-row" key={definition.key}>
                        <b>{definition.label}</b>
                        <div>
                          {dashboard.cohorts.map((cohort) => {
                            const metric = cohort[definition.key];
                            return (
                              <span
                                className={`retention-chart-cell is-${metric.state}`}
                                title={`${definition.label} · week of ${cohort.cohortWeek} · ${metric.state}`}
                                key={cohort.cohortWeek}
                              >
                                <i
                                  style={{
                                    height:
                                      metric.state === "available"
                                        ? `${Math.max(metric.ratePercent || 0, 4)}%`
                                        : undefined,
                                    background: definition.color,
                                  }}
                                />
                                <em>
                                  {metric.state === "available"
                                    ? `${metric.ratePercent}%`
                                    : metric.state === "pending"
                                      ? "◷"
                                      : "•"}
                                </em>
                              </span>
                            );
                          })}
                        </div>
                      </div>
                    ))}
                    <div className="retention-chart-axis">
                      <b>First visit week</b>
                      <div>
                        {dashboard.cohorts.map((cohort) => (
                          <span key={cohort.cohortWeek}>
                            {new Date(
                              `${cohort.cohortWeek}T12:00:00`,
                            ).toLocaleDateString("en-GH", {
                              day: "2-digit",
                              month: "short",
                            })}
                          </span>
                        ))}
                      </div>
                    </div>
                  </div>
                </div>
              ) : (
                <EmptyState
                  dense
                  title="No cohorts in this range"
                  hint="Completed named first visits will appear here once attendance is recorded."
                />
              )}
            </Card>
            <Card className="retention-latest-card">
              <header>
                <span>02 · LATEST PUBLISHABLE</span>
                <h2>
                  {latest
                    ? `Week of ${new Date(`${latest.cohortWeek}T12:00:00`).toLocaleDateString("en-GH", { day: "numeric", month: "long" })}`
                    : "Waiting for a mature cohort"}
                </h2>
                <p>
                  {latest
                    ? `${latest.cohortSize} eligible first visitors; every displayed cell meets the threshold.`
                    : `At least ${dashboard.configuration.privacyThreshold} eligible people are required before counts or rates appear.`}
                </p>
              </header>
              {latest ? (
                <div className="retention-latest-metrics">
                  {metricDefinitions.map((definition) => (
                    <MetricCard
                      metric={latest[definition.key]}
                      label={definition.label}
                      color={definition.color}
                      key={definition.key}
                    />
                  ))}
                </div>
              ) : (
                <div
                  className="retention-privacy-orbit"
                  aria-label="Sparse cohort values are protected"
                >
                  <span>≥{dashboard.configuration.privacyThreshold}</span>
                  <i />
                  <i />
                  <i />
                </div>
              )}
            </Card>
          </section>
          <section className="retention-insight-grid lower">
            <Card className="retention-partition-card">
              <header>
                <span>03 · COMMUNITY MIX</span>
                <h2>How people connected</h2>
                <p>
                  Non-overlapping pathways prevent people active in both areas
                  from being counted twice.
                </p>
              </header>
              {latest?.combinedConnection.state === "available" ? (
                <ConnectionPartition metric={latest.combinedConnection} />
              ) : (
                <ProtectedState
                  state={latest?.combinedConnection.state || "pending"}
                  threshold={dashboard.configuration.privacyThreshold}
                />
              )}
            </Card>
            <Card className="retention-quality-card">
              <header>
                <span>04 · INTERPRETATION</span>
                <h2>Read the evidence honestly</h2>
              </header>
              <div className="retention-quality-list">
                <p>
                  <b>◷</b>
                  <span>
                    <strong>Pending</strong>The full observation window has not
                    matured. It is never presented as 0%.
                  </span>
                </p>
                <p>
                  <b>•</b>
                  <span>
                    <strong>Protected</strong>Numerator, denominator and rate
                    are withheld together below{" "}
                    {dashboard.configuration.privacyThreshold} people.
                  </span>
                </p>
                <p>
                  <b>↗</b>
                  <span>
                    <strong>Observable only</strong>Connection means a recorded
                    return, active group join or accepted/completed serving
                    assignment.
                  </span>
                </p>
              </div>
              {dashboard.quality.caveats.map((caveat) => (
                <aside key={caveat}>{caveat}</aside>
              ))}
            </Card>
          </section>
        </>
      ) : null}
    </div>
  );
}

function MetricCard({
  metric,
  label,
  color,
}: {
  metric: Metric;
  label: string;
  color: string;
}) {
  return (
    <article className={`is-${metric.state}`}>
      <span style={{ background: color }} />
      <div>
        <b>{label}</b>
        {metric.state === "available" ? (
          <>
            <strong>{metric.ratePercent}%</strong>
            <small>
              {metric.achieved} of {metric.eligible}
            </small>
          </>
        ) : (
          <>
            <strong>{metric.state === "pending" ? "◷" : "•"}</strong>
            <small>
              {metric.state === "pending"
                ? "Window maturing"
                : "Privacy protected"}
            </small>
          </>
        )}
      </div>
    </article>
  );
}
function ConnectionPartition({ metric }: { metric: Combined }) {
  const total = metric.eligible || 1;
  const stops = [
    ((metric.groupOnly || 0) / total) * 100,
    (((metric.groupOnly || 0) + (metric.servingOnly || 0)) / total) * 100,
    (((metric.groupOnly || 0) +
      (metric.servingOnly || 0) +
      (metric.both || 0)) /
      total) *
      100,
  ];
  return (
    <div className="retention-partition">
      <div
        className="retention-donut"
        style={{
          background: `conic-gradient(var(--retention-blue) 0 ${stops[0]}%, var(--retention-violet) ${stops[0]}% ${stops[1]}%, var(--retention-gold) ${stops[1]}% ${stops[2]}%, var(--retention-track) ${stops[2]}% 100%)`,
        }}
      >
        <span>
          <strong>{metric.ratePercent}%</strong>
          <small>connected</small>
        </span>
      </div>
      <div className="retention-partition-legend">
        {[
          ["Group only", metric.groupOnly, "blue"],
          ["Serving only", metric.servingOnly, "violet"],
          ["Both", metric.both, "gold"],
          ["Neither yet", metric.neither, "track"],
        ].map(([label, value, tone]) => (
          <p key={String(label)}>
            <i className={`is-${tone}`} />
            <span>{label}</span>
            <b>{value}</b>
          </p>
        ))}
      </div>
    </div>
  );
}
function ProtectedState({
  state,
  threshold,
}: {
  state: string;
  threshold: number;
}) {
  return (
    <div className="retention-protected">
      <b>{state === "pending" ? "◷" : "•"}</b>
      <strong>
        {state === "pending"
          ? "Observation windows are still maturing"
          : "This pathway is privacy protected"}
      </strong>
      <p>
        {state === "pending"
          ? "It will become eligible after both connection windows close."
          : `Counts stay hidden until at least ${threshold} people are eligible.`}
      </p>
    </div>
  );
}
function InsightsSkeleton() {
  return (
    <div
      className="retention-insights-skeleton"
      aria-label="Loading retention insights"
    >
      <span />
      <span />
      <span />
      <span />
      <i />
      <i />
    </div>
  );
}
