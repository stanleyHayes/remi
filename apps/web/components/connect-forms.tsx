"use client";

import { useState, type FormEvent, type ReactNode } from "react";
import { apiBaseUrl as API_URL } from "@/lib/env";
import Select from "@/components/ui/Select";
import DatePicker from "@/components/ui/DatePicker";

const inputCls =
  "w-full rounded-2xl border border-cream/15 bg-cream/5 px-5 py-3.5 text-sm text-cream placeholder:text-cream-dim/60 focus:border-gold/50 focus:outline-none transition-colors duration-300";

type SubmitState = "idle" | "loading" | "done" | "error";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-2 block text-[10px] font-medium uppercase tracking-[0.2em] text-cream-dim">{label}</span>
      {children}
    </label>
  );
}

function FormFrame({
  children,
  onSubmit,
  state,
  submitLabel,
  successTitle,
  successCopy,
}: {
  children: ReactNode;
  onSubmit: (e: FormEvent<HTMLFormElement>) => void;
  state: SubmitState;
  submitLabel: string;
  successTitle: string;
  successCopy: string;
}) {
  if (state === "done") {
    return (
      <div className="rounded-[calc(2rem-0.375rem)] bg-ink-soft p-10 text-center sm:p-14">
        <div className="mx-auto mb-6 flex h-14 w-14 items-center justify-center rounded-full border border-gold/30 bg-gold/10 text-gold-bright">
          <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <path d="M20 6L9 17l-5-5" />
          </svg>
        </div>
        <h3 className="font-display text-2xl font-medium text-cream">{successTitle}</h3>
        <p className="mx-auto mt-3 max-w-sm text-sm leading-relaxed text-cream-dim">{successCopy}</p>
      </div>
    );
  }

  return (
    <form onSubmit={onSubmit} className="space-y-5 rounded-[calc(2rem-0.375rem)] bg-ink-soft p-7 sm:p-10">
      {children}
      {/* Honeypot — hidden from humans, irresistible to bots */}
      <input type="text" name="website" tabIndex={-1} autoComplete="off" className="hidden" aria-hidden="true" />
      <button
        type="submit"
        disabled={state === "loading"}
        className="w-full rounded-full bg-gold px-6 py-4 text-sm font-semibold text-ink transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] hover:bg-gold-bright active:scale-[0.99] disabled:opacity-60"
      >
        {state === "loading" ? "Sending…" : submitLabel}
      </button>
      {state === "error" ? (
        <p className="text-center text-xs text-red-400">
          We couldn&apos;t send that just now. Please try again in a moment.
        </p>
      ) : null}
    </form>
  );
}

