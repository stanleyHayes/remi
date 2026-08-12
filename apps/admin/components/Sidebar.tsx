"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, clearSession, getStoredUser, type AdminUser } from "@/lib/api";
import { CONTENT_TYPES } from "@/lib/content";
import { UserMenu } from "@/components/ui/UserMenu";

interface NavItem {
  href: string;
  label: string;
  icon: string;
}
type GroupKey = "workspace" | "ministry" | "publishing" | "administration";

const TOP: NavItem[] = [
  { href: "/", label: "Overview", icon: "⌂" },
  { href: "/reports", label: "Reporting library", icon: "⌁" },
  { href: "/review", label: "Review queue", icon: "✓" },
  { href: "/inbox", label: "Community inbox", icon: "↗" },
];
const MINISTRY: NavItem[] = [
  { href: "/people", label: "People & households", icon: "◎" },
  { href: "/services", label: "Services & attendance", icon: "◫" },
  { href: "/checkin", label: "Check-in station", icon: "↳" },
  { href: "/groups", label: "Groups & ministries", icon: "✦" },
  { href: "/groups/insights", label: "Ministry insights", icon: "⌁" },
  { href: "/communications", label: "Communications studio", icon: "◉" },
  { href: "/retention", label: "Retention policy lab", icon: "◌" },
  { href: "/retention/insights", label: "Connection insights", icon: "⌁" },
  { href: "/retention/safety", label: "Safety & pastoral UAT", icon: "✓" },
  { href: "/volunteers", label: "Volunteer operations", icon: "☆" },
  { href: "/finance", label: "Finance command center", icon: "◈" },
  { href: "/finance/batches", label: "Counting & approvals", icon: "⊞" },
  { href: "/finance/gifts", label: "Gift register", icon: "▤" },
  { href: "/finance/configuration", label: "Finance configuration", icon: "⚙" },
  { href: "/finance/reports", label: "Finance reports", icon: "≋" },
  { href: "/fundraising", label: "Fundraising campaigns", icon: "◇" },
  { href: "/reconciliation", label: "Finance reconciliation", icon: "≍" },
  { href: "/statements", label: "Receipts & statements", icon: "▤" },
];
const BOTTOM: NavItem[] = [
  { href: "/media", label: "Media library", icon: "◇" },
  { href: "/users", label: "Team & access", icon: "◎" },
  { href: "/audit", label: "Audit assurance", icon: "⌁" },
  { href: "/privacy-requests", label: "Privacy requests", icon: "◌" },
  { href: "/settings", label: "Site settings", icon: "⚙" },
  { href: "/account", label: "My account", icon: "◉" },
];
const CONTENT_ICONS: Record<string, string> = {
  pages: "▤",
  leadership: "♙",
  ministries: "✦",
  branches: "⌖",
  sermons: "▷",
  events: "□",
  announcements: "◫",
  news: "≋",
  testimonies: "❞",
  gallery: "▧",
};

function isActive(pathname: string, href: string) {
  if (href === "/") return pathname === "/";
  if (pathname === href) return true;
  if (!pathname.startsWith(`${href}/`)) return false;
  const moreSpecificRoute = [...TOP, ...MINISTRY, ...BOTTOM].some(
    (item) =>
      item.href.length > href.length &&
      (pathname === item.href || pathname.startsWith(`${item.href}/`)),
  );
  return !moreSpecificRoute;
}

