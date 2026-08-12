import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import { MemberDashboardShell } from "@/components/member-dashboard-shell";
import { Icon } from "@/components/icon";
import { apiURL } from "@/lib/server-api";
import { LeaderActions } from "@/components/leader-actions";
import { LeaderRoster } from "@/components/leader-roster";
import { LeaderTeamSchedule } from "@/components/leader-team-schedule";

type Meeting = { id: string; topic: string; startsAt: string; location?: string };
type Workspace = { groups: Array<{ group: { id: string; name: string; type: string }; activeMembers: number; pendingMembers: number; upcomingMeetings: Meeting[] }>; teams: Array<{ team: { id: string; name: string }; positions: Array<{ id: string; name: string }> }>; assignments: Array<{ id: string; status: string; startsAt: string }>; generatedAt: string };
type Home = { member: { name: string; branchName?: string } };

async function get(path: string, access: string) {
  const response = await fetch(apiURL(path), { headers: { Authorization: `Bearer ${access}` }, cache: "no-store" });
  return response.ok ? response.json() : null;
}

export default async function LeadPage() {
  const access = (await cookies()).get("remi_member_access")?.value;
  if (!access) redirect("/sign-in");
  const now = new Date();
  const until = new Date(now.getTime() + 120 * 24 * 60 * 60 * 1000);
  const [home, workspace] = await Promise.all([
    get("/api/member/home", access) as Promise<Home | null>,
    get(`/api/chms/v1/leader-workspace?from=${encodeURIComponent(now.toISOString())}&to=${encodeURIComponent(until.toISOString())}`, access) as Promise<Workspace | null>,
  ]);
  if (!home) redirect("/sign-in");
  return <MemberDashboardShell active="lead" member={home.member}>
    <main className="member-leader-page">
      <header className="member-leader-hero"><div><span>LEADER DESK · PRIVATE</span><h1>Care for the people<br/><em>entrusted to you.</em></h1><p>Your owned groups, serving teams and next actions—without exposing the wider church directory.</p></div><aside><b>{String(workspace?.groups.length || 0).padStart(2,"0")}</b><small>groups in your care</small></aside></header>
      {!workspace ? <section className="member-leader-empty"><Icon name="users"/><h2>No leader workspace is connected</h2><p>Your member identity must be named as a leader on a group or serving team. Ask an administrator to review the assignment.</p></section> : <>
        <section className="member-leader-summary"><article><span>01</span><b>{workspace.groups.reduce((sum,item)=>sum+item.activeMembers,0)}</b><p>active group members</p></article><article><span>02</span><b>{workspace.groups.reduce((sum,item)=>sum+item.pendingMembers,0)}</b><p>people awaiting review</p></article><article><span>03</span><b>{workspace.teams.length}</b><p>serving teams</p></article><article><span>04</span><b>{workspace.assignments.length}</b><p>personal assignments ahead</p></article></section>
        <div className="member-leader-grid"><section><header><span>01 · COMMUNITY</span><h2>Your groups</h2></header><div>{workspace.groups.map(item=><article key={item.group.id}><div><span>{item.group.type}</span><h3>{item.group.name}</h3><p>{item.activeMembers} active · {item.pendingMembers} awaiting review</p></div><footer>{item.upcomingMeetings[0]?<><time>{new Date(item.upcomingMeetings[0].startsAt).toLocaleString("en-GH",{weekday:"short",day:"numeric",month:"short",hour:"numeric",minute:"2-digit"})}</time><small>{item.upcomingMeetings[0].topic}</small></>:<small>No meeting scheduled in the next 120 days</small>}</footer></article>)}{!workspace.groups.length&&<p className="member-leader-placeholder">No owned groups are connected to this identity.</p>}</div></section>
        <section><header><span>02 · SERVING</span><h2>Your teams</h2></header><div>{workspace.teams.map(item=><article key={item.team.id}><div><span>serving team</span><h3>{item.team.name}</h3><p>{item.positions.length} active position{item.positions.length===1?"":"s"}</p></div><footer>{item.positions.map(position=><small key={position.id}>{position.name}</small>)}</footer></article>)}{!workspace.teams.length&&<p className="member-leader-placeholder">No owned serving teams are connected to this identity.</p>}</div></section></div>
        <LeaderActions groups={workspace.groups.map(item=>({id:item.group.id,name:item.group.name}))}/>
        <LeaderRoster groups={workspace.groups.map(item=>({id:item.group.id,name:item.group.name}))}/>
        <LeaderTeamSchedule teams={workspace.teams.map(item=>({id:item.team.id,name:item.team.name}))}/>
        <p className="member-leader-freshness">Workspace generated {new Date(workspace.generatedAt).toLocaleString("en-GH")}. Leadership scope is checked again on every request.</p>
      </>}
    </main>
  </MemberDashboardShell>;
}
