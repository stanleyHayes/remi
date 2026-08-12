"use client";

import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiError, getStoredUser } from "@/lib/api";
import { displayMemberName, loadMemberProfile, type MemberProfile, type MemberSection, visibleMemberSections } from "@/lib/chms";
import { Card, EmptyState, ErrorBox } from "@/components/ui";
import { Skeleton } from "@/components/ui/Skeleton";

const SECTION_META: Record<MemberSection, { label: string; note: string; mark: string }> = {
  identity: { label: "Identity", note: "Contact and personal record", mark: "01" },
  household: { label: "Household", note: "Family and shared details", mark: "02" },
  journey: { label: "Journey", note: "Membership milestones", mark: "03" },
  attendance: { label: "Attendance", note: "Presence and patterns", mark: "04" },
  groups: { label: "Groups", note: "Community connections", mark: "05" },
  serving: { label: "Serving", note: "Teams and schedules", mark: "06" },
  care: { label: "Pastoral care", note: "Restricted care activity", mark: "07" },
  giving: { label: "Giving", note: "Finance-authorized summary", mark: "08" },
};

export default function MemberProfilePage() {
  const { id } = useParams<{ id: string }>();
  const search = useSearchParams();
  const router = useRouter();
  const [profile, setProfile] = useState<MemberProfile | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const role = getStoredUser()?.role;
  const sections = useMemo(() => visibleMemberSections(role, profile?.permissions?.sections), [profile?.permissions?.sections, role]);
  const requested = search.get("section") as MemberSection | null;
  const active = requested && sections.includes(requested) ? requested : sections[0] || "identity";

  const load = useCallback(async () => {
    setLoading(true); setError(null);
    try { setProfile(await loadMemberProfile(id)); }
    catch (cause) { setError(cause instanceof ApiError ? cause.message : "The member profile could not be loaded."); }
    finally { setLoading(false); }
  }, [id]);

  useEffect(() => { load(); }, [load]);

  function setSection(section: MemberSection) {
    const next = new URLSearchParams(search.toString()); next.set("section", section);
    router.replace(`/people/${id}?${next.toString()}`, { scroll: false });
  }

  if (loading) return <ProfileSkeleton />;
  if (error || !profile) return <div className="max-w-5xl"><BackToPeople /><ErrorBox message={error || "Member not found."} onRetry={load} /></div>;
  if (sections.length === 0) return <div className="max-w-5xl"><BackToPeople /><EmptyState title="This profile is restricted" hint="Your current role does not grant access to any section of this member record." /></div>;

  const person = profile.person;
  const name = displayMemberName(person.names);
  const actions = new Set(profile.permissions?.actions || []);
  return (
    <div className="member-profile-shell">
      <BackToPeople />
      <header className="member-profile-hero">
        <div className="member-profile-avatar" aria-hidden={!person.photoUrl}>
          {person.photoUrl ? <>{/* eslint-disable-next-line @next/next/no-img-element */}<img src={person.photoUrl} alt="" /></> : <span>{initials(person.names.given, person.names.family)}</span>}
        </div>
        <div className="min-w-0 flex-1">
          <div className="member-profile-kicker"><span>{person.personNumber}</span><i /> <span>{person.homeBranchName || person.homeBranchId}</span></div>
          <h1>{name}</h1>
          <div className="member-profile-status-row">
            <StageBadge value={person.membershipStage} />
            {person.archivedAt && <span className="member-profile-muted-badge">Archived</span>}
            <span>Updated {formatDate(person.updatedAt)}</span>
          </div>
        </div>
        <div className="member-profile-actions">
          {actions.has("people.update") && <Link className="admin-secondary-action rounded-lg px-4 py-2.5 text-sm font-semibold" href={`/people/${person.id}/edit`}>Edit profile</Link>}
          {actions.has("people.care.create") && <button className="admin-primary-button rounded-lg px-4 py-2.5 text-sm font-semibold">Add care note</button>}
        </div>
      </header>

      <div className="member-profile-grid">
        <nav className="member-profile-nav" aria-label="Member profile sections">
          {sections.map((section) => {
            const meta = SECTION_META[section];
            return <button key={section} type="button" aria-current={active === section ? "page" : undefined} onClick={() => setSection(section)}><span className="member-profile-nav-mark">{meta.mark}</span><span><b>{meta.label}</b><small>{meta.note}</small></span><i aria-hidden="true">→</i></button>;
          })}
        </nav>
        <main className="min-w-0" aria-live="polite">
          <SectionHeading section={active} />
          {active === "identity" && <IdentitySection profile={profile} />}
          {active === "household" && <HouseholdSection profile={profile} />}
          {active === "journey" && <JourneySection profile={profile} />}
          {active === "attendance" && <AttendanceSection profile={profile} />}
          {active === "groups" && <GroupsSection profile={profile} />}
          {active === "serving" && <ServingSection profile={profile} />}
          {active === "care" && <CareSection profile={profile} />}
          {active === "giving" && <GivingSection profile={profile} />}
        </main>
      </div>
    </div>
  );
}

