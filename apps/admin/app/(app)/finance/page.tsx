"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { Card, ErrorBox, PageHeader } from "@/components/ui";
import { api, ApiError } from "@/lib/api";
import { Select } from "@/components/ui/Select";

type Campus = { branchId: string; name: string };
type Fund = { id: string; name: string; active: boolean };
type Contribution = { id: string; total?: { amountMinor?: number }; effectiveState?: string; receivedAt?: string };
type Batch = { id: string; state: string; declaredTotal?: { amountMinor?: number }; enteredTotal?: { amountMinor?: number } };
type Settlement = { id: string; state: string; netAmountMinor: number; unexplainedVarianceMinor?: number };
type Pledge = { id: string; state: string; target?: { amountMinor?: number }; fulfilledAmountMinor?: number };

function items<T>(value: unknown): T[] {
  if (Array.isArray(value)) return value as T[];
  if (value && typeof value === "object" && Array.isArray((value as { items?: unknown[] }).items)) return (value as { items: T[] }).items;
  return [];
}

const money = (minor: number) => new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS", maximumFractionDigits: 0 }).format(minor / 100);

export default function FinanceCommandCenter() {
  const [campuses, setCampuses] = useState<Campus[]>([]);
  const [branchId, setBranchId] = useState("");
  const [funds, setFunds] = useState<Fund[]>([]);
  const [contributions, setContributions] = useState<Contribution[]>([]);
  const [batches, setBatches] = useState<Batch[]>([]);
  const [settlements, setSettlements] = useState<Settlement[]>([]);
  const [pledges, setPledges] = useState<Pledge[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadFoundation = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [campusData, fundData, settlementData, pledgeData] = await Promise.all([
        api("/api/chms/v1/finance/campuses"),
        api("/api/chms/v1/finance/funds"),
        api("/api/chms/v1/finance/settlements"),
        api("/api/chms/v1/finance/pledges"),
      ]);
      const nextCampuses = items<Campus>(campusData);
      setCampuses(nextCampuses);
      setBranchId((current) => current || nextCampuses[0]?.branchId || "");
      setFunds(items<Fund>(fundData));
      setSettlements(items<Settlement>(settlementData));
      setPledges(items<Pledge>(pledgeData));
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "Finance operations could not be loaded.");
    } finally {
      setLoading(false);
    }
  }, []);

  const loadBranch = useCallback(async () => {
    if (!branchId) {
      setContributions([]);
      setBatches([]);
      return;
    }
    try {
      const query = `branchId=${encodeURIComponent(branchId)}&limit=100`;
      const [contributionData, batchData] = await Promise.all([
        api(`/api/chms/v1/finance/contributions?${query}`),
        api(`/api/chms/v1/finance/batches?${query}`),
      ]);
      setContributions(items<Contribution>(contributionData));
      setBatches(items<Batch>(batchData));
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "Branch finance activity could not be loaded.");
    }
  }, [branchId]);

  useEffect(() => { void loadFoundation(); }, [loadFoundation]);
  useEffect(() => { void loadBranch(); }, [loadBranch]);

  const postedMinor = useMemo(() => contributions.reduce((sum, item) => sum + (item.effectiveState === "reversed" ? 0 : item.total?.amountMinor || 0), 0), [contributions]);
  const pendingBatches = batches.filter((item) => !["posted", "cancelled"].includes(item.state));
  const unresolvedSettlements = settlements.filter((item) => item.state !== "reconciled" || (item.unexplainedVarianceMinor || 0) !== 0);
  const activePledges = pledges.filter((item) => item.state === "active");
  const pledgeTargetMinor = activePledges.reduce((sum, item) => sum + (item.target?.amountMinor || 0), 0);
  const pledgeFulfilledMinor = activePledges.reduce((sum, item) => sum + (item.fulfilledAmountMinor || 0), 0);

  return <div>
    <PageHeader title="Finance command center" subtitle="Move from counted offerings to posted gifts, reconciled deposits and private member records without losing the audit trail.">
      <label className="admin-field min-w-52"><span>Branch</span><Select value={branchId} onChange={(event) => setBranchId(event.target.value)}>{campuses.map((campus) => <option key={campus.branchId} value={campus.branchId}>{campus.name}</option>)}</Select></label>
    </PageHeader>

    <section className="grid gap-6 overflow-hidden rounded-2xl border border-white/[.08] bg-[var(--remi-green)] p-6 text-white shadow-[0_24px_60px_rgba(16,25,19,.18)] [background-image:radial-gradient(circle_at_90%_0%,rgba(209,173,85,.2),transparent_32%)] lg:grid-cols-[1.2fr_.8fr] lg:p-8">
      <div><span className="font-mono text-[10px] uppercase tracking-[.18em] text-[var(--altar-mint)]">Ledger pulse</span><h2 className="mt-3 max-w-xl text-[clamp(2rem,4vw,3.8rem)] font-bold leading-[.94] tracking-[-.055em]">Every gift accounted for. Every exception owned.</h2><p className="mt-4 max-w-xl text-sm leading-6 text-white/55">Amounts shown here come from immutable posted contributions and controlled reconciliation records—not editable dashboard totals.</p></div>
      <div className="grid grid-cols-2 gap-px overflow-hidden rounded-xl border border-white/10 bg-white/10"><Pulse label="Posted gifts" value={loading ? "—" : money(postedMinor)} /><Pulse label="Open batches" value={loading ? "—" : String(pendingBatches.length).padStart(2, "0")} /><Pulse label="Exceptions" value={loading ? "—" : String(unresolvedSettlements.length).padStart(2, "0")} /><Pulse label="Active funds" value={loading ? "—" : String(funds.filter((fund) => fund.active).length).padStart(2, "0")} /></div>
    </section>

    {error && <div className="mt-5"><ErrorBox message={error} onRetry={() => void loadFoundation()} /></div>}

    <section className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4" aria-label="Finance workflow status">
      <Workflow index="01" title="Count & approve" value={`${pendingBatches.length} open`} copy="Dual-control counting batches and posting approvals." href="/finance/batches" state={pendingBatches.length ? "attention" : "clear"} />
      <Workflow index="02" title="Gift register" value={`${contributions.length} records`} copy="Search posted gifts and append authorized corrections." href="/finance/gifts" state="ready" />
      <Workflow index="03" title="Reconcile" value={`${unresolvedSettlements.length} exceptions`} copy="Match processor and bank evidence to the ledger." href="/reconciliation" state={unresolvedSettlements.length ? "attention" : "clear"} />
      <Workflow index="04" title="Reports & exports" value="Ledger reconciled" copy="Fund movement, deposits, pledges, accounting CSV and audit packages." href="/finance/reports" state="ready" />
    </section>

    <section id="configuration" className="mt-6 grid scroll-mt-6 gap-5 xl:grid-cols-[1.15fr_.85fr]">
      <Card className="overflow-hidden"><header className="flex items-end justify-between gap-4 border-b border-black/[.045] p-6"><div><span className="font-mono text-[10px] uppercase tracking-[.17em] text-[var(--remi-gold-deep)]">Operator routes</span><h2 className="mt-1 text-xl font-bold">Finance workspaces</h2></div><small className="text-[var(--remi-muted)]">Permission scoped</small></header><div className="grid sm:grid-cols-2"><Route href="/finance/gifts" title="Gift register & donor match" copy={`${contributions.length} recent records · immutable detail and controlled attribution correction`} /><Route href="/fundraising" title="Funds, campaigns & pledges" copy={`${activePledges.length} active intentions · ${money(pledgeFulfilledMinor)} of ${money(pledgeTargetMinor)}`} /><Route href="/reconciliation" title="Deposits & reconciliation" copy={`${settlements.length} imported settlements · ${unresolvedSettlements.length} requiring attention`} /><Route href="/statements" title="Receipts & statements" copy="Private recipient search, versioned PDFs and delivery history." /></div></Card>
      <Card className="p-6"><span className="font-mono text-[10px] uppercase tracking-[.17em] text-[var(--remi-gold-deep)]">Control posture</span><h2 className="mt-1 text-xl font-bold">What needs attention</h2><div className="mt-5 space-y-3"><Control label="Counting batches awaiting completion" value={pendingBatches.length} href="/finance/batches" /><Control label="Settlement exceptions or variances" value={unresolvedSettlements.length} href="/reconciliation" /><Control label="Active voluntary pledges" value={activePledges.length} href="/fundraising" quiet /></div><Link href="/finance/configuration" className="admin-secondary-button mt-5 flex w-full items-center justify-between">Manage finance configuration <span>→</span></Link><p className="mt-5 border-t border-black/[.045] pt-4 text-xs leading-5 text-[var(--remi-muted)]">Posting, variance approval, attribution correction and period control remain separate permissioned commands. This page never mutates ledger state.</p></Card>
    </section>
  </div>;
}

