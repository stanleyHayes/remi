"use client";

import { useState, type FormEvent } from "react";
import { apiBaseUrl as API_URL } from "@/lib/env";

export default function NewsletterForm() {
  const [email, setEmail] = useState("");
  const [state, setState] = useState<"idle" | "loading" | "done" | "error">(
    "idle",
  );

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
      <p className="public-newsletter-success">
        You&apos;re on the list. Watch your inbox for a word of encouragement.
      </p>
    );
  }

  return (
    <form onSubmit={onSubmit} className="public-newsletter-form">
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
        className="public-newsletter-input"
      />
      <button
        type="submit"
        disabled={state === "loading"}
        className="public-newsletter-button"
      >
        {state === "loading" ? "Joining…" : "Subscribe"}
      </button>
      {state === "error" ? (
        <p className="public-newsletter-error">
          We couldn&apos;t subscribe you. Please try again.
        </p>
      ) : null}
    </form>
  );
}
