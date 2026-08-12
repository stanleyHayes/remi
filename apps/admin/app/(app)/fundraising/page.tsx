"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import { ApiError, api, asList } from "@/lib/api";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { PledgeWorkspace } from "@/components/pledge-workspace";
import { Select } from "@/components/ui/Select";
import DatePicker from "@/components/ui/DatePicker";

interface Fund {
  id: string;
  name: string;
}
interface Campaign {
  id: string;
  version: number;
  slug: string;
  title: string;
  summary: string;
  story: string;
  coverImageUrl?: string;
  fundId: string;
  goal: { amountMinor: number; currency: string };
  startsAt: string;
  endsAt: string;
  status: string;
  featured: boolean;
  raisedAmountMinor: number;
  giftCount: number;
}
const empty = {
  title: "",
  slug: "",
  summary: "",
  story: "",
  coverImageUrl: "",
  fundId: "",
  goal: "25000",
  startsAt: "2026-08-11",
  endsAt: "2026-12-31",
  status: "draft",
  featured: false,
};
const currency = (minor: number) =>
  new Intl.NumberFormat("en-GH", {
    style: "currency",
    currency: "GHS",
    maximumFractionDigits: 0,
  }).format(minor / 100);

export default function FundraisingPage() {
  const [campaigns, setCampaigns] = useState<Campaign[]>([]),
    [funds, setFunds] = useState<Fund[]>([]),
    [selected, setSelected] = useState<Campaign | null>(null);
  const [form, setForm] = useState(empty),
    [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null);
  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [campaignData, fundData] = await Promise.all([
        api("/api/chms/v1/finance/campaigns"),
        api("/api/chms/v1/finance/funds"),
      ]);
      setCampaigns(asList<Campaign>(campaignData));
      const nextFunds = asList<Fund>(fundData);
      setFunds(nextFunds);
      setForm((v) => ({ ...v, fundId: v.fundId || nextFunds[0]?.id || "" }));
    } catch (e) {
      setError(
        e instanceof ApiError ? e.message : "Campaigns could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  function edit(c: Campaign) {
    setSelected(c);
    setForm({
      title: c.title,
      slug: c.slug,
      summary: c.summary,
      story: c.story,
      coverImageUrl: c.coverImageUrl || "",
      fundId: c.fundId,
      goal: String(c.goal.amountMinor / 100),
      startsAt: c.startsAt.slice(0, 10),
      endsAt: c.endsAt.slice(0, 10),
      status: c.status,
      featured: c.featured,
    });
  }
  function reset() {
    setSelected(null);
    setForm({ ...empty, fundId: funds[0]?.id || "" });
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body = {
        ...form,
        goal: {
          amountMinor: Math.round(Number(form.goal) * 100),
          currency: "GHS",
        },
        startsAt: new Date(`${form.startsAt}T00:00:00Z`).toISOString(),
        endsAt: new Date(`${form.endsAt}T23:59:59Z`).toISOString(),
        expectedVersion: selected?.version,
      };
      await api(
        `/api/chms/v1/finance/campaigns${selected ? `/${selected.id}` : ""}`,
        { method: selected ? "PATCH" : "POST", body },
      );
      reset();
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Campaign could not be saved.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <div>
      <PageHeader
        title="Fundraising campaigns"
        subtitle="Shape transparent, time-bound campaigns and follow every verified gift against its goal."
      >
        <button className="admin-primary-button" onClick={reset}>
          New campaign
        </button>
      </PageHeader>
      {error && <ErrorBox message={error} onRetry={load} />}
      <div className="grid gap-6 xl:grid-cols-[1.1fr_.9fr]">
        <Card className="overflow-hidden">
          <div className="border-b border-black/5 p-5">
            <h2 className="font-semibold">Campaign portfolio</h2>
            <p className="mt-1 text-sm text-zinc-500">
              Published, paused and draft appeals in one place.
            </p>
          </div>
          <div className="grid gap-3 p-5">
            {loading ? (
              <p className="text-sm text-zinc-500">Loading campaigns…</p>
            ) : campaigns.length === 0 ? (
              <EmptyState
                dense
                title="No campaigns yet"
                hint="Create a focused appeal with a goal, story and deadline."
              />
            ) : (
              campaigns.map((c) => (
                <button
                  key={c.id}
                  onClick={() => edit(c)}
                  className="admin-card grid gap-4 p-5 text-left md:grid-cols-[1fr_auto]"
                >
                  <div>
                    <span className="font-mono text-[10px] uppercase tracking-[.18em] text-amber-700">
                      {c.status}
                    </span>
                    <h3 className="mt-2 text-lg font-semibold">{c.title}</h3>
                    <p className="mt-1 text-sm text-zinc-500">{c.summary}</p>
                    <div className="mt-4 h-1.5 overflow-hidden rounded-full bg-black/5">
                      <i
                        className="block h-full bg-amber-600"
                        style={{
                          width: `${Math.min(100, c.goal.amountMinor ? (c.raisedAmountMinor / c.goal.amountMinor) * 100 : 0)}%`,
                        }}
                      />
                    </div>
                  </div>
                  <div className="text-right">
                    <strong className="block font-mono">
                      {currency(c.raisedAmountMinor)}
                    </strong>
                    <small className="text-zinc-500">
                      of {currency(c.goal.amountMinor)} · {c.giftCount} gifts
                    </small>
                  </div>
                </button>
              ))
            )}
          </div>
        </Card>
        <Card className="p-5">
          <h2 className="text-lg font-semibold">
            {selected ? "Edit campaign" : "Create campaign"}
          </h2>
          <form onSubmit={save} className="mt-5 grid gap-4">
            <label className="admin-field">
              <span>Campaign title</span>
              <input
                required
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
              />
            </label>
            <label className="admin-field">
              <span>URL slug</span>
              <input
                required
                pattern="[a-z0-9]+(?:-[a-z0-9]+)*"
                value={form.slug}
                onChange={(e) =>
                  setForm({
                    ...form,
                    slug: e.target.value
                      .toLowerCase()
                      .replace(/[^a-z0-9-]/g, "-"),
                  })
                }
              />
            </label>
            <label className="admin-field">
              <span>Short summary</span>
              <textarea
                required
                rows={2}
                value={form.summary}
                onChange={(e) => setForm({ ...form, summary: e.target.value })}
              />
            </label>
            <label className="admin-field">
              <span>Campaign story</span>
              <textarea
                required
                rows={5}
                value={form.story}
                onChange={(e) => setForm({ ...form, story: e.target.value })}
              />
            </label>
            <label className="admin-field">
              <span>Cover image URL</span>
              <input
                type="url"
                value={form.coverImageUrl}
                onChange={(e) =>
                  setForm({ ...form, coverImageUrl: e.target.value })
                }
              />
            </label>
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="admin-field">
                <span>Fund</span>
                <Select
                  required
                  value={form.fundId}
                  onChange={(e) => setForm({ ...form, fundId: e.target.value })}
                >
                  {funds.map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.name}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="admin-field">
                <span>Goal (GHS)</span>
                <input
                  required
                  min="1"
                  step="0.01"
                  type="number"
                  value={form.goal}
                  onChange={(e) => setForm({ ...form, goal: e.target.value })}
                />
              </label>
              <label className="admin-field">
                <span>Starts</span>
                <DatePicker
                  value={form.startsAt}
                  onChange={(startsAt) => setForm({ ...form, startsAt })}
                />
              </label>
              <label className="admin-field">
                <span>Ends</span>
                <DatePicker
                  min={form.startsAt}
                  value={form.endsAt}
                  onChange={(endsAt) => setForm({ ...form, endsAt })}
                />
              </label>
              <label className="admin-field">
                <span>Status</span>
                <Select
                  value={form.status}
                  onChange={(e) => setForm({ ...form, status: e.target.value })}
                >
                  <option value="draft">Draft</option>
                  <option value="published">Published</option>
                  <option value="paused">Paused</option>
                  <option value="completed">Completed</option>
                </Select>
              </label>
              <label className="flex items-center gap-3 pt-7 text-sm">
                <input
                  type="checkbox"
                  checked={form.featured}
                  onChange={(e) =>
                    setForm({ ...form, featured: e.target.checked })
                  }
                />{" "}
                Feature publicly
              </label>
            </div>
            <div className="flex justify-end gap-2">
              <button
                type="button"
                className="admin-secondary-button"
                onClick={reset}
              >
                Clear
              </button>
              <button disabled={busy} className="admin-primary-button">
                {busy ? "Saving…" : "Save campaign"}
              </button>
            </div>
          </form>
        </Card>
      </div>
      <PledgeWorkspace funds={funds} campaigns={campaigns} />
    </div>
  );
}
