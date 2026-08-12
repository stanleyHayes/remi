"use client";

import { useRef, useState, type FormEvent } from "react";
import Select from "@/components/ui/Select";
import { apiBaseUrl as API_URL } from "@/lib/env";

const PRESETS = [50, 100, 200, 500, 1000];

export default function GiveForm({ categories, campaign }: { categories: string[]; campaign?: { id: string; title: string } }) {
  const [amount, setAmount] = useState("100");
  const [state, setState] = useState<"idle" | "loading" | "error">("idle");
  const lastRequest = useRef<{ payload: string; key: string } | null>(null);

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
      const payload = JSON.stringify({
        amount: Math.round(cedis * 100),
        email: data.get("email"),
        category: data.get("category"),
        campaignId: campaign?.id,
        website: data.get("website"),
      });
      if (lastRequest.current?.payload !== payload) {
        lastRequest.current = { payload, key: crypto.randomUUID() };
      }
      const res = await fetch(`${API_URL}/api/payments/paystack/intents`, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          "Idempotency-Key": lastRequest.current.key,
        },
        body: payload,
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
    <form onSubmit={onSubmit} className="give-form">
      <div className="give-form-heading">
        <p>Secure online giving</p>
        <span>GHS</span>
      </div>

      <fieldset>
        <legend>Choose an amount</legend>
        <div className="give-amount-presets">
          {PRESETS.map((preset) => (
            <button
              key={preset}
              type="button"
              onClick={() => {
                setAmount(String(preset));
                setState("idle");
              }}
              aria-pressed={amount === String(preset)}
            >
              <span>₵</span>
              {preset.toLocaleString()}
            </button>
          ))}
        </div>
        <label className="give-custom-amount">
          <span>Or enter another amount</span>
          <div>
            <b>₵</b>
            <input
              name="amount"
              inputMode="decimal"
              value={amount}
              onChange={(e) => {
                setAmount(e.target.value.replace(/[^0-9.]/g, ""));
                setState("idle");
              }}
              placeholder="0.00"
              aria-label="Custom amount in Ghana cedis"
            />
          </div>
        </label>
      </fieldset>

      <div className="give-form-fields">
        <label>
          <span>Direct my gift to</span>
          {campaign ? (
            <input name="category" value={campaign.title} readOnly aria-label="Fundraising campaign" />
          ) : (
            <Select name="category" defaultValue={categories[0] ?? "Offering"} aria-label="Giving category">
              {categories.map((category) => <option key={category} value={category}>{category}</option>)}
            </Select>
          )}
        </label>
        <label>
          <span>Receipt email</span>
          <input
            name="email"
            type="email"
            required
            autoComplete="email"
            placeholder="you@example.com"
          />
        </label>
      </div>

      <input
        type="text"
        name="website"
        tabIndex={-1}
        autoComplete="off"
        className="hidden"
        aria-hidden="true"
      />

      <button
        type="submit"
        disabled={state === "loading"}
        className="give-submit"
      >
        <span>
          {state === "loading"
            ? "Preparing checkout…"
            : "Continue to secure checkout"}
        </span>
        <svg viewBox="0 0 20 20" fill="none" aria-hidden>
          <path d="M4 10h11M11 5l5 5-5 5" />
        </svg>
      </button>

      {state === "error" && (
        <p className="give-form-error" role="alert">
          We couldn’t start checkout. Check your amount and try again.
        </p>
      )}

      <div className="give-form-trust">
        <span>
          <svg viewBox="0 0 24 24" fill="none" aria-hidden>
            <rect x="4" y="10" width="16" height="10" rx="2" />
            <path d="M8 10V7a4 4 0 0 1 8 0v3" />
          </svg>
          Encrypted checkout
        </span>
        <span>Powered by Paystack</span>
      </div>
    </form>
  );
}
