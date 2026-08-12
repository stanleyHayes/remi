import type { Metadata } from "next";
import Reveal from "@/components/reveal";
import { LeaderCard } from "@/components/cards";
import { EmptyState, PageBanner } from "@/components/ui";
import { getLeadership } from "@/lib/api";
import { pageMetadata } from "@/lib/seo";

export const metadata: Metadata = pageMetadata({
  title: "Leadership",
  description:
    "Meet the leaders of Ruach Elohim Ministries International, founded by Dr. Ismaila Hans Awudu.",
  path: "/leadership",
});

export default async function LeadershipPage() {
  const leaders = (await getLeadership()) ?? [];
  const sorted = [...leaders].sort((a, b) => a.order - b.order);
  const founder = sorted.find((l) => l.isFounder) ?? null;
  const rest = sorted.filter((l) => l !== founder);

  return (
    <section className="listing-page-shell">
      <Reveal>
        <PageBanner
          eyebrow="Servant Leaders"
          title="The people God has set over the house"
          copy="Called, tested, and anointed — our leadership team serves the REMI family with humility and fire."
          index="05"
        />
      </Reveal>

      {leaders.length === 0 ? (
        <div className="mt-14">
          <EmptyState
            title="Leadership profiles coming soon"
            copy="We're putting the finishing touches on this page. REMI is led by our founder, Dr. Ismaila Hans Awudu, together with a dedicated team of pastors and ministers."
          />
        </div>
      ) : (
        <>
          {founder ? (
            <Reveal delay={120}>
              <div className="mt-14">
                <LeaderCard leader={founder} featured />
              </div>
            </Reveal>
          ) : null}
          {rest.length > 0 ? (
            <div className="mt-6 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
              {rest.map((leader, i) => (
                <Reveal key={leader.id} delay={(i % 3) * 100}>
                  <LeaderCard leader={leader} />
                </Reveal>
              ))}
            </div>
          ) : null}
        </>
      )}
    </section>
  );
}
