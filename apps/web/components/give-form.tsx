"use client";

import { useState, type FormEvent } from "react";
import { apiBaseUrl as API_URL } from "@/lib/env";
import Select from "@/components/ui/Select";

const inputCls =
  "w-full rounded-2xl border border-cream/15 bg-cream/5 px-5 py-3.5 text-sm text-cream placeholder:text-cream-dim/60 focus:border-gold/50 focus:outline-none transition-colors duration-300";

const PRESETS = [50, 100, 200, 500, 1000];

export default function GiveForm({ categories }: { categories: string[] }) {
  const [amount, setAmount] = useState<string>("100");
  const [state, setState] = useState<"idle" | "loading" | "error">("idle");

  async function onSubmit(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setState("loading");
    const data = new FormData(e.currentTarget);
    const cedis = Number(amount);
    if (!Number.isFinite(cedis) || cedis <= 0) {
      setState("error");
      return;
    }
    try {
      const res = await fetch(`${API_URL}/api/giving/initialize`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          amount: Math.round(cedis * 100), // pesewas
          email: data.get("email"),
          category: data.get("category"),
          website: data.get("website"),
        }),
      });
      if (!res.ok) throw new Error("failed");
      const json = (await res.json()) as { authorizationUrl?: string };
      if (json.authorizationUrl) {
        window.location.href = json.authorizationUrl;
        return;
      }
      setState("error");
    } catch {
      setState("error");
    }
  }

  return (
    <form onSubmit={onSubmit} className="space-y-5 rounded-[calc(2rem-0.375rem)] bg-ink-soft p-7 sm:p-10">
      <div>
        <span className="mb-2 block text-[10px] font-medium uppercase tracking-[0.2em] text-cream-dim">
          Amount (GHS)
        </span>
        <div className="flex flex-wrap gap-2">
          {PRESETS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => setAmount(String(p))}
              className={`rounded-full px-5 py-2.5 text-sm font-medium transition-all duration-300 ${
                amount === String(p)
                  ? "bg-gold text-ink"
                  : "border border-cream/15 text-cream-dim hover:border-gold/40 hover:text-cream"
              }`}
            >
              ₵{p.toLocaleString()}
            </button>
          ))}
        </div>
        <div className="relative mt-3">
          <span className="absolute left-5 top-1/2 -translate-y-1/2 text-sm text-cream-dim">₵</span>
          <input
            name="amount"
            inputMode="decimal"
            value={amount}
            onChange={(e) => setAmount(e.target.value.replace(/[^0-9.]/g, ""))}
            className={`${inputCls} pl-9`}
            placeholder="Custom amount"
            aria-label="Amount in Ghana cedis"
          />
        </div>
      </div>

      <label className="block">
        <span className="mb-2 block text-[10px] font-medium uppercase tracking-[0.2em] text-cream-dim">
          Giving Category
        </span>
        <Select name="category" defaultValue={categories[0] ?? "Offering"} aria-label="Giving Category">
          {categories.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </Select>
      </label>

      <label className="block">
        <span className="mb-2 block text-[10px] font-medium uppercase tracking-[0.2em] text-cream-dim">
          Email
        </span>
        <input name="email" type="email" required className={inputCls} placeholder="you@example.com" />
      </label>

      <input type="text" name="website" tabIndex={-1} autoComplete="off" className="hidden" aria-hidden="true" />

      <button
        type="submit"
        disabled={state === "loading"}
        className="w-full rounded-full bg-gold px-6 py-4 text-sm font-semibold text-ink transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] hover:bg-gold-bright active:scale-[0.99] disabled:opacity-60"
      >
        {state === "loading" ? "Preparing secure checkout…" : "Give Securely"}
      </button>

      {state === "error" ? (
        <p className="text-center text-xs text-red-400">
          We couldn&apos;t start the checkout — please check the amount and try again.
        </p>
      ) : null}

      <p className="flex items-center justify-center gap-2 text-center text-[11px] text-cream-dim/70">
        <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <rect x="4" y="10" width="16" height="10" rx="2" />
          <path d="M8 10V7a4 4 0 0 1 8 0v3" />
        </svg>
        Secure giving via Paystack
      </p>
    </form>
  );
}
