import Link from "next/link";
import { FALLBACK_SETTINGS, getSettings } from "@/lib/api";
import NewsletterForm from "./newsletter-form";

const QUICK_LINKS = [
  { href: "/about", label: "About" },
  { href: "/leadership", label: "Leadership" },
  { href: "/sermons", label: "Sermons" },
  { href: "/events", label: "Events" },
  { href: "/ministries", label: "Ministries" },
  { href: "/branches", label: "Branches" },
  { href: "/gallery", label: "Gallery" },
];

const CONNECT_LINKS = [
  { href: "/connect/visit", label: "Plan Your Visit" },
  { href: "/connect/prayer", label: "Prayer Request" },
  { href: "/connect/testimony", label: "Share a Testimony" },
  { href: "/connect/contact", label: "Contact Us" },
  { href: "/live", label: "Watch Live" },
  { href: "/give", label: "Give" },
];

function SocialIcon({ label, href, path }: { label: string; href: string; path: string }) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={label}
      className="flex h-10 w-10 items-center justify-center rounded-full border border-cream/10 bg-cream/5 text-cream-dim transition-all duration-500 ease-[cubic-bezier(0.32,0.72,0,1)] hover:border-gold/40 hover:text-gold-bright"
    >
      <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden>
        <path d={path} />
      </svg>
    </a>
  );
}

const SOCIAL_PATHS: Record<string, string> = {
  facebook:
    "M13.5 21v-7h2.4l.4-3h-2.8V9.1c0-.9.3-1.5 1.6-1.5h1.3V4.9c-.6-.1-1.4-.2-2.3-.2-2.3 0-3.9 1.4-3.9 4V11H7.5v3h2.7v7h3.3Z",
  instagram:
    "M12 8.8A3.2 3.2 0 1 0 12 15.2 3.2 3.2 0 0 0 12 8.8Zm0-1.8a5 5 0 1 1 0 10 5 5 0 0 1 0-10Zm6.4-.3a1.2 1.2 0 1 1-2.4 0 1.2 1.2 0 0 1 2.4 0ZM12 4.2c-2.1 0-2.4 0-3.2.1-.8 0-1.4.2-1.9.4-.5.2-1 .5-1.4.9-.4.4-.7.9-.9 1.4-.2.5-.4 1.1-.4 1.9-.1.8-.1 1.1-.1 3.2s0 2.4.1 3.2c0 .8.2 1.4.4 1.9.2.5.5 1 .9 1.4.4.4.9.7 1.4.9.5.2 1.1.4 1.9.4.8.1 1.1.1 3.2.1s2.4 0 3.2-.1c.8 0 1.4-.2 1.9-.4.5-.2 1-.5 1.4-.9.4-.4.7-.9.9-1.4.2-.5.4-1.1.4-1.9.1-.8.1-1.1.1-3.2s0-2.4-.1-3.2c0-.8-.2-1.4-.4-1.9-.2-.5-.5-1-.9-1.4a3.6 3.6 0 0 0-1.4-.9c-.5-.2-1.1-.4-1.9-.4-.8-.1-1.1-.1-3.2-.1Z",
  youtube:
    "M21.6 7.2a2.5 2.5 0 0 0-1.8-1.8C18.2 5 12 5 12 5s-6.2 0-7.8.4A2.5 2.5 0 0 0 2.4 7.2 26 26 0 0 0 2 12a26 26 0 0 0 .4 4.8 2.5 2.5 0 0 0 1.8 1.8c1.6.4 7.8.4 7.8.4s6.2 0 7.8-.4a2.5 2.5 0 0 0 1.8-1.8A26 26 0 0 0 22 12a26 26 0 0 0-.4-4.8ZM10 15.2V8.8L15.5 12 10 15.2Z",
  tiktok:
    "M16.6 3c.3 1.9 1.5 3.4 3.4 3.6v2.9c-1.2 0-2.4-.4-3.4-1v6.1A5.9 5.9 0 1 1 10.7 8.7v3.1a2.9 2.9 0 1 0 2.9 2.9V3h3Z",
};

