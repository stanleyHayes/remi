"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { api, setSession, ApiError, type AdminUser } from "@/lib/api";
import { useToast } from "@/components/ui/Toast";
import { OtpInput } from "@/components/OtpInput";

export default function LoginPage() {
  const router = useRouter();
  const { showToast } = useToast();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
	const [challengeToken, setChallengeToken] = useState("");
	const [mfaCode, setMfaCode] = useState("");
	const [useRecoveryCode, setUseRecoveryCode] = useState(false);

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
		const res = await api<{ token?: string; user?: AdminUser; mfaRequired?: boolean; challengeToken?: string }>(challengeToken ? "/api/auth/mfa/verify" : "/api/auth/login", {
        method: "POST",
			body: challengeToken ? { challengeToken, code: mfaCode } : { email, password },
      });
		if (res.mfaRequired && res.challengeToken) { setChallengeToken(res.challengeToken); setMfaCode(""); return; }
		if (!res.token || !res.user) throw new ApiError(500,"Sign-in response was incomplete.");
	  setSession(res.token, res.user);
	  showToast(`Welcome back, ${res.user.name || res.user.email}.`);
      router.replace("/");
    } catch (err) {
      const message = err instanceof ApiError ? err.message : "We could not reach the REMI service. Try again shortly.";
      setError(message);
      showToast(message, "error");
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="remi-auth-stage grid min-h-[100dvh] lg:grid-cols-[.9fr_1.1fr]">
      <section className="relative flex min-w-0 items-center justify-center overflow-hidden px-5 py-10 sm:px-8 lg:px-12">
        <div className="remi-auth-orbit remi-auth-orbit-one" aria-hidden="true" />
        <div className="remi-auth-orbit remi-auth-orbit-two" aria-hidden="true" />
        <div className="relative w-full max-w-md">
          <div className="mb-7 flex items-center gap-3">
            <span className="grid size-11 place-items-center rounded-2xl bg-[var(--remi-green)] text-[var(--remi-gold)] shadow-lg"><b>R</b></span>
            <div><strong className="block text-sm font-extrabold tracking-[-.02em] text-[#172019]">REMI Workspace</strong><span className="font-mono text-[9px] font-bold uppercase tracking-[.16em] text-[#8a887f]">Ministry administration</span></div>
          </div>
          <div className="remi-auth-panel">
            <div className="mb-8">
              <span className="mb-5 grid size-12 place-items-center rounded-2xl bg-[rgba(209,173,85,.12)] text-xl text-[#8b681f]">✦</span>
              <p className="font-mono text-[10px] font-bold uppercase tracking-[.18em] text-[#8b681f]">Secure workspace access</p>
              <h1 className="mt-3 text-4xl font-extrabold tracking-[-.045em] text-[#172019]">Welcome back.</h1>
              <p className="mt-2 text-sm leading-6 text-[#73756f]">Sign in to continue managing REMI&apos;s digital ministry.</p>
            </div>
			<form onSubmit={onSubmit} className="space-y-5">
          {error && <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3.5 py-3 text-sm font-medium text-rose-700">{error}</p>}
			{challengeToken ? <div className="block text-sm font-semibold text-[#30362f]"><span>{useRecoveryCode ? "Recovery code" : "Six-digit authenticator code"}</span>{useRecoveryCode ? <input required autoFocus autoComplete="one-time-code" value={mfaCode} onChange={(e)=>setMfaCode(e.target.value.trim())} className="mt-2 w-full rounded-xl border border-[#d7d2c6] !bg-[#fffef9] px-4 py-3 font-mono text-sm !text-[#172019] outline-none" placeholder="Enter a recovery code" /> : <div className="mt-3"><OtpInput value={mfaCode} onChange={setMfaCode} autoFocus disabled={loading} label="Authenticator code" /></div>}<button type="button" className="mt-3 text-xs font-bold text-[#8b681f] underline-offset-4 hover:underline" onClick={()=>{setUseRecoveryCode(value=>!value);setMfaCode("");}}>{useRecoveryCode ? "Use authenticator code" : "Use a recovery code instead"}</button></div> : <><label className="block text-sm font-semibold text-[#30362f]">
            Email
            <input
              type="email"
              required
              autoComplete="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="mt-2 w-full rounded-xl border border-[#d7d2c6] !bg-[#fffef9] px-4 py-3 text-sm !text-[#172019] outline-none"
              placeholder="admin@remi.church"
            />
          </label>
		  <label className="block text-sm font-semibold text-[#30362f]">
            <span className="flex items-center justify-between"><span>Password</span><span className="text-xs font-medium text-[#8b681f]">Protected access</span></span>
            <span className="relative mt-2 block">
            <input
              type={showPassword ? "text" : "password"}
              required
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="w-full rounded-xl border border-[#d7d2c6] !bg-[#fffef9] px-4 py-3 pr-16 text-sm !text-[#172019] outline-none"
              placeholder="••••••••••••"
            />
            <button type="button" onClick={() => setShowPassword((value) => !value)} className="absolute inset-y-0 right-3 text-xs font-bold text-[#777a74]">{showPassword ? "Hide" : "Show"}</button>
            </span>
          </label></>}

          <button
            type="submit"
            disabled={loading}
            className="mt-1 flex min-h-13 w-full items-center justify-between rounded-xl bg-[var(--remi-green)] px-5 py-3.5 text-sm font-extrabold text-white shadow-[0_12px_28px_rgba(23,32,25,.18)] transition hover:-translate-y-0.5 hover:bg-[#243027] disabled:opacity-60"
          >
			{loading ? "Verifying…" : <><span>{challengeToken ? "Verify and continue" : "Continue to dashboard"}</span><span className="text-[var(--remi-gold)]">→</span></>}
          </button>
        </form>
            <div className="mt-6 border-t border-[#e5e0d5] pt-5 text-xs leading-5 text-[#8a887f]"><span className="font-semibold text-[#555a54]">Demo access</span><br /><span className="font-mono">admin@remi.church · remi-admin-2026</span></div>
          </div>
        </div>
      </section>
      <aside className="remi-auth-story relative hidden min-h-[100dvh] overflow-hidden text-white lg:flex lg:items-center">
        <div className="absolute inset-0 opacity-80 [background-image:radial-gradient(circle_at_20%_15%,rgba(209,173,85,.16),transparent_24rem),radial-gradient(circle_at_90%_85%,rgba(134,173,123,.12),transparent_26rem)]" />
        <span className="absolute -right-24 top-10 select-none font-mono text-[24rem] font-black leading-none text-white/[.025]">R</span>
        <div className="relative mx-auto w-full max-w-2xl px-12 py-16 xl:px-16">
          <div className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/[.055] px-4 py-2 font-mono text-[10px] font-bold uppercase tracking-[.2em] text-white/55"><span className="text-[var(--remi-gold)]">✦</span> Your ministry command centre</div>
          <h2 className="mt-8 max-w-xl text-5xl font-extrabold leading-[.95] tracking-[-.05em] xl:text-6xl">Serve with clarity.<br /><span className="text-[var(--remi-gold)]">Lead with care.</span></h2>
          <p className="mt-6 max-w-lg text-base leading-7 text-white/50">One calm workspace for the messages, people and moments that shape REMI&apos;s digital ministry.</p>
          <div className="mt-10 space-y-3">
            <Journey index="01" icon="✓" title="Review with confidence" copy="Approve every message before it reaches the congregation." />
            <Journey index="02" icon="↗" title="Respond with care" copy="Keep prayer, visits and community messages close." />
            <Journey index="03" icon="◇" title="Publish from one place" copy="Manage events, sermons, branches and media together." />
          </div>
          <div className="mt-10 flex items-center gap-3 border-t border-white/10 pt-6 text-xs text-white/35"><span className="text-[#9fc493]">●</span> Secure identity and role-aware access</div>
        </div>
      </aside>
    </main>
  );
}

function Journey({ index, icon, title, copy }: { index: string; icon: string; title: string; copy: string }) {
  return <div className="group flex items-center gap-4 rounded-3xl border border-white/10 bg-white/[.05] p-4 backdrop-blur-md transition hover:bg-white/[.08]"><span className="grid size-12 shrink-0 place-items-center rounded-2xl border border-white/10 bg-black/20 text-[var(--remi-gold)] shadow-[6px_6px_16px_rgba(0,0,0,.3)]">{icon}</span><div className="min-w-0 flex-1"><p className="text-sm font-extrabold">{title}</p><p className="mt-1 text-xs leading-5 text-white/40">{copy}</p></div><span className="font-mono text-[10px] text-white/20">{index}</span></div>;
}
