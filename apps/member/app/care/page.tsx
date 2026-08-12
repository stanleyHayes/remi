import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { MemberDashboardShell } from "@/components/member-dashboard-shell";
import { CareContentWorkspace, type CareContentData } from "@/components/care-content-workspace";
import { apiURL } from "@/lib/server-api";

async function get(path: string, access: string) { const response = await fetch(apiURL(path), { headers: { Authorization: `Bearer ${access}` }, cache: "no-store" }); return response.ok ? response.json() : null; }
export default async function CarePage() { const access = (await cookies()).get("remi_member_access")?.value; if (!access) redirect("/sign-in"); const [home, data] = await Promise.all([get("/api/member/home", access), get("/api/member/care-content", access)]); if (!home) redirect("/sign-in"); return <MemberDashboardShell active="care" member={home.member}><CareContentWorkspace initial={data as CareContentData | null} /></MemberDashboardShell>; }
