"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Icon } from "@/components/icon";
import { MemberDashboardShell } from "@/components/member-dashboard-shell";
import { BrandedSelect } from "@/components/branded-select";
import { GivingPaymentWorkspace } from "@/components/giving-payment-workspace";

type Campaign = { id: string; title: string; summary: string; fundId: string; goal: { amountMinor: number }; raisedAmountMinor: number; endsAt: string };
type Pledge = { id: string; version: number; branchId: string; donor: { type: string; personId?: string; householdId?: string }; fundId: string; campaignId?: string; target: { amountMinor: number; currency?: string }; schedule: { frequency: string; installmentAmount: { amountMinor: number; currency?: string }; startsAt: string; endsAt?: string }; reminder: { optedIn: boolean; channels: string[]; cadence: string; recipientPersonId?: string }; state: string; effectiveState: string; fulfilledAmountMinor: number; remainingAmountMinor: number; note?: string };
type Home = { member: { branchId: string; name: string; branchName?: string } };
type Statement = { id: string; year: number; statementVersion: number; generatedAt: string; totalAmountMinor: number; rowCount?: number; rows: unknown[]; artifactHash: string; deliveryCount: number; supersedesId?: string };
type Receipt = { contributionId: string; receiptNumber: string; receivedAt: string; amountMinor: number; effectiveState: string; allocations: { fund: string; amountMinor: number }[] };
type Household = { household: null | { id: string; name: string; statementPreference: string }; editable: boolean };

const currentYear = new Date().getFullYear();
const money = (minor: number) => new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS" }).format(minor / 100);
const date = (value: string) => new Intl.DateTimeFormat("en-GH", { day: "numeric", month: "short", year: "numeric" }).format(new Date(value));

async function responseData(response: Response) {
  const data = await response.json();
  if (!response.ok) throw new Error(data.message || data.error?.message || data.error || "The request could not be completed.");
  return data;
}

