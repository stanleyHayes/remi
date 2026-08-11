"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";

interface Props {
  /** Value as an ISO date string (yyyy-mm-dd) or '' when empty. */
  value: string;
  onChange: (value: string) => void;
  /** Inclusive bounds as yyyy-mm-dd; days outside are disabled. */
  min?: string;
  max?: string;
  placeholder?: string;
  disabled?: boolean;
  /** Show a clear (×) affordance and a Clear button in the popover. */
  clearable?: boolean;
  id?: string;
  /** Wrapper classes (width, margins). */
  className?: string;
  /** Override the trigger box; defaults to the REMI form-input look. */
  triggerClassName?: string;
}

const WEEKDAYS = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];
const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
const MONTHS_SHORT = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

const pad = (n: number) => String(n).padStart(2, "0");
const toISO = (d: Date) => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
const fmtDisplay = (d: Date) => `${pad(d.getDate())} ${MONTHS_SHORT[d.getMonth()]} ${d.getFullYear()}`;
const addDays = (d: Date, n: number) => new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
const addMonths = (d: Date, n: number) => new Date(d.getFullYear(), d.getMonth() + n, 1);
const sameDay = (a: Date | null, b: Date | null) => !!a && !!b && a.getTime() === b.getTime();

function parseISO(s: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(s || "");
  if (!m) return null;
  const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
  return isNaN(d.getTime()) ? null : d;
}

const DEFAULT_TRIGGER =
  "admin-form-trigger flex w-full items-center justify-between gap-2 rounded-md border border-zinc-300 bg-white px-3 py-2 text-left text-sm outline-none transition focus:border-gold";
const CAL_WIDTH = 280; // px

function CalendarIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-zinc-400" aria-hidden="true">
      <rect width="18" height="18" x="3" y="4" rx="2" />
      <path d="M16 2v4" />
      <path d="M8 2v4" />
      <path d="M3 10h18" />
    </svg>
  );
}

function ChevronLeftIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m15 18-6-6 6-6" />
    </svg>
  );
}

function ChevronRightIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
}

function XIcon() {
  return (
    <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </svg>
  );
}

