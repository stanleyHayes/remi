import type { Metadata } from "next";
import ConnectShell from "@/components/connect-shell";
import { PrayerForm } from "@/components/connect-forms";

export const metadata: Metadata = {
  title: "Prayer Request",
  description: "Send a prayer request to the REMI intercessory team — you are not alone.",
};

export default function PrayerPage() {
  return (
    <ConnectShell
      eyebrow="We Pray With You"
      title="Let us carry it with you"
      copy="Whatever you're facing — sickness, family, direction, breakthrough — our intercessory team would be honoured to stand with you in prayer. Every request is handled with care and confidentiality."
      scripture={{
        text: "Cast all your anxiety on him because he cares for you.",
        ref: "1 Peter 5:7",
      }}
    >
      <PrayerForm />
    </ConnectShell>
  );
}
