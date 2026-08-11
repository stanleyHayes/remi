import type { Metadata } from "next";

const configuredSiteUrl = process.env.NEXT_PUBLIC_SITE_URL ?? "https://remi.vercel.app";

export const SITE_URL = configuredSiteUrl.replace(/\/$/, "");
export const SITE_NAME = "Ruach Elohim Ministries International";
export const SITE_SHORT_NAME = "REMI Church";
export const DEFAULT_DESCRIPTION =
  "Ruach Elohim Ministries International is a Spirit-filled apostolic and prophetic church in Accra, Ghana, founded by Dr. Ismaila Hans Awudu.";

export function absoluteUrl(path = "/"): string {
  return new URL(path, `${SITE_URL}/`).toString();
}

export function pageMetadata({
  title,
  description,
  path,
  image,
}: {
  title: string;
  description: string;
  path: string;
  image?: string;
}): Metadata {
  const url = absoluteUrl(path);
  const socialImage = image ? new URL(image, `${SITE_URL}/`).toString() : absoluteUrl("/opengraph-image");
  const images = [{ url: socialImage }];
  return {
    title,
    description,
    alternates: { canonical: url },
    openGraph: {
      title,
      description,
      url,
      siteName: SITE_SHORT_NAME,
      type: "website",
      images,
    },
    twitter: {
      card: "summary_large_image",
      title,
      description,
      images: [socialImage],
    },
  };
}

export function safeJsonLd(value: unknown): string {
  return JSON.stringify(value).replace(/</g, "\\u003c");
}
