import { apiBaseUrl as API_URL } from "./env";
import type {
  Announcement,
  Branch,
  EventItem,
  Leader,
  Ministry,
  Page,
  Sermon,
  SermonListResponse,
  Settings,
  Testimony,
} from "./types";

/**
 * Server-side fetch against the REMI API. Every call degrades gracefully:
 * network failure or non-2xx returns `null` so pages can render empty states
 * instead of crashing (the API may simply not be running during the demo).
 */
async function get<T>(path: string): Promise<T | null> {
  try {
    const res = await fetch(`${API_URL}${path}`, {
      next: { revalidate: 30 },
    });
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
}

export async function getSettings(): Promise<Settings> {
  const remote = await get<Partial<Settings>>("/api/settings");
  return {
    ...FALLBACK_SETTINGS,
    ...remote,
    serviceTimes: Array.isArray(remote?.serviceTimes) ? remote.serviceTimes : FALLBACK_SETTINGS.serviceTimes,
    socials: remote?.socials && typeof remote.socials === "object" ? remote.socials : FALLBACK_SETTINGS.socials,
    givingCategories: Array.isArray(remote?.givingCategories)
      ? remote.givingCategories
      : FALLBACK_SETTINGS.givingCategories,
  };
}

export function getPage(pageKey: string): Promise<Page | null> {
  return get<Page>(`/api/pages/${pageKey}`);
}

export function getLeadership(): Promise<Leader[] | null> {
  return get<Leader[]>("/api/leadership");
}

export function getMinistries(): Promise<Ministry[] | null> {
  return get<Ministry[]>("/api/ministries");
}

export function getMinistry(slug: string): Promise<Ministry | null> {
  return get<Ministry>(`/api/ministries/${slug}`);
}

export function getBranches(): Promise<Branch[] | null> {
  return get<Branch[]>("/api/branches");
}

export interface SermonQuery {
  preacher?: string;
  series?: string;
  topic?: string;
  q?: string;
  page?: string;
  pageSize?: string;
}

export function getSermons(query: SermonQuery = {}): Promise<SermonListResponse | null> {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value) params.set(key, value);
  }
  const qs = params.toString();
  return get<SermonListResponse>(`/api/sermons${qs ? `?${qs}` : ""}`);
}

export function getSermon(slug: string): Promise<Sermon | null> {
  return get<Sermon>(`/api/sermons/${slug}`);
}

export function getEvents(when: "upcoming" | "past" = "upcoming"): Promise<EventItem[] | null> {
  return get<EventItem[]>(`/api/events?when=${when}`);
}

export function getEvent(slug: string): Promise<EventItem | null> {
  return get<EventItem>(`/api/events/${slug}`);
}

export function getAnnouncements(): Promise<Announcement[] | null> {
  return get<Announcement[]>("/api/announcements");
}

export function getTestimonies(): Promise<Testimony[] | null> {
  return get<Testimony[]>("/api/testimonies");
}

/** Fallback site identity, used when the API is unreachable. */
export const FALLBACK_SETTINGS: Settings = {
  churchName: "Ruach Elohim Ministries International",
  tagline: "Where the Spirit breathes life",
  serviceTimes: [
    { name: "Sunday Celebration Service", day: "Sunday", time: "9:00 AM", location: "Main Auditorium, Accra" },
    { name: "Midweek Prophetic Service", day: "Wednesday", time: "6:30 PM", location: "Main Auditorium, Accra" },
  ],
  socials: {},
  isLive: false,
  givingCategories: ["Tithe", "Offering", "Seed", "Building Fund", "Missions"],
  nextServiceOverride: null,
};
