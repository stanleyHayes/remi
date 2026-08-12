"use client";

import { useEffect } from "react";
import { reportMemberEvent } from "@/lib/member-observability";

export default function ErrorPage({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => { reportMemberEvent("route-error", { digest: error.digest }); }, [error]);
  return <main className="member-error-page"><span>MY REMI · RECOVERY</span><h1>That page lost its place.</h1><p>Your information is safe. Try loading the workspace again, or return home and choose another path.</p><div><button onClick={reset}>Try again</button><a href="/">Return home</a></div></main>;
}
