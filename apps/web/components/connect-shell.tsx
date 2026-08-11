import type { ReactNode } from "react";
import Reveal from "@/components/reveal";
import { BezelCard, Eyebrow } from "@/components/ui";

export default function ConnectShell({
  eyebrow,
  title,
  copy,
  scripture,
  children,
}: {
  eyebrow: string;
  title: string;
  copy: string;
  scripture?: { text: string; ref: string };
  children: ReactNode;
}) {
  return (
    <section className="mx-auto max-w-7xl px-5 pb-28 pt-40 sm:px-8 sm:pt-48">
      <div className="grid items-start gap-12 lg:grid-cols-2 lg:gap-16">
        <Reveal>
          <Eyebrow>{eyebrow}</Eyebrow>
          <h1 className="mt-6 font-display text-5xl font-medium leading-[1.0] tracking-tight text-cream sm:text-6xl">
            {title}
          </h1>
          <p className="mt-6 max-w-md text-base leading-relaxed text-cream-dim">{copy}</p>
          {scripture ? (
            <blockquote className="mt-10 border-l-2 border-gold pl-6">
              <p className="font-display text-xl italic leading-relaxed text-cream">{scripture.text}</p>
              <cite className="mt-3 block text-[11px] font-medium uppercase tracking-[0.22em] not-italic text-gold-bright">
                {scripture.ref}
              </cite>
            </blockquote>
          ) : null}
        </Reveal>
        <Reveal delay={180}>
          <BezelCard>{children}</BezelCard>
        </Reveal>
      </div>
    </section>
  );
}
