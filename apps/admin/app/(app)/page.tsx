"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { api, ApiError } from "@/lib/api";
import { Card, ErrorBox } from "@/components/ui";
import { SkeletonDashboard } from "@/components/ui/Skeleton";
import OperationsOverview from "@/components/operations-overview";

type MetricKey = "pendingReview" | "newPrayerRequests" | "newContacts" | "registrations" | "subscribers" | "pages" | "sermons" | "events";
interface Datum { label: string; value: number }
interface TrendDatum extends Datum { date: string }
interface Stats {
  pages: number; sermons: number; events: number; pendingReview: number; newPrayerRequests: number;
  newContacts: number; registrations: number; subscribers: number; upcomingEvents: number;
  totalContent: number; publishedContent: number; publishingRate: number; engagementTotal: number;
  contentStatus: Record<string, number>; contentMix: Datum[]; submissionMix: Datum[]; engagementTrend: TrendDatum[];
}

const CARDS: { key: MetricKey; label: string; href: string; note: string; index: string; urgent?: boolean }[] = [
  { key: "pendingReview", label: "Pending review", href: "/review", note: "Needs a decision", index: "01", urgent: true },
  { key: "newPrayerRequests", label: "Prayer requests", href: "/inbox?tab=prayer", note: "Pastoral attention", index: "02", urgent: true },
  { key: "newContacts", label: "Contact messages", href: "/inbox?tab=contact", note: "Awaiting response", index: "03" },
  { key: "registrations", label: "Registrations", href: "/content/events", note: "Across all events", index: "04" },
  { key: "subscribers", label: "Subscribers", href: "/", note: "Newsletter reach", index: "05" },
  { key: "pages", label: "Pages", href: "/content/pages", note: "Website structure", index: "06" },
  { key: "sermons", label: "Sermons", href: "/content/sermons", note: "Message archive", index: "07" },
  { key: "events", label: "Events", href: "/content/events", note: "Calendar entries", index: "08" },
];

export default function DashboardPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  async function load() { setLoading(true); setError(null); try { setStats(await api<Stats>("/api/admin/stats")); } catch (err) { setError(err instanceof ApiError ? err.message : "Failed to load ministry intelligence."); } finally { setLoading(false); } }
  useEffect(() => { load(); }, []);

  return <div>
    <OperationsOverview />
    <section className="mb-7 grid grid-cols-1 items-end gap-7 overflow-hidden rounded-2xl border border-white/10 bg-[var(--remi-green)] p-7 [background-image:radial-gradient(circle_at_88%_0%,rgba(209,173,85,.18),transparent_31%)] shadow-[0_22px_55px_rgba(23,32,25,.15)] lg:grid-cols-[1.2fr_.8fr] lg:p-9">
      <div><p className="text-[.65rem] font-bold uppercase tracking-[.17em] text-[var(--altar-mint)]">Digital ministry</p><h1 className="mt-4 max-w-3xl text-[clamp(2rem,3.2vw,3.15rem)] font-[740] leading-none tracking-[-.052em] text-[#f1f8f6]">Publishing and community response.</h1><p className="mt-4 max-w-2xl text-sm leading-6 text-[var(--altar-copy)]">Website readiness, public enquiries and the content pipeline remain visible beneath the church-operations pulse.</p></div>
      <div className="grid grid-cols-3 gap-px overflow-hidden rounded-xl border border-white/10 bg-white/10">
        <HeroStat label="Published" value={stats ? `${Math.round(stats.publishingRate)}%` : "—"}/><HeroStat label="Engagement" value={stats ? String(stats.engagementTotal) : "—"}/><HeroStat label="Upcoming" value={stats ? String(stats.upcomingEvents) : "—"}/>
      </div>
    </section>
    {loading && <SkeletonDashboard />}
    {error && <ErrorBox message={error} onRetry={load} />}
    {stats && <>
      <section aria-label="Operational metrics" className="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {CARDS.map(({ key, ...card })=><MetricCard key={key} {...card} value={stats[key]}/>) }
      </section>

      <section className="mt-8 grid grid-cols-1 gap-5 xl:grid-cols-[1.45fr_.55fr]">
        <Card className="overflow-hidden p-6 md:p-8"><PanelHead eyebrow="Seven-day pulse" title="Community engagement" note={`${stats.engagementTotal} total recorded`} /><TrendChart data={stats.engagementTrend ?? []}/></Card>
        <Card className="p-6 md:p-8"><PanelHead eyebrow="Publishing" title="Content readiness" note={`${stats.publishedContent}/${stats.totalContent} live`} /><StatusDonut stats={stats.contentStatus ?? {}} rate={stats.publishingRate}/></Card>
      </section>

      <section className="mt-5 grid grid-cols-1 gap-5 xl:grid-cols-2">
        <Card className="p-6 md:p-8"><PanelHead eyebrow="Library" title="Content mix" note={`${stats.totalContent} records`} /><BarList data={stats.contentMix ?? []}/></Card>
        <Card className="p-6 md:p-8"><PanelHead eyebrow="People" title="Response channels" note="All-time volume" /><BarList data={stats.submissionMix ?? []} warm/></Card>
      </section>

      <section className="mt-5 grid grid-cols-1 gap-5 xl:grid-cols-[1.1fr_.9fr]">
        <Card className="p-6 md:p-8"><PanelHead eyebrow="Priority queue" title="What needs ministry attention?" note="Live workload"/><div className="admin-activity-list mt-2"><Activity href="/review" icon="✓" value={stats.pendingReview} title="Items awaiting review" copy="Drafts and submissions waiting for a publishing decision"/><Activity href="/inbox?tab=prayer" icon="↗" value={stats.newPrayerRequests} title="New prayer requests" copy="Pastoral messages that have not been acknowledged"/><Activity href="/inbox?tab=contact" icon="◇" value={stats.newContacts} title="Contact messages" copy="Community enquiries awaiting a response"/><Activity href="/content/events" icon="□" value={stats.registrations} title="Event registrations" copy="People registered across the ministry calendar"/></div></Card>
        <Card className="!border-white/10 !bg-[var(--remi-green)] p-6 text-white md:p-8"><span className="text-[10px] font-bold uppercase tracking-[.16em] text-[var(--altar-mint)]">Quick actions</span><h2 className="mt-1 text-xl font-bold tracking-[-.03em] text-white">Move the work forward.</h2><p className="mt-2 max-w-sm text-sm leading-relaxed text-[#9fa89f]">Open the ministry operations staff use most.</p><div className="mt-6 grid grid-cols-1 gap-px overflow-hidden rounded-lg border border-white/10 bg-white/10 sm:grid-cols-2"><QuickLink href="/review" label="Review content"/><QuickLink href="/inbox" label="Open inbox"/><QuickLink href="/content/sermons/new" label="New sermon"/><QuickLink href="/content/events/new" label="New event"/><QuickLink href="/settings" label="Site settings"/><QuickLink href="/account" label="Profile & security"/></div></Card>
      </section>
    </>}
  </div>;
}

