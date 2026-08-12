"use client";

import type { ReactNode } from "react";
import { Icon } from "@/components/icon";

const PUBLIC_SITE = (process.env.NEXT_PUBLIC_SITE_URL || "http://localhost:3010").replace(/\/$/, "");

const desktopNavigation = [
  { key: "home", icon: "home", label: "Home", href: "/" },
  { key: "participation", icon: "calendar", label: "My journey", href: "/participation" },
  { key: "events", icon: "calendar", label: "Events", href: `${PUBLIC_SITE}/events` },
  { key: "community", icon: "users", label: "Community", href: "/participation#groups" },
  { key: "serve", icon: "heart", label: "Serve", href: "/serving" },
  { key: "care", icon: "heart", label: "Care", href: "/care" },
  { key: "messages", icon: "bell", label: "Messages", href: "/messages" },
  { key: "giving", icon: "give", label: "Giving", href: "/giving" },
  { key: "lead", icon: "users", label: "Lead", href: "/lead" },
] as const;

const mobileNavigation = [
  desktopNavigation[0],
  desktopNavigation[1],
  desktopNavigation[3],
  desktopNavigation[4],
  desktopNavigation[6],
  { key: "account", icon: "profile", label: "Account", href: "/account" },
] as const;

type MemberDashboardShellProps = {
  active: "home" | "participation" | "events" | "community" | "serve" | "care" | "messages" | "giving" | "lead" | "account" | "support";
  member?: { name?: string; branchName?: string } | null;
  children: ReactNode;
};

export function MemberDashboardShell({ active, member, children }: MemberDashboardShellProps) {
  const name = member?.name?.trim() || "REMI Member";
  const branchName = member?.branchName?.trim() || "Your REMI branch";
  const firstName = name.split(" ")[0];
  const initials = name
    .split(/\s+/)
    .filter(Boolean)
    .map((part) => part[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

  return (
    <div className="member-shell">
      <aside className="member-sidebar">
        <a className="member-brand" href="/">
          <span>R</span>
          <div>
            <strong>MY REMI</strong>
            <small>Belong · Grow · Serve</small>
          </div>
        </a>
        <nav aria-label="Member navigation">
          <p>Your church</p>
          {desktopNavigation.map((item) => (
            <a
              href={item.href}
              className={active === item.key ? "active" : undefined}
              aria-current={active === item.key ? "page" : undefined}
              key={item.key}
            >
              <Icon name={item.icon} />
              <span>{item.label}</span>
            </a>
          ))}
          <p>Your account</p>
          <a
            href="/account"
            className={active === "account" ? "active" : undefined}
            aria-current={active === "account" ? "page" : undefined}
          >
            <Icon name="profile" />
            <span>My profile</span>
          </a>
          <a href="/support" className={active === "support" ? "active" : undefined} aria-current={active === "support" ? "page" : undefined}>
            <Icon name="help" />
            <span>Help & support</span>
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
            <strong>{name}</strong>
            <small>{branchName}</small>
          </div>
          <button type="button" aria-label="Open account settings" onClick={() => { window.location.href = "/account"; }}>•••</button>
        </div>
      </aside>

      <section className="member-workspace">
        <header className="member-topbar">
          <div>
            <p>{new Intl.DateTimeFormat("en-GH", { weekday: "long", day: "numeric", month: "long" }).format(new Date())}</p>
            <strong>Good {greeting()}, {firstName}.</strong>
          </div>
          <div>
            <button aria-label="Open messages" onClick={() => { window.location.href = "/messages"; }}><Icon name="bell" /><i /></button>
            <span aria-label="Your profile">{initials}</span>
          </div>
        </header>
        <div className="member-content member-route-content">{children}</div>
      </section>

      <nav className="member-mobile-nav" aria-label="Mobile navigation">
        {mobileNavigation.map((item) => (
          <a
            href={item.href}
            className={active === item.key ? "active" : undefined}
            aria-current={active === item.key ? "page" : undefined}
            key={item.key}
          >
            <Icon name={item.icon} />
            <span>{item.label}</span>
          </a>
        ))}
      </nav>
    </div>
  );
}

function greeting() {
  const hour = new Date().getHours();
  if (hour < 12) return "morning";
  if (hour < 18) return "afternoon";
  return "evening";
}
