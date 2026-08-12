"use client";

import { useState } from "react";

export type ConsentItem = { id: string; version: number; purpose: string; channel: string; state: "granted" | "withdrawn"; noticeVersion: string };
export type ConsentSuppression = { id: string; purpose?: string; channel?: string; reason: string; source: string; state: "active" | "released" };
export type ConsentResponse = { items: ConsentItem[]; suppressions: ConsentSuppression[]; noticeVersion: string };

const PURPOSES = [
  { id: "church-updates", label: "Church updates", copy: "News, announcements and ministry updates.", channels: ["email", "sms", "whatsapp", "push"] },
  { id: "event-reminders", label: "Event reminders", copy: "Registration details and schedule changes.", channels: ["email", "sms", "push"] },
  { id: "group-messages", label: "Group messages", copy: "Messages from groups you choose to join.", channels: ["email", "whatsapp", "push"] },
  { id: "serving-reminders", label: "Serving reminders", copy: "Schedule requests, changes and check-in prompts.", channels: ["email", "sms", "push"] },
  { id: "pastoral-care", label: "Pastoral care", copy: "Personal follow-up when you ask for support.", channels: ["email", "sms", "whatsapp", "phone"] },
  { id: "giving-communications", label: "Giving communication", copy: "Statements, pledge updates and generosity news.", channels: ["email", "sms"] },
] as const;

export default function ConsentCentre({ initial }: { initial: ConsentResponse | null }) {
  const [data, setData] = useState<ConsentResponse>(initial ?? { items: [], suppressions: [], noticeVersion: "communications-2026-01" });
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const activeSuppressions = data.suppressions.filter(item => item.state === "active" && item.source !== "member-withdrawal");

  async function toggle(purpose: string, channel: string) {
    const key = `${purpose}:${channel}`;
    const current = data.items.find(item => item.purpose === purpose && item.channel === channel);
    setBusy(key); setError("");
    try {
      const response = await fetch("/api/member/consents", { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ purpose, channel, state: current?.state === "granted" ? "withdrawn" : "granted", expectedVersion: current?.version ?? 0, noticeVersion: data.noticeVersion, evidenceReference: "member-privacy-centre" }) });
      const value = await response.json();
      if (!response.ok) throw new Error(typeof value?.error === "string" ? value.error : value?.error?.message || "That preference could not be saved.");
      setData(previous => ({ ...previous, items: [...previous.items.filter(item => !(item.purpose === purpose && item.channel === channel)), value] }));
    } catch (cause) { setError(cause instanceof Error ? cause.message : "That preference could not be saved."); }
    finally { setBusy(""); }
  }

  return <section className="member-consent-centre" aria-labelledby="member-consent-title">
    <div><h3 id="member-consent-title">Communication choices</h3><p>Choose both what REMI may contact you about and how. Changes take effect immediately.</p></div>
    {activeSuppressions.length > 0 && <div className="member-consent-warning" role="status"><b>Some delivery methods are paused</b><p>{activeSuppressions.map(item => `${item.channel || "All channels"}: ${item.reason}`).join(" · ")}. A new consent choice cannot override a provider, complaint or legal suppression.</p></div>}
    {error && <p className="member-consent-error" role="alert">{error}</p>}
    <div className="member-consent-purposes">{PURPOSES.map(purpose => <article key={purpose.id}><div><strong>{purpose.label}</strong><small>{purpose.copy}</small></div><div>{purpose.channels.map(channel => { const item=data.items.find(value=>value.purpose===purpose.id&&value.channel===channel);const on=item?.state==="granted";const key=`${purpose.id}:${channel}`;return <button type="button" role="switch" aria-checked={on} aria-label={`${purpose.label} by ${channel}`} className={on?"on":""} disabled={busy!==""} onClick={()=>void toggle(purpose.id,channel)} key={channel}><span>{channel}</span><i aria-hidden="true"><em/></i>{busy===key&&<b>Saving</b>}</button>})}</div></article>)}</div>
    <p className="member-consent-notice">Recorded under notice {data.noticeVersion}. Essential security and transaction messages may still be sent when required to deliver a service you requested.</p>
  </section>;
}