function BackToPeople() { return <Link href="/people" className="member-back-link"><span aria-hidden="true">←</span> Back to people</Link>; }
function SectionHeading({ section }: { section: MemberSection }) { const meta = SECTION_META[section]; return <div className="member-section-heading"><div><span>Member record / {meta.mark}</span><h2>{meta.label}</h2></div><p>{meta.note}</p></div>; }

function IdentitySection({ profile }: { profile: MemberProfile }) {
  const { person } = profile; const preferences = person.communicationPreferences || {};
  return <div className="space-y-5"><Card className="member-panel"><PanelTitle title="Personal record" note="Identity fields are shown according to your access."/><dl className="member-detail-grid"><Detail label="Legal name" value={[person.names.given,person.names.middle,person.names.family].filter(Boolean).join(" ")} /><Detail label="Preferred name" value={person.names.preferred}/><Detail label="Date of birth" value={person.dateOfBirth?.value}/><Detail label="Gender" value={person.gender}/><Detail label="Home branch" value={person.homeBranchName || person.homeBranchId}/><Detail label="Member since" value={formatDate(person.createdAt)}/></dl></Card>
    <div className="grid gap-5 xl:grid-cols-2"><Card className="member-panel"><PanelTitle title="Contact points" note="Verified channels are marked."/>{person.contactPoints?.length ? <div className="member-row-list">{person.contactPoints.map((contact,index)=><div key={`${contact.type}-${index}`}><span className="member-row-icon">{contact.type.slice(0,1).toUpperCase()}</span><span><b>{contact.value}</b><small>{contact.type}{contact.primary?" · Primary":""}</small></span>{contact.verifiedAt&&<em>Verified</em>}</div>)}</div>:<InlineEmpty text="No contact points recorded."/>}</Card>
      <Card className="member-panel"><PanelTitle title="Communication" note="Current channel preferences."/><div className="member-preference-grid">{(["email","sms","whatsapp","phone"] as const).map(channel=><div key={channel} data-enabled={Boolean(preferences[channel])}><span>{channel}</span><b>{preferences[channel]?"Allowed":"Off"}</b></div>)}</div></Card></div>
    <Card className="member-panel"><PanelTitle title="Tags and record context" note="Operational labels—not judgments about faith or worth."/><div className="flex flex-wrap gap-2">{person.tags?.length?person.tags.map(tag=><span className="member-tag" key={tag}>{tag}</span>):<InlineEmpty text="No tags assigned."/>}</div></Card></div>;
}

function HouseholdSection({ profile }: { profile: MemberProfile }) { const h=profile.household; if(!h)return <EmptyState title="No household linked" hint="Add this person to a household when shared contact or family context is useful."/>; return <Card className="member-panel"><PanelTitle title={h.name} note={h.sharedAddress || "No shared household address"}/><div className="member-row-list">{h.members.map(member=><Link href={`/people/${member.id}`} key={member.id}><span className="member-row-icon">{initials(member.name," ")}</span><span><b>{member.name}</b><small>{[member.role,member.relationship].filter(Boolean).join(" · ")||"Household member"}</small></span>{member.id===h.primaryContactId&&<em>Primary contact</em>}</Link>)}</div></Card>; }
function JourneySection({profile}:{profile:MemberProfile}) { const events=profile.journey||[]; if(!events.length)return <EmptyState title="No journey events yet" hint="Membership transitions will appear here with their effective dates and reasons."/>; return <Card className="member-panel"><div className="member-timeline">{events.map(event=><div key={event.id} data-reversed={event.reversed}><i/><time>{formatDate(event.effectiveAt)}</time><span><b>{stageLabel(event.fromStage)} → {stageLabel(event.toStage)}</b><small>{event.reasonCode.replaceAll("-"," ")}{event.reversed?" · Reversed":""}</small></span></div>)}</div></Card>; }
function AttendanceSection({profile}:{profile:MemberProfile}) { const data=profile.attendance; if(!data)return <EmptyState title="Attendance data is not available" hint="Records will appear after service occurrences and check-in are enabled."/>; return <div className="space-y-5"><div className="grid gap-4 sm:grid-cols-3"><MiniMetric label="Last seen" value={formatDate(data.lastSeenAt)}/><MiniMetric label="Past 90 days" value={String(data.services90Days)}/><MiniMetric label="Current streak" value={String(data.currentStreak||0)}/></div><Card className="member-panel"><PanelTitle title="Recent attendance" note="Named attendance only; anonymous headcounts are excluded."/>{data.records.length?<div className="member-row-list">{data.records.map(record=><div key={record.id}><span className="member-row-icon">✓</span><span><b>{record.serviceName}</b><small>{[record.branchName,record.source].filter(Boolean).join(" · ")}</small></span><time>{formatDate(record.occurredAt)}</time></div>)}</div>:<InlineEmpty text="No named attendance recorded."/>}</Card></div>; }
function GroupsSection({profile}:{profile:MemberProfile}) { const groups=profile.groups||[]; if(!groups.length)return <EmptyState title="Not connected to a group" hint="Group memberships and ministry communities will appear here."/>; return <div className="grid gap-4 md:grid-cols-2">{groups.map(group=><Card key={group.id} className="member-connection-card"><span>{group.type||"Group"}</span><h3>{group.name}</h3><p>{[group.role,group.status].filter(Boolean).join(" · ")}</p><small>Joined {formatDate(group.joinedAt)}</small></Card>)}</div>; }
function ServingSection({profile}:{profile:MemberProfile}) { const serving=profile.serving||[]; if(!serving.length)return <EmptyState title="No serving assignment" hint="Team positions, availability and upcoming schedules will appear here."/>; return <div className="grid gap-4 md:grid-cols-2">{serving.map(item=><Card key={item.id} className="member-connection-card"><span>{item.status||"Serving"}</span><h3>{item.team}</h3><p>{item.position||"Team member"}</p><small>{item.nextServingAt?`Next: ${formatDate(item.nextServingAt)}`:"No upcoming schedule"}</small></Card>)}</div>; }
function CareSection({profile}:{profile:MemberProfile}) { const care=profile.care; if(!care)return <EmptyState title="Care details are restricted" hint="Only assigned pastoral care workers can view case activity."/>; return <div className="space-y-5"><div className="grid gap-4 sm:grid-cols-3"><MiniMetric label="Open cases" value={String(care.openCases)}/><MiniMetric label="Overdue tasks" value={String(care.overdueTasks)}/><MiniMetric label="Last contact" value={formatDate(care.lastContactAt)}/></div><Card className="member-panel"><PanelTitle title="Active care" note="Notes remain inside their assigned care team."/>{care.cases?.length?<div className="member-row-list">{care.cases.map(item=><div key={item.id}><span className="member-row-icon">◇</span><span><b>{item.category}</b><small>{[item.urgency,item.assignedTo].filter(Boolean).join(" · ")}</small></span><em>{item.status}</em></div>)}</div>:<InlineEmpty text="No open pastoral care cases."/>}</Card></div>; }
function GivingSection({profile}:{profile:MemberProfile}) { const giving=profile.giving; if(!giving)return <EmptyState title="Giving summary is restricted" hint="Finance access is required. General ministry roles cannot view individual giving."/>; return <div className="space-y-5"><Card className="member-giving-hero"><span>Year to date</span><strong>{formatMoney(giving.yearToDateMinor,giving.currency)}</strong><p>Last gift {formatDate(giving.lastGiftAt)} · {giving.fundsSupported} funds supported</p></Card><Card className="member-panel"><PanelTitle title="Statement access" note="Detailed gifts stay in the finance workspace."/><p className="member-copy">{giving.statementAvailable?"A current period statement is available for authorized delivery.":"No statement is available for the current period."}</p></Card></div>; }