export default function Sidebar({
  open = false,
  collapsed = false,
  onClose,
  onCollapse,
}: {
  open?: boolean;
  collapsed?: boolean;
  onClose?: () => void;
  onCollapse?: () => void;
}) {
  const pathname = usePathname();
  const router = useRouter();
  const [user, setUser] = useState<AdminUser | null>(null);
  const [pendingReview, setPendingReview] = useState<number | null>(null);
  const [groups, setGroups] = useState<Record<GroupKey, boolean>>({
    workspace: true,
    ministry: true,
    publishing: true,
    administration: true,
  });

  useEffect(() => {
    setUser(getStoredUser());
    api<{ pendingReview?: number }>("/api/admin/stats")
      .then((s) =>
        setPendingReview(
          typeof s?.pendingReview === "number" ? s.pendingReview : null,
        ),
      )
      .catch(() => setPendingReview(null));
  }, [pathname]);

  useEffect(() => {
    const activeGroup: GroupKey =
      pathname.startsWith("/people") ||
      pathname.startsWith("/services") ||
      pathname.startsWith("/checkin") ||
      pathname.startsWith("/groups") ||
      pathname.startsWith("/communications") ||
      pathname.startsWith("/retention") ||
      pathname.startsWith("/volunteers") ||
      pathname.startsWith("/finance") ||
      pathname.startsWith("/fundraising") ||
      pathname.startsWith("/reconciliation") ||
      pathname.startsWith("/statements")
        ? "ministry"
        : pathname.startsWith("/content/")
          ? "publishing"
          : BOTTOM.some((item) => isActive(pathname, item.href))
            ? "administration"
            : "workspace";
    setGroups((current) =>
      current[activeGroup] ? current : { ...current, [activeGroup]: true },
    );
  }, [pathname]);

  function logout() {
    clearSession();
    router.replace("/login");
  }
  const linkCls = (active: boolean) =>
    `admin-nav-link ${active ? "is-active" : ""}`;
  const NavLink = ({
    item,
    count,
  }: {
    item: NavItem;
    count?: number | null;
  }) => (
    <Link
      href={item.href}
      onClick={onClose}
      title={collapsed ? item.label : undefined}
      className={linkCls(isActive(pathname, item.href))}
    >
      <span className="admin-nav-icon" aria-hidden="true">
        {item.icon}
      </span>
      <span>{item.label}</span>
      {!!count && <span className="admin-nav-count">{count}</span>}
    </Link>
  );
  const NavGroup = ({
    id,
    label,
    children,
  }: {
    id: GroupKey;
    label: string;
    children: React.ReactNode;
  }) => {
    const expanded = groups[id];
    return (
      <section className="admin-nav-group">
        <button
          type="button"
          className="admin-nav-group-trigger"
          aria-expanded={expanded}
          aria-controls={`admin-nav-${id}`}
          onClick={() =>
            setGroups((current) => ({ ...current, [id]: !current[id] }))
          }
        >
          <span className="admin-nav-group-node" aria-hidden="true" />
          <span>{label}</span>
          <span
            className={`admin-nav-group-chevron ${expanded ? "is-open" : ""}`}
            aria-hidden="true"
          >
            ⌄
          </span>
        </button>
        <div
          id={`admin-nav-${id}`}
          className={`admin-nav-group-body ${expanded ? "is-open" : ""}`}
        >
          <div className="admin-nav-tree">{children}</div>
        </div>
      </section>
    );
  };

  return (
    <>
      {open && (
        <button
          className="admin-sidebar-scrim"
          onClick={onClose}
          aria-label="Close navigation"
        />
      )}
      <aside
        className={`admin-sidebar ${open ? "is-open" : ""} ${collapsed ? "is-collapsed" : ""}`}
      >
        <div className="admin-sidebar-head">
          <Brand />
          <button
            onClick={onCollapse}
            className="admin-sidebar-collapse"
            aria-label={collapsed ? "Expand navigation" : "Collapse navigation"}
          >
            <svg
              viewBox="0 0 20 20"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.25"
              strokeLinecap="round"
              strokeLinejoin="round"
              aria-hidden="true"
            >
              <path
                d={collapsed ? "m7.5 4.5 5 5.5-5 5.5" : "m12.5 4.5-5 5.5 5 5.5"}
              />
            </svg>
          </button>
          <button
            onClick={onClose}
            className="admin-sidebar-close"
            aria-label="Close navigation"
          >
            ×
          </button>
        </div>
        <div className="admin-site-status">
          <span /> Website operational <b>LIVE</b>
        </div>
        <nav className="admin-nav" aria-label="Admin navigation">
          <NavGroup id="workspace" label="Workspace">
            {TOP.map((item) => (
              <NavLink
                key={item.href}
                item={item}
                count={item.href === "/review" ? pendingReview : null}
              />
            ))}
          </NavGroup>
          <NavGroup id="ministry" label="Church management">
            {MINISTRY.map((item) => (
              <NavLink key={item.href} item={item} />
            ))}
          </NavGroup>
          <NavGroup id="publishing" label="Publishing">
            {CONTENT_TYPES.map((t) => (
              <NavLink
                key={t.type}
                item={{
                  href: `/content/${t.type}`,
                  label: t.labelPlural,
                  icon: CONTENT_ICONS[t.type] || "·",
                }}
              />
            ))}
          </NavGroup>
          <NavGroup id="administration" label="Administration">
            {BOTTOM.map((item) => (
              <NavLink key={item.href} item={item} />
            ))}
          </NavGroup>
        </nav>
        <div className="admin-sidebar-foot">
          <p className="admin-role-kicker">Current session</p>
          <UserMenu
            user={{
              name: user?.name || user?.email || "Signed in",
              email: user?.name ? user.email : undefined,
              roleLabel: user?.role,
            }}
            items={[
              {
                label: "Profile & security",
                caption: "Identity, password and MFA",
                href: "/account",
              },
              {
                label: "Preferences",
                caption: "Workspace behavior",
                href: "/account?section=preferences",
              },
            ]}
            onNavigate={(href) => router.push(href)}
            onLogout={logout}
          />
        </div>
      </aside>
    </>
  );
}

function Brand({ compact = false }: { compact?: boolean }) {
  return (
    <div className={`admin-brand ${compact ? "is-compact" : ""}`}>
      <span className="admin-brand-mark">
        <i>R</i>
      </span>
      <span>
        <strong>REMI</strong>
        {!compact && <small>Ministry operations</small>}
      </span>
    </div>
  );
}
