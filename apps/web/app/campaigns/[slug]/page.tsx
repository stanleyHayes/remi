import { notFound } from "next/navigation";
import GiveForm from "@/components/give-form";
import Reveal from "@/components/reveal";
import { getFundraisingCampaign } from "@/lib/api";

const money = (minor: number) => new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS", maximumFractionDigits: 0 }).format(minor / 100);

export default async function CampaignPage({ params }: { params: Promise<{ slug: string }> }) {
  const { slug } = await params;
  const campaign = await getFundraisingCampaign(slug);
  if (!campaign) notFound();
  const progress = Math.min(100, Math.max(0, campaign.goal.amountMinor ? campaign.raisedAmountMinor / campaign.goal.amountMinor * 100 : 0));
  return <main className="campaign-detail">
    <section className="campaign-detail-hero" style={campaign.coverImageUrl ? { backgroundImage: `linear-gradient(90deg,rgba(13,21,15,.96),rgba(13,21,15,.42)),url(${campaign.coverImageUrl})` } : undefined}>
      <Reveal><a href="/campaigns">← All campaigns</a><p>Community-funded mission</p><h1>{campaign.title}</h1><span>{campaign.summary}</span></Reveal>
    </section>
    <section className="campaign-detail-body"><Reveal><article><p>{campaign.story}</p><div className="campaign-progress large"><i style={{ width: `${progress}%` }} /><strong>{Math.round(progress)}% funded</strong><small>{money(campaign.raisedAmountMinor)} of {money(campaign.goal.amountMinor)} · {campaign.giftCount} gifts</small></div></article></Reveal>
      <Reveal delay={120}><aside><h2>Give to this campaign</h2><p>Your payment is processed securely by Paystack. REMI never stores card details.</p><GiveForm categories={[campaign.title]} campaign={{ id: campaign.id, title: campaign.title }} /></aside></Reveal>
    </section>
  </main>;
}
