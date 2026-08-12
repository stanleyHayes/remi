"use client";

import { useEffect } from "react";
import { reportMemberEvent } from "@/lib/member-observability";

export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => { reportMemberEvent("global-error", { digest: error.digest }); }, [error]);
  return <html lang="en"><body><main className="member-error-page member-global-error"><span>MY REMI · SAFE RECOVERY</span><h1>We could not open your space.</h1><p>No action was completed. Try once more or return after checking your connection.</p><div><button onClick={reset}>Open My REMI again</button><a href="/sign-in">Return to sign in</a></div></main></body></html>;
}
