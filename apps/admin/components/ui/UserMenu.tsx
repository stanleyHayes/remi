"use client";

import { useEffect, useRef, useState } from "react";

export type UserMenuItem = {
  label: string;
  caption?: string;
  href: string;
};

export type UserMenuUser = {
  name: string;
  email?: string;
  roleLabel?: string;
};

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  const first = parts[0]?.[0] ?? "";
  const second = parts[1]?.[0] ?? "";
  return (first + second || name.slice(0, 2)).toUpperCase();
}

function InitialsAvatar({ name, size }: { name: string; size: number }) {
  return (
    <span
      className="grid shrink-0 place-items-center rounded-full bg-gold text-sidebar"
      style={{ width: size, height: size, fontSize: size * 0.38, fontWeight: 700 }}
      aria-hidden="true"
    >
      {initials(name)}
    </span>
  );
}

export { InitialsAvatar };

function ChevronDownIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden="true">
      <path d="m6 9 6 6 6-6" />
    </svg>
  );
}

function LogOutIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden="true">
      <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
      <polyline points="16 17 21 12 16 7" />
      <line x1="21" x2="9" y1="12" y2="12" />
    </svg>
  );
}

/**
 * Account menu for the bottom of the sidebar: an initials-avatar trigger with
 * name + role, an upward-opening dropdown with an identity band and a danger
 * sign-out. Closes on item click, outside click, and Escape.
 */
export function UserMenu({
  user,
  items = [],
  onNavigate,
  onLogout,
}: {
  user: UserMenuUser;
  items?: UserMenuItem[];
  onNavigate: (href: string) => void;
  onLogout?: () => void;
}) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return undefined;

    function onMouseDown(event: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    }
    function onKeyDown(event: KeyboardEvent) {
      if (event.key === "Escape") {
        setOpen(false);
      }
    }

    document.addEventListener("mousedown", onMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  function go(href: string) {
    setOpen(false);
    onNavigate(href);
  }

  return (
    <div ref={rootRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        aria-haspopup="menu"
        className="flex w-full items-center gap-2.5 rounded-md px-2 py-1.5 text-left transition hover:bg-sidebar-hover"
      >
        <InitialsAvatar name={user.name} size={32} />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-sm font-semibold text-zinc-100">{user.name}</span>
          {user.roleLabel ? (
            <span className="block truncate text-xs capitalize text-zinc-500">{user.roleLabel.replace(/_/g, " ")}</span>
          ) : null}
        </span>
        <ChevronDownIcon className={`h-4 w-4 shrink-0 text-zinc-500 transition-transform ${open ? "rotate-180" : ""}`} />
      </button>

      {open ? (
        <div
          role="menu"
          className="absolute bottom-full left-0 z-50 mb-2 w-full overflow-hidden rounded-xl border border-zinc-700 bg-[#1c1c24] shadow-2xl"
        >
          <div className="flex items-center gap-3 border-b border-zinc-700/70 bg-sidebar-hover px-4 py-3.5">
            <InitialsAvatar name={user.name} size={40} />
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold text-zinc-100">{user.name}</p>
              {user.email ? <p className="truncate text-xs text-zinc-500">{user.email}</p> : null}
              {user.roleLabel ? (
                <p className="mt-0.5 inline-block rounded-md bg-sidebar px-1.5 py-0.5 text-[0.65rem] font-semibold uppercase tracking-wide text-gold">
                  {user.roleLabel.replace(/_/g, " ")}
                </p>
              ) : null}
            </div>
          </div>

          {items.length > 0 ? (
            <div className="py-1.5">
              {items.map((item) => (
                <button
                  key={item.href + item.label}
                  type="button"
                  role="menuitem"
                  onClick={() => go(item.href)}
                  className="flex w-full items-center gap-3 px-4 py-2.5 text-left transition hover:bg-sidebar-hover"
                >
                  <span className="min-w-0">
                    <span className="block truncate text-sm font-semibold text-zinc-100">{item.label}</span>
                    {item.caption ? <span className="block truncate text-xs text-zinc-500">{item.caption}</span> : null}
                  </span>
                </button>
              ))}
            </div>
          ) : null}

          {onLogout ? (
            <div className="border-t border-zinc-700/70 py-1.5">
              <button
                type="button"
                role="menuitem"
                onClick={() => {
                  setOpen(false);
                  onLogout();
                }}
                className="flex w-full items-center gap-3 px-4 py-2.5 text-left transition hover:bg-rose-500/10"
              >
                <span className="grid h-8 w-8 shrink-0 place-items-center rounded-[10px] bg-rose-500/10 text-rose-400" aria-hidden="true">
                  <LogOutIcon className="h-4 w-4" />
                </span>
                <span className="min-w-0">
                  <span className="block truncate text-sm font-semibold text-rose-400">Sign out</span>
                  <span className="block truncate text-xs text-zinc-500">End your session on this device</span>
                </span>
              </button>
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
