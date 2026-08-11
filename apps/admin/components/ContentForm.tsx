"use client";

import Link from "next/link";
import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { api, ApiError } from "@/lib/api";
import {
  CONTENT_STATUSES,
  docTitle,
  slugify,
  toDateInput,
  toLocalInput,
  type ContentStatus,
  type ContentTypeDef,
  type Doc,
  type FieldDef,
} from "@/lib/content";
import { Card, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import DatePicker from "@/components/ui/DatePicker";
import { SkeletonText } from "@/components/ui/Skeleton";
import { useToast } from "@/components/ui/Toast";

type Values = Record<string, unknown>;

function initialValues(def: ContentTypeDef, doc?: Doc | null): Values {
  const v: Values = {};
  for (const f of def.fields) {
    const raw = doc?.[f.key];
    switch (f.type) {
      case "checkbox":
        v[f.key] = Boolean(raw);
        break;
      case "number":
        v[f.key] = raw === undefined || raw === null ? "" : String(raw);
        break;
      case "datetime":
        v[f.key] = toLocalInput(typeof raw === "string" ? raw : undefined);
        break;
      case "date":
        v[f.key] = toDateInput(typeof raw === "string" ? raw : undefined);
        break;
      case "stringlist":
        v[f.key] = Array.isArray(raw) ? (raw as string[]).join(", ") : "";
        break;
      default:
        v[f.key] = typeof raw === "string" ? raw : "";
    }
  }
  v.slug = typeof doc?.slug === "string" ? doc.slug : "";
  v.contentStatus = doc?.contentStatus ?? "in-review";
  return v;
}

export default function ContentForm({ def, docId }: { def: ContentTypeDef; docId?: string }) {
  const router = useRouter();
  const { showToast } = useToast();
  const isNew = !docId;
  const [values, setValues] = useState<Values>(() => initialValues(def));
  const [doc, setDoc] = useState<Doc | null>(null);
  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);

  useEffect(() => {
    if (isNew) return;
    api<Doc[]>(def.endpoint)
      .then(async (data) => {
        // The contract has no GET-by-id for admin content; find it in the list.
        const list = Array.isArray(data) ? data : ((data as unknown as { items?: Doc[] }).items ?? []);
        const found = list.find((d) => d.id === docId);
        if (!found) {
          setLoadError(`Could not find this ${def.label.toLowerCase()} — it may have been deleted.`);
          return;
        }
        setDoc(found);
        setValues(initialValues(def, found));
      })
      .catch((err) => setLoadError(err instanceof ApiError ? err.message : "Failed to load."))
      .finally(() => setLoading(false));
  }, [def, docId, isNew]);

  function set(key: string, value: unknown) {
    setValues((prev) => ({ ...prev, [key]: value }));
  }

  function buildPayload(): Record<string, unknown> {
    const payload: Record<string, unknown> = {};
    for (const f of def.fields) {
      const raw = values[f.key];
      switch (f.type) {
        case "checkbox":
          payload[f.key] = Boolean(raw);
          break;
        case "number": {
          if (raw === "" || raw === undefined || raw === null) break;
          const n = Number(raw);
          if (!Number.isNaN(n)) payload[f.key] = n;
          break;
        }
        case "datetime":
        case "date": {
          const s = typeof raw === "string" ? raw : "";
          if (s) {
            const d = new Date(s);
            if (!Number.isNaN(d.getTime())) payload[f.key] = d.toISOString();
          }
          break;
        }
        case "stringlist": {
          const s = typeof raw === "string" ? raw : "";
          payload[f.key] = s
            .split(",")
            .map((x) => x.trim())
            .filter(Boolean);
          break;
        }
        default: {
          const s = typeof raw === "string" ? raw.trim() : "";
          if (s) payload[f.key] = s;
        }
      }
    }

    const title =
      (typeof values.title === "string" && values.title.trim()) ||
      (typeof values.name === "string" && values.name.trim()) ||
      (typeof values.author === "string" && values.author.trim()) ||
      docTitle(doc ?? { id: "" });
    payload.title = title;

    const slug = typeof values.slug === "string" ? values.slug.trim() : "";
    payload.slug = slug || slugify(title);
    payload.contentStatus = values.contentStatus as ContentStatus;
    return payload;
  }

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setSaving(true);
    try {
      const payload = buildPayload();
      if (isNew) {
        await api(def.endpoint, { method: "POST", body: payload });
        showToast(`${def.label} created.`);
      } else {
        await api(`${def.endpoint}/${docId}`, { method: "PUT", body: payload });
        showToast(`${def.label} saved.`);
      }
      router.push(`/content/${def.type}`);
    } catch (err) {
      showToast(err instanceof ApiError ? err.message : "Save failed.", "error");
      setSaving(false);
    }
  }

  if (loading) {
    return (
      <div className="max-w-3xl">
        <BackToListing href={`/content/${def.type}`} label={def.labelPlural} />
        <Card className="p-6">
          <SkeletonText lines={6} />
        </Card>
      </div>
    );
  }
  if (loadError) {
    return (
      <div className="max-w-3xl">
        <BackToListing href={`/content/${def.type}`} label={def.labelPlural} />
        <div className="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">{loadError}</div>
      </div>
    );
  }

  return (
    <div className="max-w-3xl">
      <BackToListing href={`/content/${def.type}`} label={def.labelPlural} />
      <PageHeader
        title={isNew ? `New ${def.label}` : `Edit ${def.label}`}
        subtitle={isNew ? undefined : docTitle(doc ?? { id: "" })}
      />

      <form onSubmit={onSubmit}>
        <Card className="space-y-5 p-6">
          {def.fields.map((f) => (
            <Field key={f.key} field={f} value={values[f.key]} onChange={(v) => set(f.key, v)} />
          ))}

          <div className="grid gap-5 border-t border-zinc-100 pt-5 sm:grid-cols-2">
            <label className="block text-sm font-medium text-zinc-700">
              Slug
              <input
                type="text"
                value={typeof values.slug === "string" ? values.slug : ""}
                onChange={(e) => set("slug", e.target.value)}
                placeholder="auto-generated from title"
                className="mt-1.5 w-full rounded-md border border-zinc-300 px-3 py-2 text-sm outline-none focus:border-gold"
              />
            </label>
            <span className="block text-sm font-medium text-zinc-700">
              Content status
              <Select
                className="mt-1.5"
                value={values.contentStatus as string}
                onChange={(e) => set("contentStatus", e.target.value)}
              >
                {CONTENT_STATUSES.map((s) => (
                  <option key={s} value={s}>
                    {s}
                  </option>
                ))}
              </Select>
            </span>
          </div>
        </Card>

        <div className="mt-5 flex items-center gap-3">
          <button
            type="submit"
            disabled={saving}
            className="rounded-md bg-gold px-5 py-2.5 text-sm font-semibold text-sidebar transition hover:bg-gold-dark disabled:opacity-60"
          >
            {saving ? "Saving…" : isNew ? `Create ${def.label}` : "Save changes"}
          </button>
          <button
            type="button"
            onClick={() => router.push(`/content/${def.type}`)}
            className="rounded-md border border-zinc-300 px-4 py-2.5 text-sm font-medium text-zinc-600 hover:border-zinc-400"
          >
            Cancel
          </button>
        </div>
      </form>
    </div>
  );
}

