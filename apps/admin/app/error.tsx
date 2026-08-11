"use client";

import { useEffect } from "react";

export default function GlobalError({ error, reset }: { error: Error & { digest?: string }; reset: () => void }) {
  useEffect(() => { console.error(error); }, [error]);
  return (
    <main className="grid min-h-[100dvh] place-items-center bg-paper px-5">
      <section className="admin-card w-full max-w-lg p-7 text-center sm:p-9">
        <span className="mx-auto grid size-12 place-items-center rounded-2xl bg-rose-50 text-xl text-rose-600">!</span>
        <p className="mt-6 font-mono text-[10px] font-bold uppercase tracking-[.18em] text-rose-600">Workspace interrupted</p>
        <h1 className="mt-3 text-3xl font-extrabold tracking-[-.04em] text-zinc-900">We couldn&apos;t open this page.</h1>
        <p className="mt-3 text-sm leading-6 text-zinc-500">Your data is unchanged. Try loading the page again.</p>
        <button onClick={reset} className="mt-6 rounded-xl bg-sidebar px-5 py-3 text-sm font-bold text-white">Try again</button>
      </section>
    </main>
  );
}
