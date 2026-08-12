"use client";

import { Icon } from "@/components/icon";
import { ServingRequest, type MemberServingAssignment } from "@/components/serving-request";
import { useCallback, useEffect, useMemo, useState } from "react";

interface HomeItem { id?: string; version?: number; title?: string; name?: string; startsAt?: string; startAt?: string; location?: string; status?: string; type?: string; role?: string; teamName?: string; positionName?: string; meetingPattern?: { weekday?: string; localStart?: string } }
interface MemberHomeData {
  generatedAt: string;
  member: { name: string; branchName?: string; membershipStage: string };
  nextGathering: HomeItem | null;
  registrations: HomeItem[];
  groups: HomeItem[];
  serving: HomeItem[];
  nextSteps: Array<{ id: string; title: string; complete: boolean }>;
  announcements: Array<{ id?: string; title: string; body?: string; publishAt?: string }>;
  activity: Array<{ type: string; title: string; occurredAt: string }>;
}

const PUBLIC_SITE = (process.env.NEXT_PUBLIC_SITE_URL || "http://localhost:3010").replace(/\/$/, "");
const NAV = [
  ["home", "Home", "/"],
  ["calendar", "Events", `${PUBLIC_SITE}/events`],
  ["users", "Community", "#community"],
  ["heart", "Serve", "#serve"],
  ["play", "Care & messages", "/care"],
  ["give", "Giving", "/giving"],
];

const QUICK_ACTIONS = [
  ["calendar", "Find an event", "Register yourself or your household.", `${PUBLIC_SITE}/events`],
  ["users", "Find community", "Discover groups and manage your connections.", "/participation#groups"],
  ["heart", "Ask for prayer", "Share privately with the pastoral team.", "/care"],
  ["give", "Give", "Manage a pledge or make a secure gift.", "/giving"],
  ["play", "Watch a message", "Return to the latest sermon or series.", "/care"],
  ["profile", "Manage profile", "Update your household, privacy and devices.", "/account"],
];

