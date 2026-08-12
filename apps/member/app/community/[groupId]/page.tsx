import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { MemberDashboardShell } from "@/components/member-dashboard-shell";
import { CommunityWorkspace, type CommunityData } from "@/components/community-workspace";
import { apiURL } from "@/lib/server-api";

async function get(path:string,access:string){const response=await fetch(apiURL(path),{headers:{Authorization:`Bearer ${access}`},cache:"no-store"});return response.ok?response.json():null}
export default async function CommunityPage({params}:{params:Promise<{groupId:string}>}){const access=(await cookies()).get("remi_member_access")?.value;if(!access)redirect("/sign-in");const{groupId}=await params;const[home,data]=await Promise.all([get("/api/member/home",access),get(`/api/member/groups/${encodeURIComponent(groupId)}/community`,access)]);if(!home)redirect("/sign-in");if(!data)redirect("/participation#groups");return <MemberDashboardShell active="participation" member={home.member}><CommunityWorkspace initial={data as CommunityData}/></MemberDashboardShell>}