function Pulse({ label, value }: { label: string; value: string }) { return <div className="min-h-24 bg-white/[.045] p-4"><b className="block font-mono text-xl tabular-nums">{value}</b><span className="mt-2 block text-[9px] font-bold uppercase tracking-[.14em] text-white/40">{label}</span></div>; }
function Workflow({ index, title, value, copy, href, state }: { index: string; title: string; value: string; copy: string; href: string; state: string }) { return <Link href={href} className="admin-card group min-h-44 p-5 transition hover:-translate-y-0.5"><div className="flex items-center justify-between"><span className="font-mono text-[10px] tracking-[.16em] text-[var(--remi-muted)]">{index}</span><i className={`size-2 rounded-full ${state === "attention" ? "bg-[var(--remi-gold)]" : state === "clear" ? "bg-emerald-600" : "bg-sky-600"}`} /></div><strong className="mt-5 block text-base">{title}</strong><b className="mt-1 block font-mono text-lg text-[var(--remi-gold-deep)]">{value}</b><p className="mt-2 text-xs leading-5 text-[var(--remi-muted)]">{copy}</p></Link>; }
function Route({ href, title, copy }: { href: string; title: string; copy: string }) { return <Link href={href} className="group min-h-36 border-b border-black/[.045] p-6 even:border-l sm:[&:nth-last-child(-n+2)]:border-b-0"><div className="flex justify-between gap-4"><strong>{title}</strong><span className="text-[var(--remi-gold-deep)] transition group-hover:translate-x-1">→</span></div><p className="mt-2 text-xs leading-5 text-[var(--remi-muted)]">{copy}</p></Link>; }
function Control({ label, value, href, quiet = false }: { label: string; value: number; href: string; quiet?: boolean }) { return <Link href={href} className="flex items-center gap-3 rounded-xl bg-black/[.025] p-4"><span className={`grid size-9 place-items-center rounded-lg font-mono text-sm ${quiet ? "bg-sky-500/10 text-sky-700" : value ? "bg-[var(--remi-gold)]/20 text-[var(--remi-gold-deep)]" : "bg-emerald-500/10 text-emerald-700"}`}>{String(value).padStart(2, "0")}</span><span className="flex-1 text-sm font-medium">{label}</span><span className="text-[var(--remi-muted)]">→</span></Link>; }
