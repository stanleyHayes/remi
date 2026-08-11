import AboutPage, { aboutMetadata } from "@/components/about-page";

export const metadata = aboutMetadata("beliefs");

export default function Page() {
  return <AboutPage pageKey="beliefs" />;
}