function HeroStat({label,value}:{label:string;value:string}) { return <div className="bg-white/[.045] px-3 py-4 text-center"><b className="block font-mono text-xl text-white tabular-nums">{value}</b><span className="mt-1 block text-[9px] font-bold uppercase tracking-[.14em] text-white/35">{label}</span></div> }
function MetricCard({href,index,label,note,value,urgent}:{href:string;index:string;label:string;note:string;value:number;urgent?:boolean}) { return <Link href={href} className={`admin-metric-card group relative min-h-40 overflow-hidden rounded-2xl border border-[#e1ddd2] bg-white p-6 shadow-[0_12px_36px_rgba(23,32,25,.06)] transition hover:-translate-y-0.5 ${urgent&&value>0?"before:absolute before:inset-y-0 before:left-0 before:w-[3px] before:bg-[var(--remi-gold)]":""}`}><div className="flex items-start justify-between"><span className="admin-metric-index font-mono text-[10px] font-bold tracking-[.17em] text-[#9b998f]">{index}</span><span className="admin-metric-arrow grid size-8 place-items-center rounded-lg bg-[#f5f1e7] text-[#8b681f] transition group-hover:bg-[var(--remi-gold)] group-hover:text-[var(--remi-green)]">↗</span></div><p className="admin-metric-value mt-4 font-mono text-[2.35rem] font-bold leading-none tracking-[-.05em] text-[#172019] tabular-nums">{String(value).padStart(2,"0")}</p><p className="admin-metric-label mt-3 text-sm font-semibold text-[#30362f]">{label}</p><p className="admin-metric-note mt-1 text-xs text-[#85877f]">{note}</p></Link> }
function PanelHead({eyebrow,title,note}:{eyebrow:string;title:string;note:string}) { return <div className="flex items-end justify-between gap-4 border-b border-zinc-100 pb-4"><div><span className="text-[10px] font-bold uppercase tracking-[.16em] text-[#8b681f]">{eyebrow}</span><h2 className="mt-1 text-xl font-bold tracking-[-.03em] text-zinc-900">{title}</h2></div><span className="text-right font-mono text-[10px] text-zinc-400">{note}</span></div> }

