import Link from "next/link";
import { getSettings } from "@/lib/api";
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

function SocialIcon({
  label,
  href,
  path,
}: {
  label: string;
  href: string;
  path: string;
}) {
  return (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      aria-label={label}
      className="public-footer-social"
    >
      <svg
        width="16"
        height="16"
        viewBox="0 0 24 24"
        fill="currentColor"
        aria-hidden
      >
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
  const settings = await getSettings();
  const socials = Object.entries(settings.socials).filter(([, url]) => !!url);
  const serviceTimes = settings.serviceTimes ?? [];
  const primaryService = serviceTimes[0];

  return (
    <footer className="public-footer">
      <div className="public-footer-orbit" aria-hidden>
        <span />
        <span />
        <span />
      </div>
      <div className="public-footer-word" aria-hidden>
        RUACH
      </div>
      <div className="public-footer-inner">
        <section
          className="public-footer-invitation"
          aria-labelledby="footer-invitation"
        >
          <div>
            <p className="public-footer-kicker">
              <span /> Spirit · Word · Community
            </p>
            <h2 id="footer-invitation">
              There is a place
              <br />
              <em>for you here.</em>
            </h2>
          </div>
          <div className="public-footer-invitation-side">
            <p>
              Come as you are. Meet a community growing in faith, purpose and
              the life of the Spirit.
            </p>
            <div className="public-footer-actions">
              <Link href="/connect/visit" className="public-footer-primary">
                Plan your visit <Arrow />
              </Link>
              <Link href="/live" className="public-footer-text-link">
                Watch online <Arrow />
              </Link>
            </div>
          </div>
        </section>

        <div className="public-footer-main">
          <section className="public-footer-brand" aria-label="About REMI">
            <div className="public-footer-mark" aria-hidden>
              <span>R</span>
              <i />
            </div>
            <p className="public-footer-church-name">{settings.churchName}</p>
            <p className="public-footer-tagline">{settings.tagline}</p>
            {socials.length > 0 ? (
              <div className="public-footer-socials">
                {socials.map(([name, url]) => (
                  <SocialIcon
                    key={name}
                    label={name}
                    href={url as string}
                    path={SOCIAL_PATHS[name] ?? SOCIAL_PATHS.facebook}
                  />
                ))}
              </div>
            ) : null}
          </section>

          <section
            className="public-footer-gathering"
            aria-labelledby="footer-gathering"
          >
            <p className="public-footer-label">Next gathering</p>
            {primaryService ? (
              <>
                <h3 id="footer-gathering">{primaryService.name}</h3>
                <p className="public-footer-time">
                  {primaryService.day}s <span>·</span> {primaryService.time}
                </p>
                <p className="public-footer-location">
                  {primaryService.location}
                </p>
                {serviceTimes.length > 1 ? (
                  <div className="public-footer-more-times">
                    {serviceTimes.slice(1).map((service) => (
                      <div key={`${service.name}-${service.day}`}>
                        <span>{service.name}</span>
                        <time>
                          {service.day}s · {service.time}
                        </time>
                      </div>
                    ))}
                  </div>
                ) : null}
              </>
            ) : (
              <>
                <h3 id="footer-gathering">Join us in Accra</h3>
                <p className="public-footer-location">
                  Service details are being updated.
                </p>
              </>
            )}
            <Link href="/connect/visit" className="public-footer-directions">
              Plan the journey <Arrow />
            </Link>
          </section>

          <section
            className="public-footer-letter"
            aria-labelledby="footer-letter"
          >
            <p className="public-footer-label">A note for your week</p>
            <h3 id="footer-letter">Stay close to what God is doing.</h3>
            <p>
              Occasional encouragement, ministry updates and invitations. No
              noise.
            </p>
            <NewsletterForm />
          </section>
        </div>

        <nav
          className="public-footer-navigation"
          aria-label="Footer navigation"
        >
          <div>
            <p className="public-footer-label">Explore REMI</p>
            {QUICK_LINKS.map((link, index) => (
              <Link href={link.href} key={link.href}>
                <span>{String(index + 1).padStart(2, "0")}</span>
                {link.label}
                <Arrow />
              </Link>
            ))}
          </div>
          <div>
            <p className="public-footer-label">Take a next step</p>
            {CONNECT_LINKS.map((link, index) => (
              <Link href={link.href} key={link.href}>
                <span>{String(index + 1).padStart(2, "0")}</span>
                {link.label}
                <Arrow />
              </Link>
            ))}
          </div>
        </nav>

        <div className="public-footer-bottom">
          <p>
            © {new Date().getFullYear()} {settings.churchName}
          </p>
          <p className="public-footer-contact">
            {[settings.address, settings.phone, settings.email]
              .filter(Boolean)
              .join(" · ") || "Accra, Ghana"}
          </p>
          <Link href="#top">
            Back to top <span aria-hidden>↑</span>
          </Link>
        </div>
      </div>
    </footer>
  );
}

function Arrow() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden>
      <path d="M4 10h11M11 5l5 5-5 5" />
    </svg>
  );
}
