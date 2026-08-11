"use client";

import { useSearchParams } from "next/navigation";
import { Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { api, asList, ApiError } from "@/lib/api";
import { formatDate } from "@/lib/content";
import StatusBadge from "@/components/StatusBadge";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import Pagination from "@/components/ui/Pagination";
import { Select } from "@/components/ui/Select";
import { SkeletonCards } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";

type TabKey = "prayer" | "contact" | "testimonies" | "visits";

interface TabDef {
  key: TabKey;
  label: string;
  endpoint: string;
  statuses: string[];
}

const TABS: TabDef[] = [
  { key: "prayer", label: "Prayer requests", endpoint: "/api/admin/forms/prayer", statuses: ["new", "in-prayer", "closed"] },
  { key: "contact", label: "Contact", endpoint: "/api/admin/forms/contact", statuses: ["new", "read", "archived"] },
  { key: "testimonies", label: "Testimonies", endpoint: "/api/admin/forms/testimonies", statuses: ["new", "read", "archived"] },
  { key: "visits", label: "Visit requests", endpoint: "/api/admin/forms/visits", statuses: ["new", "confirmed", "archived"] },
];

interface Submission {
  id: string;
  [key: string]: unknown;
}

const FIELD_LABELS: Record<string, string> = {
  name: "Name",
  email: "Email",
  phone: "Phone",
  subject: "Subject",
  message: "Message",
  request: "Request",
  author: "Author",
  body: "Body",
  visitDate: "Visit date",
  branch: "Branch",
  notes: "Notes",
};

const DISPLAY_FIELDS: Record<TabKey, string[]> = {
  prayer: ["name", "email", "request"],
  contact: ["name", "email", "phone", "subject", "message"],
  testimonies: ["author", "body"],
  visits: ["name", "email", "phone", "visitDate", "branch", "notes"],
};

const PAGE_SIZE = 8;

function SearchIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-zinc-400" aria-hidden="true">
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.35-4.35" />
    </svg>
  );
}

