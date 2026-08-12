import type { Metadata } from "next";
import Link from "next/link";
import Reveal from "@/components/reveal";
import { getFundraisingCampaigns } from "@/lib/api";
import { pageMetadata } from "@/lib/seo";

export const metadata: Metadata = pageMetadata({ title: "Fundraising campaigns", description: "See the projects REMI Church is building with our community and give securely toward a shared goal.", path: "/campaigns" });

const money = (minor: number) => new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS", maximumFractionDigits: 0 }).format(minor / 100);

export default async function CampaignsPage() {
  const campaigns = await getFundraisingCampaigns();
  return <main className="campaign-page">
    <section className="campaign-hero"><Reveal><p>Generosity in motion</p><h1>Build something<br/><em>that outlives us.</em></h1><span>Transparent goals. Secure giving. Shared impact.</span></Reveal></section>
    <section className="campaign-grid" aria-label="Active fundraising campaigns">
      {campaigns.map((campaign, index) => {
        const progress = Math.min(100, Math.max(0, campaign.goal.amountMinor ? campaign.raisedAmountMinor / campaign.goal.amountMinor * 100 : 0));
        return <Reveal key={campaign.id} delay={index * 80}><article className="campaign-card">
          {campaign.coverImageUrl && <div className="campaign-card-image" style={{ backgroundImage: `linear-gradient(180deg,transparent,rgba(15,22,17,.62)),url(${campaign.coverImageUrl})` }} />}
          <div><span>{campaign.featured ? "Featured campaign" : "Fundraising campaign"}</span><h2>{campaign.title}</h2><p>{campaign.summary}</p>
            <div className="campaign-progress"><i style={{ width: `${progress}%` }} /><small>{money(campaign.raisedAmountMinor)} raised of {money(campaign.goal.amountMinor)}</small></div>
            <Link href={`/campaigns/${campaign.slug}`}>View campaign <b aria-hidden>↗</b></Link>
          </div>
        </article></Reveal>;
      })}
      {campaigns.length === 0 && <p className="campaign-empty">No public campaigns are open right now. Please check back soon.</p>}
    </section>
  </main>;
}
