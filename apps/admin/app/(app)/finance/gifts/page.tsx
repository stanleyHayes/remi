"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import DatePicker from "@/components/ui/DatePicker";
import { api, ApiError } from "@/lib/api";

type Money = { amountMinor: number; currency: string };
type Campus = { branchId: string; name: string };
type Fund = { id: string; name: string };
type Method = { id: string; name: string; kind: string };
type Subject = {
  id: string;
  type: "person" | "household";
  name: string;
  branchId: string;
};
type Gift = {
  id: string;
  branchId: string;
  receiptNumber: string;
  receivedAt: string;
  postedAt: string;
  donor: {
    type: "person" | "household" | "anonymous" | "unresolved";
    personId?: string;
    householdId?: string;
  };
  source: string;
  paymentMethodId: string;
  paymentMethodReference?: string;
  providerReference?: string;
  sourceReceiptNumber?: string;
  batchId?: string;
  campaignId?: string;
  pledgeId?: string;
  fiscalPeriodId: string;
  total: Money;
  splits: Array<{ fundId: string; amount: Money }>;
  state: string;
  effectiveState: string;
  provenance?: string;
  adjustment?: {
    type?: string;
    chainRootId?: string;
    reversesContributionId?: string;
    replacesContributionId?: string;
    reason?: string;
  };
};
type Adjustment = { reversal: Gift; replacement?: Gift };

const rows = <T,>(value: unknown): T[] =>
  Array.isArray(value)
    ? (value as T[])
    : value &&
        typeof value === "object" &&
        Array.isArray((value as { items?: unknown[] }).items)
      ? (value as { items: T[] }).items
      : [];
const money = (value = 0) =>
  new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS" }).format(
    value / 100,
  );
const stamp = (value?: string) =>
  value
    ? new Date(value).toLocaleString("en-GH", {
        dateStyle: "medium",
        timeStyle: "short",
      })
    : "—";
const donorId = (gift: Gift) =>
  gift.donor.personId || gift.donor.householdId || "";