function InboxInner() {
  const searchParams = useSearchParams();
  const { showToast } = useToast();
  const initialTab = (searchParams.get("tab") as TabKey) || "prayer";
  const [tab, setTab] = useState<TabDef>(TABS.find((t) => t.key === initialTab) ?? TABS[0]);
  const [items, setItems] = useState<Submission[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);

  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [sortDir, setSortDir] = useState<"desc" | "asc">("desc");
  const [page, setPage] = useState(1);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query.trim().toLowerCase()), 300);
    return () => clearTimeout(t);
  }, [query]);

  useEffect(() => {
    setPage(1);
  }, [debouncedQuery, sortDir, tab]);

  const load = useCallback(async () => {
    setError(null);
    setItems(null);
    try {
      const data = await api(tab.endpoint);
      setItems(asList<Submission>(data));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load.");
      setItems([]);
    }
  }, [tab]);

  useEffect(() => {
    load();
  }, [load]);

  async function setStatus(id: string, status: string) {
    setBusyId(id);
    try {
      await api(`${tab.endpoint}/${id}`, { method: "PATCH", body: { status } });
      setItems((prev) => (prev ? prev.map((i) => (i.id === id ? { ...i, status } : i)) : prev));
      showToast(`Moved to ${status}.`);
    } catch (err) {
      showToast(err instanceof ApiError ? err.message : "Status update failed.", "error");
    } finally {
      setBusyId(null);
    }
  }

  const visible = useMemo(() => {
    if (!items) return null;
    let rows = items;
    if (debouncedQuery) {
      rows = rows.filter((item) =>
        [...DISPLAY_FIELDS[tab.key].map((k) => item[k]), item.status]
          .filter((v) => v !== undefined && v !== null)
          .join(" ")
          .toLowerCase()
          .includes(debouncedQuery),
      );
    }
    return [...rows].sort((a, b) => {
      const av = typeof a.createdAt === "string" ? a.createdAt : "";
      const bv = typeof b.createdAt === "string" ? b.createdAt : "";
      const cmp = av.localeCompare(bv);
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [items, debouncedQuery, sortDir, tab]);

  const totalPages = visible ? Math.max(1, Math.ceil(visible.length / PAGE_SIZE)) : 1;
  const pageItems = visible ? visible.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE) : null;

  return (
    <div>
      <PageHeader title="Inbox" subtitle="Prayer requests, contact messages, testimonies and visit requests" />

      <div className="mb-5 flex flex-wrap gap-2">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t)}
            className={`rounded-full px-4 py-1.5 text-sm font-medium transition ${
              tab.key === t.key ? "bg-sidebar text-white" : "border border-zinc-300 text-zinc-600 hover:border-zinc-400"
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>

      {error && <ErrorBox message={error} onRetry={load} />}
      {items === null && !error && <SkeletonCards count={3} />}

      {items !== null && !error && (
        <>
          <div className="mb-4 flex flex-wrap items-center gap-3">
            <div className="admin-search-control flex min-w-0 flex-1 items-center gap-2 rounded-md border border-zinc-300 bg-white px-3 py-2 focus-within:border-gold sm:max-w-xs">
              <SearchIcon />
              <input
                type="text"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={`Search ${tab.label.toLowerCase()}…`}
                className="w-full bg-transparent text-sm outline-none placeholder:text-zinc-400"
              />
            </div>
            <Select
              aria-label="Sort order"
              className="w-40 shrink-0"
              value={sortDir}
              onChange={(e) => setSortDir(e.target.value as "desc" | "asc")}
            >
              <option value="desc">Newest first</option>
              <option value="asc">Oldest first</option>
            </Select>
          </div>

          {items.length === 0 ? (
            <EmptyState title={`No ${tab.label.toLowerCase()} yet`} hint="New submissions from the website appear here." />
          ) : visible && visible.length === 0 ? (
            <EmptyState dense title={`No matches for “${debouncedQuery}”`} hint="Try a different search term." />
          ) : (
            <div className="space-y-3">
              {(pageItems ?? []).map((item) => {
                const isPrivate = tab.key === "prayer" && item.isPrivate === true;
                const status = typeof item.status === "string" ? item.status : "new";
                return (
                  <Card key={item.id} className={`p-5 ${isPrivate ? "border-rose-300 ring-1 ring-rose-200" : ""}`}>
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div className="flex items-center gap-2">
                        <StatusBadge status={status} />
                        {isPrivate && (
                          <span className="inline-flex items-center rounded-full bg-rose-600 px-2 py-0.5 text-xs font-semibold text-white">
                            PRIVATE — pastors only
                          </span>
                        )}
                      </div>
                      <span className="text-xs text-zinc-400">
                        {formatDate(typeof item.createdAt === "string" ? item.createdAt : undefined)}
                      </span>
                    </div>

                    <dl className="mt-3 space-y-2">
                      {DISPLAY_FIELDS[tab.key].map((key) => {
                        const val = item[key];
                        if (val === undefined || val === null || val === "") return null;
                        return (
                          <div key={key} className="text-sm">
                            <dt className="text-xs font-medium uppercase tracking-wide text-zinc-400">{FIELD_LABELS[key] ?? key}</dt>
                            <dd className="mt-0.5 whitespace-pre-wrap text-zinc-800">{String(val)}</dd>
                          </div>
                        );
                      })}
                    </dl>

                    <div className="mt-4 flex flex-wrap items-center gap-2 border-t border-zinc-100 pt-3">
                      <span className="text-xs font-medium text-zinc-500">Move to:</span>
                      {tab.statuses
                        .filter((s) => s !== status)
                        .map((s) => (
                          <button
                            key={s}
                            onClick={() => setStatus(item.id, s)}
                            disabled={busyId === item.id}
                            className="rounded-md border border-zinc-300 px-2.5 py-1 text-xs font-medium text-zinc-600 hover:border-gold hover:text-gold-dark disabled:opacity-50"
                          >
                            {s}
                          </button>
                        ))}
                    </div>
                  </Card>
                );
              })}

              {visible && (
                <Card>
                  <Pagination
                    page={page}
                    totalPages={totalPages}
                    onPageChange={setPage}
                    totalItems={visible.length}
                    pageSize={PAGE_SIZE}
                    className="border-t-0"
                  />
                </Card>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}

export default function InboxPage() {
  return (
    <Suspense fallback={<SkeletonCards count={3} />}>
      <InboxInner />
    </Suspense>
  );
}
