"use client";

import { usePathname, useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { api, clearSession, getStoredUser, getToken, setSession, type AdminPreferences, type AdminUser } from "@/lib/api";

const titles: Record<string, [string, string]> = {
  "/": ["Ministry overview", "Publishing and community attention"],
  "/review": ["Review queue", "Content awaiting pastoral sign-off"],
  "/inbox": ["Community inbox", "Prayer, contact and visit requests"],
  "/media": ["Media library", "Images and ministry assets"],
  "/users": ["Team & access", "Administrative roles and permissions"],
  "/settings": ["Site settings", "Church information and public configuration"],
	"/account": ["My account", "Profile, preferences and sign-in security"],
};

export default function Header({ onMenuToggle }: { onMenuToggle: () => void }) {
  const pathname = usePathname();
  const router = useRouter();
  const [user, setUser] = useState<AdminUser | null>(null);
  const [darkTheme, setDarkTheme] = useState(false);
  const [accountOpen, setAccountOpen] = useState(false);
  const [noticesOpen, setNoticesOpen] = useState(false);
  const [notices, setNotices] = useState({ pendingReview: 0, newPrayerRequests: 0, newContacts: 0 });
  useEffect(() => {
    const stored = getStoredUser();
    setUser(stored);
    setDarkTheme(document.documentElement.dataset.theme === "dark");
    api<Partial<typeof notices>>("/api/admin/stats")
      .then((stats) => setNotices((current) => ({ ...current, ...stats })))
      .catch(() => undefined);
  }, []);
  useEffect(() => { setAccountOpen(false); setNoticesOpen(false); }, [pathname]);
  useEffect(() => {
    const syncTheme = () => {
      setUser(getStoredUser());
      setDarkTheme(document.documentElement.dataset.theme === "dark");
    };
    window.addEventListener("remi-preferences", syncTheme);
    return () => window.removeEventListener("remi-preferences", syncTheme);
  }, []);
  const contentMatch = pathname.match(/^\/content\/([^/]+)/);
  const fallback: [string, string] = contentMatch
    ? [`${contentMatch[1].charAt(0).toUpperCase()}${contentMatch[1].slice(1)}`, "Website content and publishing"]
    : ["REMI administration", "Ministry operations workspace"];
  const [title, copy] = titles[pathname] ?? fallback;
  function logout() { clearSession(); router.replace("/login"); }
  async function toggleTheme() {
    if (!user) return;
    const previous = darkTheme;
    const nextDark = !previous;
    const nextUser: AdminUser = { ...user, preferences: { ...user.preferences, theme: nextDark ? "dark" : "light" } };
    setDarkTheme(nextDark);
    setUser(nextUser);
    document.documentElement.dataset.theme = nextDark ? "dark" : "light";
    const token = getToken();
    if (token) setSession(token, nextUser);
    window.dispatchEvent(new Event("remi-preferences"));
    try {
      const result = await api<{ preferences: AdminPreferences }>("/api/account/preferences", { method: "PUT", body: nextUser.preferences });
      const saved = { ...nextUser, preferences: result.preferences };
      setUser(saved);
      if (token) setSession(token, saved);
    } catch {
      const rolledBack: AdminUser = { ...user, preferences: { ...user.preferences, theme: previous ? "dark" : "light" } };
      setDarkTheme(previous);
      setUser(rolledBack);
      document.documentElement.dataset.theme = previous ? "dark" : "light";
      if (token) setSession(token, rolledBack);
    }
  }

  return (
    <header className="admin-topbar">
      <button onClick={onMenuToggle} className="admin-topbar-menu" aria-label="Open navigation">☰</button>
      <div className="admin-topbar-title"><strong>{title}</strong><span>{copy}</span></div>
      <div className="admin-topbar-health"><i /> All core systems normal</div>
      <button type="button" onClick={toggleTheme} className="admin-theme-toggle" aria-label={`Switch to ${darkTheme ? "light" : "dark"} theme`} aria-pressed={darkTheme} title={`Switch to ${darkTheme ? "light" : "dark"} theme`}>
        <span className="admin-theme-toggle-track" aria-hidden="true">
          <svg className="admin-theme-sun" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round"><circle cx="12" cy="12" r="3.25"/><path d="M12 2.5v2M12 19.5v2M4.5 12h-2M21.5 12h-2M5.3 5.3l1.4 1.4M17.3 17.3l1.4 1.4M18.7 5.3l-1.4 1.4M6.7 17.3l-1.4 1.4"/></svg>
          <svg className="admin-theme-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M19.2 15.2A8 8 0 0 1 8.8 4.8 8 8 0 1 0 19.2 15.2Z"/><path d="M16.5 5.2v2.6M15.2 6.5h2.6"/></svg>
          <span className="admin-theme-orb" />
        </span>
      </button>
      <div className="admin-notice-wrap">
        <button onClick={() => { setNoticesOpen((value) => !value); setAccountOpen(false); }} className="admin-notice-button" aria-label="Notifications" aria-expanded={noticesOpen}><span>○</span>{Object.values(notices).some((value) => value > 0) && <i />}</button>
        {noticesOpen && (
          <div className="admin-notice-menu">
            <div className="admin-notice-head"><span>Operational attention</span><b>{Object.values(notices).reduce((sum, value) => sum + value, 0)}</b></div>
            <button onClick={() => router.push("/review")}><span><strong>Content review</strong><small>Awaiting approval or publication</small></span><b>{notices.pendingReview}</b></button>
            <button onClick={() => router.push("/inbox?tab=prayer")}><span><strong>Prayer requests</strong><small>New pastoral care messages</small></span><b>{notices.newPrayerRequests}</b></button>
            <button onClick={() => router.push("/inbox?tab=contact")}><span><strong>Contact messages</strong><small>Awaiting a response</small></span><b>{notices.newContacts}</b></button>
          </div>
        )}
      </div>
      <div className="admin-account-wrap">
        <button onClick={() => { setAccountOpen((value) => !value); setNoticesOpen(false); }} className="admin-account-button" aria-expanded={accountOpen}>
          <span className="admin-account-avatar">{(user?.name || user?.email || "A").charAt(0).toUpperCase()}</span>
          <span className="admin-account-copy"><strong>{user?.name || "REMI operator"}</strong><small>{user?.role || "Administrator"}</small></span>
          <span className="admin-account-chevron">⌄</span>
        </button>
        {accountOpen && (
          <div className="admin-account-menu">
            <div><strong>{user?.name || "REMI operator"}</strong><span>{user?.email || "Privileged ministry access"}</span></div>
			<button onClick={() => router.push("/account")}>Profile & security <span>→</span></button>
            <button onClick={logout}>Sign out securely <span>→</span></button>
          </div>
        )}
      </div>
    </header>
  );
}
