import type { Metadata } from "next";
import GiveForm from "@/components/give-form";
import Reveal from "@/components/reveal";
import { FALLBACK_SETTINGS, getSettings } from "@/lib/api";
import { pageMetadata } from "@/lib/seo";

export const metadata: Metadata = pageMetadata({
  title: "Give",
  description:
    "Partner with what God is doing — give securely online to REMI Church.",
  path: "/give",
});

const IMPACT = [
  {
    number: "01",
    title: "The house",
    copy: "Gatherings, worship and the everyday care of our church family.",
  },
  {
    number: "02",
    title: "The city",
    copy: "Practical outreach that meets people with dignity and compassion.",
  },
  {
    number: "03",
    title: "The future",
    copy: "Discipleship and spaces where the next generation can flourish.",
  },
];

export default async function GivePage() {
  const settings = await getSettings();
  const categories =
    settings.givingCategories?.length > 0
      ? settings.givingCategories
      : FALLBACK_SETTINGS.givingCategories;

  return (
    <main className="give-page">
      <div className="give-page-orbit" aria-hidden>
        <i />
        <i />
        <i />
      </div>

      <section className="give-hero">
        <Reveal>
          <p className="give-kicker">
            <span />
            Generosity at REMI
          </p>
          <h1>
            We give because
            <br />
            <em>He gave first.</em>
          </h1>
        </Reveal>

        <Reveal delay={120}>
          <div className="give-hero-note">
            <p>
              Generosity is our grateful response—not an obligation. Every gift
              helps make room for people to encounter God, find community and
              become who they were created to be.
            </p>
            <a href="#give-securely">
              Give securely <span aria-hidden>↓</span>
            </a>
          </div>
        </Reveal>
      </section>

      <section
        className="give-checkout"
        id="give-securely"
        aria-labelledby="give-form-title"
      >
        <Reveal>
          <div className="give-checkout-story">
            <p className="give-section-index">01 / Your gift</p>
            <h2 id="give-form-title">Choose where your generosity goes.</h2>
            <blockquote>
              “Each of you should give what you have decided in your heart to
              give—not reluctantly or under compulsion.”
              <cite>2 Corinthians 9:7</cite>
            </blockquote>
            <div className="give-assurance">
              <span aria-hidden>⌁</span>
              <div>
                <strong>Simple. Secure. Purposeful.</strong>
                <p>
                  Your payment is processed securely by Paystack. REMI never
                  stores your card details.
                </p>
              </div>
            </div>
          </div>
        </Reveal>

        <Reveal delay={140}>
          <GiveForm categories={categories} />
        </Reveal>
      </section>

      <section className="give-impact" aria-labelledby="give-impact-title">
        <div className="give-impact-heading">
          <Reveal>
            <p className="give-section-index">02 / Why we give</p>
            <h2 id="give-impact-title">
              A gift that
              <br />
              moves outward.
            </h2>
          </Reveal>
          <Reveal delay={100}>
            <p>
              Giving sustains the shared work of {settings.churchName}. It
              becomes ministry, welcome, care and hope in motion.
            </p>
          </Reveal>
        </div>

        <div className="give-impact-list">
          {IMPACT.map((item, index) => (
            <Reveal key={item.number} delay={index * 80}>
              <article>
                <span>{item.number}</span>
                <h3>{item.title}</h3>
                <p>{item.copy}</p>
                <i aria-hidden>↗</i>
              </article>
            </Reveal>
          ))}
        </div>
      </section>

      <section className="give-closing">
        <Reveal>
          <p>Thank you for building with us.</p>
          <h2>
            Open hands.
            <br />
            <em>Open heaven.</em>
          </h2>
        </Reveal>
      </section>
    </main>
  );
}
