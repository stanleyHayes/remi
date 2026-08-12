"use client";

import { useEffect, useState } from "react";
import { reportMemberEvent } from "@/lib/member-observability";

export function MemberRuntime() {
  const [offline, setOffline] = useState(false);
  const [restored, setRestored] = useState(false);

  useEffect(() => {
    const update = () => {
      const next = !navigator.onLine;
      setOffline(next);
      reportMemberEvent(next ? "connectivity-lost" : "connectivity-restored", { online: !next });
      if (!next) {
        setRestored(true);
        window.setTimeout(() => setRestored(false), 3500);
      }
    };
    setOffline(!navigator.onLine);
    window.addEventListener("online", update);
    window.addEventListener("offline", update);
    if ("serviceWorker" in navigator && window.isSecureContext) {
      void navigator.serviceWorker.register("/sw.js", { scope: "/", updateViaCache: "none" });
    }
    return () => { window.removeEventListener("online", update); window.removeEventListener("offline", update); };
  }, []);

  if (!offline && !restored) return null;
  return <div className={`member-connectivity ${offline ? "offline" : "restored"}`} role="status" aria-live="polite"><i/><span><b>{offline ? "You’re offline" : "Connection restored"}</b><small>{offline ? "You can keep reading this screen. Changes need a connection and are not queued." : "Fresh information and actions are available again."}</small></span></div>;
}
