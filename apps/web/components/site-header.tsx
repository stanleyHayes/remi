"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useState } from "react";

const NAV = [
  { href: "/about", label: "About" },
  { href: "/leadership", label: "Leadership" },
  { href: "/sermons", label: "Sermons" },
  { href: "/events", label: "Events" },
  { href: "/ministries", label: "Ministries" },
  { href: "/branches", label: "Branches" },
];

const ease = "transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)]";

export default function SiteHeader({ isLive }: { isLive: boolean }) {
  const [open, setOpen] = useState(false);
  const pathname = usePathname();

  // Close the menu on navigation.
  useEffect(() => {
    setOpen(false);
  }, [pathname]);

  // Lock body scroll while the overlay is open.
  useEffect(() => {
    document.body.style.overflow = open ? "hidden" : "";
    return () => {
      document.body.style.overflow = "";
    };
  }, [open]);

  return (
    <>
      <header className="fixed inset-x-0 top-0 z-50 px-4 pt-4 sm:pt-6">
        <div className="mx-auto flex w-max max-w-full items-center gap-1 rounded-full border border-cream/10 bg-ink/70 py-2 pl-5 pr-2 backdrop-blur-2xl">
          <Link href="/" className="mr-3 flex items-baseline gap-2 whitespace-nowrap">
            <span className="font-display text-lg font-semibold tracking-tight text-cream">REMI</span>
            <span className="hidden text-[9px] font-medium uppercase tracking-[0.25em] text-gold-bright sm:inline">
              Ruach Elohim
            </span>
          </Link>

          <nav className="hidden items-center gap-0.5 lg:flex" aria-label="Primary">
            {NAV.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                className={`rounded-full px-4 py-2 text-[13px] font-medium ${ease} ${
                  pathname.startsWith(item.href)
                    ? "bg-cream/10 text-cream"
                    : "text-cream-dim hover:bg-cream/5 hover:text-cream"
                }`}
              >
                {item.label}
              </Link>
            ))}
          </nav>

          {isLive ? (
            <Link
              href="/live"
              className="ml-2 hidden items-center gap-2 rounded-full bg-red-500/15 px-4 py-2 text-[11px] font-semibold uppercase tracking-[0.15em] text-red-400 sm:inline-flex"
            >
              <span className="relative flex h-2 w-2">
                <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-400 opacity-60" />
                <span className="relative inline-flex h-2 w-2 rounded-full bg-red-500" />
              </span>
              Live
            </Link>
          ) : null}

          <Link
            href="/give"
            className={`ml-2 hidden rounded-full bg-gold px-5 py-2 text-[13px] font-semibold text-ink hover:bg-gold-bright sm:inline-flex ${ease}`}
          >
            Give
          </Link>

          {/* Hamburger */}
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-label={open ? "Close menu" : "Open menu"}
            aria-expanded={open}
            className="relative ml-1 flex h-10 w-10 items-center justify-center rounded-full text-cream hover:bg-cream/10 lg:hidden"
          >
            <span
              className={`absolute h-px w-4 bg-current ${ease} ${open ? "translate-y-0 rotate-45" : "-translate-y-[3.5px]"}`}
            />
            <span
              className={`absolute h-px w-4 bg-current ${ease} ${open ? "translate-y-0 -rotate-45" : "translate-y-[3.5px]"}`}
            />
          </button>
        </div>
      </header>

      {/* Full-screen mobile menu */}
      <div
        className={`fixed inset-0 z-40 bg-ink/90 backdrop-blur-3xl lg:hidden ${ease} ${
          open ? "pointer-events-auto opacity-100" : "pointer-events-none opacity-0"
        }`}
      >
        <nav className="flex h-full flex-col justify-center gap-1 px-8" aria-label="Mobile">
          {[...NAV, { href: "/connect/visit", label: "Plan Your Visit" }, { href: "/give", label: "Give" }].map(
            (item, i) => (
              <Link
                key={item.href}
                href={item.href}
                className={`font-display text-4xl font-medium text-cream ${ease} ${
                  open ? "translate-y-0 opacity-100" : "translate-y-12 opacity-0"
                }`}
                style={{ transitionDelay: open ? `${100 + i * 60}ms` : "0ms" }}
              >
                <span className="mr-4 text-xs text-gold-bright">0{i + 1}</span>
                {item.label}
              </Link>
            ),
          )}
          <p
            className={`mt-10 text-[10px] uppercase tracking-[0.25em] text-cream-dim ${ease} ${
              open ? "translate-y-0 opacity-100" : "translate-y-8 opacity-0"
            }`}
            style={{ transitionDelay: open ? "560ms" : "0ms" }}
          >
            Ruach Elohim Ministries International — Accra, Ghana
          </p>
        </nav>
      </div>
    </>
  );
}
