import type { Metadata } from "next";
import Link from "next/link";
import Reveal from "@/components/reveal";
import { EventCard } from "@/components/cards";
import { EmptyState, SectionHeading } from "@/components/ui";
import { getEvents } from "@/lib/api";
import { pageMetadata } from "@/lib/seo";

export const metadata: Metadata = pageMetadata({
  title: "Events",
  description: "Conferences, services, and gatherings at Ruach Elohim Ministries International.",
  path: "/events",
});

export default async function EventsPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const sp = await searchParams;
  const when = sp.when === "past" ? "past" : "upcoming";
  const events = (await getEvents(when)) ?? [];

  const tabCls = (active: boolean) =>
    `rounded-full px-6 py-2.5 text-sm font-medium transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] ${
      active ? "bg-gold text-ink" : "text-cream-dim hover:text-cream"
    }`;

  return (
    <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <div className="flex flex-wrap items-end justify-between gap-8">
          <SectionHeading
            eyebrow="Gather With Us"
            title="Events & conferences"
            copy="Mark your calendar — these are the moments God has set apart for us as a family."
          />
          <div className="flex rounded-full border border-cream/10 bg-ink-soft p-1.5">
            <Link href="/events" className={tabCls(when === "upcoming")}>
              Upcoming
            </Link>
            <Link href="/events?when=past" className={tabCls(when === "past")}>
              Past
            </Link>
          </div>
        </div>
      </Reveal>

      {events.length === 0 ? (
        <div className="mt-14">
          <EmptyState
            title={when === "upcoming" ? "No upcoming events right now" : "No past events to show yet"}
            copy={
              when === "upcoming"
                ? "New gatherings are announced regularly — check back soon, or join us any Sunday."
                : "Our event history will appear here."
            }
          />
        </div>
      ) : (
        <div className="mt-14 grid grid-cols-1 gap-5 md:grid-cols-2">
          {events.map((event, i) => (
            <Reveal key={event.id} delay={(i % 2) * 100}>
              <EventCard event={event} />
            </Reveal>
          ))}
        </div>
      )}
    </section>
  );
}
