"use client";

import Link from "next/link";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, asList, getStoredUser } from "@/lib/api";
import { Select } from "@/components/ui/Select";

type Branch={id:string;name:string};
type Attendance={summary:{completedOccurrences:number;uniqueNamedPeople:number;namedAttendances:number;anonymousHeadcount:number;firstTimeGuests:number;returningGuests:number};trends:Array<{name:string;startsAt:string;namedPresent:number;anonymousHeadcount:number}>;quality:{namedCoveragePercent:number;latestRecordedAt?:string;caveats:string[]};generatedAt:string};
type Community={summary:{activeGroups:number;activeGroupMemberships:number;uniqueConnectedPeople:number;activeVolunteerTeams:number;uniqueScheduledVolunteers:number;plannedServingSlots:number;filledServingSlots:number};quality:{latestRecordedAt?:string;caveats:string[]};generatedAt:string};
type Retention={cohorts:Array<{cohortState:string;return30:{state:string;ratePercent?:number};combinedConnection:{state:string;ratePercent?:number}}>};
type Finance={totals:{netContributionMinor:number;contributionCount:number;unexplainedMinor:number;pledgeTargetMinor:number;pledgeFulfilledMinor:number};meta:{currency?:string;asOf:string;caveats:string[]}};
type Period={id:string;name:string;startsAt:string;endsAt:string};
type CareCase={id:string;state:string;urgency?:string;updatedAt?:string};
type Pulse={attendance?:Attendance;community?:Community;retention?:Retention;finance?:Finance;care?:CareCase[]};

const rolePerspective:Record<string,{name:string;copy:string}>={
  "super-admin":{name:"Executive pulse",copy:"A permission-aware view across congregation, gatherings, connection, serving and stewardship."},
  pastor:{name:"Pastoral pulse",copy:"Care, attendance and connection signals that support human-led ministry decisions."},
  "branch-admin":{name:"Branch pulse",copy:"Local gathering health, groups, serving capacity and follow-up."},
  "membership-admin":{name:"Membership pulse",copy:"People, attendance and connection pathways across the member journey."},
  "group-admin":{name:"Groups pulse",copy:"Group participation, capacity and connection health."},
  "volunteer-coordinator":{name:"Serving pulse",copy:"Team coverage and upcoming volunteer capacity."},
  "finance-admin":{name:"Finance pulse",copy:"Posted giving, pledges and unexplained settlement variance."},
  "finance-approver":{name:"Finance approval pulse",copy:"Ledger and reconciliation posture for controlled decisions."},
  "finance-auditor":{name:"Finance assurance pulse",copy:"Read-only ledger and reconciliation posture."},
};

