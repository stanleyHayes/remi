import type { Metadata } from "next";
import Reveal from "@/components/reveal";
import { SectionHeading } from "@/components/ui";

export const metadata: Metadata = {
  title: "Gallery",
  description: "Moments of worship, fellowship, and celebration at REMI Church.",
};

const SEEDS = [
  "remi-worship-1", "remi-praise-2", "remi-altar-3", "remi-choir-4", "remi-kids-5",
  "remi-youth-6", "remi-hands-7", "remi-light-8", "remi-family-9", "remi-dance-10",
  "remi-word-11", "remi-community-12",
];

export default function GalleryPage() {
  return (
    <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <Reveal>
        <SectionHeading
          eyebrow="Life at REMI"
          title="Gallery"
          copy="Glimpses of what God is doing in our midst — worship, word, and family."
        />
      </Reveal>
      <div className="mt-14 columns-1 gap-5 sm:columns-2 lg:columns-3 [&>*]:mb-5">
        {SEEDS.map((seed, i) => (
          <Reveal key={seed} delay={(i % 3) * 80}>
            <div className="overflow-hidden rounded-[1.5rem] border border-cream/10">
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={`https://picsum.photos/seed/${seed}/800/${i % 3 === 0 ? 1000 : 600}`}
                alt={`REMI Church moment ${i + 1}`}
                loading="lazy"
                className="w-full object-cover transition-transform duration-700 ease-[cubic-bezier(0.32,0.72,0,1)] hover:scale-[1.03]"
              />
            </div>
          </Reveal>
        ))}
      </div>
    </section>
  );
}
