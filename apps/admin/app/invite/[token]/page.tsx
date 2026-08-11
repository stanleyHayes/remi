"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { api, ApiError, setSession, type AdminUser } from "@/lib/api";
import { useToast } from "@/components/ui/Toast";

interface Invitation {
  email: string;
  role: string;
  expiresAt: string;
}

export default function InvitationPage() {
  const { token } = useParams<{ token: string }>();
  const router = useRouter();
  const { showToast } = useToast();
  const [invitation, setInvitation] = useState<Invitation | null>(null);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);

  useEffect(() => {
    if (!token) return;
    api<Invitation>(`/api/auth/invitations/${encodeURIComponent(token)}`)
      .then(setInvitation)
      .catch((err) => setError(err instanceof ApiError ? err.message : "We could not verify this invitation."))
      .finally(() => setLoading(false));
  }, [token]);

  async function acceptInvitation(event: React.FormEvent) {
    event.preventDefault();
    setError("");
    if (password !== confirmPassword) {
      setError("The passwords do not match.");
      return;
    }
    if (password.length < 12) {
      setError("Use at least 12 characters for your password.");
      return;
    }
    setSubmitting(true);
    try {
      const result = await api<{ token: string; user: AdminUser }>(`/api/auth/invitations/${encodeURIComponent(token)}/accept`, {
        method: "POST",
        body: { name, password },
      });
      setSession(result.token, result.user);
      showToast(`Welcome to REMI, ${result.user.name}.`);
      router.replace("/");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "We could not activate your account.");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <main className="remi-auth-stage grid min-h-[100dvh] lg:grid-cols-[1.05fr_.95fr]">
      <section className="relative flex min-w-0 items-center justify-center overflow-hidden px-5 py-10 sm:px-8 lg:px-12">
        <div className="remi-auth-orbit remi-auth-orbit-one" aria-hidden="true" />
        <div className="remi-auth-orbit remi-auth-orbit-two" aria-hidden="true" />
        <div className="relative w-full max-w-lg">
          <div className="mb-7 flex items-center gap-3">
            <span className="grid size-11 place-items-center rounded-2xl bg-[var(--remi-green)] text-[var(--remi-gold)] shadow-lg"><b>R</b></span>
            <div><strong className="block text-sm font-extrabold tracking-[-.02em] text-[#172019]">REMI Workspace</strong><span className="font-mono text-[9px] font-bold uppercase tracking-[.16em] text-[#8a887f]">Private team invitation</span></div>
          </div>
          <div className="remi-auth-panel">
            {loading ? (
              <div className="animate-pulse" aria-label="Verifying invitation"><div className="h-12 w-12 rounded-2xl bg-zinc-200" /><div className="mt-6 h-4 w-32 rounded bg-zinc-200" /><div className="mt-4 h-10 w-3/4 rounded bg-zinc-200" /><div className="mt-8 h-12 rounded-xl bg-zinc-200" /><div className="mt-4 h-12 rounded-xl bg-zinc-200" /></div>
            ) : !invitation ? (
              <div className="py-4">
                <span className="grid size-12 place-items-center rounded-2xl bg-rose-50 text-xl text-rose-700">×</span>
                <p className="mt-6 font-mono text-[10px] font-bold uppercase tracking-[.18em] text-rose-700">Invitation unavailable</p>
                <h1 className="mt-3 text-4xl font-extrabold tracking-[-.045em] text-[#172019]">This link can&apos;t be used.</h1>
                <p role="alert" className="mt-3 text-sm leading-6 text-[#73756f]">{error || "It may have expired or already been accepted. Ask a super-admin to send a new invitation."}</p>
                <Link href="/login" className="mt-7 inline-flex rounded-xl bg-[var(--remi-green)] px-5 py-3 text-sm font-bold text-white">Return to sign in</Link>
              </div>
            ) : (
              <>
                <div className="mb-7">
                  <span className="grid size-12 place-items-center rounded-2xl bg-[rgba(209,173,85,.12)] text-xl text-[#8b681f]">✦</span>
                  <p className="mt-5 font-mono text-[10px] font-bold uppercase tracking-[.18em] text-[#8b681f]">You&apos;re invited</p>
                  <h1 className="mt-3 text-4xl font-extrabold tracking-[-.045em] text-[#172019]">Create your account.</h1>
                  <p className="mt-2 text-sm leading-6 text-[#73756f]">Joining as <strong className="text-[#30362f]">{invitation.email}</strong> with <strong className="capitalize text-[#30362f]">{invitation.role}</strong> access.</p>
                </div>
                <form onSubmit={acceptInvitation} className="space-y-5" aria-busy={submitting}>
                  {error && <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3.5 py-3 text-sm font-medium text-rose-700">{error}</p>}
                  <label className="block text-sm font-semibold text-[#30362f]">Your name<input required autoComplete="name" value={name} onChange={(event) => setName(event.target.value)} className="mt-2 w-full rounded-xl border border-[#d7d2c6] !bg-[#fffef9] px-4 py-3 text-sm !text-[#172019] outline-none" placeholder="How your team will see you" /></label>
                  <label className="block text-sm font-semibold text-[#30362f]">Create password<span className="relative mt-2 block"><input required minLength={12} type={showPassword ? "text" : "password"} autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} className="w-full rounded-xl border border-[#d7d2c6] !bg-[#fffef9] px-4 py-3 pr-16 text-sm !text-[#172019] outline-none" placeholder="At least 12 characters" /><button type="button" onClick={() => setShowPassword((value) => !value)} className="absolute inset-y-0 right-3 text-xs font-bold text-[#777a74]">{showPassword ? "Hide" : "Show"}</button></span></label>
                  <label className="block text-sm font-semibold text-[#30362f]">Confirm password<input required minLength={12} type={showPassword ? "text" : "password"} autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} className="mt-2 w-full rounded-xl border border-[#d7d2c6] !bg-[#fffef9] px-4 py-3 text-sm !text-[#172019] outline-none" /></label>
                  <button type="submit" disabled={submitting} className="flex min-h-13 w-full items-center justify-between rounded-xl bg-[var(--remi-green)] px-5 py-3.5 text-sm font-extrabold text-white shadow-[0_12px_28px_rgba(23,32,25,.18)] transition hover:-translate-y-0.5 disabled:opacity-60"><span>{submitting ? "Creating your account…" : "Accept invitation"}</span><span className="text-[var(--remi-gold)]">→</span></button>
                </form>
              </>
            )}
          </div>
        </div>
      </section>
      <aside className="remi-auth-story relative hidden min-h-[100dvh] overflow-hidden p-14 text-white lg:flex lg:items-end">
        <div className="absolute inset-0 opacity-80 [background-image:radial-gradient(circle_at_70%_20%,rgba(209,173,85,.18),transparent_22rem),radial-gradient(circle_at_15%_85%,rgba(134,173,123,.13),transparent_25rem)]" />
        <span className="absolute -right-20 top-0 select-none font-mono text-[25rem] font-black leading-none text-white/[.025]">R</span>
        <div className="relative max-w-lg"><p className="font-mono text-[10px] font-bold uppercase tracking-[.2em] text-[var(--remi-gold)]">A considered beginning</p><h2 className="mt-5 text-5xl font-extrabold leading-[.96] tracking-[-.05em]">Your identity.<br />Your secure access.</h2><p className="mt-5 max-w-md text-sm leading-7 text-white/50">No shared passwords and no administrator-created credentials. This one-time invitation belongs only to you.</p><div className="mt-9 border-t border-white/10 pt-6 font-mono text-[10px] uppercase tracking-[.16em] text-white/35">Encrypted · Role-aware · Single-use</div></div>
      </aside>
    </main>
  );
}