function Field({ field, value, onChange }: { field: FieldDef; value: unknown; onChange: (v: unknown) => void }) {
  const base =
    "mt-1.5 w-full rounded-md border border-zinc-300 px-3 py-2 text-sm outline-none focus:border-gold bg-white";

  if (field.type === "checkbox") {
    return (
      <label className="flex items-center gap-3 text-sm font-medium text-zinc-700">
        <input
          type="checkbox"
          checked={Boolean(value)}
          onChange={(e) => onChange(e.target.checked)}
          className="h-4 w-4 rounded border-zinc-300 accent-[#c9a227]"
        />
        {field.label}
      </label>
    );
  }

  if (field.type === "image") {
    return (
      <div className="text-sm font-medium text-zinc-700">
        <span>{field.label}</span>
        <ImageUploadField value={typeof value === "string" ? value : ""} onChange={onChange} />
        {field.hint && <span className="mt-1 block text-xs font-normal text-zinc-400">{field.hint}</span>}
      </div>
    );
  }

  return (
    <label className="block text-sm font-medium text-zinc-700">
      {field.label}
      {field.required && <span className="ml-1 text-rose-500">*</span>}
      {field.type === "textarea" ? (
        <textarea
          rows={field.rows ?? 4}
          required={field.required}
          value={typeof value === "string" ? value : ""}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
          className={base}
        />
      ) : field.type === "select" ? (
        <Select
          required={field.required}
          value={typeof value === "string" ? value : ""}
          onChange={(e) => onChange(e.target.value)}
          className="mt-1.5"
        >
          <option value="">Select…</option>
          {(field.options ?? []).map((o) => (
            <option key={o} value={o}>
              {o}
            </option>
          ))}
        </Select>
      ) : field.type === "date" ? (
        <DatePicker
          clearable={!field.required}
          value={typeof value === "string" ? value : ""}
          onChange={(v) => onChange(v)}
          className="mt-1.5"
        />
      ) : field.type === "datetime" ? (
        <DateTimeInput
          required={field.required}
          value={typeof value === "string" ? value : ""}
          onChange={(v) => onChange(v)}
        />
      ) : (
        <input
          type={field.type === "number" ? "number" : field.type === "url" ? "url" : "text"}
          required={field.required}
          value={typeof value === "string" ? value : String(value ?? "")}
          onChange={(e) => onChange(e.target.value)}
          placeholder={field.placeholder}
          className={base}
        />
      )}
      {field.hint && <span className="mt-1 block text-xs font-normal text-zinc-400">{field.hint}</span>}
    </label>
  );
}

