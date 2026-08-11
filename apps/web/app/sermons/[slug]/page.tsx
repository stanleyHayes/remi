import type { Metadata } from "next";
import Link from "next/link";
import Reveal from "@/components/reveal";
import VideoEmbed from "@/components/youtube-embed";
import { BezelCard, EmptyState, Eyebrow } from "@/components/ui";
import { getSermon } from "@/lib/api";
import { formatDate } from "@/lib/format";

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const sermon = await getSermon(slug);
  return {
    title: sermon?.title ?? "Sermon",
    description: sermon?.description ?? `Watch this message from REMI Church.`,
  };
}

const metaLinkCls =
  "inline-flex items-center gap-2 rounded-full border border-cream/15 px-5 py-2.5 text-xs font-medium text-cream transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] hover:border-gold/40 hover:text-gold-bright";

export default async function SermonDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const sermon = await getSermon(slug);

  if (!sermon) {
    return (
      <section className="mx-auto max-w-3xl px-5 pb-28 pt-48 sm:px-8">
        <EmptyState
          title="This sermon isn't available right now"
          copy="It may have been moved, or our media server is taking a breath. Browse the full library instead."
        />
        <p className="mt-8 text-center">
          <Link href="/sermons" className="text-sm text-gold-bright underline underline-offset-4">
            ← Back to all sermons
          </Link>
        </p>
      </section>
    );
  }

  return (
    <section className="mx-auto max-w-5xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <Link href="/sermons" className="text-xs uppercase tracking-[0.2em] text-cream-dim transition-colors hover:text-cream">
          ← All sermons
        </Link>
        <div className="mt-8 flex flex-wrap items-center gap-2">
          <Eyebrow>{sermon.series ?? "Sermon"}</Eyebrow>
          {sermon.topic ? (
            <span className="rounded-full border border-cream/10 px-3.5 py-1.5 text-[10px] uppercase tracking-[0.2em] text-cream-dim">
              {sermon.topic}
            </span>
          ) : null}
        </div>
        <h1 className="mt-6 font-display text-4xl font-medium leading-[1.05] tracking-tight text-cream sm:text-6xl">
          {sermon.title}
        </h1>
        <p className="mt-5 text-sm text-cream-dim">
          {sermon.preacher}
          {sermon.date ? ` · ${formatDate(sermon.date)}` : ""}
        </p>
      </Reveal>

      <Reveal delay={150}>
        <div className="mt-12">
          <BezelCard>
            <VideoEmbed url={sermon.videoUrl} title={sermon.title} seed={`sermon-${sermon.slug}`} />
          </BezelCard>
        </div>
      </Reveal>

      <Reveal delay={250}>
        <div className="mt-8 flex flex-wrap gap-3">
          {sermon.audioUrl ? (
            <a href={sermon.audioUrl} target="_blank" rel="noopener noreferrer" className={metaLinkCls}>
              Listen to audio
            </a>
          ) : null}
          {sermon.notesUrl ? (
            <a href={sermon.notesUrl} target="_blank" rel="noopener noreferrer" className={metaLinkCls}>
              Download notes
            </a>
          ) : null}
        </div>
      </Reveal>

      {sermon.bibleRefs?.length ? (
        <Reveal delay={300}>
          <div className="mt-12 rounded-[2rem] border border-gold/20 bg-gold/5 p-8 sm:p-10">
            <p className="text-[10px] font-medium uppercase tracking-[0.22em] text-gold-bright">Scripture</p>
            <p className="mt-3 font-display text-xl italic text-cream sm:text-2xl">{sermon.bibleRefs.join(" · ")}</p>
          </div>
        </Reveal>
      ) : null}

      {sermon.description ? (
        <Reveal delay={350}>
          <div className="prose-remi mt-12 max-w-3xl">
            <p>{sermon.description}</p>
          </div>
        </Reveal>
      ) : null}
    </section>
  );
}
