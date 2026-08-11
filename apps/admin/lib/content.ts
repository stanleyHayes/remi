export type ContentStatus = "ai-draft" | "in-review" | "approved" | "published";

export const CONTENT_STATUSES: ContentStatus[] = ["ai-draft", "in-review", "approved", "published"];

export interface Doc {
  id: string;
  title?: string;
  slug?: string;
  contentStatus?: ContentStatus;
  createdAt?: string;
  updatedAt?: string;
  [key: string]: unknown;
}

export type FieldType =
  | "text"
  | "image"
  | "url"
  | "textarea"
  | "number"
  | "checkbox"
  | "date"
  | "datetime"
  | "stringlist"
  | "select";

export interface FieldDef {
  key: string;
  label: string;
  type: FieldType;
  required?: boolean;
  placeholder?: string;
  hint?: string;
  options?: string[]; // for select
  rows?: number; // for textarea
}

export interface ContentTypeDef {
  type: string; // url segment, also used for /api/admin/content/:type/:id/status
  label: string;
  labelPlural: string;
  endpoint: string; // e.g. /api/admin/pages
  canCreate: boolean;
  canDelete: boolean;
  fields: FieldDef[];
}

export const CONTENT_TYPES: ContentTypeDef[] = [
  {
    type: "pages",
    label: "Page",
    labelPlural: "Pages",
    endpoint: "/api/admin/pages",
    canCreate: true,
    canDelete: false,
    fields: [
      { key: "title", label: "Title", type: "text", required: true },
      {
        key: "pageKey",
        label: "Page key",
        type: "select",
        required: true,
        options: ["history", "mission", "beliefs", "visit", "about"],
      },
      { key: "subtitle", label: "Subtitle", type: "text" },
      { key: "heroImage", label: "Hero image URL", type: "url", hint: "Paste an image URL (Cloudinary not required)." },
      { key: "body", label: "Body (Markdown / HTML)", type: "textarea", rows: 12 },
    ],
  },
  {
    type: "leadership",
    label: "Leader",
    labelPlural: "Leadership",
    endpoint: "/api/admin/leadership",
    canCreate: true,
    canDelete: true,
    fields: [
      { key: "name", label: "Name", type: "text", required: true },
      { key: "position", label: "Position", type: "text" },
      { key: "bio", label: "Bio", type: "textarea", rows: 6 },
      {
        key: "photo",
        label: "Profile image",
        type: "image",
        hint: "Upload a portrait or paste an existing image URL.",
      },
      { key: "order", label: "Display order", type: "number" },
      { key: "isFounder", label: "Founder", type: "checkbox" },
    ],
  },
  {
    type: "ministries",
    label: "Ministry",
    labelPlural: "Ministries",
    endpoint: "/api/admin/ministries",
    canCreate: true,
    canDelete: true,
    fields: [
      { key: "name", label: "Name", type: "text", required: true },
      { key: "description", label: "Description", type: "textarea", rows: 6 },
      { key: "image", label: "Image URL", type: "url" },
      { key: "leaderName", label: "Leader name", type: "text" },
      { key: "meetingTime", label: "Meeting time", type: "text", placeholder: "e.g. Sundays 2pm" },
    ],
  },
  {
    type: "branches",
    label: "Branch",
    labelPlural: "Branches",
    endpoint: "/api/admin/branches",
    canCreate: true,
    canDelete: true,
    fields: [
      { key: "name", label: "Name", type: "text", required: true },
      { key: "address", label: "Address", type: "text" },
      { key: "city", label: "City", type: "text" },
      { key: "phone", label: "Phone", type: "text" },
      { key: "email", label: "Email", type: "text" },
      { key: "pastorName", label: "Pastor name", type: "text" },
      { key: "serviceTimes", label: "Service times", type: "text", placeholder: "e.g. Sun 9am & 11am" },
      { key: "mapQuery", label: "Map query", type: "text", hint: "Google Maps search string for this branch." },
      { key: "image", label: "Image URL", type: "url" },
    ],
  },
  {
    type: "sermons",
    label: "Sermon",
    labelPlural: "Sermons",
    endpoint: "/api/admin/sermons",
    canCreate: true,
    canDelete: true,
    fields: [
      { key: "title", label: "Title", type: "text", required: true },
      { key: "preacher", label: "Preacher", type: "text" },
      { key: "series", label: "Series", type: "text" },
      { key: "topic", label: "Topic", type: "text" },
      { key: "bibleRefs", label: "Bible references", type: "stringlist", hint: "Comma-separated, e.g. John 3:16, Romans 8:28" },
      { key: "date", label: "Date preached", type: "date" },
      { key: "videoUrl", label: "Video URL", type: "url" },
      { key: "audioUrl", label: "Audio URL", type: "url" },
      { key: "notesUrl", label: "Notes URL", type: "url" },
      { key: "image", label: "Image URL", type: "url" },
      { key: "description", label: "Description", type: "textarea", rows: 5 },
    ],
  },
  {
    type: "events",
    label: "Event",
    labelPlural: "Events",
    endpoint: "/api/admin/events",
    canCreate: true,
    canDelete: true,
    fields: [
      { key: "title", label: "Title", type: "text", required: true },
      { key: "description", label: "Description", type: "textarea", rows: 6 },
      { key: "startAt", label: "Starts at", type: "datetime", required: true },
      { key: "endAt", label: "Ends at", type: "datetime" },
      { key: "location", label: "Location", type: "text" },
      { key: "image", label: "Image URL", type: "url" },
      { key: "registrationEnabled", label: "Enable registration", type: "checkbox" },
      { key: "capacity", label: "Capacity (0 = unlimited)", type: "number" },
    ],
  },
  {
    type: "announcements",
    label: "Announcement",
    labelPlural: "Announcements",
    endpoint: "/api/admin/announcements",
    canCreate: true,
    canDelete: true,
    fields: [
      { key: "title", label: "Title", type: "text", required: true },
      { key: "body", label: "Body", type: "textarea", rows: 6 },
      { key: "publishAt", label: "Publish at", type: "datetime" },
      { key: "expiresAt", label: "Expires at", type: "datetime" },
    ],
  },
  {
    type: "testimonies",
    label: "Testimony",
    labelPlural: "Testimonies",
    endpoint: "/api/admin/testimonies",
    canCreate: false,
    canDelete: true,
    fields: [
      { key: "author", label: "Author", type: "text", required: true },
      { key: "body", label: "Testimony", type: "textarea", rows: 8 },
      { key: "image", label: "Image URL", type: "url" },
    ],
  },
];

export function getContentType(type: string): ContentTypeDef | undefined {
  return CONTENT_TYPES.find((t) => t.type === type);
}

export function docTitle(doc: Doc): string {
  const t = doc.title ?? doc.name ?? doc.author;
  return typeof t === "string" && t.trim() ? t : "(untitled)";
}

export function slugify(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9\s-]/g, "")
    .replace(/[\s_]+/g, "-")
    .replace(/-+/g, "-")
    .replace(/^-|-$/g, "");
}

export function formatDate(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  return d.toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

/** RFC3339 -> "yyyy-MM-ddTHH:mm" (the value format used by the DatePicker/DateTimeInput fields). */
export function toLocalInput(iso?: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function toDateInput(iso?: string): string {
  return toLocalInput(iso).slice(0, 10);
}
