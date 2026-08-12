"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { BrandedSelect } from "@/components/branded-select";
import { Icon } from "@/components/icon";

type Campaign = { id: string; title: string; summary: string; fundId: string; state?: string };
type Method = { id: string; brand?: string; channel: string; last4?: string; expiryMonth?: string; expiryYear?: string; bank?: string };
type Instruction = { id: string; version: number; campaignId?: string; paymentMethodId: string; amountMinor: number; currency: string; frequency: string; state: string; nextChargeAt: string; actionUrl?: string };
type Household = { household: null | { id: string; name: string }; editable: boolean };

const money = (minor: number) => new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS" }).format(minor / 100);
async function data(response: Response) { const value = await response.json(); if (!response.ok) throw new Error(value.message || value.error?.message || value.error || "The request could not be completed."); return value; }

export function GivingPaymentWorkspace({ campaigns, household }: { campaigns: Campaign[]; household: Household | null }) {
  const [methods, setMethods] = useState<Method[]>([]);
  const [instructions, setInstructions] = useState<Instruction[]>([]);
  const [amount, setAmount] = useState(10000);
  const [campaignId, setCampaignId] = useState("");
  const [scope, setScope] = useState<"person" | "household">("person");
  const [saveMethod, setSaveMethod] = useState(false);
  const [frequency, setFrequency] = useState("monthly");
  const [methodId, setMethodId] = useState("");
  const [busy, setBusy] = useState("");
  const [message, setMessage] = useState("");
  const selected = useMemo(() => campaigns.find(item => item.id === campaignId) || campaigns[0], [campaignId, campaigns]);
  const canUseHousehold = Boolean(household?.editable && household.household);

  const load = useCallback(async () => {
    try {
      const [methodResponse, instructionResponse] = await Promise.all([fetch("/api/member/finance/payment-methods", { cache: "no-store" }), fetch("/api/member/finance/recurring-instructions", { cache: "no-store" })]);
      const [methodData, instructionData] = await Promise.all([data(methodResponse), data(instructionResponse)]);
      setMethods(methodData.items || []); setInstructions(instructionData.items || []);
      setMethodId(current => current || methodData.items?.[0]?.id || "");
    } catch (cause) { setMessage(cause instanceof Error ? cause.message : "Payment controls could not be loaded."); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  useEffect(() => { if (!campaignId && campaigns[0]) setCampaignId(campaigns[0].id); }, [campaignId, campaigns]);
  useEffect(() => { if (scope === "household") setSaveMethod(false); }, [scope]);

  async function give(event: FormEvent) {
    event.preventDefault(); if (!selected) return; setBusy("gift"); setMessage("");
    try {
      const result = await data(await fetch("/api/member/finance/giving-intents", { method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": crypto.randomUUID() }, body: JSON.stringify({ amountMinor: amount, fundId: selected.fundId, campaignId: selected.id, attributionScope: scope, savePaymentMethod: saveMethod }) }));
      if (result.authorizationUrl) window.location.assign(result.authorizationUrl); else setMessage("Your gift was recorded for processing.");
    } catch (cause) { setMessage(cause instanceof Error ? cause.message : "Checkout could not be opened."); setBusy(""); }
  }

  async function recur(event: FormEvent) {
    event.preventDefault(); if (!selected || !methodId) return; setBusy("recurring"); setMessage("");
    try {
      await data(await fetch("/api/member/finance/recurring-instructions", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ donor: { type: scope }, paymentMethodId: methodId, fundId: selected.fundId, campaignId: selected.id, amountMinor: amount, currency: "GHS", frequency, startsAt: new Date(Date.now() + 86400000).toISOString() }) }));
      setMessage("Your recurring giving rhythm is ready. The first charge is scheduled for tomorrow."); await load();
    } catch (cause) { setMessage(cause instanceof Error ? cause.message : "Recurring giving could not be created."); } finally { setBusy(""); }
  }

  async function update(instruction: Instruction, state: "active" | "paused" | "cancelled") {
    setBusy(instruction.id); setMessage("");
    try { await data(await fetch(`/api/member/finance/recurring-instructions/${instruction.id}`, { method: "PATCH", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ expectedVersion: instruction.version, state }) })); await load(); }
    catch (cause) { setMessage(cause instanceof Error ? cause.message : "The giving rhythm could not be updated."); } finally { setBusy(""); }
  }

  async function removeMethod(id: string) {
    setBusy(id); setMessage("");
    try { const response = await fetch(`/api/member/finance/payment-methods/${id}`, { method: "DELETE" }); if (!response.ok) await data(response); await load(); }
    catch (cause) { setMessage(cause instanceof Error ? cause.message : "The payment method could not be removed."); } finally { setBusy(""); }
  }

  return <section className="member-payment-workspace" aria-labelledby="give-now-title">
    <header><div><span>01 · SECURE GIVING</span><h2 id="give-now-title">Give now. Shape a rhythm.</h2><p>One clear checkout, private attribution and payment details protected by Paystack. REMI never stores card numbers or security codes.</p></div><i><Icon name="give" /></i></header>
    {message && <div className="member-payment-notice" role="status">{message}</div>}
    <div className="member-payment-grid">
      <form onSubmit={give}><small>ONE-TIME GIFT</small><h3>Choose what this gift supports</h3><label><span>Campaign</span><BrandedSelect value={selected?.id || ""} onChange={event => setCampaignId(event.target.value)}>{campaigns.map(item => <option value={item.id} key={item.id}>{item.title}</option>)}</BrandedSelect></label>{selected && <p className="member-payment-campaign">{selected.summary}</p>}<AmountChoice amount={amount} onChange={setAmount}/><Attribution value={scope} household={household} onChange={setScope}/><button type="button" className="member-save-method" aria-pressed={saveMethod} disabled={scope === "household"} onClick={() => setSaveMethod(value => !value)}><i>{saveMethod ? "✓" : "+"}</i><span><b>Save this payment method</b><small>{scope === "household" ? "Save methods from an individual gift." : "Use it later for recurring giving."}</small></span></button><button className="member-payment-primary" disabled={busy !== "" || !selected}>{busy === "gift" ? "Opening secure checkout…" : `Continue with ${money(amount)}`}<Icon name="arrow" /></button></form>
      <div className="member-recurring-panel"><small>RECURRING GIVING</small><h3>Keep generosity intentional</h3>{methods.length ? <form onSubmit={recur}><label><span>Payment method</span><BrandedSelect value={methodId} onChange={event => setMethodId(event.target.value)}>{methods.map(method => <option value={method.id} key={method.id}>{method.brand || method.channel} •••• {method.last4 || "saved"}</option>)}</BrandedSelect></label><label><span>Rhythm</span><BrandedSelect value={frequency} onChange={event => setFrequency(event.target.value)}><option value="weekly">Every week</option><option value="monthly">Every month</option><option value="quarterly">Every three months</option></BrandedSelect></label><button className="member-payment-primary" disabled={busy !== "" || !selected}>Create giving rhythm<Icon name="arrow" /></button></form> : <div className="member-payment-empty"><b>No saved method yet</b><p>Make an individual gift and choose “Save this payment method.” It appears here only after Paystack verifies the payment.</p></div>}<div className="member-method-list">{methods.map(method => <article key={method.id}><span><b>{method.brand || method.channel}</b><small>•••• {method.last4 || "saved"}{method.expiryMonth ? ` · ${method.expiryMonth}/${method.expiryYear}` : ""}</small></span><button disabled={busy !== ""} onClick={() => void removeMethod(method.id)}>Remove</button></article>)}</div></div>
    </div>
    {instructions.length > 0 && <div className="member-rhythm-list"><header><h3>Your active rhythms</h3><span>{instructions.length.toString().padStart(2, "0")}</span></header>{instructions.map(item => <article key={item.id}><div><span>{item.frequency}</span><b>{campaigns.find(c => c.id === item.campaignId)?.title || "Recurring gift"}</b><small>{money(item.amountMinor)} · next {new Date(item.nextChargeAt).toLocaleDateString("en-GH", { day: "numeric", month: "short", year: "numeric" })}</small>{item.actionUrl && <a href={item.actionUrl}>Complete payment check</a>}</div><em>{item.state}</em>{item.state !== "cancelled" && <footer>{item.state === "active" ? <button disabled={busy !== ""} onClick={() => void update(item, "paused")}>Pause</button> : <button disabled={busy !== ""} onClick={() => void update(item, "active")}>Resume</button>}<button disabled={busy !== ""} onClick={() => void update(item, "cancelled")}>Cancel</button></footer>}</article>)}</div>}
  </section>;
}

function AmountChoice({ amount, onChange }: { amount: number; onChange: (value: number) => void }) { return <fieldset className="member-amount-choice"><legend>Gift amount</legend><div>{[5000, 10000, 25000, 50000].map(value => <button type="button" aria-pressed={amount === value} onClick={() => onChange(value)} key={value}>{money(value)}</button>)}</div><label><span>Custom amount (GHS)</span><input inputMode="decimal" value={(amount / 100).toString()} onChange={event => onChange(Math.max(100, Math.round(Number(event.target.value || 0) * 100)))} /></label></fieldset>; }
function Attribution({ value, household, onChange }: { value: "person" | "household"; household: Household | null; onChange: (value: "person" | "household") => void }) { const enabled = Boolean(household?.editable && household.household); return <fieldset className="member-attribution-choice"><legend>Attribute this gift to</legend><button type="button" aria-pressed={value === "person"} onClick={() => onChange("person")}><b>My profile</b><small>Private individual record</small></button><button type="button" disabled={!enabled} aria-pressed={value === "household"} onClick={() => onChange("household")}><b>{household?.household?.name || "My household"}</b><small>{enabled ? "Shared household statement" : "Primary contact access required"}</small></button></fieldset>; }
