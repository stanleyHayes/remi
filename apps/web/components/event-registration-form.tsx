"use client";

import { useState, type FormEvent } from "react";
import { apiBaseUrl as API_URL } from "@/lib/env";

const inputCls =
  "w-full rounded-2xl border border-cream/15 bg-cream/5 px-5 py-3.5 text-sm text-cream placeholder:text-cream-dim/60 focus:border-gold/50 focus:outline-none transition-colors duration-300";

export default function EventRegistrationForm({ eventId }: { eventId: string }) {
  const [state, setState] = useState<"idle" | "loading" | "done" | "error">("idle");

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState("loading");
    const data = new FormData(e.currentTarget);
    try {
      const res = await fetch(`${API_URL}/api/events/${eventId}/register`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: data.get("name"),
          email: data.get("email"),
          phone: data.get("phone") || undefined,
          website: data.get("website"),
        }),
      });
      setState(res.ok ? "done" : "error");
    } catch {
      setState("error");
    }
  }

  if (state === "done") {
    return (
      <div className="rounded-[calc(2rem-0.375rem)] bg-ink-soft p-8 text-center">
        <div className="mx-auto mb-5 flex h-12 w-12 items-center justify-center rounded-full border border-gold/30 bg-gold/10 text-gold-bright">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
            <path d="M20 6L9 17l-5-5" />
          </svg>
        </div>
        <p className="font-display text-xl text-cream">You&apos;re registered</p>
        <p className="mx-auto mt-2 max-w-xs text-sm leading-relaxed text-cream-dim">
          We&apos;ve saved your seat. A confirmation is on its way to your inbox.
        </p>
      </div>
    );
  }

  return (
    <form onSubmit={onSubmit} className="space-y-4 rounded-[calc(2rem-0.375rem)] bg-ink-soft p-7">
      <input name="name" required autoComplete="name" placeholder="Full name" className={inputCls} aria-label="Full name" />
      <input name="email" type="email" required autoComplete="email" inputMode="email" placeholder="Email address" className={inputCls} aria-label="Email address" />
      <input name="phone" type="tel" autoComplete="tel" inputMode="tel" placeholder="Phone (optional)" className={inputCls} aria-label="Phone" />
      <input type="text" name="website" tabIndex={-1} autoComplete="off" className="hidden" aria-hidden="true" />
      <button
        type="submit"
        disabled={state === "loading"}
        className="w-full rounded-full bg-gold px-6 py-3.5 text-sm font-semibold text-ink transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] hover:bg-gold-bright active:scale-[0.99] disabled:opacity-60"
      >
        {state === "loading" ? "Registering…" : "Reserve My Seat"}
      </button>
      {state === "error" ? (
        <p className="text-center text-xs text-red-400">Registration failed — please try again.</p>
      ) : null}
    </form>
  );
}
