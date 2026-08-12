"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { api, apiDownload, ApiError, asList } from "@/lib/api";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";

type Subject = { type: "person" | "household"; id: string; name: string; branchId: string };
type Period = { id: string; name: string; code: string; status: string; startsAt: string; endsAt: string };
type Statement = { id: string; year: number; periodId?: string; statementVersion: number; generatedAt: string; totalAmountMinor: number; rows: unknown[]; artifactHash: string; deliveryCount: number; supersedesId?: string };
type Receipt = { contributionId: string; receiptNumber: string; receivedAt: string; amountMinor: number; effectiveState: string; allocations: { fund: string; amountMinor: number }[] };

const yearNow = new Date().getFullYear();
const money = (value: number) => new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS" }).format(value / 100);
const formatDate = (value: string) => new Intl.DateTimeFormat("en-GH", { day: "numeric", month: "short", year: "numeric" }).format(new Date(value));

export default function StatementsPage() {
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<Subject[]>([]);
  const [subject, setSubject] = useState<Subject | null>(null);
  const [periods, setPeriods] = useState<Period[]>([]);
  const [rangeMode, setRangeMode] = useState<"year" | "period">("year");
  const [year, setYear] = useState(yearNow);
  const [periodId, setPeriodId] = useState("");
  const [statements, setStatements] = useState<Statement[]>([]);
  const [receipts, setReceipts] = useState<Receipt[]>([]);
  const [searching, setSearching] = useState(false);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const selectedPeriod = periods.find((period) => period.id === periodId);

  useEffect(() => {
    api("/api/chms/v1/finance/periods").then((data) => {
      const next = asList<Period>(data);
      setPeriods(next);
      setPeriodId(next[0]?.id || "");
    }).catch(() => setPeriods([]));
  }, []);

  useEffect(() => {
    if (subject || query.trim().length < 2) { setResults([]); return; }
    const timeout = window.setTimeout(async () => {
      setSearching(true);
      try { setResults(asList<Subject>(await api(`/api/chms/v1/finance/statement-subjects?q=${encodeURIComponent(query.trim())}`))); }
      catch { setResults([]); }
      finally { setSearching(false); }
    }, 240);
    return () => window.clearTimeout(timeout);
  }, [query, subject]);

  const loadDocuments = useCallback(async () => {
    if (!subject) return;
    setLoading(true);
    setError(null);
    const recipient = new URLSearchParams({ subjectType: subject.type, subjectId: subject.id });
    const receiptFilter = new URLSearchParams(recipient);
    if (rangeMode === "period") receiptFilter.set("periodId", periodId);
    else receiptFilter.set("year", String(year));
    try {
      const [statementData, receiptData] = await Promise.all([
        api(`/api/chms/v1/finance/statements?${recipient}`),
        api(`/api/chms/v1/finance/receipts?${receiptFilter}`),
      ]);
      setStatements(asList<Statement>(statementData));
      setReceipts(asList<Receipt>(receiptData));
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "Giving documents could not be loaded.");
    } finally { setLoading(false); }
  }, [periodId, rangeMode, subject, year]);

  useEffect(() => { void loadDocuments(); }, [loadDocuments]);

  async function generate() {
    if (!subject) return;
    setBusy(true);
    setError(null);
    try {
      await api("/api/chms/v1/finance/statements", { method: "POST", body: { subjectType: subject.type, subjectId: subject.id, branchId: subject.branchId, ...(rangeMode === "period" ? { periodId } : { year }) } });
      await loadDocuments();
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "The statement could not be generated.");
    } finally { setBusy(false); }
  }

  async function deliver(type: "statements" | "receipts", id: string) {
    setBusy(true);
    setError(null);
    try {
      await api(`/api/chms/v1/finance/${type}/${id}/deliveries`, { method: "POST" });
      if (type === "statements") await loadDocuments();
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "The private document could not be delivered.");
    } finally { setBusy(false); }
  }

  const total = useMemo(() => receipts.reduce((sum, receipt) => sum + receipt.amountMinor, 0), [receipts]);

  return <div>
    <PageHeader title="Receipts & statements" subtitle="Issue reproducible giving records from the immutable contribution ledger, with every delivery and correction preserved." />
    {error && <ErrorBox message={error} onRetry={loadDocuments} />}
    <div className="grid gap-6 xl:grid-cols-[22rem_minmax(0,1fr)]">
      <div className="space-y-5">
        <Card className="relative overflow-visible p-5">
          <p className="font-mono text-[10px] font-bold uppercase tracking-[.17em] text-[var(--remi-gold-deep)]">01 · Recipient</p>
          <h2 className="mt-2 text-xl font-bold tracking-[-.03em]">Find a member or household</h2>
          <p className="mt-1 text-xs leading-5 text-[var(--remi-muted)]">Searches only statement-safe identity fields.</p>
          <label className="admin-field relative mt-5 block"><span>Recipient</span><input autoComplete="off" value={subject?.name || query} placeholder="Name or member number" onChange={(event) => { setSubject(null); setQuery(event.target.value); }} />{(searching || results.length > 0) && !subject && <div className="absolute left-0 right-0 top-full z-30 mt-1 overflow-hidden rounded-xl border border-black/[.06] bg-[var(--remi-surface)] p-1 shadow-2xl">{searching ? <p className="px-3 py-3 text-xs text-[var(--remi-muted)]">Searching…</p> : results.map((value) => <button key={`${value.type}-${value.id}`} type="button" onClick={() => { setSubject(value); setQuery(value.name); setResults([]); }} className="flex w-full items-center justify-between rounded-lg px-3 py-3 text-left hover:bg-[var(--remi-canvas)]"><span><b className="block text-sm">{value.name}</b><small className="mt-0.5 block font-mono text-[9px] uppercase tracking-wider text-[var(--remi-muted)]">{value.type}</small></span><span className="text-[var(--remi-gold-deep)]">→</span></button>)}</div>}</label>
          {subject && <div className="mt-4 rounded-xl bg-[var(--remi-canvas)] p-3"><span className="font-mono text-[9px] uppercase tracking-wider text-[var(--remi-gold-deep)]">Selected {subject.type}</span><strong className="mt-1 block text-sm">{subject.name}</strong><button className="mt-2 text-[11px] font-bold text-[var(--remi-muted)] underline" onClick={() => { setSubject(null); setQuery(""); setStatements([]); setReceipts([]); }}>Change recipient</button></div>}
        </Card>
        <Card className="p-5">
          <p className="font-mono text-[10px] font-bold uppercase tracking-[.17em] text-[var(--remi-gold-deep)]">02 · Period</p>
          <div className="mt-4 grid grid-cols-2 gap-2 rounded-xl bg-[var(--remi-canvas)] p-1"><button aria-pressed={rangeMode === "year"} onClick={() => setRangeMode("year")} className={`rounded-lg px-3 py-2 text-xs font-bold ${rangeMode === "year" ? "bg-[var(--remi-green)] text-white" : "text-[var(--remi-muted)]"}`}>Calendar year</button><button aria-pressed={rangeMode === "period"} onClick={() => setRangeMode("period")} className={`rounded-lg px-3 py-2 text-xs font-bold ${rangeMode === "period" ? "bg-[var(--remi-green)] text-white" : "text-[var(--remi-muted)]"}`}>Fiscal period</button></div>
          {rangeMode === "year" ? <label className="admin-field mt-4 block"><span>Year</span><Select value={year} onChange={(event) => setYear(Number(event.target.value))}>{Array.from({ length: 8 }, (_, index) => yearNow - index).map((value) => <option key={value}>{value}</option>)}</Select></label> : <label className="admin-field mt-4 block"><span>Fiscal period</span><Select value={periodId} onChange={(event) => setPeriodId(event.target.value)}>{periods.map((period) => <option key={period.id} value={period.id}>{period.code} · {period.name}</option>)}</Select></label>}
          {selectedPeriod && rangeMode === "period" && <p className="mt-2 text-[10px] text-[var(--remi-muted)]">{formatDate(selectedPeriod.startsAt)} – {formatDate(selectedPeriod.endsAt)}</p>}
          <button disabled={!subject || busy || (rangeMode === "period" && !periodId)} onClick={() => void generate()} className="admin-primary-button mt-5 w-full rounded-xl px-4 py-3 text-sm font-bold disabled:opacity-45">{busy ? "Working…" : "Generate statement"}</button>
        </Card>
      </div>

      <div className="min-w-0 space-y-6">
        {!subject ? <Card className="grid min-h-96 place-items-center p-8"><EmptyState title="Choose a statement recipient" hint="Find a person or household to see posted receipts, prior statement versions and delivery history." /></Card> : loading ? <div className="grid gap-4 sm:grid-cols-3" aria-label="Loading giving documents">{[1, 2, 3].map((value) => <div key={value} className="h-40 animate-pulse rounded-2xl bg-[var(--remi-skeleton)]" />)}</div> : <>
          <section className="grid gap-3 sm:grid-cols-3"><Metric index="01" label="Period total" value={money(total)} /><Metric index="02" label="Posted records" value={String(receipts.length).padStart(2, "0")} /><Metric index="03" label="Statement versions" value={String(statements.length).padStart(2, "0")} /></section>
          <Card className="overflow-hidden"><header className="flex items-end justify-between gap-4 border-b border-black/[.045] p-5"><div><p className="font-mono text-[10px] uppercase tracking-[.16em] text-[var(--remi-gold-deep)]">Versioned artifacts</p><h2 className="mt-1 text-xl font-bold">Statements</h2></div><small className="text-[var(--remi-muted)]">Latest first</small></header>{statements.length ? <div className="divide-y divide-black/[.045]">{statements.map((statement) => <article key={statement.id} className="grid gap-4 p-5 md:grid-cols-[1fr_auto]"><div><div className="flex flex-wrap items-center gap-2"><span className="rounded-full bg-[var(--remi-gold)]/15 px-2 py-1 font-mono text-[9px] uppercase tracking-wider text-[var(--remi-gold-deep)]">Version {statement.statementVersion}</span>{statement.supersedesId && <span className="text-[10px] text-[var(--remi-muted)]">Supersedes a corrected snapshot</span>}</div><strong className="mt-3 block">{statement.periodId ? periods.find((period) => period.id === statement.periodId)?.name || "Fiscal statement" : `${statement.year} giving statement`}</strong><small className="mt-1 block text-[var(--remi-muted)]">{statement.rows.length} entries · {formatDate(statement.generatedAt)} · {statement.deliveryCount} deliveries</small><code className="mt-2 block truncate font-mono text-[9px] text-[var(--remi-muted)]">SHA-256 {statement.artifactHash}</code></div><div className="flex flex-col items-end justify-between gap-3"><b>{money(statement.totalAmountMinor)}</b><div className="flex gap-2"><button className="admin-secondary-button !px-3 !py-2 text-xs" onClick={() => void apiDownload(`/api/chms/v1/finance/statements/${statement.id}/pdf`, `REMI-statement-v${statement.statementVersion}.pdf`)}>Download</button><button disabled={busy} className="admin-secondary-button !px-3 !py-2 text-xs" onClick={() => void deliver("statements", statement.id)}>Email</button></div></div></article>)}</div> : <div className="p-6"><EmptyState dense title="No statement versions yet" hint="Generate the first immutable snapshot for this period." /></div>}</Card>
          <Card className="overflow-hidden"><header className="border-b border-black/[.045] p-5"><p className="font-mono text-[10px] uppercase tracking-[.16em] text-[var(--remi-gold-deep)]">Posted gift register</p><h2 className="mt-1 text-xl font-bold">Receipts</h2></header>{receipts.length ? <div className="divide-y divide-black/[.045]">{receipts.map((receipt) => <article key={receipt.contributionId} className="grid items-center gap-4 p-5 sm:grid-cols-[1fr_auto_auto]"><div><span className="font-mono text-[10px] text-[var(--remi-gold-deep)]">{receipt.receiptNumber}</span><strong className="mt-1 block">{receipt.allocations.map((allocation) => allocation.fund).join(", ")}</strong><small className="mt-1 block text-[var(--remi-muted)]">{formatDate(receipt.receivedAt)}</small></div><div className="sm:text-right"><b>{money(receipt.amountMinor)}</b><span className="mt-1 block font-mono text-[9px] uppercase text-[var(--remi-muted)]">{receipt.effectiveState}</span></div><div className="flex gap-2"><button className="admin-secondary-button !px-3 !py-2 text-xs" onClick={() => void apiDownload(`/api/chms/v1/finance/receipts/${receipt.contributionId}/pdf`, `${receipt.receiptNumber}.pdf`)}>PDF</button><button disabled={busy} className="admin-secondary-button !px-3 !py-2 text-xs" onClick={() => void deliver("receipts", receipt.contributionId)}>Email</button></div></article>)}</div> : <div className="p-6"><EmptyState dense title="No posted receipts in this period" hint="Anonymous and unresolved gifts remain excluded until an authorized attribution correction is posted." /></div>}</Card>
        </>}
      </div>
    </div>
  </div>;
}

function Metric({ index, label, value }: { index: string; label: string; value: string }) {
  return <Card className="relative overflow-hidden p-5"><span className="font-mono text-[9px] text-[var(--remi-gold-deep)]">{index}</span><b className="mt-5 block text-2xl tracking-[-.04em]">{value}</b><small className="mt-1 block text-[var(--remi-muted)]">{label}</small></Card>;
}
