"use client";

import Link from "next/link";
import { useParams, useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, asList, ApiError } from "@/lib/api";
import { docTitle, formatDate, getContentType, type Doc } from "@/lib/content";
import StatusBadge from "@/components/StatusBadge";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import Pagination from "@/components/ui/Pagination";
import { SkeletonTable } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";

const PAGE_SIZE = 10;

type SortKey = "title" | "updatedAt";

function SearchIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-zinc-400" aria-hidden="true">
      <circle cx="11" cy="11" r="8" />
      <path d="m21 21-4.35-4.35" />
    </svg>
  );
}

function SortHeader({
  label,
  sortKey,
  activeKey,
  dir,
  onSort,
  className = "",
}: {
  label: string;
  sortKey: SortKey;
  activeKey: SortKey;
  dir: "asc" | "desc";
  onSort: (key: SortKey) => void;
  className?: string;
}) {
  const active = activeKey === sortKey;
  return (
    <th className={`px-4 py-3 font-medium ${className}`}>
      <button
        type="button"
        onClick={() => onSort(sortKey)}
        className={`inline-flex items-center gap-1 uppercase tracking-wide transition hover:text-zinc-800 ${active ? "text-zinc-800" : ""}`}
      >
        {label}
        <span aria-hidden="true" className={`text-[10px] ${active ? "text-gold-dark" : "text-zinc-300"}`}>
          {active ? (dir === "asc" ? "▲" : "▼") : "▲"}
        </span>
      </button>
    </th>
  );
}

