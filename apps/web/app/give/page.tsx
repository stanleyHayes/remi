import type { Metadata } from "next";
import Reveal from "@/components/reveal";
import GiveForm from "@/components/give-form";
import { BezelCard, Eyebrow } from "@/components/ui";
import { FALLBACK_SETTINGS, getSettings } from "@/lib/api";
import { pageMetadata } from "@/lib/seo";

export const metadata: Metadata = pageMetadata({
  title: "Give",
  description: "Partner with what God is doing — give securely online to REMI Church.",
  path: "/give",
});

export default async function GivePage() {
  const settings = await getSettings();
  const categories =
    settings.givingCategories?.length > 0 ? settings.givingCategories : FALLBACK_SETTINGS.givingCategories;

  return (
    <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <div className="grid items-start gap-12 lg:grid-cols-2 lg:gap-16">
        <Reveal>
          <Eyebrow>Generosity</Eyebrow>
          <h1 className="mt-6 font-display text-5xl font-medium leading-[1.0] tracking-tight text-cream sm:text-6xl">
            Give with a cheerful heart
          </h1>
          <p className="mt-6 max-w-md text-base leading-relaxed text-cream-dim">
            &ldquo;Each of you should give what you have decided in your heart to give, not reluctantly or under
            compulsion, for God loves a cheerful giver.&rdquo; — 2 Corinthians 9:7
          </p>
          <p className="mt-5 max-w-md text-base leading-relaxed text-cream-dim">
            Your tithes, offerings, and seeds sustain the work of {settings.churchName} — from Sunday services and
            outreach to raising the next generation of believers.
          </p>
          <ul className="mt-8 space-y-3">
            {categories.map((c) => (
              <li key={c} className="flex items-center gap-3 text-sm text-cream">
                <span className="h-1 w-1 rounded-full bg-gold" />
                {c}
              </li>
            ))}
          </ul>
        </Reveal>

        <Reveal delay={180}>
          <BezelCard>
            <GiveForm categories={categories} />
          </BezelCard>
        </Reveal>
      </div>
    </section>
  );
}