export default function GiftRegisterPage() {
  const [campuses, setCampuses] = useState<Campus[]>([]),
    [funds, setFunds] = useState<Fund[]>([]),
    [methods, setMethods] = useState<Method[]>([]);
  const [branchId, setBranchId] = useState(""),
    [gifts, setGifts] = useState<Gift[]>([]),
    [selectedId, setSelectedId] = useState(""),
    [detail, setDetail] = useState<Gift | null>(null);
  const [query, setQuery] = useState(""),
    [state, setState] = useState("all"),
    [donorType, setDonorType] = useState("all"),
    [fundId, setFundId] = useState("all"),
    [from, setFrom] = useState(""),
    [to, setTo] = useState("");
  const [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null),
    [notice, setNotice] = useState("");
  const [correctionType, setCorrectionType] = useState<
      "person" | "household" | "anonymous" | "unresolved"
    >("person"),
    [subjectQuery, setSubjectQuery] = useState(""),
    [subject, setSubject] = useState<Subject | null>(null),
    [subjects, setSubjects] = useState<Subject[]>([]),
    [searching, setSearching] = useState(false),
    [reason, setReason] = useState(""),
    [acknowledged, setAcknowledged] = useState(false);

  const loadFoundation = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [campusData, fundData, methodData] = await Promise.all([
        api("/api/chms/v1/finance/campuses"),
        api("/api/chms/v1/finance/funds"),
        api("/api/chms/v1/finance/payment-methods"),
      ]);
      const nextCampuses = rows<Campus>(campusData);
      setCampuses(nextCampuses);
      setFunds(rows<Fund>(fundData));
      setMethods(rows<Method>(methodData));
      setBranchId((current) => current || nextCampuses[0]?.branchId || "");
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The gift register configuration could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, []);

  const loadGifts = useCallback(async () => {
    if (!branchId) {
      setGifts([]);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const next = rows<Gift>(
        await api(
          `/api/chms/v1/finance/contributions?branchId=${encodeURIComponent(branchId)}&limit=200`,
        ),
      );
      setGifts(next);
      setSelectedId((current) =>
        next.some((gift) => gift.id === current) ? current : next[0]?.id || "",
      );
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Posted gifts could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [branchId]);

  const loadDetail = useCallback(async () => {
    if (!selectedId) {
      setDetail(null);
      return;
    }
    try {
      setDetail(
        await api<Gift>(
          `/api/chms/v1/finance/contributions/${encodeURIComponent(selectedId)}`,
        ),
      );
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Gift detail could not be loaded.",
      );
    }
  }, [selectedId]);

  useEffect(() => {
    void loadFoundation();
  }, [loadFoundation]);
  useEffect(() => {
    void loadGifts();
  }, [loadGifts, notice]);
  useEffect(() => {
    void loadDetail();
  }, [loadDetail, notice]);
  useEffect(() => {
    if (
      !subjectQuery.trim() ||
      subject ||
      !["person", "household"].includes(correctionType)
    ) {
      setSubjects([]);
      return;
    }
    const timer = window.setTimeout(async () => {
      setSearching(true);
      try {
        setSubjects(
          rows<Subject>(
            await api(
              `/api/chms/v1/finance/statement-subjects?q=${encodeURIComponent(subjectQuery.trim())}`,
            ),
          ).filter(
            (item) =>
              item.type === correctionType &&
              (!item.branchId || item.branchId === branchId),
          ),
        );
      } catch {
        setSubjects([]);
      } finally {
        setSearching(false);
      }
    }, 250);
    return () => window.clearTimeout(timer);
  }, [branchId, correctionType, subject, subjectQuery]);

  const filtered = useMemo(
    () =>
      gifts.filter((gift) => {
        const needle = query.trim().toLowerCase();
        const searchable = [
          gift.receiptNumber,
          gift.paymentMethodReference,
          gift.providerReference,
          gift.sourceReceiptNumber,
          gift.batchId,
          gift.campaignId,
          gift.pledgeId,
          donorId(gift),
          gift.source,
        ]
          .filter(Boolean)
          .join(" ")
          .toLowerCase();
        if (needle && !searchable.includes(needle)) return false;
        if (state !== "all" && gift.effectiveState !== state) return false;
        if (donorType !== "all" && gift.donor.type !== donorType) return false;
        if (
          fundId !== "all" &&
          !gift.splits.some((split) => split.fundId === fundId)
        )
          return false;
        const date = gift.receivedAt.slice(0, 10);
        return (!from || date >= from) && (!to || date <= to);
      }),
    [donorType, from, fundId, gifts, query, state, to],
  );

  const totalMinor = filtered.reduce(
    (sum, gift) => sum + (gift.total?.amountMinor || 0),
    0,
  );
  const canCorrect = Boolean(
    detail && !detail.adjustment && detail.effectiveState === "posted",
  );
  const correctionReady =
    reason.trim().length >= 10 &&
    acknowledged &&
    (!["person", "household"].includes(correctionType) || subject);

  function resetCorrection(type: typeof correctionType) {
    setCorrectionType(type);
    setSubject(null);
    setSubjectQuery("");
    setSubjects([]);
    setAcknowledged(false);
  }
  async function correctAttribution(event: FormEvent) {
    event.preventDefault();
    if (!detail || !correctionReady) return;
    const donor =
      correctionType === "person" && subject
        ? { type: "person", personId: subject.id }
        : correctionType === "household" && subject
          ? { type: "household", householdId: subject.id }
          : { type: correctionType };
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      const result = await api<Adjustment>(
        `/api/chms/v1/finance/contributions/${encodeURIComponent(detail.id)}/attribution`,
        { method: "POST", body: { donor, reason: reason.trim() } },
      );
      setNotice(
        `Correction posted. ${result.replacement?.receiptNumber ? `Replacement receipt ${result.replacement.receiptNumber} created.` : "The adjustment chain was created."}`,
      );
      setReason("");
      setAcknowledged(false);
      setSubject(null);
      setSubjectQuery("");
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The attribution correction could not be posted.",
      );
    } finally {
      setBusy(false);
    }
  }

  return (
    <div>
      <PageHeader
        title="Gift register"
        subtitle="Find posted gifts, inspect their immutable source facts, and correct donor attribution through linked reversal and replacement entries."
      >
        <label className="admin-field min-w-56">
          <span>Branch</span>
          <Select
            value={branchId}
            onChange={(event) => setBranchId(event.target.value)}
          >
            {campuses.map((campus) => (
              <option key={campus.branchId} value={campus.branchId}>
                {campus.name}
              </option>
            ))}
          </Select>
        </label>
      </PageHeader>
      {error && <ErrorBox message={error} onRetry={() => void loadGifts()} />}
      {notice && (
        <p
          role="status"
          className="mb-5 rounded-xl bg-emerald-500/10 px-4 py-3 text-sm font-semibold text-emerald-800"
        >
          {notice}
        </p>
      )}

      <section
        className="mb-6 grid gap-px overflow-hidden rounded-2xl border border-white/[.08] bg-white/[.08] text-white shadow-[0_22px_55px_rgba(16,25,19,.16)] sm:grid-cols-3"
        style={{ backgroundColor: "var(--remi-green)" }}
      >
        <RegisterStat
          label="Visible records"
          value={String(filtered.length).padStart(2, "0")}
        />
        <RegisterStat label="Visible value" value={money(totalMinor)} />
        <RegisterStat
          label="Needs donor match"
          value={String(
            filtered.filter(
              (gift) =>
                ["anonymous", "unresolved"].includes(gift.donor.type) &&
                gift.effectiveState === "posted",
            ).length,
          ).padStart(2, "0")}
        />
      </section>

      <Card className="mb-6 p-5">
        <div className="grid gap-4 lg:grid-cols-[minmax(15rem,1.5fr)_repeat(3,minmax(9rem,.7fr))]">
          <label className="admin-field">
            <span>Search register</span>
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Receipt, reference, batch or donor ID"
            />
          </label>
          <label className="admin-field">
            <span>Effective state</span>
            <Select
              value={state}
              onChange={(event) => setState(event.target.value)}
            >
              <option value="all">All states</option>
              <option value="posted">Posted</option>
              <option value="corrected">Corrected</option>
              <option value="reversed">Reversed</option>
              <option value="adjustment">Adjustments</option>
            </Select>
          </label>
          <label className="admin-field">
            <span>Donor type</span>
            <Select
              value={donorType}
              onChange={(event) => setDonorType(event.target.value)}
            >
              <option value="all">All donors</option>
              <option value="person">Person</option>
              <option value="household">Household</option>
              <option value="anonymous">Anonymous</option>
              <option value="unresolved">Unresolved</option>
            </Select>
          </label>
          <label className="admin-field">
            <span>Fund</span>
            <Select
              value={fundId}
              onChange={(event) => setFundId(event.target.value)}
            >
              <option value="all">All funds</option>
              {funds.map((fund) => (
                <option value={fund.id} key={fund.id}>
                  {fund.name}
                </option>
              ))}
            </Select>
          </label>
        </div>
        <div className="mt-4 grid gap-4 sm:grid-cols-2 lg:max-w-xl">
          <label className="admin-field">
            <span>Received from</span>
            <DatePicker value={from} onChange={setFrom} />
          </label>
          <label className="admin-field">
            <span>Received through</span>
            <DatePicker min={from} value={to} onChange={setTo} />
          </label>
        </div>
      </Card>

      <div className="grid gap-6 xl:grid-cols-[minmax(22rem,.85fr)_minmax(0,1.35fr)]">
        <Card className="overflow-hidden">
          <header className="flex items-end justify-between gap-3 border-b border-black/[.045] p-5">
            <div>
              <h2 className="font-bold">Posted activity</h2>
              <p className="mt-1 text-xs text-[var(--remi-muted)]">
                Newest first · branch scoped
              </p>
            </div>
            <span className="font-mono text-xs text-[var(--remi-gold-deep)]">
              {filtered.length}
            </span>
          </header>
          <div className="max-h-[54rem] overflow-auto p-2">
            {filtered.map((gift) => (
              <button
                type="button"
                key={gift.id}
                onClick={() => setSelectedId(gift.id)}
                className={`mb-1 w-full rounded-xl p-4 text-left transition ${selectedId === gift.id ? "bg-[var(--remi-gold)]/14" : "hover:bg-black/[.025]"}`}
              >
                <div className="flex items-start justify-between gap-3">
                  <span>
                    <b className="block font-mono text-xs">
                      {gift.receiptNumber}
                    </b>
                    <small className="mt-1 block capitalize text-[var(--remi-muted)]">
                      {gift.donor.type} · {gift.source.replaceAll("-", " ")}
                    </small>
                  </span>
                  <b className="font-mono text-sm">
                    {money(gift.total.amountMinor)}
                  </b>
                </div>
                <div className="mt-3 flex items-center justify-between gap-3">
                  <time className="text-[10px] text-[var(--remi-muted)]">
                    {stamp(gift.receivedAt)}
                  </time>
                  <StatePill value={gift.effectiveState} />
                </div>
              </button>
            ))}
            {!loading && filtered.length === 0 && (
              <div className="p-3">
                <EmptyState
                  dense
                  title="No gifts match these filters"
                  hint="Clear a filter or choose another branch."
                />
              </div>
            )}
          </div>
        </Card>

        <main className="min-w-0">
          {!detail ? (
            <EmptyState
              title={loading ? "Loading gift register" : "Choose a posted gift"}
              hint="Immutable source facts and any correction chain will appear here."
            />
          ) : (
            <div className="space-y-5">
              <section className="overflow-hidden rounded-2xl border border-white/10 bg-[var(--remi-green)] p-6 text-white [background-image:radial-gradient(circle_at_92%_0%,rgba(209,173,85,.2),transparent_36%)]">
                <div className="flex flex-wrap items-start justify-between gap-5">
                  <div>
                    <span className="font-mono text-[10px] uppercase tracking-[.17em] text-[var(--altar-mint)]">
                      {detail.receiptNumber} · {detail.effectiveState}
                    </span>
                    <h2 className="mt-2 text-3xl font-bold tracking-[-.04em]">
                      {money(detail.total.amountMinor)}
                    </h2>
                    <p className="mt-2 text-sm capitalize text-white/55">
                      {detail.donor.type} gift ·{" "}
                      {detail.source.replaceAll("-", " ")} ·{" "}
                      {stamp(detail.receivedAt)}
                    </p>
                  </div>
                  <StatePill value={detail.effectiveState} inverse />
                </div>
              </section>
              <Card className="overflow-hidden">
                <DetailHeader
                  title="Immutable gift facts"
                  note="The original record has no edit or delete command."
                />
                <dl className="grid sm:grid-cols-2">
                  {" "}
                  <Fact
                    label="Donor attribution"
                    value={`${detail.donor.type}${donorId(detail) ? ` · ${donorId(detail)}` : ""}`}
                  />
                  <Fact
                    label="Payment method"
                    value={
                      methods.find((item) => item.id === detail.paymentMethodId)
                        ?.name || detail.paymentMethodId
                    }
                  />
                  <Fact label="Posted" value={stamp(detail.postedAt)} />
                  <Fact label="Fiscal period" value={detail.fiscalPeriodId} />
                  <Fact
                    label="Batch"
                    value={detail.batchId || "Direct or online gift"}
                  />
                  <Fact
                    label="Provider / tender reference"
                    value={
                      detail.providerReference ||
                      detail.paymentMethodReference ||
                      detail.sourceReceiptNumber ||
                      "No external reference"
                    }
                  />
                  <Fact
                    label="Campaign"
                    value={detail.campaignId || "Not campaign-attributed"}
                  />
                  <Fact
                    label="Pledge"
                    value={detail.pledgeId || "Not pledge-attributed"}
                  />
                </dl>
                <div className="border-t border-black/[.045] p-5">
                  <h3 className="text-sm font-bold">Fund allocation</h3>
                  <div className="mt-3 space-y-2">
                    {detail.splits.map((split) => (
                      <div
                        className="flex justify-between gap-4 rounded-xl bg-black/[.025] p-3 text-sm"
                        key={split.fundId}
                      >
                        <span>
                          {funds.find((fund) => fund.id === split.fundId)
                            ?.name || split.fundId}
                        </span>
                        <b className="font-mono">
                          {money(split.amount.amountMinor)}
                        </b>
                      </div>
                    ))}
                  </div>
                </div>
                {detail.adjustment && (
                  <div className="border-t border-black/[.045] bg-[var(--remi-gold)]/[.07] p-5">
                    <span className="font-mono text-[10px] uppercase tracking-wider text-[var(--remi-gold-deep)]">
                      Linked adjustment
                    </span>
                    <p className="mt-2 text-sm font-semibold capitalize">
                      {detail.adjustment.type?.replaceAll("-", " ")}
                    </p>
                    <p className="mt-1 text-xs leading-5 text-[var(--remi-muted)]">
                      {detail.adjustment.reason || "No public reason returned."}
                    </p>
                  </div>
                )}
              </Card>

              <Card className="overflow-hidden">
                <DetailHeader
                  title="Correct donor attribution"
                  note="Creates a reversal and replacement. It never overwrites this gift."
                />
                {canCorrect ? (
                  <form className="space-y-5 p-5" onSubmit={correctAttribution}>
                    <div
                      className="grid grid-cols-2 gap-2 sm:grid-cols-4"
                      role="group"
                      aria-label="Replacement donor type"
                    >
                      {(
                        [
                          "person",
                          "household",
                          "anonymous",
                          "unresolved",
                        ] as const
                      ).map((type) => (
                        <button
                          type="button"
                          key={type}
                          aria-pressed={correctionType === type}
                          onClick={() => resetCorrection(type)}
                          className={`rounded-xl px-3 py-3 text-xs font-bold capitalize ${correctionType === type ? "bg-[var(--remi-green)] text-white" : "bg-black/[.03] text-[var(--remi-muted)]"}`}
                        >
                          {type}
                        </button>
                      ))}
                    </div>
                    {["person", "household"].includes(correctionType) && (
                      <label className="admin-field relative">
                        <span>Verified {correctionType}</span>
                        <input
                          autoComplete="off"
                          value={subject?.name || subjectQuery}
                          onChange={(event) => {
                            setSubject(null);
                            setSubjectQuery(event.target.value);
                          }}
                          placeholder={`Search ${correctionType === "person" ? "member name or number" : "household name"}`}
                        />
                        {!subject && (searching || subjects.length > 0) && (
                          <div className="absolute left-0 right-0 top-full z-20 mt-1 rounded-xl border border-black/[.06] bg-[var(--remi-surface)] p-1 shadow-xl">
                            {searching ? (
                              <p className="p-3 text-xs text-[var(--remi-muted)]">
                                Searching verified records…
                              </p>
                            ) : (
                              subjects.map((item) => (
                                <button
                                  type="button"
                                  key={item.id}
                                  onClick={() => {
                                    setSubject(item);
                                    setSubjectQuery(item.name);
                                    setSubjects([]);
                                  }}
                                  className="block w-full rounded-lg p-3 text-left text-sm hover:bg-black/[.035]"
                                >
                                  <b>{item.name}</b>
                                  <small className="ml-2 font-mono text-[9px] uppercase text-[var(--remi-muted)]">
                                    {item.type}
                                  </small>
                                </button>
                              ))
                            )}
                          </div>
                        )}
                      </label>
                    )}
                    <label className="admin-field">
                      <span>Correction reason · minimum 10 characters</span>
                      <textarea
                        required
                        minLength={10}
                        rows={4}
                        value={reason}
                        onChange={(event) => setReason(event.target.value)}
                        placeholder="Describe the reviewed evidence and why the original attribution is incorrect."
                      />
                    </label>
                    <label className="flex items-start gap-3 rounded-xl bg-[var(--remi-gold)]/[.08] p-4 text-sm leading-5">
                      <input
                        className="mt-0.5"
                        type="checkbox"
                        checked={acknowledged}
                        onChange={(event) =>
                          setAcknowledged(event.target.checked)
                        }
                      />
                      <span>
                        <b className="block">
                          Post an auditable correction chain
                        </b>
                        <small className="mt-1 block text-[var(--remi-muted)]">
                          I understand this creates new receipt-register entries
                          and cannot be undone by deleting history.
                        </small>
                      </span>
                    </label>
                    <button
                      disabled={busy || !correctionReady}
                      className="admin-primary-button"
                    >
                      {busy
                        ? "Posting correction…"
                        : "Post attribution correction"}
                    </button>
                    <p className="text-xs leading-5 text-[var(--remi-muted)]">
                      Recent MFA and finance approval authority are enforced by
                      the API. Anonymous and unresolved gifts stay off member
                      statements until this command succeeds.
                    </p>
                  </form>
                ) : (
                  <div className="p-5">
                    <EmptyState
                      dense
                      title="This gift cannot be corrected again"
                      hint="Posted gifts accept one controlled manual correction. Existing adjustment history remains immutable."
                    />
                  </div>
                )}
              </Card>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

function RegisterStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="bg-white/[.035] p-5">
      <b className="block font-mono text-xl tabular-nums">{value}</b>
      <span className="mt-2 block text-[9px] font-bold uppercase tracking-[.15em] text-white/40">
        {label}
      </span>
    </div>
  );
}
function StatePill({
  value,
  inverse = false,
}: {
  value: string;
  inverse?: boolean;
}) {
  const tone =
    value === "posted" ? "emerald" : value === "corrected" ? "amber" : "slate";
  return (
    <span
      className={`rounded-full px-2.5 py-1 font-mono text-[9px] font-bold uppercase tracking-wider ${inverse ? "bg-white/10 text-white/70" : tone === "emerald" ? "bg-emerald-500/10 text-emerald-700" : tone === "amber" ? "bg-amber-500/10 text-amber-700" : "bg-slate-500/10 text-slate-600"}`}
    >
      {value}
    </span>
  );
}
function DetailHeader({ title, note }: { title: string; note: string }) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-3 border-b border-black/[.045] p-5">
      <h2 className="font-bold">{title}</h2>
      <small className="max-w-md text-right text-[var(--remi-muted)]">
        {note}
      </small>
    </header>
  );
}
function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 border-b border-black/[.045] p-5 even:sm:border-l">
      <dt className="font-mono text-[9px] uppercase tracking-wider text-[var(--remi-muted)]">
        {label}
      </dt>
      <dd className="mt-2 break-words text-sm font-semibold">{value}</dd>
    </div>
  );
}