function TrendChart({data}:{data:TrendDatum[]}) {
  const values=data.map(d=>d.value), max=Math.max(1,...values); const points=data.map((d,i)=>`${i*(100/Math.max(1,data.length-1))},${88-(d.value/max)*68}`).join(" "); const area=`0,88 ${points} 100,88`;
  return <div className="mt-6"><div className="relative h-56"><div className="absolute inset-0 flex flex-col justify-between">{[0,1,2,3].map(i=><span key={i} className="border-t border-dashed border-zinc-100"/>)}</div><svg viewBox="0 0 100 100" preserveAspectRatio="none" className="absolute inset-0 h-full w-full overflow-visible" aria-label="Seven-day engagement trend" role="img"><defs><linearGradient id="engagement-fill" x1="0" y1="0" x2="0" y2="1"><stop offset="0" stopColor="#d1ad55" stopOpacity=".34"/><stop offset="1" stopColor="#d1ad55" stopOpacity="0"/></linearGradient></defs><polygon points={area} fill="url(#engagement-fill)"/><polyline points={points} fill="none" stroke="#8b681f" strokeWidth="2" vectorEffect="non-scaling-stroke" strokeLinecap="round" strokeLinejoin="round"/></svg></div><div className="mt-3 grid grid-cols-7 gap-1">{data.map(d=><div key={d.date} className="text-center"><b className="block font-mono text-xs text-zinc-700 tabular-nums">{d.value}</b><span className="text-[9px] uppercase tracking-wider text-zinc-400">{d.label}</span></div>)}</div></div>
}
function StatusDonut({stats,rate}:{stats:Record<string,number>;rate:number}) { const total=Object.values(stats).reduce((a,b)=>a+b,0)||1; const published=stats.published||0, approved=stats.approved||0, review=stats["in-review"]||0; const p1=published/total*100,p2=p1+approved/total*100,p3=p2+review/total*100; const style={background:`conic-gradient(#173127 0 ${p1}%, #8b681f ${p1}% ${p2}%, #d1ad55 ${p2}% ${p3}%, #e8e4da ${p3}% 100%)`}; return <div className="mt-7"><div className="mx-auto grid size-44 place-items-center rounded-full" style={style}><div className="admin-donut-core grid size-28 place-items-center rounded-full bg-white text-center shadow-inner"><div><b className="block font-mono text-3xl text-[#172019]">{Math.round(rate)}%</b><span className="text-[9px] font-bold uppercase tracking-[.14em] text-zinc-400">published</span></div></div></div><div className="mt-7 grid grid-cols-2 gap-3"><Legend color="#173127" label="Published" value={published}/><Legend color="#8b681f" label="Approved" value={approved}/><Legend color="#d1ad55" label="In review" value={review}/><Legend color="#e8e4da" label="AI draft" value={stats["ai-draft"]||0}/></div></div> }
function Legend({color,label,value}:{color:string;label:string;value:number}) { return <div className="flex items-center gap-2 text-xs"><i className="size-2 rounded-sm" style={{background:color}}/><span className="flex-1 text-zinc-500">{label}</span><b className="font-mono text-zinc-800">{value}</b></div> }
function BarList({data,warm=false}:{data:Datum[];warm?:boolean}) { const max=Math.max(1,...data.map(d=>d.value)); return <div className="mt-6 space-y-4">{data.map((d,i)=><div key={d.label}><div className="mb-1.5 flex justify-between text-xs"><span className="font-medium text-zinc-600">{d.label}</span><b className="font-mono text-zinc-800 tabular-nums">{d.value}</b></div><div className="admin-bar-track h-2 overflow-hidden rounded-full bg-[#efede7]"><div className={`h-full rounded-full ${warm?"bg-[var(--remi-gold)]":"bg-[var(--remi-green)]"}`} style={{width:`${Math.max(d.value?5:0,d.value/max*100)}%`,opacity:.96-i*.055}}/></div></div>)}</div> }
function Activity({href,icon,value,title,copy}:{href:string;icon:string;value:number;title:string;copy:string}) { return <Link href={href} className="group flex items-center gap-4 py-4"><span className="admin-activity-icon grid size-10 shrink-0 place-items-center rounded-xl bg-[#f3efe4] text-[#8b681f]">{icon}</span><span className="min-w-0 flex-1"><b className="block text-sm text-zinc-800">{title}</b><small className="mt-1 block text-zinc-400">{copy}</small></span><b className="font-mono text-lg text-zinc-800 tabular-nums">{value}</b><span className="text-zinc-300 transition group-hover:translate-x-1 group-hover:text-[#8b681f]">→</span></Link> }
function QuickLink({href,label}:{href:string;label:string}) { return <Link href={href} className="group flex min-h-14 items-center justify-between bg-[var(--altar-panel)] px-4 py-3 text-sm font-medium text-[#d9ddd8] hover:bg-[rgba(113,215,197,.08)] hover:text-white">{label}<span className="text-[var(--altar-mint)] transition group-hover:translate-x-1">→</span></Link> }
