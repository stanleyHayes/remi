import type { Metadata } from "next";
import Reveal from "@/components/reveal";
import { Eyebrow } from "@/components/ui";
import { getPage } from "@/lib/api";
import { img } from "@/lib/format";
import { pageMetadata } from "@/lib/seo";

const ABOUT_META: Record<
  string,
  { title: string; eyebrow: string; fallbackSubtitle: string; fallbackBody: string[] }
> = {
  history: {
    title: "Our History",
    eyebrow: "About REMI",
    fallbackSubtitle: "From a small prayer gathering to a family of believers across Ghana.",
    fallbackBody: [
      "Ruach Elohim Ministries International was founded by Dr. Ismaila Hans Awudu in Accra, Ghana, birthed out of a season of prayer and a clear prophetic mandate: to raise a people who carry the breath of God — Ruach Elohim — into every sphere of life.",
      "What began as a handful of believers meeting in a living room has grown into a vibrant, Spirit-filled ministry with branches and partners across the nation. Through every season, the heartbeat has remained the same: undiluted worship, the apostolic and prophetic word, and genuine family.",
      "Today, REMI continues to pursue that mandate — equipping saints, transforming communities, and declaring that the Spirit still breathes life into dry bones.",
    ],
  },
  mission: {
    title: "Our Mission",
    eyebrow: "About REMI",
    fallbackSubtitle: "Why we exist, and where we are going.",
    fallbackBody: [
      "Our mission is to raise Spirit-filled believers who manifest the life, power, and character of Christ in their generation.",
      "We pursue this through apostolic teaching, prophetic ministry, fervent prayer, and authentic community — equipping every member to discover and walk in their God-given assignment.",
      "Our vision is a church without walls: a movement of worshippers whose lives preach louder than their words, in Accra, across Ghana, and to the nations.",
    ],
  },
  beliefs: {
    title: "What We Believe",
    eyebrow: "About REMI",
    fallbackSubtitle: "The foundations we build on.",
    fallbackBody: [
      "We believe the Bible is the inspired, infallible Word of God — our final authority in faith and practice.",
      "We believe in one God, eternally existing as Father, Son, and Holy Spirit; in the deity of Jesus Christ, His virgin birth, sinless life, atoning death, bodily resurrection, ascension, and soon return.",
      "We believe salvation is by grace through faith in Jesus Christ, and that the Holy Spirit empowers every believer for holy living and effective witness — with the gifts of the Spirit active in the church today.",
      "We believe in water baptism, the Lord's Supper, the priesthood of all believers, and the mission of the church to make disciples of all nations.",
    ],
  },
};

export function aboutMetadata(pageKey: string): Metadata {
  const meta = ABOUT_META[pageKey];
  const path = pageKey === "history" ? "/about" : `/about/${pageKey}`;
  return pageMetadata({
    title: meta.title,
    description: meta.fallbackSubtitle,
    path,
  });
}

export default async function AboutPage({ pageKey }: { pageKey: string }) {
  const page = await getPage(pageKey);
  const meta = ABOUT_META[pageKey] ?? ABOUT_META.history;

  const title = page?.title || meta.title;
  const subtitle = page?.subtitle || meta.fallbackSubtitle;
  const heroImage = img(page?.heroImage, `remi-${pageKey}`);
  const bodyHtml = page?.body?.trim() ? page.body : null;

  return (
    <>
      <section className="relative flex min-h-[60dvh] items-end overflow-hidden">
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img src={heroImage} alt={title} className="absolute inset-0 h-full w-full object-cover" />
        <div className="absolute inset-0 bg-gradient-to-t from-ink via-ink/60 to-ink/40" />
        <div className="relative mx-auto w-full max-w-4xl px-5 pb-16 pt-44 sm:px-8">
          <Reveal>
            <Eyebrow>{meta.eyebrow}</Eyebrow>
            <h1 className="mt-6 font-display text-5xl font-medium leading-[1.02] tracking-tight text-cream sm:text-6xl">
              {title}
            </h1>
            <p className="mt-5 max-w-xl font-display text-lg italic text-cream/80 sm:text-xl">{subtitle}</p>
          </Reveal>
        </div>
      </section>

      <section className="mx-auto max-w-3xl px-5 py-20 sm:px-8 sm:py-28">
        <Reveal>
          {bodyHtml ? (
            <div className="prose-remi" dangerouslySetInnerHTML={{ __html: bodyHtml }} />
          ) : (
            <div className="prose-remi">
              {meta.fallbackBody.map((p, i) => (
                <p key={i}>{p}</p>
              ))}
            </div>
          )}
        </Reveal>
      </section>
    </>
  );
}
