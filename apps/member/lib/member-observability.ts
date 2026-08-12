export type MemberClientEventType = "route-error" | "global-error" | "unhandled-error" | "unhandled-rejection" | "navigation-slow" | "connectivity-lost" | "connectivity-restored";

const routeNames = new Set(["account", "care", "community", "giving", "lead", "messages", "participation", "serving", "support"]);

export function coarseMemberRoute(path = typeof window === "undefined" ? "/" : window.location.pathname) {
  const first = path.split("/").filter(Boolean)[0] || "home";
  return routeNames.has(first) ? first : "unknown";
}

export function safeDigest(value?: string) {
  const normalized = (value || "").trim();
  return /^[A-Za-z0-9_-]{1,96}$/.test(normalized) ? normalized : "";
}

export function durationBucket(milliseconds: number) {
  if (milliseconds < 1000) return "under-1s";
  if (milliseconds < 3000) return "1-3s";
  if (milliseconds < 8000) return "3-8s";
  return "over-8s";
}

export function reportMemberEvent(type: MemberClientEventType, values: { digest?: string; durationBucket?: string; online?: boolean } = {}) {
  try {
    const body = JSON.stringify({ type, route: coarseMemberRoute(), digest: safeDigest(values.digest), durationBucket: values.durationBucket || "", online: values.online ?? navigator.onLine });
    void fetch("/api/member/client-events", { method: "POST", headers: { "Content-Type": "application/json" }, body, cache: "no-store", keepalive: true }).catch(() => undefined);
  } catch {
    // Observability must never interfere with member work.
  }
}