export default function GivingPage() {
  const [home, setHome] = useState<Home | null>(null);
  const [campaigns, setCampaigns] = useState<Campaign[]>([]);
  const [pledges, setPledges] = useState<Pledge[]>([]);
  const [household, setHousehold] = useState<Household | null>(null);
  const [statements, setStatements] = useState<Statement[]>([]);
  const [receipts, setReceipts] = useState<Receipt[]>([]);
  const [scope, setScope] = useState<"individual" | "household">("individual");
  const [year, setYear] = useState(currentYear);
  const [error, setError] = useState("");
  const [documentError, setDocumentError] = useState("");
  const [busy, setBusy] = useState(false);
  const [documentsLoading, setDocumentsLoading] = useState(true);
  const [form, setForm] = useState({ campaignId: "", target: "500", installment: "100", frequency: "monthly", reminders: true, email: true, sms: false });
  const selected = useMemo(() => campaigns.find((campaign) => campaign.id === form.campaignId), [campaigns, form.campaignId]);
  const householdStatements = household?.editable && household.household?.statementPreference === "household";

  const loadDocuments = useCallback(async (nextScope: string, nextYear: number) => {
    setDocumentsLoading(true);
    setDocumentError("");
    const query = new URLSearchParams({ scope: nextScope, year: String(nextYear) });
    try {
      const [statementResponse, receiptResponse] = await Promise.all([
        fetch(`/api/member/statements?scope=${encodeURIComponent(nextScope)}`, { cache: "no-store" }),
        fetch(`/api/member/receipts?${query}`, { cache: "no-store" }),
      ]);
      const [statementData, receiptData] = await Promise.all([responseData(statementResponse), responseData(receiptResponse)]);
      setStatements(statementData.items || []);
      setReceipts(receiptData.items || []);
    } catch (cause) {
      setDocumentError(cause instanceof Error ? cause.message : "Giving documents could not be loaded.");
      setStatements([]);
      setReceipts([]);
    } finally {
      setDocumentsLoading(false);
    }
  }, []);

  const load = useCallback(async () => {
    setError("");
    try {
      const responses = await Promise.all([
        fetch("/api/member/home", { cache: "no-store" }),
        fetch("/api/member/campaigns", { cache: "no-store" }),
        fetch("/api/member/pledges", { cache: "no-store" }),
        fetch("/api/member/household-details", { cache: "no-store" }),
      ]);
      const data = await Promise.all(responses.map(responseData));
      setHome(data[0]);
      const nextCampaigns = Array.isArray(data[1]) ? data[1] : data[1].items || [];
      setCampaigns(nextCampaigns);
      setPledges(data[2].items || []);
      setHousehold(data[3]);
      setForm((value) => ({ ...value, campaignId: value.campaignId || nextCampaigns[0]?.id || "" }));
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Giving could not be loaded.");
    }
  }, []);

  useEffect(() => { void load(); }, [load]);
  useEffect(() => { void loadDocuments(scope, year); }, [loadDocuments, scope, year]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!home || !selected) return;
    setBusy(true);
    setError("");
    try {
      const channels = [form.email && "email", form.sms && "sms"].filter(Boolean);
      await responseData(await fetch("/api/member/pledges", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ branchId: home.member.branchId, fundId: selected.fundId, campaignId: selected.id, target: { amountMinor: Math.round(Number(form.target) * 100), currency: "GHS" }, schedule: { frequency: form.frequency, installmentAmount: { amountMinor: Math.round(Number(form.installment) * 100), currency: "GHS" }, startsAt: new Date().toISOString() }, reminder: { optedIn: form.reminders, channels, cadence: "monthly" }, state: "active" }) }));
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Pledge could not be saved.");
    } finally { setBusy(false); }
  }

  async function changeState(pledge: Pledge, state: "active" | "paused" | "cancelled") {
    setBusy(true);
    setError("");
    try {
      await responseData(await fetch(`/api/member/pledges/${pledge.id}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ branchId: pledge.branchId, donor: pledge.donor, fundId: pledge.fundId, campaignId: pledge.campaignId, target: { ...pledge.target, currency: "GHS" }, schedule: { ...pledge.schedule, installmentAmount: { ...pledge.schedule.installmentAmount, currency: "GHS" } }, reminder: pledge.reminder, state, note: pledge.note || "", expectedVersion: pledge.version }) }));
      await load();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Pledge could not be updated.");
    } finally { setBusy(false); }
  }

  async function generateStatement() {
    setBusy(true);
    setDocumentError("");
    try {
      await responseData(await fetch(`/api/member/statements?scope=${scope}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ year }) }));
      await loadDocuments(scope, year);
    } catch (cause) {
      setDocumentError(cause instanceof Error ? cause.message : "Statement could not be generated.");
    } finally { setBusy(false); }
  }

  async function emailDocument(type: "statements" | "receipts", id: string) {
    setBusy(true);
    setDocumentError("");
    try {
      await responseData(await fetch(`/api/member/${type}/${id}/deliveries`, { method: "POST", headers: { "Idempotency-Key": crypto.randomUUID() } }));
      if (type === "statements") await loadDocuments(scope, year);
    } catch (cause) {
      setDocumentError(cause instanceof Error ? cause.message : "The document could not be emailed.");
    } finally { setBusy(false); }
  }

  return <MemberDashboardShell active="giving" member={home?.member}>
    <main className="member-giving-shell">
    <header><a href="/" aria-label="Back to dashboard">←</a><div><span>MY REMI · GENEROSITY</span><h1>Give with intention.</h1><p>A pledge is a personal intention, never a debt. Your posted gifts, receipts and statements stay private to you.</p></div></header>
    {error && <div className="member-giving-error" role="alert">{error}<button onClick={() => void load()}>Try again</button></div>}
    <GivingPaymentWorkspace campaigns={campaigns} household={household} />
    <section className="member-giving-layout">
      <div><div className="member-giving-heading"><span>01</span><h2>Your pledges</h2></div><div className="member-pledge-list">{pledges.length ? pledges.map((pledge) => <article key={pledge.id}><div><span>{pledge.effectiveState}</span><strong>{campaigns.find((campaign) => campaign.id === pledge.campaignId)?.title || "Giving intention"}</strong><small>{pledge.schedule.frequency} · {money(pledge.schedule.installmentAmount.amountMinor)} installments</small></div><div><b>{money(pledge.fulfilledAmountMinor)}</b><small>of {money(pledge.target.amountMinor)}</small></div><i><span style={{ width: `${Math.min(100, pledge.target.amountMinor ? pledge.fulfilledAmountMinor / pledge.target.amountMinor * 100 : 0)}%` }} /></i>{pledge.state !== "cancelled" && <footer>{pledge.state === "active" ? <button disabled={busy} onClick={() => void changeState(pledge, "paused")}>Pause</button> : <button disabled={busy} onClick={() => void changeState(pledge, "active")}>Resume</button>}<button disabled={busy} onClick={() => void changeState(pledge, "cancelled")}>Cancel pledge</button></footer>}</article>) : <div className="member-giving-empty"><Icon name="give" /><strong>No pledge yet</strong><p>Choose a campaign when you are ready to set an intention.</p></div>}</div></div>
      <aside><div className="member-giving-heading"><span>02</span><h2>Make a pledge</h2></div><form onSubmit={submit}><label><span>Campaign</span><BrandedSelect required value={form.campaignId} onChange={(event) => setForm({ ...form, campaignId: event.target.value })}>{campaigns.map((campaign) => <option value={campaign.id} key={campaign.id}>{campaign.title}</option>)}</BrandedSelect></label>{selected && <div className="member-campaign-context"><strong>{selected.title}</strong><p>{selected.summary}</p><small>{money(selected.raisedAmountMinor)} raised · closes {date(selected.endsAt)}</small></div>}<div className="member-giving-form-grid"><label><span>Total intention (GHS)</span><input type="number" min="1" step="0.01" required value={form.target} onChange={(event) => setForm({ ...form, target: event.target.value })} /></label><label><span>Installment (GHS)</span><input type="number" min="1" step="0.01" required value={form.installment} onChange={(event) => setForm({ ...form, installment: event.target.value })} /></label></div><label><span>Rhythm</span><BrandedSelect value={form.frequency} onChange={(event) => setForm({ ...form, frequency: event.target.value })}><option value="one-time">One time</option><option value="weekly">Weekly</option><option value="monthly">Monthly</option><option value="quarterly">Quarterly</option></BrandedSelect></label><label className="member-reminder-choice"><input type="checkbox" checked={form.reminders} onChange={(event) => setForm({ ...form, reminders: event.target.checked })} /><span><strong>Helpful progress reminders</strong><small>Only where your communication consent remains active.</small></span></label>{form.reminders && <div className="member-channel-choices"><label><input type="checkbox" checked={form.email} onChange={(event) => setForm({ ...form, email: event.target.checked })} /> Email</label><label><input type="checkbox" checked={form.sms} onChange={(event) => setForm({ ...form, sms: event.target.checked })} /> SMS</label></div>}<button disabled={busy || !selected}>{busy ? "Saving intention…" : "Create pledge"}<Icon name="arrow" /></button><p className="member-pledge-note">This is not a bill or receivable. REMI does not rank members by giving.</p></form></aside>
    </section>

    <section className="member-giving-documents" aria-labelledby="giving-documents-title">
      <header><div><span>03 · PRIVATE RECORDS</span><h2 id="giving-documents-title">Receipts & statements</h2><p>Reproducible records generated directly from posted gifts and approved corrections.</p></div><div className="member-document-controls">{householdStatements && <div className="member-document-scope" role="group" aria-label="Statement scope"><button aria-pressed={scope === "individual"} onClick={() => setScope("individual")}>My giving</button><button aria-pressed={scope === "household"} onClick={() => setScope("household")}>Household</button></div>}<label><span>Year</span><BrandedSelect value={year} onChange={(event) => setYear(Number(event.target.value))}>{Array.from({ length: 6 }, (_, index) => currentYear - index).map((value) => <option key={value} value={value}>{value}</option>)}</BrandedSelect></label><button className="member-generate-statement" disabled={busy || documentsLoading} onClick={() => void generateStatement()}>Generate statement</button></div></header>
      {documentError && <div className="member-document-error" role="alert">{documentError}<button onClick={() => void loadDocuments(scope, year)}>Try again</button></div>}
      {documentsLoading ? <div className="member-document-skeleton" role="status" aria-label="Loading receipts and statements"><i /><i /><i /></div> : <div className="member-document-grid">
        <div><div className="member-giving-heading"><span>A</span><h3>Statements</h3></div>{statements.length ? <div className="member-statement-list">{statements.map((statement) => <article key={statement.id}><div><span>Version {statement.statementVersion}</span><strong>{statement.year} giving statement</strong><small>{statement.rows.length} entries · generated {date(statement.generatedAt)}</small></div><b>{money(statement.totalAmountMinor)}</b><footer><a href={`/api/member/statements/${statement.id}/pdf`}>Download PDF</a><button disabled={busy} onClick={() => void emailDocument("statements", statement.id)}>Email me</button></footer>{statement.supersedesId && <em>Updated after an approved correction</em>}</article>)}</div> : <div className="member-document-empty"><strong>No statement generated</strong><p>Create a statement for this year whenever you need one.</p></div>}</div>
        <div><div className="member-giving-heading"><span>B</span><h3>Receipts</h3></div>{receipts.length ? <div className="member-receipt-list">{receipts.map((receipt) => <article key={receipt.contributionId}><div><span>{receipt.receiptNumber}</span><strong>{money(receipt.amountMinor)}</strong><small>{date(receipt.receivedAt)} · {receipt.allocations.map((allocation) => allocation.fund).join(", ")}</small></div><span className={`member-receipt-state is-${receipt.effectiveState}`}>{receipt.effectiveState}</span><footer><a href={`/api/member/receipts/${receipt.contributionId}/pdf`}>PDF</a><button disabled={busy} onClick={() => void emailDocument("receipts", receipt.contributionId)}>Email</button></footer></article>)}</div> : <div className="member-document-empty"><strong>No posted gifts for {year}</strong><p>Receipts appear only after a gift is posted to the finance ledger.</p></div>}</div>
      </div>}
      <p className="member-document-privacy">Private by design · Files are never publicly cached · Corrections preserve every prior statement version</p>
    </section>
    </main>
  </MemberDashboardShell>;
}