export default function ContentListPage() {
  const params = useParams<{ type: string }>();
  const router = useRouter();
  const { showToast } = useToast();
  const def = getContentType(params.type);

  const [docs, setDocs] = useState<Doc[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pendingDelete, setPendingDelete] = useState<Doc | null>(null);
  const [deleting, setDeleting] = useState(false);

  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [sortKey, setSortKey] = useState<SortKey>("updatedAt");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [page, setPage] = useState(1);

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQuery(query.trim().toLowerCase()), 300);
    return () => clearTimeout(t);
  }, [query]);

  useEffect(() => {
    setPage(1);
  }, [debouncedQuery, sortKey, sortDir, params.type]);

  const load = useCallback(async () => {
    if (!def) return;
    setError(null);
    try {
      const data = await api(def.endpoint);
      setDocs(asList<Doc>(data));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load.");
      setDocs([]);
    }
  }, [def]);

  useEffect(() => {
    load();
  }, [load]);

  async function confirmDelete() {
    if (!def || !pendingDelete) return;
    const doc = pendingDelete;
    setDeleting(true);
    try {
      await api(`${def.endpoint}/${doc.id}`, { method: "DELETE" });
      setDocs((prev) => (prev ? prev.filter((d) => d.id !== doc.id) : prev));
      showToast(`Deleted “${docTitle(doc)}”.`);
      setPendingDelete(null);
    } catch (err) {
      showToast(err instanceof ApiError ? err.message : "Delete failed.", "error");
    } finally {
      setDeleting(false);
    }
  }

  function onSort(key: SortKey) {
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir(key === "title" ? "asc" : "desc");
    }
  }

  const visible = useMemo(() => {
    if (!docs) return null;
    let rows = docs;
    if (debouncedQuery) {
      rows = rows.filter((d) =>
        [docTitle(d), typeof d.slug === "string" ? d.slug : "", d.contentStatus ?? ""]
          .join(" ")
          .toLowerCase()
          .includes(debouncedQuery),
      );
    }
    return [...rows].sort((a, b) => {
      const av = sortKey === "title" ? docTitle(a) : (a.updatedAt ?? "");
      const bv = sortKey === "title" ? docTitle(b) : (b.updatedAt ?? "");
      const cmp = String(av).localeCompare(String(bv), undefined, { numeric: true, sensitivity: "base" });
      return sortDir === "asc" ? cmp : -cmp;
    });
  }, [docs, debouncedQuery, sortKey, sortDir]);

  const totalPages = visible ? Math.max(1, Math.ceil(visible.length / PAGE_SIZE)) : 1;
  const pageDocs = visible ? visible.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE) : null;

  if (!def) {
    return (
      <EmptyState title="Unknown content type" hint={`No manager is registered for "${params.type}".`} />
    );
  }

  return (
    <div>
      <PageHeader title={def.labelPlural} subtitle={`Manage ${def.labelPlural.toLowerCase()} shown on the site`}>
        {def.canCreate && (
          <Link
            href={`/content/${def.type}/new`}
            className="rounded-md bg-gold px-4 py-2 text-sm font-semibold text-sidebar transition hover:bg-gold-dark"
          >
            + New {def.label}
          </Link>
        )}
      </PageHeader>

      {error && <ErrorBox message={error} onRetry={load} />}
      {!error && docs === null && <SkeletonTable rows={6} cols={4} />}

      {docs !== null && !error && docs.length === 0 && (
        <EmptyState
          title={`No ${def.labelPlural.toLowerCase()} yet`}
          hint={def.canCreate ? `Create your first ${def.label.toLowerCase()} to get started.` : undefined}
        />
      )}

      {docs !== null && docs.length > 0 && visible && pageDocs && (
        <>
          <div className="admin-search-control mb-4 flex items-center gap-2 rounded-md border border-zinc-300 bg-white px-3 py-2 focus-within:border-gold">
            <SearchIcon />
            <input
              type="text"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={`Search ${def.labelPlural.toLowerCase()}…`}
              className="w-full bg-transparent text-sm outline-none placeholder:text-zinc-400"
            />
          </div>

          {visible.length === 0 ? (
            <EmptyState dense title={`No matches for “${debouncedQuery}”`} hint="Try a different search term." />
          ) : (
            <Card className="overflow-x-auto">
              <table className="w-full min-w-[640px] text-left text-sm">
                <thead>
                  <tr className="border-b border-zinc-200 text-xs uppercase tracking-wide text-zinc-500">
                    <SortHeader label="Title" sortKey="title" activeKey={sortKey} dir={sortDir} onSort={onSort} />
                    <th className="px-4 py-3 font-medium">Status</th>
                    <SortHeader label="Updated" sortKey="updatedAt" activeKey={sortKey} dir={sortDir} onSort={onSort} />
                    <th className="px-4 py-3 text-right font-medium">Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {pageDocs.map((doc) => (
                    <tr key={doc.id} className="border-b border-zinc-100 last:border-0 hover:bg-zinc-50">
                      <td className="px-4 py-3">
                        <Link href={`/content/${def.type}/${doc.id}`} className="font-medium text-zinc-900 hover:text-gold-dark">
                          {docTitle(doc)}
                        </Link>
                        {typeof doc.slug === "string" && doc.slug && (
                          <p className="text-xs text-zinc-400">/{doc.slug}</p>
                        )}
                      </td>
                      <td className="px-4 py-3">
                        <StatusBadge status={doc.contentStatus} />
                      </td>
                      <td className="px-4 py-3 text-zinc-500">{formatDate(doc.updatedAt)}</td>
                      <td className="px-4 py-3">
                        <div className="flex items-center justify-end gap-2">
                          {def.type === "events" && (
                            <button
                              onClick={() => router.push(`/events/${doc.id}/registrations`)}
                              className="rounded-md border border-zinc-200 px-2.5 py-1 text-xs font-medium text-zinc-600 hover:border-gold hover:text-gold-dark"
                            >
                              Registrations
                              {typeof doc.registeredCount === "number" ? ` (${doc.registeredCount})` : ""}
                            </button>
                          )}
                          <Link
                            href={`/content/${def.type}/${doc.id}`}
                            className="rounded-md border border-zinc-200 px-2.5 py-1 text-xs font-medium text-zinc-600 hover:border-zinc-400"
                          >
                            View
                          </Link>
                          <Link
                            href={`/content/${def.type}/${doc.id}/edit`}
                            className="rounded-md border border-zinc-200 px-2.5 py-1 text-xs font-medium text-zinc-600 hover:border-zinc-400"
                          >
                            Edit
                          </Link>
                          {def.canDelete && (
                            <button
                              onClick={() => setPendingDelete(doc)}
                              className="admin-danger-button rounded-md px-2.5 py-1 text-xs font-medium"
                            >
                              Delete
                            </button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <Pagination
                page={page}
                totalPages={totalPages}
                onPageChange={setPage}
                totalItems={visible.length}
                pageSize={PAGE_SIZE}
              />
            </Card>
          )}
        </>
      )}

      <ConfirmDialog
        isOpen={pendingDelete !== null}
        title={`Delete ${def.label.toLowerCase()}`}
        message={
          pendingDelete
            ? `Delete “${docTitle(pendingDelete)}”? This cannot be undone.${deleting ? " Deleting…" : ""}`
            : ""
        }
        confirmLabel={deleting ? "Deleting…" : "Delete"}
        variant="danger"
        onConfirm={confirmDelete}
        onCancel={() => !deleting && setPendingDelete(null)}
      />
    </div>
  );
}
