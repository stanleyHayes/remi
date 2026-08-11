"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { useEffect, useState } from "react";
import { api, asList, ApiError } from "@/lib/api";
import { docTitle, formatDate, getContentType, type Doc, type FieldDef } from "@/lib/content";
import StatusBadge from "@/components/StatusBadge";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { SkeletonText } from "@/components/ui/Skeleton";

export default function ContentDetailPage() {
  const params = useParams<{ type: string; id: string }>();
  const def = getContentType(params.type);
  const [doc, setDoc] = useState<Doc | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!def) return;
    setLoading(true);
    setError(null);
    api(def.endpoint)
      .then((data) => {
        const found = asList<Doc>(data).find((item) => item.id === params.id);
        if (!found) throw new Error(`Could not find this ${def.label.toLowerCase()}.`);
        setDoc(found);
      })
      .catch((err) => setError(err instanceof ApiError || err instanceof Error ? err.message : "Failed to load."))
      .finally(() => setLoading(false));
  }, [def, params.id]);

  if (!def) return <EmptyState title="Unknown content type" />;

  if (loading) {
    return <div className="max-w-5xl"><BackLink href={`/content/${def.type}`} label={def.labelPlural} /><Card className="p-7"><SkeletonText lines={8} /></Card></div>;
  }

  if (error || !doc) {
    return <div className="max-w-5xl"><BackLink href={`/content/${def.type}`} label={def.labelPlural} /><ErrorBox message={error || "Content not found."} /></div>;
  }

  const imageField = def.fields.find((field) => field.type === "image" || ["photo", "image", "heroImage"].includes(field.key));
  const imageUrl = imageField && typeof doc[imageField.key] === "string" ? String(doc[imageField.key]) : "";
  const fields = def.fields.filter((field) => field.key !== imageField?.key);

  return (
    <article className="max-w-5xl">
      <BackLink href={`/content/${def.type}`} label={def.labelPlural} />
      <PageHeader title={docTitle(doc)} subtitle={`${def.label} details`}>
        <Link href={`/content/${def.type}/${doc.id}/edit`} className="rounded-md bg-gold px-4 py-2 text-sm font-semibold text-sidebar transition hover:bg-gold-dark active:translate-y-px">
          Edit {def.label.toLowerCase()}
        </Link>
      </PageHeader>

      <div className={`grid gap-5 ${imageUrl ? "lg:grid-cols-[minmax(0,1fr)_18rem]" : ""}`}>
        <Card className="overflow-hidden">
          <div className="flex flex-wrap items-center gap-3 border-b border-zinc-100 px-6 py-4">
            <StatusBadge status={doc.contentStatus} />
            {typeof doc.slug === "string" && doc.slug && <span className="font-mono text-xs text-zinc-400">/{doc.slug}</span>}
          </div>
          <dl className="divide-y divide-zinc-100 px-6">
            {fields.map((field) => <DetailField key={field.key} field={field} value={doc[field.key]} />)}
          </dl>
        </Card>

        {imageUrl && imageField && (
          <aside>
            <Card className="overflow-hidden p-3">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img src={imageUrl} alt={`${docTitle(doc)} — ${imageField.label}`} className="aspect-[4/5] w-full rounded-lg object-cover" />
              <p className="px-2 pb-1 pt-3 text-xs font-semibold uppercase tracking-[.12em] text-zinc-400">{imageField.label}</p>
            </Card>
          </aside>
        )}
      </div>

      <footer className="mt-5 grid gap-3 rounded-xl border border-[#dedbd2] bg-[#f8f6f0] p-5 text-xs text-zinc-500 sm:grid-cols-3">
        <Meta label="Record ID" value={doc.id} mono />
        <Meta label="Created" value={formatDate(doc.createdAt)} />
        <Meta label="Last updated" value={formatDate(doc.updatedAt)} />
      </footer>
    </article>
  );
}

function BackLink({ href, label }: { href: string; label: string }) {
  return <Link href={href} className="mb-5 inline-flex items-center gap-2 text-sm font-semibold text-[#566159] transition hover:-translate-x-0.5 hover:text-[#8b681f]">← Back to {label.toLowerCase()}</Link>;
}

function DetailField({ field, value }: { field: FieldDef; value: unknown }) {
  if (value === undefined || value === null || value === "" || (Array.isArray(value) && value.length === 0)) return null;
  let content: React.ReactNode;
  if (field.type === "checkbox") content = value ? "Yes" : "No";
  else if (field.type === "date" || field.type === "datetime") content = formatDate(String(value));
  else if (field.type === "stringlist" && Array.isArray(value)) content = <span className="flex flex-wrap gap-2">{value.map((item) => <span key={String(item)} className="rounded-md bg-[#f1eee5] px-2 py-1 text-xs text-zinc-700">{String(item)}</span>)}</span>;
  else if (field.type === "url") content = <a href={String(value)} target="_blank" rel="noreferrer" className="break-all font-medium text-[#8b681f] underline decoration-[#d1ad55] underline-offset-4">{String(value)}</a>;
  else content = <span className={field.type === "textarea" ? "whitespace-pre-wrap leading-7" : ""}>{String(value)}</span>;
  return <div className="grid gap-2 py-5 sm:grid-cols-[10rem_minmax(0,1fr)]"><dt className="text-xs font-bold uppercase tracking-[.12em] text-zinc-400">{field.label}</dt><dd className="min-w-0 text-sm text-zinc-800">{content}</dd></div>;
}

function Meta({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return <div><span className="block font-semibold uppercase tracking-[.1em] text-zinc-400">{label}</span><span className={`mt-1 block break-all text-zinc-700 ${mono ? "font-mono" : ""}`}>{value}</span></div>;
}
