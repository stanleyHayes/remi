import type { Metadata } from "next";
import ConnectShell from "@/components/connect-shell";
import { VisitForm } from "@/components/connect-forms";
import { getBranches } from "@/lib/api";
import { pageMetadata } from "@/lib/seo";

export const metadata: Metadata = pageMetadata({
  title: "Plan Your Visit",
  description: "Coming to REMI Church for the first time? Tell us you're coming and we'll be ready for you.",
  path: "/connect/visit",
});

export default async function VisitPage() {
  const branches = (await getBranches()) ?? [];
  const branchNames = branches.map((b) => b.name);

  return (
    <ConnectShell
      eyebrow="New Here?"
      title="Plan your visit"
      copy="First time? Wonderful. Tell us when you're coming and we'll meet you at the door, help you find a seat, and answer anything you'd like to know. Come as you are — you'll fit right in."
      scripture={{
        text: "For where two or three gather in my name, there am I with them.",
        ref: "Matthew 18:20",
      }}
    >
      <VisitForm branches={branchNames} />
    </ConnectShell>
  );
}
