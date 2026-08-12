import { durationBucket, reportMemberEvent } from "@/lib/member-observability";

try {
  window.addEventListener("error", () => reportMemberEvent("unhandled-error"));
  window.addEventListener("unhandledrejection", () => reportMemberEvent("unhandled-rejection"));
  window.addEventListener("load", () => {
    const navigation = performance.getEntriesByType("navigation")[0] as PerformanceNavigationTiming | undefined;
    if (navigation && navigation.duration >= 3000) reportMemberEvent("navigation-slow", { durationBucket: durationBucket(navigation.duration) });
  }, { once: true });
} catch {
  // Early instrumentation is intentionally fail-safe.
}
