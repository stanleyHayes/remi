"use client";

import { useEffect, useState } from "react";
import { api, ApiError } from "@/lib/api";
import { Card, Loading, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";

interface ServiceTime {
  name: string;
  day: string;
  time: string;
  location: string;
}

interface Settings {
  churchName: string;
  tagline: string;
  serviceTimes: ServiceTime[];
  socials: { facebook: string; instagram: string; youtube: string; tiktok: string };
  whatsapp: string;
  phone: string;
  email: string;
  address: string;
  livestreamUrl: string;
  isLive: boolean;
  givingCategories: string[];
  nextServiceOverride: string | null;
}

const EMPTY: Settings = {
  churchName: "",
  tagline: "",
  serviceTimes: [],
  socials: { facebook: "", instagram: "", youtube: "", tiktok: "" },
  whatsapp: "",
  phone: "",
  email: "",
  address: "",
  livestreamUrl: "",
  isLive: false,
  givingCategories: [],
  nextServiceOverride: null,
};

const input =
  "mt-1.5 w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-gold";
const SERVICE_TIMES = ["06:00 GMT","07:00 GMT","08:00 GMT","09:00 GMT","10:00 GMT","10:30 GMT","11:00 GMT","12:00 GMT","13:00 GMT","14:00 GMT","15:00 GMT","16:00 GMT","17:00 GMT","18:00 GMT","18:30 GMT","19:00 GMT","20:00 GMT"];

export default function SettingsPage() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [givingText, setGivingText] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  useEffect(() => {
    api<Partial<Settings>>("/api/admin/settings")
      .then((data) => {
        const merged: Settings = {
          ...EMPTY,
          ...data,
          socials: { ...EMPTY.socials, ...(data.socials ?? {}) },
          serviceTimes: Array.isArray(data.serviceTimes) ? data.serviceTimes : [],
          givingCategories: Array.isArray(data.givingCategories) ? data.givingCategories : [],
        };
        setSettings(merged);
        setGivingText(merged.givingCategories.join(", "));
      })
      .catch((err) => setError(err instanceof ApiError ? err.message : "Failed to load settings."));
  }, []);

  function patch(p: Partial<Settings>) {
    setSettings((prev) => (prev ? { ...prev, ...p } : prev));
  }

  function patchSocial(key: keyof Settings["socials"], value: string) {
    setSettings((prev) => (prev ? { ...prev, socials: { ...prev.socials, [key]: value } } : prev));
  }

  function patchServiceTime(i: number, p: Partial<ServiceTime>) {
    setSettings((prev) =>
      prev
        ? { ...prev, serviceTimes: prev.serviceTimes.map((st, idx) => (idx === i ? { ...st, ...p } : st)) }
        : prev
    );
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!settings) return;
    setSaving(true);
    setSaved(false);
    setError(null);
    try {
      const payload: Settings = {
        ...settings,
        givingCategories: givingText
          .split(",")
          .map((s) => s.trim())
          .filter(Boolean),
        nextServiceOverride: settings.nextServiceOverride?.trim() ? settings.nextServiceOverride.trim() : null,
      };
      const updated = await api<Partial<Settings>>("/api/admin/settings", { method: "PUT", body: payload });
      setSettings({ ...payload, ...updated, socials: { ...payload.socials, ...(updated.socials ?? {}) } });
      setSaved(true);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Save failed.");
    } finally {
      setSaving(false);
    }
  }

  if (!settings && !error) return <Loading label="Loading settings…" />;
  if (!settings) {
    return (
      <div className="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">{error}</div>
    );
  }

  return (
    <div className="max-w-6xl">
      <PageHeader title="Site configuration" subtitle="Public identity, weekly rhythms, contact channels, livestream and giving" />
	  <section className="mb-6 grid gap-4 rounded-2xl bg-[var(--remi-green)] p-6 text-white md:grid-cols-[1fr_auto] md:items-center"><div><p className="font-mono text-[10px] font-bold uppercase tracking-[.18em] text-[var(--remi-gold)]">Public website control</p><h2 className="mt-2 text-2xl font-bold tracking-[-.035em]">One source of truth for REMI online.</h2><p className="mt-2 max-w-2xl text-sm leading-6 text-white/50">Changes here affect public contact details, service planning, giving choices and livestream visibility.</p></div><span className="w-fit rounded-xl border border-white/10 bg-white/5 px-4 py-3 font-mono text-[10px] font-bold text-[#9fc493]">● API CONNECTED</span></section>

      <form onSubmit={onSubmit} className="space-y-5">
		<Card className="space-y-4 p-6 md:p-8">
          <h2 className="text-xl font-bold tracking-[-.025em] text-zinc-900">Church identity</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="block text-sm font-medium text-zinc-700">
              Church name
              <input className={input} value={settings.churchName} onChange={(e) => patch({ churchName: e.target.value })} />
            </label>
            <label className="block text-sm font-medium text-zinc-700">
              Tagline
              <input className={input} value={settings.tagline} onChange={(e) => patch({ tagline: e.target.value })} />
            </label>
          </div>
        </Card>

        <Card className="p-6">
          <div className="flex items-center justify-between">
            <h2 className="text-sm font-semibold text-zinc-900">Service times</h2>
            <button
              type="button"
              onClick={() => patch({ serviceTimes: [...settings.serviceTimes, { name: "", day: "", time: "", location: "" }] })}
              className="rounded-md border border-zinc-300 px-3 py-1.5 text-xs font-medium text-zinc-600 hover:border-gold hover:text-gold-dark"
            >
              + Add service
            </button>
          </div>
          {settings.serviceTimes.length === 0 && (
            <p className="mt-3 text-sm text-zinc-400">No service times configured.</p>
          )}
          <div className="mt-3 space-y-3">
            {settings.serviceTimes.map((st, i) => (
              <div key={i} className="grid grid-cols-2 items-end gap-3 rounded-md border border-zinc-200 p-3 sm:grid-cols-5">
                <label className="block text-xs font-medium text-zinc-500">
                  Name
                  <input className={input} value={st.name} onChange={(e) => patchServiceTime(i, { name: e.target.value })} placeholder="Sunday Service" />
                </label>
                <label className="block text-xs font-medium text-zinc-500">
                  Day
                  <Select className="mt-1.5" value={st.day} onChange={(e) => patchServiceTime(i, { day: e.target.value })}><option value="">Choose day</option>{["Sunday","Monday","Tuesday","Wednesday","Thursday","Friday","Saturday"].map(day=><option key={day} value={day}>{day}</option>)}</Select>
                </label>
                <label className="block text-xs font-medium text-zinc-500">
                  Time
                  <Select className="mt-1.5" value={st.time} onChange={(e) => patchServiceTime(i, { time: e.target.value })}><option value="">Choose time</option>{st.time && !SERVICE_TIMES.includes(st.time) && <option value={st.time}>{st.time}</option>}{SERVICE_TIMES.map(time=><option key={time} value={time}>{time}</option>)}</Select>
                </label>
                <label className="block text-xs font-medium text-zinc-500">
                  Location
                  <input className={input} value={st.location} onChange={(e) => patchServiceTime(i, { location: e.target.value })} placeholder="Main auditorium" />
                </label>
                <button
                  type="button"
                  onClick={() => patch({ serviceTimes: settings.serviceTimes.filter((_, idx) => idx !== i) })}
                  className="admin-danger-button rounded-md px-3 py-2 text-xs font-medium"
                >
                  Remove
                </button>
              </div>
            ))}
          </div>
          <label className="mt-4 block text-sm font-medium text-zinc-700">
            Next service override
            <input
              className={input}
              value={settings.nextServiceOverride ?? ""}
              onChange={(e) => patch({ nextServiceOverride: e.target.value })}
              placeholder="e.g. No service this Sunday — leave empty to auto-compute"
            />
          </label>
        </Card>

        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold text-zinc-900">Contact & socials</h2>
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="block text-sm font-medium text-zinc-700">
              Email
              <input className={input} value={settings.email} onChange={(e) => patch({ email: e.target.value })} />
            </label>
            <label className="block text-sm font-medium text-zinc-700">
              Phone
              <input className={input} value={settings.phone} onChange={(e) => patch({ phone: e.target.value })} />
            </label>
            <label className="block text-sm font-medium text-zinc-700">
              WhatsApp
              <input className={input} value={settings.whatsapp} onChange={(e) => patch({ whatsapp: e.target.value })} />
            </label>
            <label className="block text-sm font-medium text-zinc-700 sm:col-span-2">
              Address
              <input className={input} value={settings.address} onChange={(e) => patch({ address: e.target.value })} />
            </label>
            {(["facebook", "instagram", "youtube", "tiktok"] as const).map((key) => (
              <label key={key} className="block text-sm font-medium text-zinc-700">
                {key.charAt(0).toUpperCase() + key.slice(1)}
                <input className={input} value={settings.socials[key]} onChange={(e) => patchSocial(key, e.target.value)} placeholder="https://…" />
              </label>
            ))}
          </div>
        </Card>

        <Card className="space-y-4 p-6">
          <h2 className="text-sm font-semibold text-zinc-900">Livestream & giving</h2>
          <label className="flex items-center gap-3 text-sm font-medium text-zinc-700">
            <input
              type="checkbox"
              checked={settings.isLive}
              onChange={(e) => patch({ isLive: e.target.checked })}
              className="h-4 w-4 rounded border-zinc-300 accent-[#c9a227]"
            />
            We are live now (shows the live banner on the site)
          </label>
          <label className="block text-sm font-medium text-zinc-700">
            Livestream URL
            <input className={input} value={settings.livestreamUrl} onChange={(e) => patch({ livestreamUrl: e.target.value })} placeholder="https://youtube.com/live/…" />
          </label>
          <label className="block text-sm font-medium text-zinc-700">
            Giving categories
            <input
              className={input}
              value={givingText}
              onChange={(e) => setGivingText(e.target.value)}
              placeholder="Tithe, Offering, Building Fund, Missions"
            />
            <span className="mt-1 block text-xs font-normal text-zinc-400">Comma-separated.</span>
          </label>
        </Card>

        {error && (
          <p className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p>
        )}
        {saved && (
          <p className="rounded-md border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">
            Settings saved.
          </p>
        )}

        <button
          type="submit"
          disabled={saving}
          className="rounded-md bg-gold px-5 py-2.5 text-sm font-semibold text-sidebar transition hover:bg-gold-dark disabled:opacity-60"
        >
          {saving ? "Saving…" : "Save settings"}
        </button>
      </form>
    </div>
  );
}
