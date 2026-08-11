"use client";

import Link from "next/link";
import { useCallback, useEffect, useState } from "react";
import { api, asList, ApiError } from "@/lib/api";
import { CONTENT_TYPES, docTitle, formatDate, type ContentStatus, type Doc } from "@/lib/content";
import StatusBadge from "@/components/StatusBadge";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import ConfirmDialog from "@/components/ui/ConfirmDialog";
import { SkeletonTable } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";

interface QueueItem {
  type: string;
  typeLabel: string;
  doc: Doc;
}

const REVIEW_STATUSES: ContentStatus[] = ["ai-draft", "in-review"];

export default function ReviewPage() {
  const { showToast } = useToast();
  const [items, setItems] = useState<QueueItem[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busyKey, setBusyKey] = useState<string | null>(null);
  const [filter, setFilter] = useState<"all" | ContentStatus>("all");
  const [pendingAction, setPendingAction] = useState<{ item: QueueItem; status: ContentStatus } | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const results = await Promise.allSettled(CONTENT_TYPES.map((t) => api(t.endpoint)));
      const next: QueueItem[] = [];
      const failures: string[] = [];
      results.forEach((res, i) => {
        const t = CONTENT_TYPES[i];
        if (res.status === "fulfilled") {
          for (const doc of asList<Doc>(res.value)) {
            if (doc.contentStatus && REVIEW_STATUSES.includes(doc.contentStatus)) {
              next.push({ type: t.type, typeLabel: t.label, doc });
            }
          }
        } else {
          failures.push(t.labelPlural);
        }
      });
      next.sort((a, b) => (b.doc.updatedAt ?? "").localeCompare(a.doc.updatedAt ?? ""));
      setItems(next);
      if (failures.length) setError(`Some lists failed to load: ${failures.join(", ")}.`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load review queue.");
      setItems([]);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function confirmStatus() {
    if (!pendingAction) return;
    const { item, status: contentStatus } = pendingAction;
    const key = `${item.type}:${item.doc.id}`;
    setBusyKey(key);
    try {
      await api(`/api/admin/content/${item.type}/${item.doc.id}/status`, {
        method: "PATCH",
        body: { contentStatus },
      });
      setItems((prev) => (prev ? prev.filter((i) => `${i.type}:${i.doc.id}` !== key) : prev));
      showToast(
        contentStatus === "published"
          ? `Published “${docTitle(item.doc)}”.`
          : `Approved “${docTitle(item.doc)}”.`,
      );
      setPendingAction(null);
    } catch (err) {
      showToast(err instanceof ApiError ? err.message : "Status update failed.", "error");
    } finally {
      setBusyKey(null);
    }
  }

  const visible = items?.filter((i) => filter === "all" || i.doc.contentStatus === filter) ?? null;
  const counts = {
    all: items?.length ?? 0,
    "ai-draft": items?.filter((i) => i.doc.contentStatus === "ai-draft").length ?? 0,
    "in-review": items?.filter((i) => i.doc.contentStatus === "in-review").length ?? 0,
  };

  return (
    <div>
      <PageHeader
        title="Review Queue"
        subtitle="Pastor sign-off: approve AI drafts and submitted content before it goes live"
      >
        <button
          onClick={load}
          className="rounded-md border border-zinc-300 px-3 py-2 text-sm font-medium text-zinc-600 hover:border-zinc-400"
        >
          Refresh
        </button>
      </PageHeader>

      <div className="mb-4 flex gap-2">
        {(["all", "ai-draft", "in-review"] as const).map((f) => (
          <button
            key={f}
            onClick={() => setFilter(f)}
            className={`rounded-full px-3.5 py-1.5 text-sm font-medium transition ${
              filter === f ? "bg-sidebar text-white" : "border border-zinc-300 text-zinc-600 hover:border-zinc-400"
            }`}
          >
            {f === "all" ? "All" : f} ({counts[f]})
          </button>
        ))}
      </div>

      {error && <div className="mb-4"><ErrorBox message={error} onRetry={load} /></div>}
      {visible === null && !error && <SkeletonTable rows={5} cols={5} />}

      {visible !== null && visible.length === 0 && (
        <EmptyState title="Nothing waiting for review" hint="AI drafts and submitted content will appear here." />
      )}

      {visible !== null && visible.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[720px] text-left text-sm">
            <thead>
              <tr className="border-b border-zinc-200 text-xs uppercase tracking-wide text-zinc-500">
                <th className="px-4 py-3 font-medium">Content</th>
                <th className="px-4 py-3 font-medium">Type</th>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Updated</th>
                <th className="px-4 py-3 text-right font-medium">Sign-off</th>
              </tr>
            </thead>
            <tbody>
              {visible.map((item) => {
                const key = `${item.type}:${item.doc.id}`;
                const busy = busyKey === key;
                return (
                  <tr key={key} className="border-b border-zinc-100 last:border-0 hover:bg-zinc-50">
                    <td className="px-4 py-3">
                      <Link
                        href={`/content/${item.type}/${item.doc.id}`}
                        className="font-medium text-zinc-900 hover:text-gold-dark"
                      >
                        {docTitle(item.doc)}
                      </Link>
                      {typeof item.doc.slug === "string" && item.doc.slug && (
                        <p className="text-xs text-zinc-400">/{item.doc.slug}</p>
                      )}
                    </td>
                    <td className="px-4 py-3 text-zinc-600">{item.typeLabel}</td>
                    <td className="px-4 py-3">
                      <StatusBadge status={item.doc.contentStatus} />
                    </td>
                    <td className="px-4 py-3 text-zinc-500">{formatDate(item.doc.updatedAt)}</td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        <button
                          onClick={() => setPendingAction({ item, status: "approved" })}
                          disabled={busy}
                          className="admin-secondary-action rounded-md px-3 py-1.5 text-xs font-semibold disabled:opacity-50"
                        >
                          Approve
                        </button>
                        <button
                          onClick={() => setPendingAction({ item, status: "published" })}
                          disabled={busy}
                          className="rounded-md bg-gold px-3 py-1.5 text-xs font-semibold text-sidebar hover:bg-gold-dark disabled:opacity-50"
                        >
                          Publish
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </Card>
      )}

      <ConfirmDialog
        isOpen={pendingAction !== null}
        title={pendingAction?.status === "published" ? "Publish content" : "Approve content"}
        message={
          pendingAction
            ? pendingAction.status === "published"
              ? `Publish “${docTitle(pendingAction.item.doc)}” to the live site?`
              : `Approve “${docTitle(pendingAction.item.doc)}” for publication?`
            : ""
        }
        confirmLabel={
          busyKey
            ? "Working…"
            : pendingAction?.status === "published"
              ? "Publish"
              : "Approve"
        }
        variant="warning"
        onConfirm={confirmStatus}
        onCancel={() => !busyKey && setPendingAction(null)}
      />
    </div>
  );
}
