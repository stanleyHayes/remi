import Link from "next/link";
import type { ReactNode } from "react";

export function Eyebrow({ children }: { children: ReactNode }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-gold/25 bg-gold/10 px-3.5 py-1.5 text-[10px] font-medium uppercase tracking-[0.22em] text-gold-bright">
      {children}
    </span>
  );
}

const ease = "transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]";

interface CTAButtonProps {
  href: string;
  children: ReactNode;
  variant?: "primary" | "ghost";
  className?: string;
  external?: boolean;
}

/** Pill CTA with the nested "button-in-button" trailing icon. */
export function CTAButton({ href, children, variant = "primary", className = "", external }: CTAButtonProps) {
  const base = `group inline-flex items-center gap-3 rounded-full py-2.5 pl-7 pr-2.5 text-sm font-medium active:scale-[0.98] ${ease}`;
  const styles =
    variant === "primary"
      ? "bg-gold text-ink hover:bg-gold-bright"
      : "border border-cream/15 bg-cream/5 text-cream hover:border-gold/40 hover:text-gold-bright";
  const icon = (
    <span
      className={`flex h-9 w-9 items-center justify-center rounded-full ${ease} group-hover:translate-x-0.5 group-hover:-translate-y-px group-hover:scale-105 ${
        variant === "primary" ? "bg-ink/10" : "bg-cream/10"
      }`}
      aria-hidden
    >
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
        <path d="M7 17L17 7M17 7H8M17 7v9" />
      </svg>
    </span>
  );

  if (external) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" className={`${base} ${styles} ${className}`}>
        {children}
        {icon}
      </a>
    );
  }
  return (
    <Link href={href} className={`${base} ${styles} ${className}`}>
      {children}
      {icon}
    </Link>
  );
}

export function SectionHeading({
  eyebrow,
  title,
  copy,
  align = "left",
}: {
  eyebrow: string;
  title: string;
  copy?: string;
  align?: "left" | "center";
}) {
  return (
    <div className={align === "center" ? "mx-auto max-w-2xl text-center" : "max-w-2xl"}>
      <Eyebrow>{eyebrow}</Eyebrow>
      <h2 className="mt-6 font-display text-4xl leading-[1.05] font-medium tracking-tight text-cream sm:text-5xl">
        {title}
      </h2>
      {copy ? <p className="mt-5 text-base leading-relaxed text-cream-dim">{copy}</p> : null}
    </div>
  );
}

export function EmptyState({ title, copy }: { title: string; copy?: string }) {
  return (
    <div className="rounded-[2rem] border border-cream/10 bg-ink-soft p-10 text-center sm:p-14">
      <div className="mx-auto mb-5 flex h-12 w-12 items-center justify-center rounded-full border border-gold/25 bg-gold/10 text-gold-bright">
        <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.25" strokeLinecap="round" strokeLinejoin="round">
          <path d="M12 3v18M5 10l7-7 7 7" />
        </svg>
      </div>
      <p className="font-display text-xl text-cream">{title}</p>
      {copy ? <p className="mx-auto mt-2 max-w-md text-sm leading-relaxed text-cream-dim">{copy}</p> : null}
    </div>
  );
}

/** Double-bezel (Doppelrand) card shell. */
export function BezelCard({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div className={`rounded-[2rem] border border-cream/10 bg-cream/[0.03] p-1.5 ${className}`}>
      <div className="h-full rounded-[calc(2rem-0.375rem)] bg-ink-soft shadow-[inset_0_1px_1px_rgba(245,241,232,0.06)]">
        {children}
      </div>
    </div>
  );
}
