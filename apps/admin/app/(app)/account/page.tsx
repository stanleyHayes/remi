"use client";

import { useEffect, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { api, ApiError, getStoredUser, setSession, getToken, type AdminPreferences, type AdminUser } from "@/lib/api";
import { Card, Loading, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/Toast";
import { OtpInput } from "@/components/OtpInput";

type Section = "profile" | "security" | "preferences";
const SECTIONS: { id: Section; index: string; label: string; copy: string; icon: string }[] = [
  { id: "profile", index: "01", label: "Profile", copy: "Identity and contact", icon: "◎" },
  { id: "security", index: "02", label: "Security", copy: "Password and MFA", icon: "⌁" },
  { id: "preferences", index: "03", label: "Preferences", copy: "Workspace behavior", icon: "◫" },
];
const TIMEZONES = ["Africa/Accra", "Africa/Lagos", "Africa/Nairobi", "Europe/London", "America/New_York", "America/Chicago", "America/Los_Angeles"];
const field = "mt-2 w-full rounded-xl border border-[#d7d2c6] bg-white px-4 py-3 text-sm text-[#172019] outline-none transition focus:border-[#8b681f] focus:ring-4 focus:ring-[#d1ad55]/10";

export default function AccountPage() {
  const router = useRouter();
  const search = useSearchParams();
  const { showToast } = useToast();
  const requested = search.get("section");
  const [section, setSection] = useState<Section>(requested === "security" || requested === "preferences" ? requested : "profile");
  const [user, setUser] = useState<AdminUser | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [passwords, setPasswords] = useState({ currentPassword: "", newPassword: "", confirm: "" });
  const [mfa, setMfa] = useState<{ secret: string; otpauthUrl: string } | null>(null);
  const [mfaCode, setMfaCode] = useState("");
  const [recovery, setRecovery] = useState<string[]>([]);
  const [disablePassword, setDisablePassword] = useState("");

  useEffect(() => { api<{ user: AdminUser }>("/api/auth/me").then(x => setUser(x.user)).catch(() => setUser(getStoredUser())); }, []);
  useEffect(() => { if (requested === "profile" || requested === "security" || requested === "preferences") setSection(requested); }, [requested]);

  function open(next: Section) { setSection(next); setError(""); router.replace(`/account?section=${next}`, { scroll: false }); }
  async function run(fn: () => Promise<void>, message: string) { setBusy(true); setError(""); try { await fn(); showToast(message); } catch (e) { setError(e instanceof ApiError ? e.message : "We could not save this change."); } finally { setBusy(false); } }
  if (!user) return <Loading label="Loading your account…" />;

  async function saveProfile(e: React.FormEvent) { e.preventDefault(); await run(async () => { const x = await api<{ user: AdminUser; token: string }>("/api/account/profile", { method: "PUT", body: user }); setUser(x.user); setSession(x.token, x.user); }, "Profile updated."); }
  async function savePreferences(e: React.FormEvent) { e.preventDefault(); const current = user!; await run(async () => { const x = await api<{ preferences: AdminPreferences }>("/api/account/preferences", { method: "PUT", body: current.preferences || {} }); const next: AdminUser = { ...current, preferences: x.preferences }; setUser(next); setSession(getToken()!, next); window.dispatchEvent(new Event("remi-preferences")); }, "Preferences saved."); }
  async function changeTheme(theme: "light" | "dark" | "system") {
    if (!user || user.preferences?.theme === theme) return;
    const previous = user;
    const next: AdminUser = { ...user, preferences: { ...user.preferences, theme } };
    const systemDark = window.matchMedia("(prefers-color-scheme: dark)").matches;
    const dark = theme === "dark" || (theme === "system" && systemDark);
    setUser(next);
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    const token = getToken();
    if (token) setSession(token, next);
    window.dispatchEvent(new Event("remi-preferences"));
    try {
      const result = await api<{ preferences: AdminPreferences }>("/api/account/preferences", { method: "PUT", body: next.preferences });
      const saved = { ...next, preferences: result.preferences };
      setUser(saved);
      if (token) setSession(token, saved);
      window.dispatchEvent(new Event("remi-preferences"));
      showToast(`${theme === "system" ? "System" : theme === "dark" ? "Dark" : "Light"} theme applied.`);
    } catch (err) {
      setUser(previous);
      const previousTheme = previous.preferences?.theme ?? "system";
      const previousDark = previousTheme === "dark" || (previousTheme === "system" && systemDark);
      document.documentElement.dataset.theme = previousDark ? "dark" : "light";
      if (token) setSession(token, previous);
      window.dispatchEvent(new Event("remi-preferences"));
      showToast(err instanceof ApiError ? err.message : "Theme change could not be saved.", "error");
    }
  }
  async function savePassword(e: React.FormEvent) { e.preventDefault(); if (passwords.newPassword !== passwords.confirm) { setError("New passwords do not match."); return; } await run(async () => { await api("/api/account/password", { method: "PUT", body: passwords }); setPasswords({ currentPassword: "", newPassword: "", confirm: "" }); }, "Password updated."); }

  const initials = (user.name || user.email).split(" ").map(x => x[0]).join("").slice(0, 2).toUpperCase();
  const completion = [user.name, user.email, user.title, user.phone, user.bio, user.avatarUrl].filter(Boolean).length;

  return <div className="max-w-7xl">
    <PageHeader title="My account" subtitle="Identity, workspace behavior and sign-in protection" />
    <section className="relative mb-6 overflow-hidden rounded-[1.6rem] bg-[var(--remi-green)] p-6 text-white shadow-[0_24px_70px_rgba(23,32,25,.18)] md:p-8">
      <div className="pointer-events-none absolute -right-24 -top-32 size-96 rounded-full border border-white/10" /><div className="pointer-events-none absolute -right-5 -top-16 size-56 rounded-full border border-[var(--remi-gold)]/20" />
      <div className="relative flex flex-col gap-6 md:flex-row md:items-end md:justify-between">
        <div className="flex items-center gap-5"><span className="grid size-20 shrink-0 place-items-center rounded-[1.35rem] bg-[var(--remi-gold)] text-2xl font-black text-[var(--remi-green)] shadow-[inset_0_1px_0_rgba(255,255,255,.5)]">{initials}</span><div><p className="font-mono text-[10px] font-bold uppercase tracking-[.18em] text-[var(--remi-gold)]">Operator identity</p><h2 className="mt-2 text-3xl font-bold tracking-[-.045em]">{user.name || "REMI operator"}</h2><p className="mt-1 text-sm text-white/50">{user.title || user.role} · {user.email}</p></div></div>
        <div className="w-full max-w-xs"><div className="flex justify-between text-[10px] font-bold uppercase tracking-[.12em] text-white/45"><span>Profile completeness</span><span>{completion}/6</span></div><div className="mt-2 h-1.5 overflow-hidden rounded-full bg-white/10"><span className="block h-full rounded-full bg-[var(--remi-gold)] transition-all duration-700" style={{ width: `${completion / 6 * 100}%` }} /></div></div>
      </div>
    </section>

    <div className="grid gap-6 lg:grid-cols-[17rem_minmax(0,1fr)]">
      <aside className="h-fit rounded-[1.35rem] border border-[#dedbd2] bg-[var(--remi-surface)] p-2 shadow-[0_14px_38px_rgba(23,32,25,.06)]" aria-label="Account sections">
        {SECTIONS.map(item => <button key={item.id} type="button" onClick={() => open(item.id)} aria-current={section === item.id ? "page" : undefined} className={`admin-account-section group grid w-full grid-cols-[2.5rem_1fr_auto] items-center gap-3 rounded-xl px-3 py-3.5 text-left transition ${section === item.id ? "bg-[var(--remi-green)] text-white shadow-lg" : "text-zinc-600 hover:bg-[#f3f0e7]"}`}><span className={`admin-account-section-icon grid size-9 place-items-center rounded-lg ${section === item.id ? "bg-white/10 text-[var(--remi-gold)]" : "bg-[#f1eee5] text-[#8b681f]"}`}>{item.icon}</span><span><b className="block text-sm">{item.label}</b><small className={`mt-0.5 block ${section === item.id ? "text-white/45" : "text-zinc-400"}`}>{item.copy}</small></span><i className="font-mono text-[9px] not-italic opacity-40">{item.index}</i></button>)}
      </aside>

      <div className="min-w-0">
        {error && <p role="alert" className="mb-5 rounded-xl border border-rose-200 bg-rose-50 p-4 text-sm font-medium text-rose-700">{error}</p>}
        {section === "profile" && <form onSubmit={saveProfile}><Panel eyebrow="Public identity" title="Personal profile" copy="The identity other operators see throughout the workspace."><div className="grid gap-x-5 gap-y-6 sm:grid-cols-2"><Field label="Full name" value={user.name || ""} autoComplete="name" required onChange={v => setUser({ ...user, name: v })} /><Field label="Email address" type="email" value={user.email} autoComplete="email" required onChange={v => setUser({ ...user, email: v })} /><Field label="Ministry title" value={user.title || ""} onChange={v => setUser({ ...user, title: v })} /><Field label="Phone number" type="tel" value={user.phone || ""} autoComplete="tel" onChange={v => setUser({ ...user, phone: v })} /><Field className="sm:col-span-2" label="Profile image URL" type="url" value={user.avatarUrl || ""} onChange={v => setUser({ ...user, avatarUrl: v })} /><label className="sm:col-span-2 text-sm font-semibold">Short biography<textarea rows={5} className={field} value={user.bio || ""} onChange={e => setUser({ ...user, bio: e.target.value })} /><span className="mt-1 block text-xs font-normal text-zinc-400">Keep this concise—two or three sentences works best.</span></label></div><Save busy={busy} label="Save profile" /></Panel></form>}

        {section === "security" && <div className="space-y-6"><form onSubmit={savePassword}><Panel eyebrow="Credentials" title="Change password" copy="Use at least 12 characters. A passphrase is easier to remember and harder to guess."><div className="grid gap-5 sm:grid-cols-2"><Field className="sm:col-span-2" label="Current password" type="password" autoComplete="current-password" value={passwords.currentPassword} required onChange={v => setPasswords({ ...passwords, currentPassword: v })} /><Field label="New password" type="password" autoComplete="new-password" value={passwords.newPassword} required onChange={v => setPasswords({ ...passwords, newPassword: v })} /><Field label="Confirm password" type="password" autoComplete="new-password" value={passwords.confirm} required onChange={v => setPasswords({ ...passwords, confirm: v })} /></div><Save busy={busy} label="Update password" /></Panel></form><Panel eyebrow="Second factor" title="Multi-factor authentication" copy="Require a rotating authenticator code after your password." badge={user.mfaEnabled ? "Enabled" : "Off"}>{!user.mfaEnabled && !mfa && <button onClick={() => run(async () => setMfa(await api("/api/account/mfa/setup", { method: "POST" })), "Authenticator setup started.")} className="rounded-xl bg-[var(--remi-gold)] px-5 py-3 text-sm font-bold text-[var(--remi-green)]">Set up authenticator</button>}{mfa && recovery.length === 0 && <div className="rounded-2xl bg-[#f6f2e8] p-5"><b className="text-sm">Add this key to your authenticator app</b><code className="mt-3 block break-all rounded-lg bg-white p-3 font-mono text-sm">{mfa.secret}</code><div className="mt-5"><span className="text-xs font-bold text-[#555a54]">Enter the six-digit code</span><div className="mt-2 max-w-md"><OtpInput value={mfaCode} onChange={setMfaCode} disabled={busy} label="Authenticator setup code" /></div></div><button disabled={busy || mfaCode.length !== 6} onClick={() => run(async () => { const x = await api<{ recoveryCodes: string[] }>("/api/account/mfa/confirm", { method: "POST", body: { code: mfaCode } }); setRecovery(x.recoveryCodes); setUser({ ...user, mfaEnabled: true }); }, "MFA enabled.")} className="mt-4 rounded-xl bg-[var(--remi-green)] px-5 py-3 text-sm font-bold text-white disabled:opacity-50">Verify and enable</button></div>}{recovery.length > 0 && <div className="rounded-2xl border border-amber-200 bg-amber-50 p-5"><b>Save these recovery codes now.</b><div className="mt-4 grid grid-cols-2 gap-2 font-mono text-sm">{recovery.map(x => <code key={x}>{x}</code>)}</div></div>}{user.mfaEnabled && recovery.length === 0 && <div className="grid gap-3 sm:grid-cols-[1fr_auto]"><input aria-label="Current password to disable MFA" type="password" className={field} placeholder="Current password" value={disablePassword} onChange={e => setDisablePassword(e.target.value)} /><button onClick={() => run(async () => { await api("/api/account/mfa", { method: "DELETE", body: { password: disablePassword } }); setUser({ ...user, mfaEnabled: false }); setDisablePassword(""); }, "MFA disabled.")} className="self-end rounded-xl border border-rose-200 px-5 py-3 text-sm font-bold text-rose-700">Disable MFA</button></div>}</Panel></div>}

        {section === "preferences" && <form onSubmit={savePreferences}><Panel eyebrow="Workspace behavior" title="Preferences" copy="Shape the interface around the way you work."><div className="space-y-7"><Choice label="Interface density" value={user.preferences?.density || "comfortable"} options={[{ value: "comfortable", title: "Comfortable", copy: "More breathing room" }, { value: "compact", title: "Compact", copy: "More data on screen" }]} onChange={v => setUser({ ...user, preferences: { ...user.preferences, density: v as "comfortable" | "compact" } })} /><Choice label="Color preference" value={user.preferences?.theme || "system"} options={[{ value: "system", title: "System", copy: "Follow this device" }, { value: "light", title: "Light", copy: "Warm paper canvas" }, { value: "dark", title: "Dark", copy: "Low-light workspace" }]} onChange={v => changeTheme(v as "system" | "light" | "dark")} /><label className="block text-sm font-semibold">Timezone<Select className="mt-2" value={user.preferences?.timezone || "Africa/Accra"} onChange={e => setUser({ ...user, preferences: { ...user.preferences, timezone: e.target.value } })}>{TIMEZONES.map(zone => <option key={zone} value={zone}>{zone.replaceAll("_", " ")}</option>)}</Select></label><div className="grid gap-3 sm:grid-cols-2"><Switch label="Weekly ministry digest" copy="Email an operational summary" checked={!!user.preferences?.emailDigest} onChange={v => setUser({ ...user, preferences: { ...user.preferences, emailDigest: v } })} /><Switch label="Reduce interface motion" copy="Minimize non-essential movement" checked={!!user.preferences?.reducedMotion} onChange={v => setUser({ ...user, preferences: { ...user.preferences, reducedMotion: v } })} /></div></div><Save busy={busy} label="Save other preferences" /></Panel></form>}
      </div>
    </div>
  </div>;
}

function Panel({ eyebrow, title, copy, badge, children }: { eyebrow: string; title: string; copy: string; badge?: string; children: React.ReactNode }) { return <Card className="overflow-hidden"><header className="flex items-start justify-between gap-5 border-b border-zinc-100 px-6 py-6 md:px-8"><div><p className="font-mono text-[10px] font-bold uppercase tracking-[.16em] text-[#8b681f]">{eyebrow}</p><h2 className="mt-2 text-2xl font-bold tracking-[-.035em]">{title}</h2><p className="mt-1 max-w-xl text-sm leading-6 text-zinc-500">{copy}</p></div>{badge && <span className="rounded-lg bg-emerald-50 px-2.5 py-1 font-mono text-[10px] font-bold uppercase text-emerald-700">{badge}</span>}</header><div className="p-6 md:p-8">{children}</div></Card>; }
function Field({ label, value, onChange, type = "text", required, autoComplete, className = "" }: { label: string; value: string; onChange: (v: string) => void; type?: string; required?: boolean; autoComplete?: string; className?: string }) { return <label className={`text-sm font-semibold ${className}`}>{label}<input required={required} type={type} autoComplete={autoComplete} className={field} value={value} onChange={e => onChange(e.target.value)} /></label>; }
function Save({ busy, label }: { busy: boolean; label: string }) { return <button disabled={busy} className="admin-primary-button mt-8 rounded-xl px-5 py-3 text-sm font-bold transition hover:-translate-y-0.5 disabled:opacity-50">{busy ? "Saving…" : label}</button>; }
function Switch({ label, copy, checked, onChange }: { label: string; copy: string; checked: boolean; onChange: (v: boolean) => void }) { return <button type="button" role="switch" aria-checked={checked} onClick={() => onChange(!checked)} className={`admin-preference-switch flex min-h-20 items-center justify-between gap-4 rounded-2xl border p-4 text-left transition ${checked ? "border-[#d1ad55] bg-[#f7f0dc]" : "border-zinc-200 bg-transparent hover:border-zinc-300"}`}><span><b className="block text-sm">{label}</b><small className="mt-1 block text-xs text-zinc-500">{copy}</small></span><span className={`relative h-7 w-12 shrink-0 rounded-full transition-colors ${checked ? "bg-[var(--remi-green)]" : "bg-zinc-300"}`}><i className={`absolute top-1 size-5 rounded-full bg-white shadow-sm transition-transform ${checked ? "translate-x-6" : "translate-x-1"}`} /></span></button>; }
function Choice({ label, value, options, onChange }: { label: string; value: string; options: { value: string; title: string; copy: string }[]; onChange: (v: string) => void }) { return <fieldset><legend className="text-sm font-semibold">{label}</legend><div className={`mt-3 grid gap-3 ${options.length === 3 ? "sm:grid-cols-3" : "sm:grid-cols-2"}`}>{options.map(option => <button type="button" key={option.value} aria-pressed={value === option.value} onClick={() => onChange(option.value)} className={`admin-preference-choice relative rounded-2xl border p-4 text-left transition hover:-translate-y-0.5 ${value === option.value ? "border-[#d1ad55] bg-[#f7f0dc] shadow-[0_10px_24px_rgba(139,104,31,.08)]" : "border-zinc-200"}`}><b className="block text-sm">{option.title}</b><small className="mt-1 block text-xs text-zinc-500">{option.copy}</small>{value === option.value && <span className="absolute right-3 top-3 text-[#8b681f]">✓</span>}</button>)}</div></fieldset>; }
