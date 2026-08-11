import type { Metadata } from "next";
import Link from "next/link";
import Reveal from "@/components/reveal";
import { BezelCard, CTAButton, EmptyState, Eyebrow } from "@/components/ui";
import { getMinistry } from "@/lib/api";
import { img } from "@/lib/format";

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}): Promise<Metadata> {
  const { slug } = await params;
  const ministry = await getMinistry(slug);
  return {
    title: ministry?.name ?? "Ministry",
    description: ministry?.description ?? "A ministry of REMI Church.",
  };
}

export default async function MinistryDetailPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const ministry = await getMinistry(slug);

  if (!ministry) {
    return (
      <section className="mx-auto max-w-3xl px-5 pb-28 pt-48 sm:px-8">
        <EmptyState
          title="This ministry isn't available right now"
          copy="Browse all of our ministries to find your place."
        />
        <p className="mt-8 text-center">
          <Link href="/ministries" className="text-sm text-gold-bright underline underline-offset-4">
            ← Back to all ministries
          </Link>
        </p>
      </section>
    );
  }

  return (
    <section className="mx-auto max-w-5xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <Link href="/ministries" className="text-xs uppercase tracking-[0.2em] text-cream-dim transition-colors hover:text-cream">
          ← All ministries
        </Link>
      </Reveal>

      <Reveal delay={100}>
        <div className="mt-10">
          <BezelCard>
            <div className="relative aspect-[16/7] overflow-hidden rounded-[calc(2rem-0.375rem)]">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={img(ministry.image, `ministry-${ministry.slug}`)}
                alt={ministry.name}
                className="h-full w-full object-cover"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-ink/85 via-ink/20 to-transparent" />
              <div className="absolute bottom-0 left-0 p-8 sm:p-10">
                <Eyebrow>Ministry</Eyebrow>
                <h1 className="mt-4 font-display text-4xl font-medium text-cream sm:text-5xl">{ministry.name}</h1>
              </div>
            </div>
          </BezelCard>
        </div>
      </Reveal>

      <div className="mt-12 grid grid-cols-1 gap-10 md:grid-cols-3">
        <Reveal delay={200} className="md:col-span-2">
          <div className="prose-remi">
            <p>{ministry.description}</p>
          </div>
          <div className="mt-10">
            <CTAButton href="/connect/contact">Get involved</CTAButton>
          </div>
        </Reveal>
        <Reveal delay={280}>
          <BezelCard>
            <div className="space-y-5 p-7">
              {ministry.leaderName ? (
                <div>
                  <p className="text-[10px] font-medium uppercase tracking-[0.2em] text-gold-bright">Led by</p>
                  <p className="mt-2 text-sm text-cream">{ministry.leaderName}</p>
                </div>
              ) : null}
              {ministry.meetingTime ? (
                <div>
                  <p className="text-[10px] font-medium uppercase tracking-[0.2em] text-gold-bright">Meets</p>
                  <p className="mt-2 text-sm text-cream">{ministry.meetingTime}</p>
                </div>
              ) : null}
            </div>
          </BezelCard>
        </Reveal>
      </div>
    </section>
  );
}
