import type { Metadata } from "next";
import { Outfit, Space_Mono } from "next/font/google";
import "./globals.css";
import "./choice-controls.css";
import { MemberRuntime } from "@/components/member-runtime";

const outfit = Outfit({ subsets: ["latin"], variable: "--font-outfit" });
const mono = Space_Mono({
  subsets: ["latin"],
  weight: ["400", "700"],
  variable: "--font-mono",
});

export const metadata: Metadata = {
  title: "My REMI",
  description: "Your place to belong, grow, serve and give at REMI Church.",
  applicationName: "My REMI",
  manifest: "/manifest.webmanifest",
  appleWebApp: { capable: true, title: "My REMI", statusBarStyle: "black-translucent" },
  robots: { index: false, follow: false, nocache: true },
};

export default function RootLayout({
  children,
}: Readonly<{ children: React.ReactNode }>) {
  return (
    <html lang="en">
      <body className={`${outfit.variable} ${mono.variable}`}><MemberRuntime />{children}</body>
    </html>
  );
}
