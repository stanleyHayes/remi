"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { OTPInput, REGEXP_ONLY_DIGITS, type SlotProps } from "input-otp";

function CodeSlot({ char, hasFakeCaret, isActive }: SlotProps) {
  return (
    <span
      className={`member-auth-code-slot${isActive ? " is-active" : ""}${char ? " is-filled" : ""}`}
      aria-hidden="true"
    >
      {char ?? ""}
      {hasFakeCaret && <i className="member-auth-code-caret" />}
    </span>
  );
}

export function SignInForm() {
  const router = useRouter();
  const [step, setStep] = useState<"email" | "code" | "mfa">("email");
  const [identifier, setIdentifier] = useState("");
  const [challengeId, setChallengeId] = useState("");
  const [demoCode, setDemoCode] = useState("");
  const [destination, setDestination] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function requestCode(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const response = await fetch("/api/auth/request", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ identifier }),
      });
      const data = await response.json();
      if (!response.ok || !data.challengeId)
        throw new Error(data.error ?? "We could not send your sign-in code.");
      setChallengeId(String(data.challengeId));
      setDemoCode(data.demoCode ?? "");
      setCode(data.demoCode ?? "");
      setStep("code");
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "We could not send your sign-in code.",
      );
    } finally {
      setBusy(false);
    }
  }

  async function verifyCode(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true);
    setError("");
    if (code.length !== 6) {
      setError("Enter all six digits from your sign-in code.");
      setBusy(false);
      return;
    }
    try {
      const response = await fetch(step === "mfa" ? "/api/auth/mfa/verify" : "/api/auth/verify", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          challengeId,
          code,
          ...(step === "code" ? { deviceName: "My REMI web" } : {}),
        }),
      });
      const data = await response.json();
      if (!response.ok)
        throw new Error(data.error ?? "That code is invalid or expired.");
      if (data.mfaRequired) {
        setChallengeId(String(data.challengeId));
        setDestination(data.destination ?? "your second verified contact");
        setDemoCode(data.demoCode ?? "");
        setCode(data.demoCode ?? "");
        setStep("mfa");
        return;
      }
      router.replace("/");
      router.refresh();
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "That code is invalid or expired.",
      );
    } finally {
      setBusy(false);
    }
  }

  if (step === "code" || step === "mfa")
    return (
      <form className="member-auth-form" onSubmit={verifyCode}>
        <div className="member-auth-step">
          <span>{step === "mfa" ? "03" : "02"}</span>
          <p>{step === "mfa" ? "Second verification" : "Check your inbox"}</p>
        </div>
        <h1>
          {step === "mfa" ? "One more" : "Enter your"}
          <br />
          <em>{step === "mfa" ? "security check." : "sign-in code."}</em>
        </h1>
        <p>
          We sent a six-digit code to <strong>{step === "mfa" ? destination : identifier}</strong>. It expires
          in ten minutes.
        </p>
        <label className="member-auth-code-field">
          <span id="member-auth-code-label">Six-digit code</span>
          <OTPInput
            name="code"
            value={code}
            onChange={(value) => {
              setCode(value);
              setError("");
            }}
            maxLength={6}
            inputMode="numeric"
            autoComplete="one-time-code"
            pattern={REGEXP_ONLY_DIGITS}
            aria-labelledby="member-auth-code-label"
            containerClassName="member-auth-code-input"
            required
            autoFocus
            render={({ slots }) => (
              <>
                <div className="member-auth-code-group">
                  {slots.slice(0, 3).map((slot, index) => (
                    <CodeSlot key={index} {...slot} />
                  ))}
                </div>
                <span className="member-auth-code-separator" aria-hidden="true" />
                <div className="member-auth-code-group">
                  {slots.slice(3).map((slot, index) => (
                    <CodeSlot key={index + 3} {...slot} />
                  ))}
                </div>
              </>
            )}
          />
        </label>
        {demoCode && (
          <p className="member-auth-demo">
            Demo mode filled the locally generated code.
          </p>
        )}
        {error && (
          <p className="member-auth-error" role="alert">
            {error}
          </p>
        )}
        <button disabled={busy}>
          {busy ? "Verifying…" : step === "mfa" ? "Verify and open My REMI" : "Continue to My REMI"}
          <span>→</span>
        </button>
        <button
          type="button"
          className="member-auth-back"
          onClick={() => {
            setStep(step === "mfa" ? "code" : "email");
            setCode("");
            setDemoCode("");
            setError("");
          }}
        >
          {step === "mfa" ? "Back to first verification" : "Use a different email or mobile"}
        </button>
      </form>
    );

  return (
    <form className="member-auth-form" onSubmit={requestCode}>
      <div className="member-auth-step">
        <span>01</span>
        <p>Member access</p>
      </div>
      <h1>
        Welcome
        <br />
        <em>back home.</em>
      </h1>
      <p>
        Use the email or Ghana mobile number connected to your REMI profile. No
        password to remember.
      </p>
      <label>
        <span>Email or mobile number</span>
        <input
          type="text"
          autoComplete="username"
          value={identifier}
          onChange={(event) => setIdentifier(event.target.value)}
          placeholder="you@example.com or 024…"
          required
          autoFocus
        />
      </label>
      {error && (
        <p className="member-auth-error" role="alert">
          {error}
        </p>
      )}
      <button disabled={busy}>
        {busy ? "Sending code…" : "Send me a sign-in code"}
        <span>→</span>
      </button>
      <small>
        New to My REMI? Ask a church leader to send your private invitation.
      </small>
    </form>
  );
}

export function InviteForm({ token }: { token: string }) {
  const router = useRouter();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function accept() {
    setBusy(true);
    setError("");
    try {
      const response = await fetch(
        `/api/auth/invite/${encodeURIComponent(token)}/redeem`,
        { method: "POST" },
      );
      const data = await response.json();
      if (!response.ok)
        throw new Error(
          data.error ?? "This invitation is no longer available.",
        );
      router.replace("/");
      router.refresh();
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "This invitation is no longer available.",
      );
      setBusy(false);
    }
  }
  return (
    <div className="member-auth-form">
      <div className="member-auth-step">
        <span>01</span>
        <p>Your invitation</p>
      </div>
      <h1>
        Your place is
        <br />
        <em>ready for you.</em>
      </h1>
      <p>
        Accept this private invitation to activate your My REMI account. You
        will sign in with secure one-time codes—no temporary password required.
      </p>
      {error && (
        <p className="member-auth-error" role="alert">
          {error}
        </p>
      )}
      <button onClick={accept} disabled={busy}>
        {busy ? "Opening your space…" : "Accept invitation"}
        <span>→</span>
      </button>
      <small>
        This link works once and belongs only to the person it was emailed to.
      </small>
    </div>
  );
}
