import type { Metadata } from "next";
import Reveal from "@/components/reveal";
import { BranchCard } from "@/components/cards";
import { EmptyState, SectionHeading } from "@/components/ui";
import { getBranches } from "@/lib/api";

export const metadata: Metadata = {
  title: "Branches",
  description: "Find a REMI Church branch near you — locations and service times across Ghana.",
};

export default async function BranchesPage() {
  const branches = (await getBranches()) ?? [];

  return (
    <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <SectionHeading
          eyebrow="One Family, Many Homes"
          title="Our branches"
          copy="Wherever you are, there's a REMI family nearby ready to welcome you."
        />
      </Reveal>

      {branches.length === 0 ? (
        <div className="mt-14">
          <EmptyState
            title="Branch locations coming soon"
            copy="Our head office is in Accra, Ghana. Reach out and we'll point you to the nearest gathering."
          />
        </div>
      ) : (
        <div className="mt-14 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
          {branches.map((b, i) => (
            <Reveal key={b.id} delay={(i % 3) * 100}>
              <BranchCard branch={b} />
            </Reveal>
          ))}
        </div>
      )}
    </section>
  );
}
