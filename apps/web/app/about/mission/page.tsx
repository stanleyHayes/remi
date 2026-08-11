import AboutPage, { aboutMetadata } from "@/components/about-page";

export const metadata = aboutMetadata("mission");

export default function Page() {
  return <AboutPage pageKey="mission" />;
}
