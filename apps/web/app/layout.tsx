import type { Metadata } from "next";
import { Outfit } from "next/font/google";
import { Suspense } from "react";
import "./globals.css";
import SiteHeader from "@/components/site-header";
import SiteFooter from "@/components/site-footer";
import { getSettings } from "@/lib/api";
import { FooterSkeleton, HeaderSkeleton } from "@/components/skeletons";
import {
  absoluteUrl,
  DEFAULT_DESCRIPTION,
  safeJsonLd,
  SITE_NAME,
  SITE_SHORT_NAME,
  SITE_URL,
} from "@/lib/seo";

const outfit = Outfit({
  subsets: ["latin"],
  variable: "--font-outfit",
  display: "swap",
});

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  applicationName: SITE_SHORT_NAME,
  title: {
    default: `${SITE_NAME} (REMI) — Accra, Ghana`,
    template: "%s | REMI Church",
  },
  description: DEFAULT_DESCRIPTION,
  keywords: [
    "REMI Church",
    "Ruach Elohim Ministries International",
    "church in Accra",
    "Spirit-filled church Ghana",
    "prophetic ministry Ghana",
    "Dr. Ismaila Hans Awudu",
  ],
  authors: [{ name: SITE_NAME, url: SITE_URL }],
  creator: SITE_NAME,
  publisher: SITE_NAME,
  category: "religion",
  alternates: { canonical: SITE_URL },
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-image-preview": "large",
      "max-snippet": -1,
      "max-video-preview": -1,
    },
  },
  openGraph: {
    title: `${SITE_NAME} (REMI) — Accra, Ghana`,
    description: DEFAULT_DESCRIPTION,
    url: SITE_URL,
    siteName: SITE_SHORT_NAME,
    locale: "en_GH",
    type: "website",
  },
  twitter: {
    card: "summary_large_image",
    title: `${SITE_NAME} (REMI) — Accra, Ghana`,
    description: DEFAULT_DESCRIPTION,
  },
  verification: process.env.GOOGLE_SITE_VERIFICATION
    ? { google: process.env.GOOGLE_SITE_VERIFICATION }
    : undefined,
};

async function ConnectedHeader() {
  const settings = await getSettings();
  return <SiteHeader isLive={settings?.isLive ?? false} />;
}

async function OrganizationJsonLd() {
  const settings = await getSettings();
  const sameAs = Object.values(settings.socials).filter((url): url is string =>
    Boolean(url),
  );
  const graph = {
    "@context": "https://schema.org",
    "@graph": [
      {
        "@type": ["Church", "Organization"],
        "@id": `${SITE_URL}/#organization`,
        name: settings.churchName || SITE_NAME,
        alternateName: SITE_SHORT_NAME,
        url: SITE_URL,
        logo: absoluteUrl("/icon.svg"),
        description: settings.tagline || DEFAULT_DESCRIPTION,
        email: settings.email || undefined,
        telephone: settings.phone || undefined,
        address: settings.address
          ? {
              "@type": "PostalAddress",
              streetAddress: settings.address,
              addressLocality: "Accra",
              addressCountry: "GH",
            }
          : undefined,
        sameAs: sameAs.length ? sameAs : undefined,
      },
      {
        "@type": "WebSite",
        "@id": `${SITE_URL}/#website`,
        url: SITE_URL,
        name: SITE_SHORT_NAME,
        publisher: { "@id": `${SITE_URL}/#organization` },
        inLanguage: "en-GH",
      },
    ],
  };
  return (
    <script
      type="application/ld+json"
      dangerouslySetInnerHTML={{ __html: safeJsonLd(graph) }}
    />
  );
}

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html lang="en" className={outfit.variable} data-scroll-behavior="smooth">
      <body
        id="top"
        className="grain min-h-dvh bg-ink font-sans text-cream antialiased"
      >
        <Suspense fallback={null}>
          <OrganizationJsonLd />
        </Suspense>
        <Suspense fallback={<HeaderSkeleton />}>
          <ConnectedHeader />
        </Suspense>
        <main>{children}</main>
        <Suspense fallback={<FooterSkeleton />}>
          <SiteFooter />
        </Suspense>
      </body>
    </html>
  );
}
