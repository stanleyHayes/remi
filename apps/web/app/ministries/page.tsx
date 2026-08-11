import type { Metadata } from "next";
import Reveal from "@/components/reveal";
import { MinistryCard } from "@/components/cards";
import { EmptyState, SectionHeading } from "@/components/ui";
import { getMinistries } from "@/lib/api";

export const metadata: Metadata = {
  title: "Ministries",
  description: "Find your place to serve and grow — the ministries of REMI Church.",
};

export default async function MinistriesPage() {
  const ministries = (await getMinistries()) ?? [];

  return (
    <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <SectionHeading
          eyebrow="Belong & Build"
          title="Ministries"
          copy="Every member is a minister. Discover where your gifts come alive."
        />
      </Reveal>

      {ministries.length === 0 ? (
        <div className="mt-14">
          <EmptyState
            title="Ministry details coming soon"
            copy="We're preparing something beautiful. In the meantime, ask any usher on Sunday about where you can serve."
          />
        </div>
      ) : (
        <div className="mt-14 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {ministries.map((m, i) => (
            <Reveal key={m.id} delay={(i % 3) * 100}>
              <MinistryCard ministry={m} />
            </Reveal>
          ))}
        </div>
      )}
    </section>
  );
}