export default function DatePicker({
  value,
  onChange,
  min,
  max,
  placeholder = "Select date",
  disabled = false,
  clearable = false,
  id,
  className = "",
  triggerClassName = DEFAULT_TRIGGER,
}: Props) {
  const selected = useMemo(() => parseISO(value), [value]);
  const today = useMemo(() => {
    const n = new Date();
    return new Date(n.getFullYear(), n.getMonth(), n.getDate());
  }, []);

  const [open, setOpen] = useState(false);
  const [view, setView] = useState<Date>(() => selected ?? today);
  const [focus, setFocus] = useState<Date>(() => selected ?? today);
  const [coords, setCoords] = useState<{ left: number; top: number; openUp: boolean } | null>(null);

  const rootRef = useRef<HTMLDivElement>(null);
  const popRef = useRef<HTMLDivElement>(null);

  // Re-sync the viewed month when the value changes externally (e.g. form load).
  useEffect(() => {
    if (selected) {
      setView(selected);
      setFocus(selected);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [value]);

  const belowMin = (d: Date) => !!min && toISO(d) < min;
  const aboveMax = (d: Date) => !!max && toISO(d) > max;
  const disabledDay = (d: Date) => belowMin(d) || aboveMax(d);

  const updateCoords = useCallback(() => {
    const el = rootRef.current;
    if (!el) return;
    const r = el.getBoundingClientRect();
    const calHeight = 340;
    const spaceBelow = window.innerHeight - r.bottom;
    const openUp = spaceBelow < calHeight && r.top > spaceBelow;
    setCoords({
      left: Math.max(8, Math.min(r.left, window.innerWidth - CAL_WIDTH - 8)),
      top: openUp ? r.top - 4 : r.bottom + 4,
      openUp,
    });
  }, []);

  const openMenu = () => {
    if (disabled) return;
    setView(selected ?? today);
    setFocus(selected ?? today);
    updateCoords();
    setOpen(true);
  };

  // Outside-click (accounts for the portalled popover) + reposition on scroll/resize.
  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      const t = e.target as Node;
      if (rootRef.current?.contains(t) || popRef.current?.contains(t)) return;
      setOpen(false);
    };
    const onMove = () => updateCoords();
    document.addEventListener("mousedown", onDoc);
    window.addEventListener("resize", onMove);
    window.addEventListener("scroll", onMove, true);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      window.removeEventListener("resize", onMove);
      window.removeEventListener("scroll", onMove, true);
    };
  }, [open, updateCoords]);

  const commit = (d: Date) => {
    if (disabledDay(d)) return;
    onChange(toISO(d));
    setOpen(false);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!open) {
      if (e.key === "Enter" || e.key === " " || e.key === "ArrowDown") {
        e.preventDefault();
        openMenu();
      }
      return;
    }
    let next: Date | null = null;
    switch (e.key) {
      case "ArrowLeft": next = addDays(focus, -1); break;
      case "ArrowRight": next = addDays(focus, 1); break;
      case "ArrowUp": next = addDays(focus, -7); break;
      case "ArrowDown": next = addDays(focus, 7); break;
      case "PageUp": next = new Date(focus.getFullYear(), focus.getMonth() - 1, focus.getDate()); break;
      case "PageDown": next = new Date(focus.getFullYear(), focus.getMonth() + 1, focus.getDate()); break;
      case "Home": next = new Date(focus.getFullYear(), focus.getMonth(), 1); break;
      case "End": next = new Date(focus.getFullYear(), focus.getMonth() + 1, 0); break;
      case "Enter":
      case " ": e.preventDefault(); commit(focus); return;
      case "Escape": e.preventDefault(); setOpen(false); return;
      default: return;
    }
    e.preventDefault();
    setFocus(next);
    setView(next);
  };

  // Day cells for the viewed month, with leading blanks for weekday alignment.
  const cells = useMemo(() => {
    const year = view.getFullYear();
    const month = view.getMonth();
    const firstWeekday = new Date(year, month, 1).getDay();
    const daysInMonth = new Date(year, month + 1, 0).getDate();
    const arr: (Date | null)[] = [];
    for (let i = 0; i < firstWeekday; i++) arr.push(null);
    for (let d = 1; d <= daysInMonth; d++) arr.push(new Date(year, month, d));
    return arr;
  }, [view]);

  return (
    <div ref={rootRef} className={`relative ${className}`}>
      <div
        role="button"
        id={id}
        tabIndex={disabled ? -1 : 0}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-disabled={disabled}
        onClick={() => (open ? setOpen(false) : openMenu())}
        onKeyDown={onKeyDown}
        className={`${triggerClassName} ${disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer"}`}
      >
        <span className={`truncate ${selected ? "text-zinc-900" : "text-zinc-400"}`}>
          {selected ? fmtDisplay(selected) : placeholder}
        </span>
        <span className="flex shrink-0 items-center gap-1.5">
          {clearable && selected && !disabled && (
            <button
              type="button"
              aria-label="Clear date"
              onClick={(e) => {
                e.stopPropagation();
                onChange("");
              }}
              className="text-zinc-400 hover:text-zinc-600"
            >
              <XIcon />
            </button>
          )}
          <CalendarIcon />
        </span>
      </div>

      {open &&
        coords &&
        createPortal(
          <div
            ref={popRef}
            role="dialog"
            aria-label="Choose date"
            onKeyDown={onKeyDown}
            style={{
              position: "fixed",
              left: coords.left,
              top: coords.top,
              width: CAL_WIDTH,
              transform: coords.openUp ? "translateY(-100%)" : undefined,
            }}
            className="admin-date-popover z-[150] rounded-xl border border-zinc-200 bg-white p-3 shadow-xl"
          >
            {/* Month navigation */}
            <div className="mb-2 flex items-center justify-between">
              <button
                type="button"
                aria-label="Previous month"
                onClick={() => setView((v) => addMonths(v, -1))}
                className="rounded-lg p-1.5 text-zinc-500 hover:bg-zinc-100 hover:text-zinc-700"
              >
                <ChevronLeftIcon />
              </button>
              <div className="text-sm font-semibold text-zinc-800">
                {MONTHS[view.getMonth()]} {view.getFullYear()}
              </div>
              <button
                type="button"
                aria-label="Next month"
                onClick={() => setView((v) => addMonths(v, 1))}
                className="rounded-lg p-1.5 text-zinc-500 hover:bg-zinc-100 hover:text-zinc-700"
              >
                <ChevronRightIcon />
              </button>
            </div>

            {/* Weekday header */}
            <div className="mb-1 grid grid-cols-7">
              {WEEKDAYS.map((w) => (
                <div key={w} className="py-1 text-center text-[11px] font-medium uppercase tracking-wide text-zinc-400">
                  {w}
                </div>
              ))}
            </div>

            {/* Day grid */}
            <div className="grid grid-cols-7 gap-0.5">
              {cells.map((d, i) => {
                if (!d) return <div key={`b${i}`} className="h-8" />;
                const isSel = sameDay(d, selected);
                const isToday = sameDay(d, today);
                const isFocus = sameDay(d, focus);
                const dis = disabledDay(d);
                return (
                  <button
                    type="button"
                    key={toISO(d)}
                    disabled={dis}
                    onClick={() => commit(d)}
                    onMouseEnter={() => !dis && setFocus(d)}
                    className={[
                      "mx-auto flex h-8 w-8 items-center justify-center rounded-lg text-sm transition-colors",
                      dis ? "cursor-not-allowed text-zinc-300" : "cursor-pointer",
                      isSel ? "bg-gold font-semibold text-sidebar hover:bg-gold" : !dis ? "text-zinc-700 hover:bg-[#f6ecce]" : "",
                      !isSel && isToday ? "font-semibold text-gold-dark ring-1 ring-inset ring-gold/60" : "",
                      !isSel && isFocus ? "ring-2 ring-inset ring-gold/50" : "",
                    ].join(" ")}
                  >
                    {d.getDate()}
                  </button>
                );
              })}
            </div>

            {/* Footer */}
            <div className="mt-2 flex items-center justify-between border-t border-zinc-100 pt-2">
              <button
                type="button"
                onClick={() => commit(today)}
                disabled={disabledDay(today)}
                className="rounded-md px-2 py-1 text-xs font-medium text-gold-dark hover:bg-[#f6ecce] disabled:opacity-40"
              >
                Today
              </button>
              {(clearable || selected) && (
                <button
                  type="button"
                  onClick={() => {
                    onChange("");
                    setOpen(false);
                  }}
                  className="rounded-md px-2 py-1 text-xs font-medium text-zinc-500 hover:bg-zinc-100"
                >
                  Clear
                </button>
              )}
            </div>
          </div>,
          document.body,
        )}
    </div>
  );
}
