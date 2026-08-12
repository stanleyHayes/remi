"use client";

import { useState, type FormEvent } from "react";
import { OTPInput, REGEXP_ONLY_DIGITS, type SlotProps } from "input-otp";
import { BrandedSelect } from "@/components/branded-select";

export type MemberMFAStatus = { enabled: boolean; emailAvailable: boolean; smsAvailable: boolean; email: string; sms: string; enabledAt?: string };

function Slot({ char, hasFakeCaret, isActive }: SlotProps) { return <span className={`member-security-code-slot${isActive ? " active" : ""}${char ? " filled" : ""}`} aria-hidden="true">{char ?? ""}{hasFakeCaret && <i />}</span>; }

export function MemberMFASettings({ initial }: { initial: MemberMFAStatus | null }) {
  const [status, setStatus] = useState(initial);
  const [channel, setChannel] = useState(initial?.smsAvailable ? "sms" : "email");
  const [action, setAction] = useState<"enable" | "disable" | "">("");
  const [challengeId, setChallengeId] = useState("");
  const [destination, setDestination] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");

  async function begin(next: "enable" | "disable") {
    setBusy(true); setError(""); setNotice("");
    const path = next === "enable" ? "setup" : "disable-request";
    const response = await fetch(`/api/member/mfa/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ channel }) });
    const data = await response.json();
    if (!response.ok) setError(message(data, "A security code could not be sent."));
    else { setAction(next); setChallengeId(String(data.challengeId)); setDestination(data.destination); setCode(data.demoCode ?? ""); }
    setBusy(false);
  }
  async function confirm(event: FormEvent) {
    event.preventDefault(); if (code.length !== 6) { setError("Enter all six digits."); return; }
    setBusy(true); setError("");
    const response = await fetch(action === "enable" ? "/api/member/mfa/confirm" : "/api/member/mfa", { method: action === "enable" ? "POST" : "DELETE", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ challengeId, code }) });
    const data = response.status === 204 ? {} : await response.json();
    if (!response.ok) setError(message(data, "The security code is invalid or expired."));
    else { const enabled = action === "enable"; setStatus(current => current ? { ...current, enabled, enabledAt: enabled ? new Date().toISOString() : undefined } : current); setAction(""); setCode(""); setNotice(enabled ? "Two-step verification is now protecting your account." : "Two-step verification has been turned off."); }
    setBusy(false);
  }

  if (!status) return <section className="member-security-mfa"><div className="member-security-mfa-unavailable"><strong>Security status unavailable</strong><p>Refresh this page to check two-step verification.</p></div></section>;
  return <section className="member-security-mfa" aria-labelledby="member-mfa-title">
    <div className="member-security-mfa-mark" aria-hidden="true"><span>{status.enabled ? "02" : "01"}</span><i /><i /></div>
    <div className="member-security-mfa-copy"><p>ACCOUNT SHIELD</p><h3 id="member-mfa-title">Two-step verification</h3><span>{status.enabled ? "Active protection" : "Optional protection"}</span><p>{status.enabled ? "Every new sign-in must prove both verified contact methods before a session can begin." : "Add a second one-time check when you sign in from a new browser. You need both a verified email and mobile number."}</p></div>
    <aside><b className={status.enabled ? "on" : ""}><i />{status.enabled ? "ON" : "OFF"}</b></aside>
    {notice && <div className="member-security-message" role="status">{notice}</div>}
    {error && <div className="member-security-message error" role="alert">{error}</div>}
    {action ? <form onSubmit={confirm} className="member-security-confirm"><div><strong>{action === "enable" ? "Confirm protection" : "Confirm turn off"}</strong><p>Enter the code sent to {destination}. It expires in ten minutes.</p></div><label><span id="member-security-code-label">Six-digit security code</span><OTPInput value={code} onChange={setCode} maxLength={6} inputMode="numeric" autoComplete="one-time-code" pattern={REGEXP_ONLY_DIGITS} aria-labelledby="member-security-code-label" containerClassName="member-security-code" render={({ slots }) => slots.map((slot, index) => <Slot key={index} {...slot} />)} /></label><div><button disabled={busy}>{busy ? "Verifying…" : action === "enable" ? "Enable protection" : "Turn off protection"}</button><button type="button" onClick={() => { setAction(""); setCode(""); setError(""); }}>Cancel</button></div></form> : <div className="member-security-mfa-actions"><label><span>Send fresh proof by</span><BrandedSelect value={channel} onChange={event => setChannel(event.target.value)}>{status.smsAvailable && <option value="sms">SMS · {status.sms}</option>}{status.emailAvailable && <option value="email">Email · {status.email}</option>}</BrandedSelect></label><button disabled={busy || !status.emailAvailable || !status.smsAvailable} onClick={() => void begin(status.enabled ? "disable" : "enable")}>{busy ? "Sending code…" : status.enabled ? "Verify to turn off" : "Protect my account"}</button></div>}
  </section>;
}

function message(data: any, fallback: string) { return typeof data?.error === "string" ? data.error : typeof data?.error?.message === "string" ? data.error.message : fallback; }