function BackToListing({ href, label }: { href: string; label: string }) {
  return (
    <Link
      href={href}
      className="mb-5 inline-flex items-center gap-2 text-sm font-semibold text-[#566159] transition hover:-translate-x-0.5 hover:text-[#8b681f] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--remi-gold)]"
    >
      <span aria-hidden="true">←</span> Back to {label.toLowerCase()}
    </Link>
  );
}

interface UploadSignature {
  cloudName: string;
  apiKey: string;
  timestamp: number;
  folder: string;
  signature: string;
}

function ImageUploadField({ value, onChange }: { value: string; onChange: (value: string) => void }) {
  const inputRef = useRef<HTMLInputElement>(null);
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);

  async function upload(file?: File) {
    if (!file) return;
    setError(null);
    if (!file.type.startsWith("image/")) {
      setError("Choose an image file.");
      return;
    }
    if (file.size > 10 * 1024 * 1024) {
      setError("The image must be smaller than 10 MB.");
      return;
    }

    setUploading(true);
    try {
      const signed = await api<UploadSignature>("/api/admin/uploads/signature", {
        method: "POST",
        body: { folder: "remi/leadership" },
      });
      const form = new FormData();
      form.append("file", file);
      form.append("api_key", signed.apiKey);
      form.append("timestamp", String(signed.timestamp));
      form.append("folder", signed.folder);
      form.append("signature", signed.signature);
      const response = await fetch(`https://api.cloudinary.com/v1_1/${signed.cloudName}/image/upload`, {
        method: "POST",
        body: form,
      });
      const result = (await response.json()) as { secure_url?: string; error?: { message?: string } };
      if (!response.ok || !result.secure_url) throw new Error(result.error?.message || "Cloudinary rejected the upload.");
      onChange(result.secure_url);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Image upload failed.");
    } finally {
      setUploading(false);
      if (inputRef.current) inputRef.current.value = "";
    }
  }

  return (
    <div
      className={`admin-upload-zone mt-2 overflow-hidden rounded-xl border border-dashed p-4 transition ${dragging ? "is-dragging border-[var(--remi-gold)] bg-[#f2ead4] shadow-[0_0_0_4px_rgba(209,173,85,.12)]" : "border-[#cfc9ba] bg-[#f8f6f0]"}`}
      onDragEnter={(event) => { event.preventDefault(); setDragging(true); }}
      onDragOver={(event) => { event.preventDefault(); event.dataTransfer.dropEffect = "copy"; }}
      onDragLeave={(event) => { if (!event.currentTarget.contains(event.relatedTarget as Node)) setDragging(false); }}
      onDrop={(event) => { event.preventDefault(); setDragging(false); upload(event.dataTransfer.files?.[0]); }}
    >
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <div className="admin-upload-preview grid aspect-[4/5] w-24 shrink-0 place-items-center overflow-hidden rounded-lg border border-[#dedbd2] bg-white text-xs text-zinc-400">
          {value ? (
            // A plain img supports arbitrary signed Cloudinary and existing external URLs.
            // eslint-disable-next-line @next/next/no-img-element
            <img src={value} alt="Leadership portrait preview" className="h-full w-full object-cover" />
          ) : (
            <span className="px-2 text-center">No image</span>
          )}
        </div>
        <div className="min-w-0 flex-1">
          <input
            ref={inputRef}
            type="file"
            accept="image/jpeg,image/png,image/webp,image/avif"
            className="sr-only"
            onChange={(event) => upload(event.target.files?.[0])}
          />
          <button
            type="button"
            disabled={uploading}
            onClick={() => inputRef.current?.click()}
            className="rounded-md bg-[var(--remi-green)] px-4 py-2.5 text-sm font-semibold text-white transition hover:bg-[#29483a] active:translate-y-px disabled:cursor-wait disabled:opacity-60"
          >
            {uploading ? "Uploading…" : value ? "Replace image" : "Upload image"}
          </button>
          {value && (
            <button type="button" onClick={() => onChange("")} className="ml-3 text-sm font-medium text-zinc-500 hover:text-rose-700">
              Remove
            </button>
          )}
          <p className="mt-2 text-xs leading-5 text-zinc-500">Drag and drop here, or choose a JPG, PNG, WebP or AVIF. Maximum 10 MB.</p>
        </div>
      </div>
      <label className="mt-4 block text-xs font-medium text-zinc-600">
        Or paste an image URL
        <input
          type="url"
          value={value}
          onChange={(event) => onChange(event.target.value)}
          placeholder="https://…"
          className="mt-1.5 w-full rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm outline-none focus:border-gold"
        />
      </label>
      {error && <p role="alert" className="mt-3 text-xs font-medium text-rose-700">{error}</p>}
    </div>
  );
}

