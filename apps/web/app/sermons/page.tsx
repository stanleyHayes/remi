import type { Metadata } from "next";
import Reveal from "@/components/reveal";
import { SermonCard } from "@/components/cards";
import { EmptyState, SectionHeading } from "@/components/ui";
import Select from "@/components/ui/Select";
import { getSermons } from "@/lib/api";
import { pageMetadata } from "@/lib/seo";

export const metadata: Metadata = pageMetadata({
  title: "Sermons",
  description: "Watch, listen, and grow — the sermon library of Ruach Elohim Ministries International.",
  path: "/sermons",
});

const selectCls =
  "w-full rounded-full border border-cream/15 bg-ink-soft px-5 py-3 text-sm text-cream focus:border-gold/50 focus:outline-none sm:w-auto";

const filterTriggerCls =
  "flex w-full items-center justify-between gap-3 rounded-full border border-cream/15 bg-ink-soft px-5 py-3 text-left text-sm text-cream transition-colors duration-300 hover:border-cream/25 focus:border-gold/50 focus:outline-none focus-visible:ring-2 focus-visible:ring-gold/25";

export default async function SermonsPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const sp = await searchParams;
  const query = {
    preacher: typeof sp.preacher === "string" ? sp.preacher : undefined,
    series: typeof sp.series === "string" ? sp.series : undefined,
    topic: typeof sp.topic === "string" ? sp.topic : undefined,
    q: typeof sp.q === "string" ? sp.q : undefined,
    page: typeof sp.page === "string" ? sp.page : undefined,
  };

  const data = await getSermons(query);
  const items = data?.items ?? [];
  const facets = data?.facets ?? { preachers: [], series: [], topics: [] };
  const hasFilters = Boolean(query.preacher || query.series || query.topic || query.q);
  const page = data?.page ?? 1;
  const totalPages = data ? Math.max(1, Math.ceil(data.total / (data.pageSize || 1))) : 1;

  function pageUrl(p: number) {
    const params = new URLSearchParams();
    for (const [k, v] of Object.entries(query)) {
      if (v && k !== "page") params.set(k, v);
    }
    if (p > 1) params.set("page", String(p));
    const qs = params.toString();
    return `/sermons${qs ? `?${qs}` : ""}`;
  }

  return (
    <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <SectionHeading
          eyebrow="The Word"
          title="Sermon library"
          copy="Search by preacher, series, or topic — every message is an invitation to go deeper."
        />
      </Reveal>

      {/* Filters (plain GET form — submits without JS; the custom Selects
          enhance to dropdowns once hydrated) */}
      <Reveal delay={120}>
        <form action="/sermons" method="get" className="mt-12 flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-center">
          <input
            type="search"
            name="q"
            defaultValue={query.q ?? ""}
            placeholder="Search messages…"
            className={`${selectCls} sm:flex-1 sm:min-w-52`}
            aria-label="Search sermons"
          />
          <Select
            name="preacher"
            defaultValue={query.preacher ?? ""}
            className="sm:w-auto"
            triggerClassName={filterTriggerCls}
            aria-label="Filter by preacher"
          >
            <option value="">All preachers</option>
            {facets.preachers.map((p) => (
              <option key={p} value={p}>{p}</option>
            ))}
          </Select>
          <Select
            name="series"
            defaultValue={query.series ?? ""}
            className="sm:w-auto"
            triggerClassName={filterTriggerCls}
            aria-label="Filter by series"
          >
            <option value="">All series</option>
            {facets.series.map((s) => (
              <option key={s} value={s}>{s}</option>
            ))}
          </Select>
          <Select
            name="topic"
            defaultValue={query.topic ?? ""}
            className="sm:w-auto"
            triggerClassName={filterTriggerCls}
            aria-label="Filter by topic"
          >
            <option value="">All topics</option>
            {facets.topics.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </Select>
          <button
            type="submit"
            className="rounded-full bg-gold px-7 py-3 text-sm font-semibold text-ink transition-colors duration-300 hover:bg-gold-bright"
          >
            Filter
          </button>
          {hasFilters ? (
            <a href="/sermons" className="px-2 py-3 text-sm text-cream-dim underline-offset-4 hover:text-cream hover:underline">
              Clear
            </a>
          ) : null}
        </form>
      </Reveal>

      {items.length === 0 ? (
        <div className="mt-12">
          <EmptyState
            title={hasFilters ? "No sermons match those filters" : "Sermons coming soon"}
            copy={
              hasFilters
                ? "Try a different combination, or clear the filters to browse everything."
                : "Our message archive is being prepared. Check back shortly — or join us live this Sunday."
            }
          />
        </div>
      ) : (
        <>
          <div className="mt-12 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {items.map((sermon, i) => (
              <Reveal key={sermon.id} delay={(i % 3) * 80}>
                <SermonCard sermon={sermon} />
              </Reveal>
            ))}
          </div>

          {totalPages > 1 ? (
            <div className="mt-14 flex items-center justify-center gap-4">
              {page > 1 ? (
                <a href={pageUrl(page - 1)} className="rounded-full border border-cream/15 px-6 py-2.5 text-sm text-cream transition-colors duration-300 hover:border-gold/40 hover:text-gold-bright">
                  ← Newer
                </a>
              ) : null}
              <span className="text-xs uppercase tracking-[0.2em] text-cream-dim">
                Page {page} of {totalPages}
              </span>
              {page < totalPages ? (
                <a href={pageUrl(page + 1)} className="rounded-full border border-cream/15 px-6 py-2.5 text-sm text-cream transition-colors duration-300 hover:border-gold/40 hover:text-gold-bright">
                  Older →
                </a>
              ) : null}
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}
