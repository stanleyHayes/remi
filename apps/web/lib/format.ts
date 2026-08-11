export function formatDate(iso: string | undefined | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return new Intl.DateTimeFormat("en-GB", {
    weekday: "short",
    day: "numeric",
    month: "long",
    year: "numeric",
  }).format(d);
}

export function formatTime(iso: string | undefined | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return new Intl.DateTimeFormat("en-GB", {
    hour: "numeric",
    minute: "2-digit",
  }).format(d);
}

/** Convert a YouTube/Vimeo watch URL into an embeddable URL, or return null. */
export function toEmbedUrl(url: string | undefined | null): string | null {
  if (!url) return null;
  try {
    const u = new URL(url);
    if (u.hostname.includes("youtube.com")) {
      const id = u.searchParams.get("v") ?? u.pathname.match(/\/(embed|shorts|live)\/([^/?]+)/)?.[2];
      return id ? `https://www.youtube.com/embed/${id}` : null;
    }
    if (u.hostname === "youtu.be") {
      const id = u.pathname.replace(/^\//, "");
      return id ? `https://www.youtube.com/embed/${id}` : null;
    }
    if (u.hostname.includes("vimeo.com")) {
      const id = u.pathname.replace(/^\//, "");
      return id ? `https://player.vimeo.com/video/${id}` : null;
    }
    return url;
  } catch {
    return null;
  }
}

export function mapsUrl(mapQuery: string | undefined | null, fallback: string): string {
  const q = mapQuery || fallback;
  return `https://www.google.com/maps/search/?api=1&query=${encodeURIComponent(q)}`;
}

export function img(url: string | undefined | null, seed: string): string {
  return url && url.length > 0 ? url : `https://picsum.photos/seed/${seed}/1600/900`;
}