/** DatePicker + branded time text input; value stays in "yyyy-MM-ddTHH:mm" (or "" ). */
function DateTimeInput({
  value,
  onChange,
  required,
}: {
  value: string;
  onChange: (v: string) => void;
  required?: boolean;
}) {
  const [datePart, timePart] = value.includes("T")
    ? value.split("T")
    : /^\d{4}-\d{2}-\d{2}$/.test(value)
      ? [value, ""]
      : ["", value];

  function setDate(d: string) {
    if (!d) {
      onChange("");
    } else {
      onChange(`${d}T${/^\d{2}:\d{2}$/.test(timePart) ? timePart : "09:00"}`);
    }
  }

  function setTime(t: string) {
    const clean = t.replace(/[^\d:]/g, "").slice(0, 5);
    if (!datePart) return;
    onChange(clean ? `${datePart}T${clean}` : datePart);
  }

  function normalizeTime() {
    const m = /^(\d{1,2}):(\d{2})$/.exec(timePart);
    if (m && datePart) {
      onChange(`${datePart}T${m[1].padStart(2, "0")}:${m[2]}`);
    }
  }

  return (
    <div className="relative mt-1.5 flex items-stretch gap-2">
      <DatePicker clearable={!required} value={datePart} onChange={setDate} className="min-w-0 flex-1" />
      <input
        type="text"
        inputMode="numeric"
        placeholder="HH:MM"
        aria-label="Time (24h)"
        value={timePart}
        disabled={!datePart}
        onChange={(e) => setTime(e.target.value)}
        onBlur={normalizeTime}
        className="w-24 rounded-md border border-zinc-300 bg-white px-3 py-2 text-sm outline-none transition focus:border-gold disabled:opacity-50"
      />
      {required && !value ? (
        <input className="pointer-events-none absolute h-px w-px opacity-0" required tabIndex={-1} aria-hidden="true" />
      ) : null}
    </div>
  );
}
