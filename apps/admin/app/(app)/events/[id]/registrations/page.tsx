"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { api, asList, ApiError } from "@/lib/api";
import { docTitle, formatDate, type Doc } from "@/lib/content";
import { Card, EmptyState, ErrorBox, Loading, PageHeader } from "@/components/ui";

interface Registration {
  id: string;
  eventId?: string;
  name?: string;
  email?: string;
  phone?: string;
  createdAt?: string;
}

export default function EventRegistrationsPage() {
  const params = useParams<{ id: string }>();
  const eventId = params.id;

  const [event, setEvent] = useState<Doc | null>(null);
  const [regs, setRegs] = useState<Registration[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const [eventsData, regsData] = await Promise.all([
        api("/api/admin/events"),
        api(`/api/admin/registrations?eventId=${encodeURIComponent(eventId)}`),
      ]);
      const found = asList<Doc>(eventsData).find((e) => e.id === eventId) ?? null;
      setEvent(found);
      setRegs(asList<Registration>(regsData));
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to load registrations.");
      setRegs([]);
    }
  }, [eventId]);

  useEffect(() => {
    load();
  }, [load]);

  function exportCsv() {
    if (!regs || regs.length === 0) return;
    const escape = (v: string) => (/[",\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v);
    const rows = [
      ["Name", "Email", "Phone", "Registered At"],
      ...regs.map((r) => [r.name ?? "", r.email ?? "", r.phone ?? "", r.createdAt ?? ""]),
    ];
    const csv = rows.map((row) => row.map((c) => escape(String(c))).join(",")).join("\n");
    const blob = new Blob([csv], { type: "text/csv;charset=utf-8" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `registrations-${event?.slug ?? eventId}.csv`;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }

  return (
    <div>
      <PageHeader
        title={event ? `Registrations — ${docTitle(event)}` : "Event registrations"}
        subtitle={
          event
            ? `${formatDate(typeof event.startAt === "string" ? event.startAt : undefined)}${
                typeof event.capacity === "number" && event.capacity > 0
                  ? ` · ${regs?.length ?? 0} / ${event.capacity} registered`
                  : ""
              }`
            : undefined
        }
      >
        <Link
          href="/content/events"
          className="rounded-md border border-zinc-300 px-3 py-2 text-sm font-medium text-zinc-600 hover:border-zinc-400"
        >
          ← Back to events
        </Link>
        <button
          onClick={exportCsv}
          disabled={!regs || regs.length === 0}
          className="rounded-md bg-gold px-4 py-2 text-sm font-semibold text-sidebar transition hover:bg-gold-dark disabled:opacity-50"
        >
          Export CSV
        </button>
      </PageHeader>

      {error && <ErrorBox message={error} onRetry={load} />}
      {regs === null && !error && <Loading />}

      {regs !== null && regs.length === 0 && !error && (
        <EmptyState title="No registrations yet" hint="People who register on the website will appear here." />
      )}

      {regs !== null && regs.length > 0 && (
        <Card className="overflow-x-auto">
          <table className="w-full min-w-[560px] text-left text-sm">
            <thead>
              <tr className="border-b border-zinc-200 text-xs uppercase tracking-wide text-zinc-500">
                <th className="px-4 py-3 font-medium">Name</th>
                <th className="px-4 py-3 font-medium">Email</th>
                <th className="px-4 py-3 font-medium">Phone</th>
                <th className="px-4 py-3 font-medium">Registered</th>
              </tr>
            </thead>
            <tbody>
              {regs.map((r) => (
                <tr key={r.id} className="border-b border-zinc-100 last:border-0 hover:bg-zinc-50">
                  <td className="px-4 py-3 font-medium text-zinc-900">{r.name || "—"}</td>
                  <td className="px-4 py-3 text-zinc-600">{r.email || "—"}</td>
                  <td className="px-4 py-3 text-zinc-600">{r.phone || "—"}</td>
                  <td className="px-4 py-3 text-zinc-500">{formatDate(r.createdAt)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </Card>
      )}
    </div>
  );
}
