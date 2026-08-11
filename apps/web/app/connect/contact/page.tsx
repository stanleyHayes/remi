import type { Metadata } from "next";
import ConnectShell from "@/components/connect-shell";
import { ContactForm } from "@/components/connect-forms";

export const metadata: Metadata = {
  title: "Contact Us",
  description: "Get in touch with Ruach Elohim Ministries International — we'd love to hear from you.",
};

export default function ContactPage() {
  return (
    <ConnectShell
      eyebrow="Say Hello"
      title="We'd love to hear from you"
      copy="Questions about services, ministries, partnership, or anything else? Send us a message and a member of our team will respond as soon as possible."
    >
      <ContactForm />
    </ConnectShell>
  );
}
