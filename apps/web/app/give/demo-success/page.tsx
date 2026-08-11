import type { Metadata } from "next";
import { CTAButton, Eyebrow } from "@/components/ui";

export const metadata: Metadata = {
  title: "Thank You for Giving",
  robots: { index: false },
};

export default async function GiveDemoSuccessPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const sp = await searchParams;
  const ref = typeof sp.ref === "string" ? sp.ref : null;

  return (
    <section className="mx-auto flex min-h-[80dvh] max-w-2xl flex-col items-center justify-center px-5 py-40 text-center sm:px-8">
      <div className="flex h-16 w-16 items-center justify-center rounded-full border border-gold/30 bg-gold/10 text-gold-bright">
        <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="M20 6L9 17l-5-5" />
        </svg>
      </div>
      <div className="mt-8">
        <Eyebrow>Demo checkout complete</Eyebrow>
      </div>
      <h1 className="mt-6 font-display text-4xl font-medium text-cream sm:text-5xl">
        Thank you for your seed
      </h1>
      <p className="mx-auto mt-5 max-w-md text-base leading-relaxed text-cream-dim">
        Your giving was received successfully. May the Lord multiply it back to you — pressed down, shaken together,
        and running over.
      </p>
      {ref ? (
        <p className="mt-6 rounded-full border border-cream/10 bg-cream/5 px-5 py-2 font-mono text-xs text-cream-dim">
          Reference: {ref}
        </p>
      ) : null}
      <div className="mt-10 flex flex-wrap justify-center gap-4">
        <CTAButton href="/">Back home</CTAButton>
        <CTAButton href="/sermons" variant="ghost">
          Watch a sermon
        </CTAButton>
      </div>
    </section>
  );
}
