"use client";

import { useEffect, useState } from "react";
import { api, ApiError } from "@/lib/api";
import { Card, Loading, PageHeader } from "@/components/ui";

type State =
  | { kind: "loading" }
  | { kind: "unconfigured"; message: string }
  | { kind: "configured"; detail: string }
  | { kind: "error"; message: string };

export default function MediaPage() {
  const [state, setState] = useState<State>({ kind: "loading" });

  useEffect(() => {
    api<Record<string, unknown>>("/api/admin/uploads/signature", { method: "POST", body: {} })
      .then((data) =>
        setState({
          kind: "configured",
          detail: `Signed uploads are available (cloud: ${String(data?.cloudName ?? data?.cloud_name ?? "configured")}).`,
        })
      )
      .catch((err) => {
        if (err instanceof ApiError && err.status === 501) {
          setState({ kind: "unconfigured", message: err.message });
        } else {
          setState({ kind: "error", message: err instanceof ApiError ? err.message : "Failed to check uploads." });
        }
      });
  }, []);

  return (
    <div className="max-w-2xl">
      <PageHeader title="Media" subtitle="Image uploads via Cloudinary" />

      {state.kind === "loading" && <Loading label="Checking upload configuration…" />}

      {state.kind === "unconfigured" && (
        <Card className="p-8 text-center">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-amber-100 text-xl">☁</div>
          <h2 className="mt-4 text-base font-semibold text-zinc-900">Cloudinary not configured</h2>
          <p className="mx-auto mt-2 max-w-md text-sm text-zinc-500">
            Media uploads are disabled. Add your Cloudinary keys to the API environment to enable them:
          </p>
          <pre className="mx-auto mt-4 max-w-md rounded-md bg-zinc-900 p-4 text-left text-xs text-zinc-200">
{`CLOUDINARY_CLOUD_NAME=…
CLOUDINARY_API_KEY=…
CLOUDINARY_API_SECRET=…`}
          </pre>
          <p className="mt-4 text-sm text-zinc-500">
            Until then, image fields across the admin accept a plain image URL.{" "}
            <a
              href="https://cloudinary.com/documentation"
              target="_blank"
              rel="noreferrer"
              className="font-medium text-gold-dark underline"
            >
              Cloudinary docs
            </a>
          </p>
        </Card>
      )}

      {state.kind === "configured" && (
        <Card className="p-8 text-center">
          <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-emerald-100 text-xl">✓</div>
          <h2 className="mt-4 text-base font-semibold text-zinc-900">Cloudinary configured</h2>
          <p className="mt-2 text-sm text-zinc-500">{state.detail}</p>
          <p className="mt-2 text-sm text-zinc-500">
            For this demo build, image fields accept a plain URL — paste any Cloudinary or external image link.
          </p>
        </Card>
      )}

      {state.kind === "error" && (
        <div className="rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">{state.message}</div>
      )}
    </div>
  );
}
