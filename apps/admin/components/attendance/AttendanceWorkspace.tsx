"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { ApiError, api } from "@/lib/api";
import { displayMemberName, loadPeople, type PeoplePage } from "@/lib/chms";
import { Card, EmptyState, ErrorBox } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import { SkeletonTable } from "@/components/ui/Skeleton";

interface Occurrence {
  id: string;
  name: string;
  startsAt: string;
  endsAt: string;
}

interface AttendanceFact {
  id: string;
  version: number;
  personId: string;
  status: "present" | "absent" | "excused";
  source: string;
  confidence: number;
  guest: boolean;
}

interface Headcount {
  id: string;
  version: number;
  category: string;
  count: number;
  source: string;
  confidence: number;
}

interface AttendanceControl { locked: boolean; lock?: { reason: string; lockedAt: string }; period?: { reason: string; startsAt: string; endsAt: string } }

type Person = PeoplePage["items"][number];

export function AttendanceWorkspace({ occurrence, branchId, onClose }: { occurrence: Occurrence; branchId: string; onClose: () => void }) {
  const [attendance, setAttendance] = useState<AttendanceFact[]>([]);
  const [headcounts, setHeadcounts] = useState<Headcount[]>([]);
  const [control, setControl] = useState<AttendanceControl>({ locked: false });
  const [people, setPeople] = useState<Person[]>([]);
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [source, setSource] = useState("roster");
  const [confidence, setConfidence] = useState("100");
  const [headcount, setHeadcount] = useState({ category: "main auditorium", count: "", confidence: "100" });
  const [correctionReason, setCorrectionReason] = useState("");
  const [lockReason, setLockReason] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [attendanceData, headcountData, peopleData, controlData] = await Promise.all([
        api<{ items: AttendanceFact[] }>(`/api/chms/v1/occurrences/${encodeURIComponent(occurrence.id)}/attendance`),
        api<{ items: Headcount[] }>(`/api/chms/v1/occurrences/${encodeURIComponent(occurrence.id)}/headcounts`),
        loadPeople("", branchId),
        api<AttendanceControl>(`/api/chms/v1/occurrences/${encodeURIComponent(occurrence.id)}/attendance-control`),
      ]);
      setAttendance(attendanceData.items || []);
      setHeadcounts(headcountData.items || []);
      setPeople(peopleData.items || []);
      setControl(controlData);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "Attendance operations could not be loaded.");
    } finally {
      setLoading(false);
    }
  }, [branchId, occurrence.id]);

  useEffect(() => { void load(); }, [load]);

  useEffect(() => {
    const handle = window.setTimeout(async () => {
      try {
        const result = await loadPeople(query, branchId);
        setPeople(result.items || []);
      } catch { /* The main error surface remains stable while searching. */ }
    }, 250);
    return () => window.clearTimeout(handle);
  }, [branchId, query]);

  const facts = useMemo(() => new Map(attendance.map((fact) => [fact.personId, fact])), [attendance]);

  async function mark(person: Person, status: AttendanceFact["status"], guest = false) {
    const current = facts.get(person.id);
    setBusy(person.id);
    setError(null);
    try {
      if (control.locked && correctionReason.trim().length < 3) {
        throw new ApiError(400, "Enter the approved correction reason before changing locked attendance.");
      }
      const path = control.locked ? `/api/chms/v1/occurrences/${encodeURIComponent(occurrence.id)}/attendance-corrections` : `/api/chms/v1/occurrences/${encodeURIComponent(occurrence.id)}/attendance/${encodeURIComponent(person.id)}`;
      const updated = await api<AttendanceFact>(path, {
        method: control.locked ? "POST" : "PUT",
        body: { personId: person.id, expectedVersion: current?.version || 0, status, source, confidence: Number(confidence), guest: guest || person.membershipStage === "guest", reason: correctionReason },
      });
      setAttendance((items) => [...items.filter((item) => item.personId !== person.id), updated]);
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "Attendance could not be recorded.");
    } finally {
      setBusy("");
    }
  }

  async function lockAttendance() {
    if (lockReason.trim().length < 3) return;
    setBusy("lock");
    setError(null);
    try {
      await api(`/api/chms/v1/occurrences/${encodeURIComponent(occurrence.id)}/lock-attendance`, { method: "POST", body: { reason: lockReason } });
      setLockReason("");
      await load();
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "Attendance could not be locked.");
    } finally { setBusy(""); }
  }

  async function addHeadcount() {
    if (!headcount.count) return;
    setBusy("headcount");
    setError(null);
    try {
      const created = await api<Headcount>(`/api/chms/v1/occurrences/${encodeURIComponent(occurrence.id)}/headcounts`, {
        method: "POST",
        body: { category: headcount.category, count: Number(headcount.count), source: "operator", confidence: Number(headcount.confidence), observedAt: new Date().toISOString() },
      });
      setHeadcounts((items) => [...items, created]);
      setHeadcount((value) => ({ ...value, count: "" }));
    } catch (cause) {
      setError(cause instanceof ApiError ? cause.message : "Headcount could not be recorded.");
    } finally {
      setBusy("");
    }
  }

  return (
    <Card className="member-panel mt-5">
      <div className="member-panel-title">
        <div>
          <p className="!mb-1 font-mono uppercase tracking-[.18em]">Attendance desk</p>
          <h3>{occurrence.name}</h3>
          <p>{formatDate(occurrence.startsAt)} · Named attendance and anonymous totals stay separate.</p>
        </div>
        <button type="button" className="admin-secondary-action rounded-lg px-3 py-2 text-xs font-semibold" onClick={onClose}>Back to occurrences</button>
      </div>
      {error && <div className="mt-4"><ErrorBox message={error} onRetry={load} /></div>}
      <div className="mt-4 rounded-xl border border-[var(--remi-line)] bg-[var(--remi-surface-soft)] p-4">
        {control.locked ? <div className="grid gap-3 md:grid-cols-[1fr_minmax(16rem,.7fr)] md:items-end"><div><b className="text-sm text-[var(--remi-ink)]">Attendance locked</b><p className="mt-1 text-xs text-[var(--remi-muted)]">{control.lock?.reason || control.period?.reason || "This occurrence belongs to a closed period."} Every change now uses the approved correction route.</p></div><label className="person-field"><span>Approved correction reason</span><input value={correctionReason} onChange={(event) => setCorrectionReason(event.target.value)} placeholder="Required before changing a record" /></label></div> : <div className="grid gap-3 md:grid-cols-[1fr_minmax(16rem,.7fr)_auto] md:items-end"><div><b className="text-sm text-[var(--remi-ink)]">Review remains open</b><p className="mt-1 text-xs text-[var(--remi-muted)]">Lock after reconciliation to require approval and an immutable reason for later changes.</p></div><label className="person-field"><span>Lock reason</span><input value={lockReason} onChange={(event) => setLockReason(event.target.value)} placeholder="e.g. service lead approved" /></label><button type="button" className="admin-secondary-action rounded-lg px-4 py-2.5 text-sm font-semibold" disabled={busy === "lock" || lockReason.trim().length < 3 || new Date(occurrence.endsAt) > new Date()} onClick={lockAttendance}>{busy === "lock" ? "Locking…" : "Lock attendance"}</button></div>}
      </div>
      {loading ? <div className="mt-5"><SkeletonTable rows={5} cols={4} /></div> : (
        <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(0,1.35fr)_minmax(18rem,.65fr)]">
          <section>
            <div className="grid gap-3 sm:grid-cols-[1fr_10rem_8rem]">
              <label className="person-field"><span>Find person</span><input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search name or contact" /></label>
              <label className="person-field"><span>Capture source</span><Select value={source} onChange={(event) => setSource(event.target.value)}><option value="roster">Roster</option><option value="operator">Operator</option><option value="kiosk">Kiosk</option><option value="import">Import</option></Select></label>
              <label className="person-field"><span>Confidence</span><Select value={confidence} onChange={(event) => setConfidence(event.target.value)}><option value="100">Confirmed</option><option value="85">High</option><option value="60">Reported</option></Select></label>
            </div>
            <div className="member-row-list mt-4">
              {people.map((person) => {
                const fact = facts.get(person.id);
                return <div key={person.id}>
                  <span className="member-row-icon">{fact?.status === "present" ? "✓" : fact?.status === "excused" ? "–" : "○"}</span>
                  <span><b>{displayMemberName(person.names)}</b><small>{person.membershipStage.replaceAll("-", " ")}{fact ? ` · ${fact.source} · ${fact.confidence}%` : " · not marked"}</small></span>
                  <div className="flex flex-wrap justify-end gap-1.5">
                    {(["present", "absent", "excused"] as const).map((status) => <button key={status} type="button" disabled={busy === person.id} onClick={() => mark(person, status)} className={fact?.status === status ? "admin-primary-button rounded-lg px-2.5 py-1.5 text-[11px] font-semibold" : "admin-secondary-action rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"}>{status}</button>)}
                  </div>
                </div>;
              })}
            </div>
            {!people.length && <EmptyState title="No people found" hint="Create the guest or member first, then record their named attendance." />}
          </section>
          <aside>
            <h3 className="text-sm font-bold text-[var(--remi-ink)]">Anonymous headcount</h3>
            <p className="mt-1 text-xs text-[var(--remi-muted)]">Aggregate observations never create member records.</p>
            <div className="mt-4 space-y-3">
              <label className="person-field"><span>Area</span><Select value={headcount.category} onChange={(event) => setHeadcount((value) => ({ ...value, category: event.target.value }))}><option value="main auditorium">Main auditorium</option><option value="overflow">Overflow</option><option value="children">Children</option><option value="online">Online</option></Select></label>
              <label className="person-field"><span>Observed count</span><input type="number" min="0" max="100000" inputMode="numeric" value={headcount.count} onChange={(event) => setHeadcount((value) => ({ ...value, count: event.target.value }))} /></label>
              <label className="person-field"><span>Confidence</span><Select value={headcount.confidence} onChange={(event) => setHeadcount((value) => ({ ...value, confidence: event.target.value }))}><option value="100">Confirmed</option><option value="85">High</option><option value="60">Estimated</option></Select></label>
              <button type="button" className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold" disabled={busy === "headcount" || !headcount.count} onClick={addHeadcount}>{busy === "headcount" ? "Recording…" : "Record headcount"}</button>
            </div>
            <div className="mt-5 space-y-2">{headcounts.map((item) => <div key={item.id} className="rounded-xl border border-[var(--remi-line)] p-3"><div className="flex items-center justify-between"><b className="text-sm text-[var(--remi-ink)]">{item.category}</b><strong className="font-mono text-lg text-[var(--remi-ink)]">{item.count}</strong></div><p className="mt-1 text-xs text-[var(--remi-muted)]">{item.source} · {item.confidence}% confidence</p></div>)}</div>
          </aside>
        </div>
      )}
    </Card>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }).format(new Date(value));
}
