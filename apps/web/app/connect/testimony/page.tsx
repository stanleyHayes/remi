import type { Metadata } from "next";
import ConnectShell from "@/components/connect-shell";
import { TestimonyForm } from "@/components/connect-forms";

export const metadata: Metadata = {
  title: "Share a Testimony",
  description: "What has God done for you? Share your testimony with the REMI family.",
};

export default function TestimonyPage() {
  return (
    <ConnectShell
      eyebrow="Give God Glory"
      title="Your story matters"
      copy="Healed, provided for, restored, delivered — whatever God has done, your testimony strengthens the faith of the whole family. Share it with us."
      scripture={{
        text: "They triumphed over him by the blood of the Lamb and by the word of their testimony.",
        ref: "Revelation 12:11",
      }}
    >
      <TestimonyForm />
    </ConnectShell>
  );
}
