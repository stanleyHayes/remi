import type { Metadata } from "next";
import Link from "next/link";
import Reveal from "@/components/reveal";
import EventRegistrationForm from "@/components/event-registration-form";
import { BezelCard, EmptyState, Eyebrow } from "@/components/ui";
import { getEvent } from "@/lib/api";
import { formatDate, formatTime, img } from "@/lib/format";
import { pageMetadata } from "@/lib/seo";

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const event = await getEvent(slug);
  return pageMetadata({
    title: event?.title ?? "Event",
    description: event?.description ?? "An event at REMI Church.",
    path: `/events/${slug}`,
    image: event?.image,
  });
}

export default async function EventDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const event = await getEvent(slug);

  if (!event) {
    return (
      <section className="mx-auto max-w-3xl px-5 pb-28 pt-48 sm:px-8">
        <EmptyState
          title="This event isn't available right now"
          copy="It may have passed or been moved. Take a look at everything else coming up."
        />
        <p className="mt-8 text-center">
          <Link href="/events" className="text-sm text-gold-bright underline underline-offset-4">
            ← Back to all events
          </Link>
        </p>
      </section>
    );
  }

  const spotsLeft =
    typeof event.capacity === "number" && typeof event.registeredCount === "number"
      ? Math.max(0, event.capacity - event.registeredCount)
      : null;

  return (
    <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <Link href="/events" className="text-xs uppercase tracking-[0.2em] text-cream-dim transition-colors hover:text-cream">
          ← All events
        </Link>
      </Reveal>

      <div className="mt-10 grid grid-cols-1 gap-8 lg:grid-cols-3">
        <div className="lg:col-span-2">
          <Reveal delay={100}>
            <BezelCard>
              <div className="relative aspect-[16/8] overflow-hidden rounded-[calc(2rem-0.375rem)]">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={img(event.image, `event-${event.slug}`)}
                  alt={event.title}
                  className="h-full w-full object-cover"
                />
                <div className="absolute inset-0 bg-gradient-to-t from-ink/80 via-ink/20 to-transparent" />
                <div className="absolute bottom-0 left-0 p-8 sm:p-10">
                  <Eyebrow>Event</Eyebrow>
                  <h1 className="mt-4 max-w-xl font-display text-4xl font-medium leading-[1.02] text-cream sm:text-5xl">
                    {event.title}
                  </h1>
                </div>
              </div>
            </BezelCard>
          </Reveal>

          {event.description ? (
            <Reveal delay={200}>
              <div className="prose-remi mt-10 max-w-2xl">
                <p>{event.description}</p>
              </div>
            </Reveal>
          ) : null}
        </div>

        <Reveal delay={250}>
          <div className="space-y-5">
            <BezelCard>
              <div className="space-y-5 p-7">
                <div>
                  <p className="text-[10px] font-medium uppercase tracking-[0.2em] text-gold-bright">When</p>
                  <p className="mt-2 text-sm text-cream">{formatDate(event.startAt)}</p>
                  <p className="text-sm text-cream-dim">
                    {formatTime(event.startAt)}
                    {event.endAt ? ` – ${formatTime(event.endAt)}` : ""}
                  </p>
                </div>
                {event.location ? (
                  <div>
                    <p className="text-[10px] font-medium uppercase tracking-[0.2em] text-gold-bright">Where</p>
                    <p className="mt-2 text-sm text-cream">{event.location}</p>
                  </div>
                ) : null}
                {spotsLeft !== null ? (
                  <div>
                    <p className="text-[10px] font-medium uppercase tracking-[0.2em] text-gold-bright">Availability</p>
                    <p className="mt-2 text-sm text-cream">
                      {spotsLeft > 0 ? `${spotsLeft} seats remaining` : "Fully booked"}
                    </p>
                  </div>
                ) : null}
              </div>
            </BezelCard>

            {event.registrationEnabled && spotsLeft !== 0 ? (
              <BezelCard>
                <div className="p-7 pb-3">
                  <p className="font-display text-xl text-cream">Reserve your seat</p>
                  <p className="mt-1 text-xs text-cream-dim">Free registration — it takes ten seconds.</p>
                </div>
                <div className="px-2 pb-2">
                  <EventRegistrationForm eventId={event.id} />
                </div>
              </BezelCard>
            ) : null}
          </div>
        </Reveal>
      </div>
    </section>
  );
}
