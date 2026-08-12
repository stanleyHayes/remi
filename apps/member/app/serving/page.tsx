import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { MemberDashboardShell } from "@/components/member-dashboard-shell";
import { ServingWorkspace, type ServingData } from "@/components/serving-workspace";
import { apiURL } from "@/lib/server-api";
async function get(path:string,access:string){const response=await fetch(apiURL(path),{headers:{Authorization:`Bearer ${access}`},cache:"no-store"});return response.ok?response.json():null}
export default async function ServingPage(){const access=(await cookies()).get("remi_member_access")?.value;if(!access)redirect("/sign-in");const[home,data]=await Promise.all([get("/api/member/home",access),get("/api/member/serving-workspace",access)]);if(!home)redirect("/sign-in");return <MemberDashboardShell active="serve" member={home.member}><ServingWorkspace initial={data as ServingData|null}/></MemberDashboardShell>}