async function post(path: string, payload: Record<string, unknown>): Promise<boolean> {
  try {
    const res = await fetch(`${API_URL}${path}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    return res.ok;
  } catch {
    return false;
  }
}

function readForm(e: FormEvent<HTMLFormElement>): Record<string, string> {
  const data = new FormData(e.currentTarget);
  const out: Record<string, string> = {};
  data.forEach((v, k) => {
    out[k] = String(v);
  });
  return out;
}

export function PrayerForm() {
  const [state, setState] = useState<SubmitState>("idle");
  const [identityMode, setIdentityMode] = useState<"identified" | "anonymous">("identified");
  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState("loading");
    const f = readForm(e);
    const ok = await post("/api/forms/prayer", {
      name: f.name || undefined,
      email: f.email || undefined,
      request: f.request,
      identityMode,
      visibility: f.visibility,
      linkToProfile: f.linkToProfile === "on",
      linkConsent: f.linkToProfile === "on",
      website: f.website,
    });
    setState(ok ? "done" : "error");
  }
  return (
    <FormFrame
      onSubmit={onSubmit}
      state={state}
      submitLabel="Send Prayer Request"
      successTitle="We've received your request"
      successCopy="Our intercessory team will stand with you in prayer. You are not alone — God hears."
    >
      <fieldset>
        <legend className="mb-2 text-[10px] font-medium uppercase tracking-[0.2em] text-cream-dim">How should we receive this?</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          {([['identified','With my details','The team may contact me.'],['anonymous','Anonymously','No name or email will be stored.']] as const).map(([value,label,copy])=><label className={`cursor-pointer rounded-2xl border p-4 transition-colors ${identityMode===value?'border-gold/40 bg-gold/10':'border-cream/10 bg-cream/[0.025]'}`} key={value}><input className="sr-only" type="radio" name="identityMode" value={value} checked={identityMode===value} onChange={()=>setIdentityMode(value)}/><span className="block text-sm font-medium text-cream">{label}</span><small className="mt-1 block text-xs text-cream-dim">{copy}</small></label>)}
        </div>
      </fieldset>
      {identityMode === "identified" ? <div className="grid gap-5 sm:grid-cols-2">
        <Field label="Name (optional)"><input name="name" autoComplete="name" className={inputCls} placeholder="Your name" /></Field>
        <Field label="Email (optional)"><input name="email" type="email" autoComplete="email" inputMode="email" className={inputCls} placeholder="you@example.com" /></Field>
      </div> : null}
      <Field label="Prayer Request">
        <textarea name="request" required rows={5} className={inputCls} placeholder="What would you like us to pray about?" />
      </Field>
      <Field label="Who may see this request?">
        <Select name="visibility" defaultValue="pastors-only" className={inputCls}>
          <option value="pastors-only">Pastors only</option>
          <option value="prayer-team">Approved prayer team</option>
        </Select>
      </Field>
      {identityMode === "identified" ? <label className="flex items-start gap-3 rounded-2xl border border-cream/10 bg-cream/[0.025] p-4 text-sm text-cream-dim">
        <input type="checkbox" name="linkToProfile" className="mt-0.5 h-4 w-4 accent-[#c9a227]" />
        <span><b className="block font-medium text-cream">Request profile linkage</b><small className="mt-1 block leading-relaxed">I consent to REMI verifying my identity and linking this request to my member profile under prayer notice prayer-link-2026-01. Public requests are never linked automatically.</small></span>
      </label> : null}
    </FormFrame>
  );
}

export function ContactForm() {
  const [state, setState] = useState<SubmitState>("idle");
  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState("loading");
    const f = readForm(e);
    const ok = await post("/api/forms/contact", {
      name: f.name,
      email: f.email,
      phone: f.phone || undefined,
      subject: f.subject,
      message: f.message,
      website: f.website,
    });
    setState(ok ? "done" : "error");
  }
  return (
    <FormFrame
      onSubmit={onSubmit}
      state={state}
      submitLabel="Send Message"
      successTitle="Message received"
      successCopy="Thank you for reaching out. Someone from our team will get back to you shortly."
    >
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
        <Field label="Name">
          <input name="name" required autoComplete="name" className={inputCls} placeholder="Your name" />
        </Field>
        <Field label="Email">
          <input name="email" type="email" required autoComplete="email" inputMode="email" className={inputCls} placeholder="you@example.com" />
        </Field>
      </div>
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
        <Field label="Phone (optional)">
          <input name="phone" type="tel" autoComplete="tel" inputMode="tel" className={inputCls} placeholder="+233 …" />
        </Field>
        <Field label="Subject">
          <input name="subject" required className={inputCls} placeholder="How can we help?" />
        </Field>
      </div>
      <Field label="Message">
        <textarea name="message" required rows={5} className={inputCls} placeholder="Write your message…" />
      </Field>
    </FormFrame>
  );
}

export function VisitForm({ branches }: { branches: string[] }) {
  const [state, setState] = useState<SubmitState>("idle");
  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState("loading");
    const f = readForm(e);
    const ok = await post("/api/forms/visit", {
      name: f.name,
      email: f.email,
      phone: f.phone || undefined,
      visitDate: f.visitDate,
      branch: f.branch || undefined,
      notes: f.notes || undefined,
      website: f.website,
    });
    setState(ok ? "done" : "error");
  }
  return (
    <FormFrame
      onSubmit={onSubmit}
      state={state}
      submitLabel="Plan My Visit"
      successTitle="We can't wait to meet you"
      successCopy="Your visit is noted — look out for a welcome message from us. Come as you are."
    >
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
        <Field label="Name">
          <input name="name" required autoComplete="name" className={inputCls} placeholder="Your name" />
        </Field>
        <Field label="Email">
          <input name="email" type="email" required autoComplete="email" inputMode="email" className={inputCls} placeholder="you@example.com" />
        </Field>
      </div>
      <div className="grid grid-cols-1 gap-5 sm:grid-cols-2">
        <Field label="Phone (optional)">
          <input name="phone" type="tel" autoComplete="tel" inputMode="tel" className={inputCls} placeholder="+233 …" />
        </Field>
        <Field label="When will you visit?">
          <DatePicker name="visitDate" required aria-label="When will you visit?" />
        </Field>
      </div>
      <Field label="Branch">
        <Select name="branch" defaultValue="" aria-label="Branch">
          <option value="">No preference</option>
          {branches.map((b) => (
            <option key={b} value={b}>
              {b}
            </option>
          ))}
        </Select>
      </Field>
      <Field label="Anything we should know? (optional)">
        <textarea name="notes" rows={4} className={inputCls} placeholder="Coming with family? Need directions?" />
      </Field>
    </FormFrame>
  );
}

export function TestimonyForm() {
  const [state, setState] = useState<SubmitState>("idle");
  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState("loading");
    const f = readForm(e);
    const ok = await post("/api/forms/testimony", {
      author: f.author,
      body: f.body,
      website: f.website,
    });
    setState(ok ? "done" : "error");
  }
  return (
    <FormFrame
      onSubmit={onSubmit}
      state={state}
      submitLabel="Share Testimony"
      successTitle="Thank you for sharing"
      successCopy="Your testimony glorifies God and strengthens others. It may be shared (with care) to encourage the church."
    >
      <Field label="Your Name">
        <input name="author" required autoComplete="name" className={inputCls} placeholder="Your name" />
      </Field>
      <Field label="Your Testimony">
        <textarea
          name="body"
          required
          rows={6}
          className={inputCls}
          placeholder="What has God done for you?"
        />
      </Field>
    </FormFrame>
  );
}
