import type { Metadata } from "next";
import Link from "next/link";
import Reveal from "@/components/reveal";
import VideoEmbed from "@/components/youtube-embed";
import { BezelCard, CTAButton, Eyebrow, SectionHeading } from "@/components/ui";
import { getSettings } from "@/lib/api";
import { pageMetadata } from "@/lib/seo";

export const metadata: Metadata = pageMetadata({
  title: "Watch Live",
  description: "Join REMI Church live online — worship with us from anywhere in the world.",
  path: "/live",
});

export default async function LivePage() {
  const settings = await getSettings();

  return (
    <section className="mx-auto max-w-6xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <div className="flex flex-wrap items-center gap-4">
          <Eyebrow>REMI Online</Eyebrow>
          {settings.isLive ? (
            <span className="inline-flex items-center gap-2 rounded-full bg-red-500/15 px-4 py-1.5 text-[11px] font-semibold uppercase tracking-[0.15em] text-red-400">
              <span className="relative flex h-2 w-2">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-60" />
                <span className="relative inline-flex h-2 w-2 rounded-full bg-red-500" />
              </span>
              On air now
            </span>
          ) : (
            <span className="rounded-full border border-cream/10 px-4 py-1.5 text-[11px] uppercase tracking-[0.15em] text-cream-dim">
              Currently offline
            </span>
          )}
        </div>
        <h1 className="mt-6 font-display text-5xl font-medium leading-[1.0] tracking-tight text-cream sm:text-6xl">
          Worship with us, wherever you are
        </h1>
      </Reveal>

      <Reveal delay={150}>
        <div className="mt-12">
          <BezelCard>
            {settings.livestreamUrl ? (
              <VideoEmbed url={settings.livestreamUrl} title="REMI Live" seed="remi-live" />
            ) : (
              <div className="flex aspect-video flex-col items-center justify-center rounded-[calc(2rem-0.375rem)] p-10 text-center">
                <p className="font-display text-2xl text-cream">The stream begins at service time</p>
                <p className="mx-auto mt-3 max-w-md text-sm leading-relaxed text-cream-dim">
                  We&apos;re not broadcasting right this second — check the schedule below, or catch a recent message
                  in the sermon archive.
                </p>
              </div>
            )}
          </BezelCard>
        </div>
      </Reveal>

      <div className="mt-16 grid grid-cols-1 gap-10 md:grid-cols-2">
        <Reveal delay={200}>
          <SectionHeading eyebrow="Schedule" title="Service times" />
          <ul className="mt-8 space-y-4">
            {settings.serviceTimes.map((st) => (
              <li
                key={`${st.name}-${st.day}`}
                className="flex items-center justify-between gap-4 rounded-[1.5rem] border border-cream/10 bg-ink-soft px-6 py-5"
              >
                <div>
                  <p className="text-sm font-medium text-cream">{st.name}</p>
                  <p className="mt-1 text-xs text-cream-dim">{st.location}</p>
                </div>
                <p className="shrink-0 text-sm text-gold-bright">
                  {st.day}s · {st.time}
                </p>
              </li>
            ))}
          </ul>
        </Reveal>
        <Reveal delay={280}>
          <SectionHeading eyebrow="Missed a service?" title="Catch up anytime" />
          <p className="mt-6 max-w-sm text-sm leading-relaxed text-cream-dim">
            Every message is archived in our sermon library — watch the video, listen on the go, or download the
            study notes.
          </p>
          <div className="mt-8">
            <CTAButton href="/sermons" variant="ghost">
              Browse the archive
            </CTAButton>
          </div>
          <p className="mt-10 text-sm text-cream-dim">
            Prefer to worship in person?{" "}
            <Link href="/connect/visit" className="text-gold-bright underline underline-offset-4">
              Plan your visit
            </Link>
          </p>
        </Reveal>
      </div>
    </section>
  );
}
