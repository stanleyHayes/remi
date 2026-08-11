import Link from "next/link";
import Reveal from "@/components/reveal";
import TestimoniesCarousel from "@/components/testimonies-carousel";
import { BezelCard, CTAButton, Eyebrow, SectionHeading } from "@/components/ui";
import { FALLBACK_SETTINGS, getEvents, getMinistries, getSermons, getSettings, getTestimonies } from "@/lib/api";
import { formatDate, img } from "@/lib/format";

export default async function HomePage() {
  const [settings, sermonList, events, ministries, testimonies] = await Promise.all([
    getSettings(),
    getSermons({ page: "1" }),
    getEvents("upcoming"),
    getMinistries(),
    getTestimonies(),
  ]);

  const s = settings ?? FALLBACK_SETTINGS;
  const featuredSermon = sermonList?.items?.[0] ?? null;
  const upcomingEvents = (events ?? []).slice(0, 3);
  const featuredMinistries = (ministries ?? []).slice(0, 6);

  return (
    <>
      {/* ── Cinematic hero ── */}
      <section className="relative flex min-h-[100dvh] items-end overflow-hidden">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src="https://picsum.photos/seed/remi-worship/1600/900"
          alt="Worship at REMI"
          className="absolute inset-0 h-full w-full object-cover"
        />
        <div className="absolute inset-0 bg-gradient-to-t from-ink via-ink/50 to-ink/30" />
        <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_bottom_left,rgba(201,162,39,0.15),transparent_55%)]" />

        <div className="relative mx-auto w-full max-w-7xl px-5 pb-24 pt-40 sm:px-8 sm:pb-32">
          <Reveal>
            <Eyebrow>Spirit-filled · Apostolic · Prophetic</Eyebrow>
          </Reveal>
          <Reveal delay={120}>
            <h1 className="mt-7 max-w-4xl font-display text-5xl font-medium leading-[0.98] tracking-tight text-cream sm:text-7xl lg:text-8xl">
              {s.churchName}
            </h1>
          </Reveal>
          <Reveal delay={220}>
            <p className="mt-7 max-w-xl font-display text-xl italic leading-relaxed text-cream/85 sm:text-2xl">
              {s.tagline}
            </p>
          </Reveal>
          <Reveal delay={320}>
            <div className="mt-10 flex flex-wrap items-center gap-4">
              <CTAButton href="/connect/visit">Join us this Sunday</CTAButton>
              <CTAButton href="/sermons" variant="ghost">
                Watch sermons
              </CTAButton>
            </div>
          </Reveal>
          <Reveal delay={420}>
            <div className="mt-14 flex flex-wrap gap-x-10 gap-y-4 border-t border-cream/15 pt-7">
              {(s.nextServiceOverride
                ? [{ name: s.nextServiceOverride, day: "", time: "", location: "" }]
                : s.serviceTimes.slice(0, 3)
              ).map((st, i) => (
                <div key={i}>
                  <p className="text-[10px] font-medium uppercase tracking-[0.22em] text-gold-bright">
                    {st.day ? `${st.day}s` : "Next gathering"}
                  </p>
                  <p className="mt-1 text-sm text-cream">
                    {st.name}
                    {st.time ? ` · ${st.time}` : ""}
                  </p>
                </div>
              ))}
            </div>
          </Reveal>
        </div>
      </section>

      {/* ── Live banner ── */}
      {s.isLive ? (
        <section className="border-y border-red-500/25 bg-red-500/10">
          <div className="mx-auto flex max-w-7xl flex-wrap items-center justify-between gap-4 px-5 py-5 sm:px-8">
            <p className="flex items-center gap-3 text-sm font-medium text-cream">
              <span className="relative flex h-2.5 w-2.5">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-60" />
                <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-red-500" />
              </span>
              We are live right now — worship with us from wherever you are.
            </p>
            <Link
              href="/live"
              className="rounded-full bg-red-500 px-5 py-2 text-xs font-semibold uppercase tracking-[0.12em] text-white transition-colors duration-300 hover:bg-red-400"
            >
              Watch live
            </Link>
          </div>
        </section>
      ) : null}

      {/* ── Featured sermon ── */}
      <section className="mx-auto max-w-7xl px-5 py-24 sm:px-8 sm:py-36">
        <Reveal>
          <SectionHeading
            eyebrow="Latest Word"
            title="Fuel for your week"
            copy="Catch the most recent message from our altar — stream it, download the notes, and share it with someone who needs it."
          />
        </Reveal>
        <Reveal delay={150}>
          {featuredSermon ? (
            <Link href={`/sermons/${featuredSermon.slug}`} className="group mt-14 block">
              <BezelCard>
                <div className="grid md:grid-cols-2">
                  <div className="relative min-h-[280px] overflow-hidden rounded-t-[calc(2rem-0.375rem)] md:rounded-l-[calc(2rem-0.375rem)] md:rounded-tr-none">
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    <img
                      src={img(featuredSermon.image, `sermon-${featuredSermon.slug}`)}
                      alt={featuredSermon.title}
                      className="absolute inset-0 h-full w-full object-cover transition-transform duration-700 ease-[cubic-bezier(0.32,0.72,0,1)] group-hover:scale-[1.03]"
                    />
                    <div className="absolute inset-0 bg-gradient-to-t from-ink/60 to-transparent" />
                    <span className="absolute bottom-5 left-5 flex h-14 w-14 items-center justify-center rounded-full bg-gold text-ink transition-transform duration-500 group-hover:scale-110">
                      <svg width="18" height="18" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
                        <path d="M8 5.5v13l11-6.5-11-6.5Z" />
                      </svg>
                    </span>
                  </div>
                  <div className="flex flex-col justify-center p-8 sm:p-12">
                    <div className="flex flex-wrap gap-2 text-[10px] font-medium uppercase tracking-[0.18em] text-gold-bright">
                      {featuredSermon.series ? <span>{featuredSermon.series}</span> : null}
                      {featuredSermon.topic ? <span className="text-cream-dim">· {featuredSermon.topic}</span> : null}
                    </div>
                    <h3 className="mt-4 font-display text-3xl font-medium leading-tight text-cream transition-colors duration-500 group-hover:text-gold-bright sm:text-4xl">
                      {featuredSermon.title}
                    </h3>
                    <p className="mt-4 text-sm text-cream-dim">
                      {featuredSermon.preacher}
                      {featuredSermon.date ? ` · ${formatDate(featuredSermon.date)}` : ""}
                    </p>
                    {featuredSermon.description ? (
                      <p className="mt-4 line-clamp-3 text-sm leading-relaxed text-cream-dim">
                        {featuredSermon.description}
                      </p>
                    ) : null}
                  </div>
                </div>
              </BezelCard>
            </Link>
          ) : (
            <div className="mt-14">
              <BezelCard>
                <div className="p-12 text-center sm:p-16">
                  <p className="font-display text-2xl text-cream">Messages are on the way</p>
                  <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-cream-dim">
                    Our sermon library is being prepared. Join us this Sunday to hear the word live.
                  </p>
                  <div className="mt-8">
                    <CTAButton href="/connect/visit">Plan your visit</CTAButton>
                  </div>
                </div>
              </BezelCard>
            </div>
          )}
        </Reveal>
      </section>

      {/* ── Upcoming events strip ── */}
      {upcomingEvents.length > 0 ? (
        <section className="border-y border-cream/10 bg-ink-soft/60">
          <div className="mx-auto max-w-7xl px-5 py-20 sm:px-8 sm:py-28">
            <Reveal>
              <div className="flex flex-wrap items-end justify-between gap-6">
                <SectionHeading eyebrow="What's Ahead" title="Upcoming gatherings" />
                <CTAButton href="/events" variant="ghost">
                  All events
                </CTAButton>
              </div>
            </Reveal>
            <div className="mt-12 grid grid-cols-1 gap-5 md:grid-cols-3">
              {upcomingEvents.map((ev, i) => (
                <Reveal key={ev.id} delay={i * 100}>
                  <Link href={`/events/${ev.slug}`} className="group block h-full">
                    <BezelCard className="h-full">
                      <div className="p-7">
                        <p className="text-[10px] font-medium uppercase tracking-[0.2em] text-gold-bright">
                          {formatDate(ev.startAt)}
                        </p>
                        <h3 className="mt-3 font-display text-2xl font-medium leading-snug text-cream transition-colors duration-500 group-hover:text-gold-bright">
                          {ev.title}
                        </h3>
                        {ev.location ? <p className="mt-3 text-sm text-cream-dim">{ev.location}</p> : null}
                      </div>
                    </BezelCard>
                  </Link>
                </Reveal>
              ))}
            </div>
          </div>
        </section>
      ) : null}

      {/* ── Ministries highlights ── */}
      <section className="mx-auto max-w-7xl px-5 py-24 sm:px-8 sm:py-36">
        <Reveal>
          <div className="flex flex-wrap items-end justify-between gap-6">
            <SectionHeading
              eyebrow="Find Your Place"
              title="Ministries for every season"
              copy="From children to intercession, there is a family within the family waiting for you."
            />
            <CTAButton href="/ministries" variant="ghost">
              Explore ministries
            </CTAButton>
          </div>
        </Reveal>
        {featuredMinistries.length > 0 ? (
          <div className="mt-12 grid grid-cols-1 gap-5 sm:grid-cols-2 lg:grid-cols-3">
            {featuredMinistries.map((m, i) => (
              <Reveal key={m.id} delay={(i % 3) * 100}>
                <Link href={`/ministries/${m.slug}`} className="group block h-full">
                  <BezelCard className="h-full">
                    <div className="relative aspect-[16/9] overflow-hidden rounded-t-[calc(2rem-0.375rem)]">
                      {/* eslint-disable-next-line @next/next/no-img-element */}
                      <img
                        src={img(m.image, `ministry-${m.slug}`)}
                        alt={m.name}
                        loading="lazy"
                        className="h-full w-full object-cover transition-transform duration-700 ease-[cubic-bezier(0.32,0.72,0,1)] group-hover:scale-[1.04]"
                      />
                      <div className="absolute inset-0 bg-gradient-to-t from-ink/70 to-transparent" />
                      <h3 className="absolute bottom-5 left-6 right-6 font-display text-2xl font-medium text-cream">
                        {m.name}
                      </h3>
                    </div>
                    <div className="p-6">
                      <p className="line-clamp-2 text-sm leading-relaxed text-cream-dim">{m.description}</p>
                    </div>
                  </BezelCard>
                </Link>
              </Reveal>
            ))}
          </div>
        ) : (
          <Reveal delay={120}>
            <div className="mt-12">
              <BezelCard>
                <div className="p-12 text-center">
                  <p className="font-display text-2xl text-cream">Ministries launching soon</p>
                  <p className="mx-auto mt-3 max-w-md text-sm text-cream-dim">
                    Details of our ministry departments will appear here shortly.
                  </p>
                </div>
              </BezelCard>
            </div>
          </Reveal>
        )}
      </section>

      {/* ── Testimonies ── */}
      {testimonies && testimonies.length > 0 ? (
        <section className="border-y border-cream/10 bg-ink-soft/60">
          <div className="mx-auto max-w-7xl px-5 py-24 sm:px-8 sm:py-36">
            <Reveal>
              <p className="mb-14 text-center">
                <Eyebrow>God is moving</Eyebrow>
              </p>
            </Reveal>
            <Reveal delay={120}>
              <TestimoniesCarousel testimonies={testimonies} />
            </Reveal>
          </div>
        </section>
      ) : null}

      {/* ── Give + Prayer CTAs ── */}
      <section className="mx-auto max-w-7xl px-5 py-24 sm:px-8 sm:py-36">
        <div className="grid grid-cols-1 gap-5 md:grid-cols-2">
          <Reveal>
            <BezelCard className="h-full">
              <div className="relative flex h-full flex-col justify-end overflow-hidden rounded-[calc(2rem-0.375rem)] p-8 sm:p-12">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src="https://picsum.photos/seed/remi-give/1200/800"
                  alt=""
                  aria-hidden
                  className="absolute inset-0 h-full w-full object-cover opacity-40"
                />
                <div className="absolute inset-0 bg-gradient-to-t from-ink via-ink/60 to-transparent" />
                <div className="relative">
                  <Eyebrow>Generosity</Eyebrow>
                  <h3 className="mt-5 font-display text-3xl font-medium text-cream sm:text-4xl">
                    Sow into the work
                  </h3>
                  <p className="mt-4 max-w-sm text-sm leading-relaxed text-cream-dim">
                    Your giving fuels the gospel in Accra and beyond — every seed matters.
                  </p>
                  <div className="mt-8">
                    <CTAButton href="/give">Give online</CTAButton>
                  </div>
                </div>
              </div>
            </BezelCard>
          </Reveal>
          <Reveal delay={140}>
            <BezelCard className="h-full">
              <div className="relative flex h-full flex-col justify-end overflow-hidden rounded-[calc(2rem-0.375rem)] p-8 sm:p-12">
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src="https://picsum.photos/seed/remi-prayer/1200/800"
                  alt=""
                  aria-hidden
                  className="absolute inset-0 h-full w-full object-cover opacity-40"
                />
                <div className="absolute inset-0 bg-gradient-to-t from-ink via-ink/60 to-transparent" />
                <div className="relative">
                  <Eyebrow>We stand with you</Eyebrow>
                  <h3 className="mt-5 font-display text-3xl font-medium text-cream sm:text-4xl">
                    Need prayer?
                  </h3>
                  <p className="mt-4 max-w-sm text-sm leading-relaxed text-cream-dim">
                    Whatever you&apos;re carrying, you don&apos;t have to carry it alone. Send us your request.
                  </p>
                  <div className="mt-8">
                    <CTAButton href="/connect/prayer" variant="ghost">
                      Request prayer
                    </CTAButton>
                  </div>
                </div>
              </div>
            </BezelCard>
          </Reveal>
        </div>
      </section>

      {/* ── Plan Your Visit band ── */}
      <section className="relative overflow-hidden border-t border-cream/10">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src="https://picsum.photos/seed/remi-visit/1600/700"
          alt=""
          aria-hidden
          className="absolute inset-0 h-full w-full object-cover"
        />
        <div className="absolute inset-0 bg-ink/80" />
        <div className="relative mx-auto max-w-7xl px-5 py-28 text-center sm:px-8 sm:py-40">
          <Reveal>
            <Eyebrow>New here?</Eyebrow>
            <h2 className="mx-auto mt-7 max-w-3xl font-display text-4xl font-medium leading-[1.02] text-cream sm:text-6xl">
              There&apos;s a seat with your name on it
            </h2>
            <p className="mx-auto mt-6 max-w-xl text-base leading-relaxed text-cream-dim">
              Tell us you&apos;re coming and we&apos;ll meet you at the door, show you around, and make sure you feel
              at home from the first song.
            </p>
            <div className="mt-10 flex flex-wrap justify-center gap-4">
              <CTAButton href="/connect/visit">Plan your visit</CTAButton>
              <CTAButton href="/branches" variant="ghost">
                Find a branch
              </CTAButton>
            </div>
          </Reveal>
        </div>
      </section>
    </>
  );
}
