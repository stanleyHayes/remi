import type { Metadata } from "next";
import { Outfit } from "next/font/google";
import { Suspense } from "react";
import "./globals.css";
import SiteHeader from "@/components/site-header";
import SiteFooter from "@/components/site-footer";
import { getSettings } from "@/lib/api";
import { FooterSkeleton, HeaderSkeleton } from "@/components/skeletons";

const outfit = Outfit({
  subsets: ["latin"],
  variable: "--font-outfit",
  display: "swap",
});

export const metadata: Metadata = {
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL ?? "http://localhost:3000"),
  title: {
    default: "Ruach Elohim Ministries International (REMI) — Accra, Ghana",
    template: "%s | REMI Church",
  },
  description:
    "Ruach Elohim Ministries International is a Spirit-filled apostolic and prophetic ministry in Accra, Ghana, founded by Dr. Ismaila Hans Awudu. Join us this Sunday.",
  openGraph: {
    siteName: "REMI Church",
    type: "website",
    images: ["https://picsum.photos/seed/remi-og/1200/630"],
  },
  twitter: {
    card: "summary_large_image",
  },
};

async function ConnectedHeader() {
  const settings = await getSettings();
	return <SiteHeader isLive={settings?.isLive ?? false} />;
}

export default function RootLayout({ children }: { children: React.ReactNode }) {

  return (
    <html lang="en" className={outfit.variable} data-scroll-behavior="smooth">
      <body className="grain min-h-[100dvh] bg-ink font-sans text-cream antialiased">
		<Suspense fallback={<HeaderSkeleton />}><ConnectedHeader /></Suspense>
        <main>{children}</main>
		<Suspense fallback={<FooterSkeleton />}><SiteFooter /></Suspense>
      </body>
    </html>
  );
}
