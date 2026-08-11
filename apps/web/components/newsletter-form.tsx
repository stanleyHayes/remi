"use client";

import { useState, type FormEvent } from "react";
import { apiBaseUrl as API_URL } from "@/lib/env";

export default function NewsletterForm() {
  const [email, setEmail] = useState("");
  const [state, setState] = useState<"idle" | "loading" | "done" | "error">("idle");

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!email) return;
    setState("loading");
    try {
      const res = await fetch(`${API_URL}/api/subscribe`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email }),
      });
      setState(res.ok ? "done" : "error");
    } catch {
      setState("error");
    }
  }

  if (state === "done") {
    return (
      <p className="rounded-full border border-gold/30 bg-gold/10 px-5 py-3 text-sm text-gold-bright">
        You&apos;re on the list. Watch your inbox for a word of encouragement.
      </p>
    );
  }

  return (
    <form onSubmit={onSubmit} className="flex w-full max-w-sm flex-col gap-3 sm:flex-row sm:gap-2">
      <label htmlFor="newsletter-email" className="sr-only">
        Email address
      </label>
      <input
        id="newsletter-email"
        type="email"
        required
        value={email}
        onChange={(e) => setEmail(e.target.value)}
        placeholder="Your email address"
        className="w-full rounded-full border border-cream/15 bg-cream/5 px-5 py-3 text-sm text-cream placeholder:text-cream-dim/60 focus:border-gold/50 focus:outline-none"
      />
      <button
        type="submit"
        disabled={state === "loading"}
        className="shrink-0 rounded-full bg-gold px-6 py-3 text-sm font-semibold text-ink transition-colors duration-300 hover:bg-gold-bright disabled:opacity-60"
      >
        {state === "loading" ? "Joining…" : "Subscribe"}
      </button>
      {state === "error" ? (
        <p className="text-xs text-red-400">Something went wrong — please try again.</p>
      ) : null}
    </form>
  );
}