export default function OperationsOverview(){
  const role=getStoredUser()?.role||"viewer"; const perspective=rolePerspective[role]||{name:"Ministry pulse",copy:"The live operational measures available to your role."};
  const [branches,setBranches]=useState<Branch[]>([]);const [branchId,setBranchId]=useState("");const [pulse,setPulse]=useState<Pulse>({});const [loading,setLoading]=useState(true);
  useEffect(()=>{api("/api/branches").then(value=>{const items=asList<Branch>(value);setBranches(items);setBranchId(current=>current||items[0]?.id||"");}).catch(()=>setLoading(false));},[]);
  const load=useCallback(async()=>{if(!branchId)return;setLoading(true);const to=new Date(),from=new Date(to);from.setUTCDate(from.getUTCDate()-90);const query=`branchId=${encodeURIComponent(branchId)}&from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`;
    const next:Pulse={};const financeOnly=role.startsWith("finance-");
    if(!financeOnly){const [attendance,community,retention,care]=await Promise.allSettled([api<Attendance>(`/api/chms/v1/attendance-analytics?${query}`),api<Community>(`/api/chms/v1/community-analytics?${query}`),api<Retention>(`/api/chms/v1/engagement/cohorts?${query}`),api(`/api/chms/v1/care-cases?branchId=${encodeURIComponent(branchId)}`)]);if(attendance.status==="fulfilled")next.attendance=attendance.value;if(community.status==="fulfilled")next.community=community.value;if(retention.status==="fulfilled")next.retention=retention.value;if(care.status==="fulfilled")next.care=asList<CareCase>(care.value);}
    if(role==="super-admin"||financeOnly){try{const periods=asList<Period>(await api(`/api/chms/v1/finance/periods?branchId=${encodeURIComponent(branchId)}`));const current=periods.find(period=>new Date(period.startsAt)<=to&&new Date(period.endsAt)>=to)||periods[0];if(current)next.finance=await api<Finance>(`/api/chms/v1/finance/reports/summary?branchId=${encodeURIComponent(branchId)}&periodId=${encodeURIComponent(current.id)}`);}catch{/* Permission absence intentionally removes this domain. */}}
    setPulse(next);setLoading(false);
  },[branchId,role]);useEffect(()=>{void load();},[load]);
  const cards=useMemo(()=>{const values:Array<{metric:string;label:string;value:string;note:string;href:string;accent?:boolean}>=[];const a=pulse.attendance?.summary,c=pulse.community?.summary,f=pulse.finance?.totals;if(pulse.care){const urgent=pulse.care.filter(item=>item.urgency==="urgent"||item.urgency==="priority").length;values.push({metric:"care.open_cases",label:"My open care cases",value:String(pulse.care.length),note:urgent?`${urgent} priority assignments`:"No priority assignments",href:"/inbox",accent:urgent>0});}if(a){values.push({metric:"attendance.confirmed_people",label:"People gathered",value:String(a.uniqueNamedPeople),note:"Unique named people · 90 days",href:"/services"},{metric:"attendance.headcount",label:"Approved headcount",value:String(a.anonymousHeadcount),note:`Across ${a.completedOccurrences} completed gatherings`,href:"/services"});}if(c){values.push({metric:"groups.active_connections",label:"Group connections",value:String(c.uniqueConnectedPeople),note:`${c.activeGroups} active groups`,href:"/groups/insights"},{metric:"serving.fill_rate",label:"Serving coverage",value:c.plannedServingSlots?`${Math.round(c.filledServingSlots/c.plannedServingSlots*100)}%`:"—",note:`${c.filledServingSlots}/${c.plannedServingSlots} planned positions`,href:"/volunteers",accent:c.plannedServingSlots>c.filledServingSlots});}if(f){values.push({metric:"giving.net_posted",label:"Net posted giving",value:new Intl.NumberFormat("en-GH",{style:"currency",currency:"GHS",maximumFractionDigits:0}).format(f.netContributionMinor/100),note:`${f.contributionCount} posted entries`,href:"/finance/reports"},{metric:"giving.unexplained_variance",label:"Unexplained variance",value:new Intl.NumberFormat("en-GH",{style:"currency",currency:"GHS",maximumFractionDigits:0}).format(f.unexplainedMinor/100),note:"Requires reconciliation",href:"/reconciliation",accent:f.unexplainedMinor!==0});}return values;},[pulse]);
  const latestCohort=[...(pulse.retention?.cohorts||[])].reverse().find(row=>row.cohortState==="available");
  return <section className="ops-overview" aria-labelledby="ops-overview-title"><header><div><span>Church operations · 90 day window</span><h1 id="ops-overview-title">{perspective.name}</h1><p>{perspective.copy}</p></div>{branches.length>0&&<Select aria-label="Dashboard branch" value={branchId} onChange={event=>setBranchId(event.target.value)}>{branches.map(branch=><option key={branch.id} value={branch.id}>{branch.name}</option>)}</Select>}</header>
    {loading?<div className="ops-overview-loading" aria-label="Loading church operations"><i/><i/><i/><i/></div>:cards.length===0?<div className="ops-overview-restricted"><b>Role-scoped view</b><p>No governed operational snapshot is available for this account and branch yet.</p><Link href="/reports">Review your metric access →</Link></div>:<><div className="ops-overview-cards">{cards.map((card,index)=><Link href={card.href} key={card.label} className={card.accent?"is-attention":""} title={`Open ${card.label} · ${card.metric}`}><span>{String(index+1).padStart(2,"0")}</span><strong>{card.value}</strong><b>{card.label}</b><small>{card.note}</small><em>{card.metric}</em><i aria-hidden="true">↗</i></Link>)}</div><div className="ops-overview-foot"><p><b>Source-governed</b> Values come from authorized domain projections and retain their original definitions.</p>{latestCohort&&<p><b>Latest mature cohort</b> {latestCohort.return30.ratePercent??"Protected"}% returned within 30 days · {latestCohort.combinedConnection.ratePercent??"Protected"}% connected.</p>}<Link href="/reports">Open metric dictionary</Link></div></>}
  </section>;
}