export default async function SiteFooter() {
  const remoteSettings = await getSettings();
  const settings = {
    ...FALLBACK_SETTINGS,
    ...remoteSettings,
    serviceTimes: remoteSettings?.serviceTimes ?? FALLBACK_SETTINGS.serviceTimes,
    socials: remoteSettings?.socials ?? FALLBACK_SETTINGS.socials,
    givingCategories: remoteSettings?.givingCategories ?? FALLBACK_SETTINGS.givingCategories,
  };
  const socials = Object.entries(settings.socials).filter(([, url]) => !!url);

  return (
    <footer className="border-t border-cream/10 bg-ink-soft">
      <div className="mx-auto max-w-7xl px-5 py-20 sm:px-8 sm:py-24">
        <div className="grid gap-14 lg:grid-cols-12">
          {/* Brand + newsletter */}
          <div className="lg:col-span-5">
            <p className="font-display text-3xl font-medium tracking-tight text-cream">
              {settings.churchName}
            </p>
            <p className="mt-3 max-w-sm text-sm leading-relaxed text-cream-dim">{settings.tagline}</p>
            <div className="mt-8">
              <p className="mb-4 text-[10px] font-medium uppercase tracking-[0.22em] text-gold-bright">
                Stay connected
              </p>
              <NewsletterForm />
            </div>
            {socials.length > 0 ? (
              <div className="mt-8 flex gap-3">
                {socials.map(([name, url]) => (
                  <SocialIcon key={name} label={name} href={url as string} path={SOCIAL_PATHS[name] ?? SOCIAL_PATHS.facebook} />
                ))}
              </div>
            ) : null}
          </div>

          {/* Service times */}
          <div className="lg:col-span-3">
            <p className="text-[10px] font-medium uppercase tracking-[0.22em] text-gold-bright">Service Times</p>
            <ul className="mt-5 space-y-5">
              {settings.serviceTimes.map((s) => (
                <li key={`${s.name}-${s.day}`}>
                  <p className="text-sm font-medium text-cream">{s.name}</p>
                  <p className="mt-1 text-sm text-cream-dim">
                    {s.day}s · {s.time}
                  </p>
                  <p className="text-xs text-cream-dim/70">{s.location}</p>
                </li>
              ))}
            </ul>
          </div>

          {/* Links */}
          <div className="grid grid-cols-2 gap-10 lg:col-span-4">
            <div>
              <p className="text-[10px] font-medium uppercase tracking-[0.22em] text-gold-bright">Explore</p>
              <ul className="mt-5 space-y-3">
                {QUICK_LINKS.map((l) => (
                  <li key={l.href}>
                    <Link href={l.href} className="text-sm text-cream-dim transition-colors duration-300 hover:text-cream">
                      {l.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
            <div>
              <p className="text-[10px] font-medium uppercase tracking-[0.22em] text-gold-bright">Connect</p>
              <ul className="mt-5 space-y-3">
                {CONNECT_LINKS.map((l) => (
                  <li key={l.href}>
                    <Link href={l.href} className="text-sm text-cream-dim transition-colors duration-300 hover:text-cream">
                      {l.label}
                    </Link>
                  </li>
                ))}
              </ul>
            </div>
          </div>
        </div>

        <div className="mt-16 flex flex-col gap-4 border-t border-cream/10 pt-8 text-xs text-cream-dim/70 sm:flex-row sm:items-center sm:justify-between">
          <p>
            © {new Date().getFullYear()} {settings.churchName}. All rights reserved.
          </p>
          <p>
            {[settings.address, settings.phone, settings.email].filter(Boolean).join(" · ") || "Accra, Ghana"}
          </p>
        </div>
      </div>
    </footer>
  );
}
