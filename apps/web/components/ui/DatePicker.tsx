"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { createPortal } from "react-dom";

/**
 * Dependency-free calendar date picker, retokened for the dark REMI theme
 * (ink surfaces, cream text, gold accents). Ported from the 24hour-economy
 * dashboard's common/DatePicker.tsx and adapted to an uncontrolled,
 * form-friendly API: a hidden input carries `name` so FormData and plain
 * form submission keep working, and `required` blocks empty submission.
 */

interface DatePickerProps {
  /** Field name — submitted as yyyy-mm-dd via a hidden input. */
  name?: string;
  /** Initial value as an ISO date string (yyyy-mm-dd) or '' when empty. */
  defaultValue?: string;
  /** Inclusive bounds as yyyy-mm-dd; days outside are disabled. */
  min?: string;
  max?: string;
  placeholder?: string;
  disabled?: boolean;
  required?: boolean;
  id?: string;
  "aria-label"?: string;
  /** Wrapper classes (width, margins). */
  className?: string;
  /** Override the trigger box; defaults to the site's input styling. */
  triggerClassName?: string;
  onChange?: (value: string) => void;
}

const WEEKDAYS = ["Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"];
const MONTHS = [
  "January", "February", "March", "April", "May", "June",
  "July", "August", "September", "October", "November", "December",
];
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
  "flex w-full items-center justify-between gap-2 rounded-2xl border border-cream/15 bg-cream/5 px-5 py-3.5 text-left text-sm text-cream transition-colors duration-300 hover:border-cream/25 focus:border-gold/50 focus:outline-none focus-visible:ring-2 focus-visible:ring-gold/25";
const CAL_WIDTH = 280; // px

export default function DatePicker({
  name,
  defaultValue = "",
  min,
  max,
  placeholder = "Pick a date",
  disabled = false,
  required = false,
  id,
  "aria-label": ariaLabel,
  className = "",
  triggerClassName = DEFAULT_TRIGGER,
  onChange,
}: DatePickerProps) {
  const [value, setValue] = useState(defaultValue);
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

  const update = (iso: string) => {
    setValue(iso);
    onChange?.(iso);
  };

  const commit = (d: Date) => {
    if (disabledDay(d)) return;
    update(toISO(d));
    setOpen(false);
  };

  const onKeyDown = (e: KeyboardEvent) => {
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
      {name ? <input type="hidden" name={name} value={value} /> : null}
      <div
        role="button"
        id={id}
        tabIndex={disabled ? -1 : 0}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-disabled={disabled}
        aria-label={ariaLabel}
        onClick={() => (open ? setOpen(false) : openMenu())}
        onKeyDown={onKeyDown}
        className={`${triggerClassName} ${disabled ? "cursor-not-allowed opacity-60" : "cursor-pointer"}`}
      >
        <span className={`truncate ${selected ? "text-cream" : "text-cream-dim/60"}`}>
          {selected ? fmtDisplay(selected) : placeholder}
        </span>
        <svg
          aria-hidden="true"
          viewBox="0 0 24 24"
          className="h-4 w-4 shrink-0 text-cream-dim"
          fill="none"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        >
          <rect x="3" y="5" width="18" height="16" rx="2" />
          <path d="M16 3v4M8 3v4M3 10h18" />
        </svg>
      </div>

      {open && coords
        ? createPortal(
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
              className="z-50 rounded-2xl border border-cream/15 bg-ink-raise p-3 shadow-2xl shadow-black/60"
            >
              {/* Month navigation */}
              <div className="mb-2 flex items-center justify-between">
                <button
                  type="button"
                  aria-label="Previous month"
                  onClick={() => setView((v) => addMonths(v, -1))}
                  className="rounded-lg p-1.5 text-cream-dim transition-colors duration-150 hover:bg-cream/5 hover:text-cream focus:outline-none focus-visible:ring-2 focus-visible:ring-gold/25"
                >
                  <svg aria-hidden="true" viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="m15 18-6-6 6-6" />
                  </svg>
                </button>
                <div className="text-sm font-semibold text-cream">
                  {MONTHS[view.getMonth()]} {view.getFullYear()}
                </div>
                <button
                  type="button"
                  aria-label="Next month"
                  onClick={() => setView((v) => addMonths(v, 1))}
                  className="rounded-lg p-1.5 text-cream-dim transition-colors duration-150 hover:bg-cream/5 hover:text-cream focus:outline-none focus-visible:ring-2 focus-visible:ring-gold/25"
                >
                  <svg aria-hidden="true" viewBox="0 0 24 24" className="h-4 w-4" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="m9 18 6-6-6-6" />
                  </svg>
                </button>
              </div>

              {/* Weekday header */}
              <div className="mb-1 grid grid-cols-7">
                {WEEKDAYS.map((w) => (
                  <div key={w} className="py-1 text-center text-[11px] font-medium uppercase tracking-wide text-cream-dim/70">
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
                        "mx-auto flex h-8 w-8 items-center justify-center rounded-lg text-sm transition-colors duration-150",
                        dis ? "cursor-not-allowed text-cream-dim/30" : "cursor-pointer",
                        isSel
                          ? "bg-gold font-semibold text-ink hover:bg-gold"
                          : !dis
                            ? "text-cream-dim hover:bg-gold/10 hover:text-cream"
                            : "",
                        !isSel && isToday ? "font-semibold text-gold-bright ring-1 ring-inset ring-gold/40" : "",
                        !isSel && isFocus ? "ring-2 ring-inset ring-gold/30" : "",
                      ].join(" ")}
                    >
                      {d.getDate()}
                    </button>
                  );
                })}
              </div>

              {/* Footer */}
              <div className="mt-2 flex items-center justify-between border-t border-cream/10 pt-2">
                <button
                  type="button"
                  onClick={() => commit(today)}
                  disabled={disabledDay(today)}
                  className="rounded-md px-2 py-1 text-xs font-medium text-gold-bright transition-colors duration-150 hover:bg-gold/10 disabled:opacity-40"
                >
                  Today
                </button>
                {selected ? (
                  <button
                    type="button"
                    onClick={() => {
                      update("");
                      setOpen(false);
                    }}
                    className="rounded-md px-2 py-1 text-xs font-medium text-cream-dim transition-colors duration-150 hover:bg-cream/5 hover:text-cream"
                  >
                    Clear
                  </button>
                ) : null}
              </div>
            </div>,
            document.body,
          )
        : null}
      {required && value === "" ? (
        <input
          className="pointer-events-none absolute inset-x-0 bottom-0 h-px w-px opacity-0"
          required
          tabIndex={-1}
          aria-hidden="true"
        />
      ) : null}
    </div>
  );
}
