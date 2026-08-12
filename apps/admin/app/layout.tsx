import type { Metadata } from "next";
import { Geist_Mono, Outfit } from "next/font/google";
import { ToastProvider } from "@/components/ui/Toast";
import "./globals.css";
import "./audit.css";
import "./privacy-requests.css";

const outfit = Outfit({ subsets: ["latin"], variable: "--font-outfit" });
const geistMono = Geist_Mono({ subsets: ["latin"], variable: "--font-geist-mono" });

export const metadata: Metadata = {
  title: { default: "REMI Admin", template: "%s · REMI Admin" },
  description: "Secure ministry operations and content management for REMI.",
  robots: { index: false, follow: false, nocache: true },
  icons: { icon: "/icon.svg" },
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className={`${outfit.variable} ${geistMono.variable} font-sans antialiased`}>
        <ToastProvider>{children}</ToastProvider>
      </body>
    </html>
  );
}