function PanelTitle({title,note}:{title:string;note:string}) { return <div className="member-panel-title"><div><h3>{title}</h3><p>{note}</p></div></div>; }
function Detail({label,value}:{label:string;value?:string}) { return <div><dt>{label}</dt><dd>{value||"Not recorded"}</dd></div>; }
function InlineEmpty({text}:{text:string}) { return <p className="member-inline-empty">{text}</p>; }
function MiniMetric({label,value}:{label:string;value:string}) { return <Card className="member-mini-metric"><span>{label}</span><strong>{value}</strong></Card>; }
function StageBadge({value}:{value:string}) { return <span className="member-stage-badge">{stageLabel(value)}</span>; }
function stageLabel(value:string) { return value.replaceAll("-"," ").replace(/\b\w/g,(letter)=>letter.toUpperCase()); }
function initials(first:string,last?:string) { return `${first.trim().charAt(0)}${(last||"").trim().charAt(0)}`.toUpperCase()||"M"; }
function formatDate(value?:string) { if(!value)return "Not recorded"; const date=new Date(value); return Number.isNaN(date.getTime())?value:new Intl.DateTimeFormat("en-GH",{day:"numeric",month:"short",year:"numeric"}).format(date); }
function formatMoney(minor:number,currency:string) { return new Intl.NumberFormat("en-GH",{style:"currency",currency:currency||"GHS"}).format(minor/100); }

function ProfileSkeleton() { return <div className="member-profile-shell" role="status" aria-label="Loading member profile" aria-busy="true"><Skeleton className="mb-5 h-5 w-28"/><div className="admin-skeleton-surface member-profile-hero"><Skeleton className="size-24 rounded-2xl"/><div className="flex-1"><Skeleton className="h-3 w-40"/><Skeleton className="mt-4 h-10 w-72 max-w-full"/><Skeleton className="mt-4 h-6 w-52"/></div></div><div className="member-profile-grid"><div className="admin-skeleton-surface rounded-2xl border p-4">{Array.from({length:6},(_,i)=><Skeleton key={i} className="mb-3 h-14 w-full rounded-xl"/>)}</div><div className="space-y-5"><Skeleton className="h-16 w-72 max-w-full"/><div className="admin-skeleton-surface rounded-2xl border p-7"><Skeleton className="h-6 w-44"/><div className="mt-7 grid gap-5 sm:grid-cols-2">{Array.from({length:6},(_,i)=><Skeleton key={i} className="h-14 w-full"/>)}</div></div></div></div></div>; }
