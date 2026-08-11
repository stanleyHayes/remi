"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import Sidebar from "@/components/Sidebar";
import Header from "@/components/Header";
import { api, getStoredUser, getToken, setSession, type AdminUser } from "@/lib/api";
import AdminSplash from "@/components/AdminSplash";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const router = useRouter();
  const [authState, setAuthState] = useState<"checking" | "ready" | "error">("checking");
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [collapsed, setCollapsed] = useState(false);

  useEffect(() => {
    setCollapsed(localStorage.getItem("remi-admin-nav-collapsed") === "true");
    if (!getToken()) {
      router.replace("/login");
      return;
    }
    api<{ user: AdminUser }>("/api/auth/me")
      .then(({ user }) => {
        const token = getToken();
        if (token) setSession(token, user);
        applyPreferences(user);
        setAuthState("ready");
      })
      .catch((error) => {
        if (!(error && typeof error === "object" && "status" in error && error.status === 401)) {
          setAuthState("error");
        }
      });
  }, [router]);

  useEffect(() => {
    const stored = getStoredUser();
    if (stored) applyPreferences(stored);
    const sync = () => { const next = getStoredUser(); if (next) applyPreferences(next); };
    window.addEventListener("remi-preferences", sync);
    return () => window.removeEventListener("remi-preferences", sync);
  }, []);

  function toggleCollapsed() {
    setCollapsed((value) => {
      localStorage.setItem("remi-admin-nav-collapsed", String(!value));
      return !value;
    });
  }

  if (authState === "error") {
    return <AdminSplash error onRetry={() => window.location.reload()} />;
  }

  if (authState !== "ready") {
    return <AdminSplash />;
  }

  return (
    <div className={`admin-shell ${collapsed ? "nav-is-collapsed" : ""}`}>
      <a className="admin-skip-link" href="#main-content">Skip to page content</a>
      <Sidebar open={sidebarOpen} collapsed={collapsed} onCollapse={toggleCollapsed} onClose={() => setSidebarOpen(false)} />
      <div className="admin-workspace">
        <Header onMenuToggle={() => setSidebarOpen((value) => !value)} />
        <main id="main-content" className="admin-main"><div className="admin-main-inner">{children}</div></main>
      </div>
    </div>
  );
}

function applyPreferences(user: AdminUser) {
  const theme = user.preferences?.theme ?? "system";
  const dark = theme === "dark" || (theme === "system" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.dataset.theme = dark ? "dark" : "light";
  document.documentElement.dataset.reduceMotion = user.preferences?.reducedMotion ? "true" : "false";
  document.documentElement.dataset.density = user.preferences?.density ?? "comfortable";
}
