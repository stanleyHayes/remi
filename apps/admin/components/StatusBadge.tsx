import type { ContentStatus } from "@/lib/content";

const STYLES: Record<string, string> = {
  "ai-draft": "bg-violet-100 text-violet-800 ring-violet-200",
  "in-review": "bg-amber-100 text-amber-800 ring-amber-200",
  approved: "bg-sky-100 text-sky-800 ring-sky-200",
  published: "bg-emerald-100 text-emerald-800 ring-emerald-200",
  new: "bg-rose-100 text-rose-800 ring-rose-200",
  "in-prayer": "bg-amber-100 text-amber-800 ring-amber-200",
  closed: "bg-zinc-200 text-zinc-700 ring-zinc-300",
  read: "bg-sky-100 text-sky-800 ring-sky-200",
  archived: "bg-zinc-200 text-zinc-600 ring-zinc-300",
  confirmed: "bg-emerald-100 text-emerald-800 ring-emerald-200",
  pending: "bg-amber-100 text-amber-800 ring-amber-200",
  active: "bg-emerald-100 text-emerald-800 ring-emerald-200",
};

export default function StatusBadge({ status }: { status?: string }) {
  const s = (status ?? "unknown") as ContentStatus | string;
  const cls = STYLES[s] ?? "bg-zinc-100 text-zinc-700 ring-zinc-200";
  return (
    <span className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ring-1 ring-inset ${cls}`}>
      {s}
    </span>
  );
}
