"use client";

import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import { ApiError, api, asList, getStoredUser } from "@/lib/api";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import { SkeletonTable } from "@/components/ui/Skeleton";

interface Branch { id: string; name: string }
interface Occurrence { id: string; name: string; startsAt: string; endsAt: string; status: string }
interface Session { id: string; version: number; branchId: string; deviceLabel: string; occurrenceIds: string[]; state: string; expiresAt: string; lastSequence: number }
interface Household { id?: string; name: string; members: Array<{ id: string; name: string; role?: string }> }
interface Command { clientCommandId: string; localSequence: number; type: "check-in"; occurrenceId: string; personId: string; expectedVersion: number; capturedAt: string; guest: boolean; confidence: number }
interface CommandResult { clientCommandId: string; localSequence: number; classification: "applied" | "duplicate" | "conflict" | "rejected" | "retry"; code?: string; message?: string; version?: number }
interface SelectedPerson { id: string; name: string }
interface ChildLabel { checkinId: string; attendanceId: string; securityCode: string; expiresAt: string; childDisplayName: string }

export default function CheckinPage() {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branchId, setBranchId] = useState("");
  const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
  const [occurrenceId, setOccurrenceId] = useState("");
  const [deviceLabel, setDeviceLabel] = useState("Foyer tablet");
  const [session, setSession] = useState<Session | null>(null);
  const [queue, setQueue] = useState<Command[]>([]);
  const [results, setResults] = useState<CommandResult[]>([]);
  const [versions, setVersions] = useState<Record<string, number>>({});
  const [query, setQuery] = useState("");
  const [households, setHouseholds] = useState<Household[]>([]);
  const [online, setOnline] = useState(true);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [guest, setGuest] = useState({ given: "", family: "", phone: "" });
  const [lockReason, setLockReason] = useState("");
  const [selectedChild, setSelectedChild] = useState<SelectedPerson | null>(null);
  const [selectedGuardian, setSelectedGuardian] = useState<SelectedPerson | null>(null);
  const [childLabel, setChildLabel] = useState<ChildLabel | null>(null);
  const [pickup, setPickup] = useState({ attendanceId: "", code: "" });
  const [incident, setIncident] = useState({ category: "identity concern", summary: "" });
  const [canSafeguard, setCanSafeguard] = useState(false);

  useEffect(() => {
    setCanSafeguard(getStoredUser()?.role === "super-admin");
    setOnline(navigator.onLine);
    const on = () => setOnline(true); const off = () => setOnline(false);
    window.addEventListener("online", on); window.addEventListener("offline", off);
    api("/api/branches").then((value) => { const items = asList<Branch>(value); setBranches(items); setBranchId(items[0]?.id || ""); }).catch(() => setBranches([])).finally(() => setLoading(false));
    return () => { window.removeEventListener("online", on); window.removeEventListener("offline", off); };
  }, []);

  useEffect(() => {
    if (!branchId) { setOccurrences([]); return; }
    const from = new Date(); from.setDate(from.getDate() - 1); const to = new Date(); to.setDate(to.getDate() + 7);
    api<{ items: Occurrence[] }>(`/api/chms/v1/occurrences?branchId=${encodeURIComponent(branchId)}&from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`)
      .then((value) => { const items = (value.items || []).filter((item) => item.status === "scheduled"); setOccurrences(items); setOccurrenceId(items[0]?.id || ""); })
      .catch((cause) => setError(cause instanceof ApiError ? cause.message : "Occurrences could not be loaded."));
  }, [branchId]);

  useEffect(() => {
    if (!session) return;
    try { const stored = localStorage.getItem(`remi-checkin-queue:${session.id}`); setQueue(stored ? JSON.parse(stored) as Command[] : []); } catch { setQueue([]); }
  }, [session]);
  useEffect(() => { if (session) localStorage.setItem(`remi-checkin-queue:${session.id}`, JSON.stringify(queue)); }, [queue, session]);

  const occurrence = useMemo(() => occurrences.find((item) => item.id === occurrenceId), [occurrenceId, occurrences]);
  const lookup = useCallback(async () => {
    if (!session || query.trim().length < 2 || !online) { setHouseholds([]); return; }
    try { const value = await api<{ items: Household[] }>(`/api/chms/v1/checkin/sessions/${session.id}/households?q=${encodeURIComponent(query.trim())}`); setHouseholds(value.items || []); }
    catch (cause) { setError(cause instanceof ApiError ? cause.message : "Household lookup failed."); }
  }, [online, query, session]);
  useEffect(() => { const handle = window.setTimeout(() => { void lookup(); }, 250); return () => window.clearTimeout(handle); }, [lookup]);

  async function startSession() {
    if (!branchId || !occurrenceId) return;
    setBusy("start"); setError(null);
    try {
      const expiresAt = new Date(); expiresAt.setHours(expiresAt.getHours() + 12);
      const created = await api<Session>("/api/chms/v1/checkin/sessions", { method: "POST", body: { branchId, deviceLabel, occurrenceIds: [occurrenceId], expiresAt: expiresAt.toISOString() } });
      setSession(created); setQueue([]); setResults([]);
    } catch (cause) { setError(cause instanceof ApiError ? cause.message : "Station could not be authorized."); }
    finally { setBusy(""); }
  }

  function enqueue(person: Household["members"][number], isGuest = false) {
    if (!session || !occurrenceId) return;
    setQueue((items) => { const last = Math.max(session.lastSequence || 0, ...items.map((item) => item.localSequence), 0); return [...items, { clientCommandId: crypto.randomUUID(), localSequence: last + 1, type: "check-in", occurrenceId, personId: person.id, expectedVersion: versions[person.id] || 0, capturedAt: new Date().toISOString(), guest: isGuest, confidence: 100 }]; });
  }

  async function sync() {
    if (!session || !queue.length || !online) return;
    setBusy("sync"); setError(null);
    try {
      const response = await api<{ results: CommandResult[] }>(`/api/chms/v1/checkin/sessions/${session.id}/commands:sync`, { method: "POST", body: { commands: queue } });
      setResults(response.results);
      const unresolved = new Set(response.results.filter((item) => item.classification === "retry").map((item) => item.clientCommandId));
      setQueue((items) => items.filter((item) => unresolved.has(item.clientCommandId)));
      setVersions((current) => { const next = { ...current }; response.results.forEach((item) => { const command = queue.find((queued) => queued.clientCommandId === item.clientCommandId); if (command && item.version) next[command.personId] = item.version; }); return next; });
    } catch (cause) { setError(cause instanceof ApiError ? cause.message : "Queue sync failed. Commands remain safely on this device."); }
    finally { setBusy(""); }
  }

  async function addGuest(event: FormEvent) {
    event.preventDefault(); if (!session || !online) return;
    setBusy("guest"); setError(null);
    try {
      const value = await api<{ person: { id: string; names: { given: string; family?: string } }; attendance: { version: number } }>(`/api/chms/v1/checkin/sessions/${session.id}/guests`, { method: "POST", body: { occurrenceId, names: { given: guest.given, family: guest.family }, contactPoints: guest.phone ? [{ type: "mobile", value: guest.phone, primary: true }] : [], capturedAt: new Date().toISOString() } });
      setVersions((current) => ({ ...current, [value.person.id]: value.attendance.version })); setGuest({ given: "", family: "", phone: "" });
      setResults((items) => [{ clientCommandId: value.person.id, localSequence: 0, classification: "applied", message: `${value.person.names.given} added and checked in.` }, ...items]);
    } catch (cause) { setError(cause instanceof ApiError ? cause.message : "Guest could not be added."); }
    finally { setBusy(""); }
  }

  async function lockStation() {
    if (!session || lockReason.trim().length < 3 || !online) return;
    setBusy("lock"); setError(null);
    try { const updated = await api<Session>(`/api/chms/v1/checkin/sessions/${session.id}/lock`, { method: "POST", body: { reason: lockReason } }); setSession(updated); }
    catch (cause) { setError(cause instanceof ApiError ? cause.message : "Station could not be locked."); }
    finally { setBusy(""); }
  }

  async function checkinChildSafely() {
    if (!session || !selectedChild || !selectedGuardian || !online) return;
    setBusy("child"); setError(null);
    try {
      await api("/api/chms/v1/guardian-authorizations", { method: "POST", body: { branchId, childPersonId: selectedChild.id, guardianPersonId: selectedGuardian.id, relationship: "guardian", source: "station-verification" } });
      const response = await api<{ label: ChildLabel }>(`/api/chms/v1/checkin/sessions/${session.id}/children`, { method: "POST", body: { occurrenceId, childPersonId: selectedChild.id, guardianPersonId: selectedGuardian.id, capturedAt: new Date().toISOString() } });
      setChildLabel(response.label); setPickup({ attendanceId: response.label.attendanceId, code: "" });
    } catch (cause) { setError(cause instanceof ApiError ? cause.message : "Child check-in could not be completed."); }
    finally { setBusy(""); }
  }

  async function releaseChild() {
    if (!selectedGuardian || !pickup.attendanceId || pickup.code.length < 6 || !online) return;
    setBusy("pickup"); setError(null);
    try {
      await api(`/api/chms/v1/checkin/${encodeURIComponent(pickup.attendanceId)}/pickup`, { method: "POST", body: { guardianPersonId: selectedGuardian.id, securityCode: pickup.code } });
      setResults((items) => [{ clientCommandId: pickup.attendanceId, localSequence: 0, classification: "applied", message: "Child released to the authorized guardian." }, ...items]); setPickup({ attendanceId: "", code: "" }); setChildLabel(null);
    } catch (cause) { setError(cause instanceof ApiError ? cause.message : "Pickup could not be authorized."); }
    finally { setBusy(""); }
  }

  async function reportIncident() {
    if (!childLabel || incident.summary.trim().length < 3 || !online) return;
    setBusy("incident"); setError(null);
    try { await api(`/api/chms/v1/child-checkins/${encodeURIComponent(childLabel.checkinId)}/incidents`, { method: "POST", body: incident }); setIncident((value) => ({ ...value, summary: "" })); }
    catch (cause) { setError(cause instanceof ApiError ? cause.message : "Incident could not be recorded."); }
    finally { setBusy(""); }
  }

  return <div>
    <PageHeader title="Check-in station" subtitle="Fast household arrival with a replay-safe offline queue and explicit operator control.">
      <span className={`rounded-full px-3 py-2 text-xs font-bold ${online ? "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300" : "bg-amber-500/10 text-amber-700 dark:text-amber-300"}`}>{online ? "● Online" : "○ Offline"}</span>
    </PageHeader>
    {error && <div className="mb-5"><ErrorBox message={error} /></div>}
    {loading ? <SkeletonTable rows={5} cols={3} /> : !session ? <Card className="member-panel max-w-3xl">
      <div className="member-panel-title"><div><h3>Authorize this station</h3><p>Scope one named device to one branch and one scheduled occurrence.</p></div></div>
      <div className="mt-5 grid gap-4 sm:grid-cols-2"><label className="person-field"><span>Branch</span><Select value={branchId} onChange={(event) => setBranchId(event.target.value)}><option value="">Choose branch</option>{branches.map((branch) => <option key={branch.id} value={branch.id}>{branch.name}</option>)}</Select></label><label className="person-field"><span>Service occurrence</span><Select value={occurrenceId} onChange={(event) => setOccurrenceId(event.target.value)}><option value="">Choose occurrence</option>{occurrences.map((item) => <option key={item.id} value={item.id}>{item.name} · {formatDate(item.startsAt)}</option>)}</Select></label><label className="person-field sm:col-span-2"><span>Device label</span><input value={deviceLabel} onChange={(event) => setDeviceLabel(event.target.value)} /></label></div>
      <button type="button" className="admin-primary-button mt-5 rounded-lg px-5 py-3 text-sm font-semibold" disabled={!online || !occurrenceId || busy === "start"} onClick={startSession}>{busy === "start" ? "Authorizing…" : "Start station"}</button>
    </Card> : <div className="space-y-5">
      <Card className="member-panel"><div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between"><div><p className="font-mono text-[10px] uppercase tracking-[.18em] text-[var(--remi-muted)]">{session.deviceLabel} · {session.state}</p><h2 className="mt-1 text-xl font-bold text-[var(--remi-ink)]">{occurrence?.name}</h2></div><div className="flex items-center gap-2"><span className="rounded-full bg-[var(--remi-surface-soft)] px-3 py-2 text-xs text-[var(--remi-muted)]">{queue.length} queued</span><button type="button" className="admin-primary-button rounded-lg px-4 py-2.5 text-sm font-semibold" disabled={!online || !queue.length || busy === "sync" || session.state !== "active"} onClick={sync}>{busy === "sync" ? "Syncing…" : "Sync now"}</button></div></div></Card>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.25fr)_minmax(19rem,.75fr)]"><Card className="member-panel"><div className="member-panel-title"><div><h3>Household arrival</h3><p>Search stays branch-scoped and returns only minimal check-in identity.</p></div></div><label className="person-field mt-4"><span>Find household or person</span><input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Name, phone or person number" disabled={session.state !== "active"} /></label><div className="mt-4 space-y-3">{households.map((household, index) => <div key={household.id || `${household.name}-${index}`} className="rounded-xl border border-[var(--remi-line)] p-3"><b className="text-sm text-[var(--remi-ink)]">{household.name || "Individual"}</b><div className="mt-2 space-y-2">{household.members.map((member) => <div key={member.id} className="flex flex-col gap-2 rounded-lg bg-[var(--remi-surface-soft)] p-2.5 sm:flex-row sm:items-center sm:justify-between"><span><strong className="block text-sm text-[var(--remi-ink)]">{member.name}</strong><small className="text-[var(--remi-muted)]">{member.role || "person"}</small></span><div className="flex flex-wrap gap-1.5"><button type="button" className="admin-secondary-action rounded-lg px-2.5 py-2 text-xs font-semibold" disabled={session.state !== "active"} onClick={() => setSelectedChild({ id: member.id, name: member.name })}>Child</button><button type="button" className="admin-secondary-action rounded-lg px-2.5 py-2 text-xs font-semibold" disabled={session.state !== "active"} onClick={() => setSelectedGuardian({ id: member.id, name: member.name })}>Guardian</button><button type="button" className="admin-secondary-action rounded-lg px-2.5 py-2 text-xs font-semibold" disabled={session.state !== "active"} onClick={() => enqueue(member)}>Queue check-in</button></div></div>)}</div></div>)}</div>{query.length >= 2 && !households.length && <EmptyState title="No household found" hint="Use quick guest intake only after checking spelling and contact details." />}</Card>
      <div className="space-y-5"><Card className="member-panel"><div className="member-panel-title"><div><h3>Quick guest</h3><p>Duplicate-safe intake; unavailable while offline.</p></div></div><form onSubmit={addGuest} className="mt-4 space-y-3"><label className="person-field"><span>First name</span><input required value={guest.given} onChange={(event) => setGuest((value) => ({ ...value, given: event.target.value }))} /></label><label className="person-field"><span>Family name</span><input value={guest.family} onChange={(event) => setGuest((value) => ({ ...value, family: event.target.value }))} /></label><label className="person-field"><span>Mobile</span><input type="tel" inputMode="tel" value={guest.phone} onChange={(event) => setGuest((value) => ({ ...value, phone: event.target.value }))} /></label><button className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold" disabled={!online || busy === "guest" || session.state !== "active"}>{busy === "guest" ? "Adding…" : "Add and check in"}</button></form></Card><Card className="member-panel"><div className="member-panel-title"><div><h3>Operator control</h3><p>Lock ends this device shift and rejects every later sync.</p></div></div><label className="person-field mt-4"><span>Lock reason</span><input value={lockReason} onChange={(event) => setLockReason(event.target.value)} placeholder="e.g. shift completed" /></label><button type="button" className="admin-secondary-action mt-3 w-full rounded-lg px-4 py-2.5 text-sm font-semibold" disabled={!online || lockReason.trim().length < 3 || busy === "lock" || session.state !== "active"} onClick={lockStation}>{session.state === "locked" ? "Station locked" : busy === "lock" ? "Locking…" : "Lock station"}</button></Card></div></div>
      {canSafeguard && <Card className="member-panel"><div className="member-panel-title"><div><h3>Child-safe check-in and pickup</h3><p>Restricted workflow. Codes appear once, are stored only as hashes, and every denied pickup is audited.</p></div></div><div className="mt-5 grid gap-5 lg:grid-cols-2"><section className="space-y-3"><div className="grid grid-cols-2 gap-3"><div className="rounded-xl border border-[var(--remi-line)] p-3"><small className="text-[var(--remi-muted)]">Child</small><b className="mt-1 block text-sm text-[var(--remi-ink)]">{selectedChild?.name || "Choose from household"}</b></div><div className="rounded-xl border border-[var(--remi-line)] p-3"><small className="text-[var(--remi-muted)]">Guardian</small><b className="mt-1 block text-sm text-[var(--remi-ink)]">{selectedGuardian?.name || "Choose from household"}</b></div></div><button type="button" className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold" disabled={!selectedChild || !selectedGuardian || !online || busy === "child" || session.state !== "active"} onClick={checkinChildSafely}>{busy === "child" ? "Securing check-in…" : "Authorize and check in child"}</button>{childLabel && <div className="rounded-2xl border border-[var(--remi-line)] bg-[var(--remi-surface-soft)] p-5 text-center print:border-black print:bg-white"><p className="font-mono text-[10px] uppercase tracking-[.2em] text-[var(--remi-muted)]">Pickup security label</p><h4 className="mt-2 text-lg font-bold text-[var(--remi-ink)]">{childLabel.childDisplayName}</h4><strong className="mt-4 block font-mono text-4xl tracking-[.18em] text-[var(--remi-ink)]">{childLabel.securityCode}</strong><p className="mt-3 break-all font-mono text-[10px] text-[var(--remi-muted)]">{childLabel.attendanceId}</p><button type="button" className="admin-secondary-action mt-4 rounded-lg px-4 py-2 text-xs font-semibold print:hidden" onClick={() => window.print()}>Print label</button></div>}</section><section className="space-y-3"><label className="person-field"><span>Scan or enter attendance label ID</span><input value={pickup.attendanceId} onChange={(event) => setPickup((value) => ({ ...value, attendanceId: event.target.value }))} autoComplete="off" /></label><label className="person-field"><span>Six-character security code</span><input value={pickup.code} onChange={(event) => setPickup((value) => ({ ...value, code: event.target.value.toUpperCase() }))} maxLength={6} autoComplete="off" className="font-mono uppercase tracking-[.18em]" /></label><p className="text-xs text-[var(--remi-muted)]">Releasing to: <b className="text-[var(--remi-ink)]">{selectedGuardian?.name || "choose guardian from household"}</b></p><button type="button" className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold" disabled={!selectedGuardian || pickup.code.length < 6 || !pickup.attendanceId || !online || busy === "pickup"} onClick={releaseChild}>{busy === "pickup" ? "Verifying…" : "Verify and release child"}</button><div className="border-t border-[var(--remi-line)] pt-4"><label className="person-field"><span>Safeguarding incident</span><Select value={incident.category} onChange={(event) => setIncident((value) => ({ ...value, category: event.target.value }))}><option value="identity concern">Identity concern</option><option value="pickup dispute">Pickup dispute</option><option value="missing label">Missing label</option><option value="medical concern">Medical concern</option><option value="other">Other</option></Select></label><label className="person-field mt-3"><span>Restricted summary</span><textarea value={incident.summary} onChange={(event) => setIncident((value) => ({ ...value, summary: event.target.value }))} rows={3} /></label><button type="button" className="admin-secondary-action mt-3 rounded-lg px-4 py-2.5 text-sm font-semibold" disabled={!childLabel || incident.summary.trim().length < 3 || busy === "incident"} onClick={reportIncident}>{busy === "incident" ? "Recording…" : "Record incident"}</button></div></section></div></Card>}
      {!!results.length && <Card className="member-panel"><div className="member-panel-title"><div><h3>Latest sync receipt</h3><p>Only retry-classified commands remain in the device queue.</p></div></div><div className="member-row-list">{results.map((result) => <div key={`${result.clientCommandId}-${result.classification}`}><span className="member-row-icon">{result.classification === "applied" || result.classification === "duplicate" ? "✓" : result.classification === "retry" ? "↻" : "!"}</span><span><b>{result.classification}</b><small>{result.message || result.code || "Attendance command processed."}</small></span><em>#{result.localSequence || "guest"}</em></div>)}</div></Card>}
    </div>}
  </div>;
}

function formatDate(value: string) { return new Intl.DateTimeFormat(undefined, { weekday: "short", hour: "numeric", minute: "2-digit" }).format(new Date(value)); }
