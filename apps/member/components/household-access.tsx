"use client";

import { useState, type FormEvent } from "react";
import { BrandedSelect } from "@/components/branded-select";

type Delegation = { id: string; direction: "granted" | "received"; personName: string; fields: string[]; expiresAt: string; createdAt: string };
type Candidate = { id: string; name: string };
type AccessArea = { id: string; label: string };
export type HouseholdAccessResponse = { items: Delegation[]; candidates: Candidate[]; accessAreas: AccessArea[] };

const EXPIRIES = [{ days: 30, label: "30 days" }, { days: 90, label: "3 months" }, { days: 180, label: "6 months" }, { days: 365, label: "1 year" }];

export function HouseholdAccess({ initial }: { initial: HouseholdAccessResponse | null }) {
  const [items, setItems] = useState(initial?.items ?? []);
  const candidates = initial?.candidates ?? [];
  const areas = initial?.accessAreas ?? [];
  const [delegatePersonId, setDelegatePersonId] = useState(candidates[0]?.id ?? "");
  const [fields, setFields] = useState<string[]>(["profile", "registrations", "groups", "serving"]);
  const [days, setDays] = useState(90);
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  function toggle(field: string) { setFields(current => current.includes(field) ? current.filter(value => value !== field) : [...current, field]); }
  async function create(event: FormEvent) {
    event.preventDefault(); setBusy("create"); setError(""); setNotice("");
    try {
      const expiresAt = new Date(Date.now() + days * 86400000).toISOString();
      const response = await fetch("/api/member/household-delegations", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ delegatePersonId, fields, expiresAt }) });
      const data = await response.json();
      if (!response.ok) throw new Error(message(data, "Access could not be shared."));
      const person = candidates.find(candidate => candidate.id === delegatePersonId);
      setItems(current => [{ id: data._id ?? data.id, direction: "granted", personName: person?.name ?? "Household member", fields, expiresAt, createdAt: data.createdAt }, ...current]);
      setNotice(`Access shared with ${person?.name ?? "your household member"}.`);
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Access could not be shared."); }
    finally { setBusy(""); }
  }
  async function revoke(id: string) {
    setBusy(id); setError(""); setNotice("");
    const response = await fetch(`/api/member/household-delegations/${id}`, { method: "DELETE" });
    if (response.ok) { setItems(current => current.filter(item => item.id !== id)); setNotice("Household access revoked immediately."); }
    else { const data = await response.json().catch(() => ({})); setError(message(data, "Access could not be revoked.")); }
    setBusy("");
  }

  return <section className="member-delegation-studio" aria-labelledby="household-access-title">
    <header><span aria-hidden="true">ACCESS LETTER</span><div><h3 id="household-access-title">Share only what they need</h3><p>Give another adult in your household temporary access to selected parts of your record. You remain in control and can revoke it instantly.</p></div></header>
    {notice && <div className="member-delegation-notice" role="status">{notice}</div>}
    {error && <div className="member-delegation-notice error" role="alert">{error}</div>}
    {candidates.length > 0 ? <form onSubmit={create}>
      <label><span>Trusted household member</span><BrandedSelect value={delegatePersonId} onChange={event => setDelegatePersonId(event.target.value)}>{candidates.map(candidate => <option value={candidate.id} key={candidate.id}>{candidate.name}</option>)}</BrandedSelect></label>
      <fieldset><legend>They may view</legend><div className="member-delegation-areas">{areas.map(area => <button type="button" aria-pressed={fields.includes(area.id)} className={fields.includes(area.id) ? "selected" : ""} onClick={() => toggle(area.id)} key={area.id}><i aria-hidden="true">{fields.includes(area.id) ? "✓" : "+"}</i><span>{area.label}</span></button>)}</div></fieldset>
      <label><span>Access expires after</span><BrandedSelect value={days} onChange={event => setDays(Number(event.target.value))}>{EXPIRIES.map(option => <option value={option.days} key={option.days}>{option.label}</option>)}</BrandedSelect></label>
      <button className="member-delegation-create" disabled={busy !== "" || !delegatePersonId || fields.length === 0}>{busy === "create" ? "Creating access…" : "Share selected access"}</button>
    </form> : <div className="member-delegation-empty"><strong>No eligible household account yet</strong><p>Another adult must belong to this household and activate their own My REMI account before you can delegate access.</p></div>}
    <div className="member-delegation-list"><h4>Current access</h4>{items.map(item => <article key={item.id}><div><span>{item.direction === "granted" ? "You shared with" : "Shared with you"}</span><strong>{item.personName}</strong><p>{item.fields.map(field => areas.find(area => area.id === field)?.label ?? field).join(" · ")}</p><small>Expires {new Date(item.expiresAt).toLocaleDateString("en-GH", { day: "numeric", month: "long", year: "numeric" })}</small></div>{item.direction === "granted" && <button disabled={busy !== ""} onClick={() => void revoke(item.id)}>Revoke</button>}</article>)}{items.length === 0 && <p className="member-delegation-none">No active household access arrangements.</p>}</div>
  </section>;
}

function message(data: any, fallback: string) { return typeof data?.error === "string" ? data.error : typeof data?.error?.message === "string" ? data.error.message : fallback; }
