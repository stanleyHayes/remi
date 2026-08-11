import type { MetadataRoute } from "next";
import { getEvents, getMinistries, getSermons } from "@/lib/api";
import { absoluteUrl } from "@/lib/seo";

const STATIC_ROUTES: Array<{
  path: string;
  changeFrequency: "weekly" | "monthly" | "yearly";
  priority: number;
}> = [
  { path: "/", changeFrequency: "weekly", priority: 1 },
  { path: "/about", changeFrequency: "yearly", priority: 0.8 },
  { path: "/about/mission", changeFrequency: "yearly", priority: 0.8 },
  { path: "/about/beliefs", changeFrequency: "yearly", priority: 0.8 },
  { path: "/leadership", changeFrequency: "monthly", priority: 0.8 },
  { path: "/sermons", changeFrequency: "weekly", priority: 0.9 },
  { path: "/events", changeFrequency: "weekly", priority: 0.9 },
  { path: "/ministries", changeFrequency: "monthly", priority: 0.8 },
  { path: "/branches", changeFrequency: "monthly", priority: 0.8 },
  { path: "/live", changeFrequency: "weekly", priority: 0.9 },
  { path: "/gallery", changeFrequency: "monthly", priority: 0.7 },
  { path: "/connect/visit", changeFrequency: "monthly", priority: 0.8 },
  { path: "/connect/contact", changeFrequency: "yearly", priority: 0.6 },
  { path: "/connect/prayer", changeFrequency: "yearly", priority: 0.6 },
  { path: "/connect/testimony", changeFrequency: "yearly", priority: 0.5 },
  { path: "/give", changeFrequency: "yearly", priority: 0.6 },
];

function validDate(value?: string): Date | undefined {
  if (!value) return undefined;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date;
}

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const [sermons, upcomingEvents, pastEvents, ministries] = await Promise.all([
    getSermons({ page: "1", pageSize: "100" }),
    getEvents("upcoming"),
    getEvents("past"),
    getMinistries(),
  ]);

  const staticEntries: MetadataRoute.Sitemap = STATIC_ROUTES.map((route) => ({
    url: absoluteUrl(route.path),
    changeFrequency: route.changeFrequency,
    priority: route.priority,
  }));
  const sermonEntries: MetadataRoute.Sitemap = (sermons?.items ?? []).map((sermon) => ({
    url: absoluteUrl(`/sermons/${sermon.slug}`),
    lastModified: validDate(sermon.updatedAt || sermon.date),
    changeFrequency: "monthly",
    priority: 0.7,
    images: sermon.image ? [sermon.image] : undefined,
  }));
  const eventEntries: MetadataRoute.Sitemap = [...(upcomingEvents ?? []), ...(pastEvents ?? [])].map((event) => ({
    url: absoluteUrl(`/events/${event.slug}`),
    lastModified: validDate(event.updatedAt),
    changeFrequency: "weekly",
    priority: 0.7,
    images: event.image ? [event.image] : undefined,
  }));
  const ministryEntries: MetadataRoute.Sitemap = (ministries ?? []).map((ministry) => ({
    url: absoluteUrl(`/ministries/${ministry.slug}`),
    lastModified: validDate(ministry.updatedAt),
    changeFrequency: "monthly",
    priority: 0.7,
    images: ministry.image ? [ministry.image] : undefined,
  }));

  return [...staticEntries, ...sermonEntries, ...eventEntries, ...ministryEntries];
}