export default function MemberHome() {
  const [home, setHome] = useState<MemberHomeData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const load = useCallback(async () => {
    setLoading(true); setError("");
    try {
      const response = await fetch("/api/member/home", { cache: "no-store" });
      const data = await response.json();
      if (!response.ok) throw new Error(data.message || data.error || "Your dashboard could not be loaded.");
      setHome(data);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Your dashboard could not be loaded.");
    } finally { setLoading(false); }
  }, []);
  useEffect(() => { void load(); }, [load]);
  const firstName = home?.member.name.split(" ")[0] || "there";
  const initials = home?.member.name.split(" ").map(value => value[0]).join("").slice(0,2).toUpperCase() || "MR";
  const branchName = home?.member.branchName || "Your REMI branch";
  const nextDate = home?.nextGathering?.startsAt ? new Date(home.nextGathering.startsAt) : null;
  const daysUntil = nextDate ? Math.max(0, Math.ceil((nextDate.getTime()-Date.now())/86400000)) : null;
  const agenda = useMemo(() => {
    const values = [...(home?.registrations || [])];
    if (home?.nextGathering) values.unshift({ ...home.nextGathering, title: home.nextGathering.name });
    return values.slice(0,3);
  }, [home]);
  const completedSteps = home?.nextSteps.filter(step => step.complete).length || 0;

  if (loading) return <MemberDashboardSkeleton />;
  if (error || !home) return <main className="member-dashboard-state"><span>MY REMI</span><h1>We could not open your week.</h1><p>{error}</p><button onClick={() => void load()}>Try again <Icon name="arrow" /></button></main>;
  return (
    <main className="member-shell">
      <aside className="member-sidebar">
        <a className="member-brand" href="#">
          <span>R</span>
          <div>
            <strong>MY REMI</strong>
            <small>Belong · Grow · Serve</small>
          </div>
        </a>
        <nav aria-label="Member navigation">
          <p>Your church</p>
          {NAV.map(([icon, label, href], index) => (
            <a href={href} className={index === 0 ? "active" : ""} key={label}>
              <Icon name={icon} />
              <span>{label}</span>
              {label === "Serve" && <b>1</b>}
            </a>
          ))}
          <p>Your account</p>
          <a href="/account">
            <Icon name="profile" />
            <span>My profile</span>
            <i>80%</i>
          </a>
        </nav>
        <div className="member-sidebar-card">
          <span>Need prayer?</span>
          <p>You do not have to carry it alone.</p>
          <a href="/care">Share a request <Icon name="arrow" /></a>
        </div>
        <div className="member-person">
          <span>{initials}</span>
          <div>
            <strong>{home.member.name}</strong>
            <small>{branchName}</small>
          </div>
          <button aria-label="Account menu">•••</button>
        </div>
      </aside>

      <section className="member-workspace">
        <header className="member-topbar">
          <div>
            <p>{new Intl.DateTimeFormat("en-GH", { weekday:"long", day:"numeric", month:"long" }).format(new Date(home.generatedAt))}</p>
            <strong>Good {greeting()}, {firstName}.</strong>
          </div>
          <div>
            <button aria-label="Notifications">
              <Icon name="bell" />
              <i />
            </button>
            <span>{initials}</span>
          </div>
        </header>

        <div className="member-content">
          <section className="member-welcome">
            <div className="member-welcome-copy">
              <p>
                THIS WEEK AT REMI <span>•</span> {branchName.toUpperCase()}
              </p>
              <h1>
                You have a place
                <br />
                <em>in this house.</em>
              </h1>
              <p>
                Everything you need for the week ahead—your gatherings, people,
                next steps and serving schedule.
              </p>
              <div>
                <a className="member-primary-link" href="#week">
                  View this week <Icon name="arrow" />
                </a>
                <a href="/care">Watch latest message</a>
              </div>
            </div>
            <div className="member-next-service">
              <span>Next gathering</span>
              <b>{daysUntil === null ? "—" : String(daysUntil).padStart(2,"0")}</b>
              <p>{daysUntil === 1 ? "Day" : "Days"}</p>
              <div>
                <strong>{home.nextGathering?.name || "No gathering scheduled"}</strong>
                <small>{nextDate ? new Intl.DateTimeFormat("en-GH", { weekday:"long", hour:"numeric", minute:"2-digit" }).format(nextDate) : "Your next gathering will appear here"}</small>
              </div>
            </div>
          </section>

          {home.announcements[0] && <section className="member-announcement"><span>NOTICE</span><div><strong>{home.announcements[0].title}</strong><p>{home.announcements[0].body}</p></div><Icon name="arrow" /></section>}

          <section className="member-quick-actions" aria-labelledby="member-quick-actions-title">
            <div className="member-section-heading">
              <div><span>START</span><h2 id="member-quick-actions-title">What would you like to do?</h2></div>
            </div>
            <div>{QUICK_ACTIONS.map(([icon,title,copy,href])=><a href={href} key={title}><Icon name={icon}/><span><strong>{title}</strong><small>{copy}</small></span><Icon name="arrow"/></a>)}</div>
          </section>

          <div className="member-dashboard-grid" id="week">
            <section className="member-agenda">
              <div className="member-section-heading">
                <div>
                  <span>01</span>
                  <h2>Coming up for you</h2>
                </div>
                <a href={`${PUBLIC_SITE}/events`}>
                  Full calendar <Icon name="arrow" />
                </a>
              </div>
              <div className="member-agenda-list">{agenda.length ? agenda.map((item,index) => { const date=new Date(item.startsAt||item.startAt||home.generatedAt); return <article key={item.id||`${item.title}-${index}`}><time><b>{String(date.getDate()).padStart(2,"0")}</b><span>{date.toLocaleString("en-GH",{month:"short"}).toUpperCase()}</span></time><div><p>{date.toLocaleString("en-GH",{weekday:"long",hour:"numeric",minute:"2-digit"}).toUpperCase()}</p><h3>{item.title||item.name}</h3><span>{item.location||branchName}</span></div><button className={index ? "quiet" : ""}>{index ? "View details" : "Coming up"}</button></article> }) : <DashboardEmpty title="Nothing on your calendar" body="Registrations and upcoming gatherings will appear here." />}</div>
            </section>

            <div id="serve"><ServingRequest assignment={home.serving.find(item=>item.status==="invited") as MemberServingAssignment | undefined} /></div>

            <section className="member-groups" id="community">
              <div className="member-section-heading">
                <div>
                  <span>02</span>
                  <h2>Your community</h2>
                </div>
                <span className="member-coming-label">Discovery coming next</span>
              </div>
              <div className="member-group-cards">{home.groups.length ? home.groups.slice(0,2).map((group,index)=><article className={index%2 ? "warm":""} key={group.id}><div className="member-avatars"><span>{(group.name||"GR").split(" ").map(value=>value[0]).join("").slice(0,2)}</span><span>{group.role?.slice(0,1).toUpperCase()||"M"}</span></div><p>{(group.type||"COMMUNITY").toUpperCase()}</p><h3>{group.name}</h3><small>{group.meetingPattern?.weekday ? `${group.meetingPattern.weekday} · ${group.meetingPattern.localStart||"Time shared in group"}` : `${group.status} membership`}</small><span className="member-group-status">{group.status === "active" ? "Connected" : group.status}</span></article>) : <DashboardEmpty title="Find your people" body="Groups you join will become part of this space." />}</div>
            </section>

            <aside className="member-progress">
              <div className="member-section-heading">
                <div>
                  <span>03</span>
                  <h2>Your next steps</h2>
                </div>
              </div>
              <div className="progress-ring">
                <span>
                  {completedSteps}<small>/{home.nextSteps.length}</small>
                </span>
              </div>
              <h3>Make REMI home</h3>
              <p>Two simple steps remain in your welcome journey.</p>
              <ul>{home.nextSteps.map((step,index)=><li className={step.complete?"done":""} key={step.id}>{step.complete?<Icon name="check" />:<span>{index+1}</span>}{step.title}</li>)}</ul>
              <button>
                Continue journey <Icon name="arrow" />
              </button>
            </aside>
          </div>
        </div>
      </section>

      <nav className="member-mobile-nav" aria-label="Mobile navigation">
        {NAV.slice(0, 5).map(([icon, label, href], index) => (
          <a href={href} className={index === 0 ? "active" : ""} key={label}>
            <Icon name={icon} />
            <span>{label}</span>
          </a>
        ))}
      </nav>
    </main>
  );
}

function greeting() {
  const hour = new Date().getHours();
  if (hour < 12) return "morning";
  if (hour < 18) return "afternoon";
  return "evening";
}

function DashboardEmpty({ title, body }: { title: string; body: string }) {
  return <div className="member-dashboard-empty"><Icon name="calendar" /><strong>{title}</strong><p>{body}</p></div>;
}

function MemberDashboardSkeleton() {
  return <main className="member-home-skeleton" aria-label="Loading your member dashboard" aria-busy="true"><aside><span className="skeleton-block skeleton-brand" />{Array.from({length:7},(_,index)=><span className="skeleton-block skeleton-nav" key={index}/>)}</aside><section><header><span className="skeleton-block skeleton-copy"/><span className="skeleton-block skeleton-avatar"/></header><div><span className="skeleton-block skeleton-hero"/><div className="skeleton-grid"><span className="skeleton-block"/><span className="skeleton-block"/><span className="skeleton-block"/></div></div></section></main>;
}
