import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { InstallGuide } from "@/components/install-guide";
import { MemberDashboardShell } from "@/components/member-dashboard-shell";
import { apiURL } from "@/lib/server-api";

async function home(access: string) { const response = await fetch(apiURL("/api/member/home"), { headers: { Authorization: `Bearer ${access}` }, cache: "no-store" }); return response.ok ? response.json() : null; }

export default async function SupportPage() {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) redirect("/sign-in");
  const data = await home(access);
  if (!data) redirect("/sign-in");
  return <MemberDashboardShell active="support" member={data.member}><main className="member-support-page"><header><span>HELP & SUPPORT</span><h1>You should never feel<br/><em>stuck in your own space.</em></h1><p>Choose the path that matches what you need. Sensitive care, account access and data-rights requests stay separate on purpose.</p></header><div className="member-support-grid"><a href="/care"><i>01</i><b>Prayer or pastoral care</b><p>Share a private request or ask for a conversation.</p><span>Open Care →</span></a><a href="/account?tab=privacy"><i>02</i><b>Privacy or data correction</b><p>Change consent, directory visibility or submit a verified data request.</p><span>Open Privacy →</span></a><a href={`${process.env.NEXT_PUBLIC_SITE_URL || "http://localhost:3010"}/connect/contact`}><i>03</i><b>Something is not working</b><p>Contact the church team without adding sensitive details to a public form.</p><span>Contact REMI →</span></a><a href="/account?tab=security"><i>04</i><b>Account and sign-in</b><p>Review devices, end sessions or strengthen access with MFA.</p><span>Open Security →</span></a></div><InstallGuide/><section className="member-support-legal"><span>YOUR RIGHTS</span><h2>Clear choices, visible boundaries.</h2><p>Read how this workspace handles member information, then use the authenticated request path when you need access, correction, portability, objection or deletion review.</p><div><a href="/privacy">Privacy summary</a><a href="/terms">Member terms</a></div><small>Official legal wording and support ownership must be approved by REMI’s accountable owner before launch.</small></section></main></MemberDashboardShell>;
}
