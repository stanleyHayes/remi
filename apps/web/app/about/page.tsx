import AboutPage, { aboutMetadata } from "@/components/about-page";

export const metadata = aboutMetadata("history");

export default function Page() {
  return <AboutPage pageKey="history" />;
}
