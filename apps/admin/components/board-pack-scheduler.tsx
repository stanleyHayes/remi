"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { api, apiDownload, ApiError, asList, getStoredUser } from "@/lib/api";
import { DateTimePicker } from "@/components/ui/TemporalPicker";
import { Select } from "@/components/ui/Select";
import { useToast } from "@/components/ui/Toast";

type View = { id: string; name: string; version: number; query: { metricIds: string[] } };
type Staff = { id: string; name?: string; email: string; role?: string; invitationStatus?: string };
type Schedule = { id: string; name: string; savedViewId: string; savedViewVersion: number; cadence: string; format: string; recipientUserIds: string[]; state: string; nextRunAt: string; lastRunAt?: string; lastErrorCode?: string };
type Delivery = { id: string; recipientHint: string; state: string; expiresAt: string; openedAt?: string };
type DeliveryRun = { fileName: string; artifactHash: string; dictionaryVersion: string; rowCount: number; generatedAt: string };

function localDate(value: Date) {
  const offset = value.getTimezoneOffset() * 60_000;
  return new Date(value.getTime() - offset).toISOString().slice(0, 16);
}

export default function BoardPackScheduler() {
  const { showToast } = useToast();
  const [views, setViews] = useState<View[]>([]);
  const [staff, setStaff] = useState<Staff[]>([]);
  const [schedules, setSchedules] = useState<Schedule[]>([]);
  const [staffRestricted, setStaffRestricted] = useState(false);
  const [name, setName] = useState("");
  const [viewId, setViewId] = useState("");
  const [cadence, setCadence] = useState("monthly");
  const [format, setFormat] = useState("board-pack");
  const [recipients, setRecipients] = useState<string[]>([]);
  const [nextRunAt, setNextRunAt] = useState(() => { const date = new Date(); date.setDate(date.getDate() + 1); date.setHours(8, 0, 0, 0); return localDate(date); });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [delivery, setDelivery] = useState<{ delivery: Delivery; run: DeliveryRun } | null>(null);
  const deliveryId = useMemo(() => typeof window === "undefined" ? "" : new URLSearchParams(window.location.search).get("delivery") || "", []);

  const load = useCallback(async () => {
    setError(null);
    const [viewData, scheduleData] = await Promise.all([
      api("/api/chms/v1/reporting/saved-views"),
      api("/api/chms/v1/reporting/schedules"),
    ]);
    const nextViews = asList<View>(viewData);
    setViews(nextViews);
    setViewId(current => current || nextViews[0]?.id || "");
    setSchedules(asList<Schedule>(scheduleData));
    try {
      const users = asList<Staff>(await api("/api/admin/users")).filter(user => user.invitationStatus === "accepted" || user.invitationStatus === "active");
      setStaff(users);
      const currentID = getStoredUser()?.id;
      setRecipients(current => current.length ? current : currentID && users.some(user => user.id === currentID) ? [currentID] : []);
      setStaffRestricted(false);
    } catch (cause) {
      if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403)) setStaffRestricted(true);
      else throw cause;
    }
  }, []);

  useEffect(() => { load().catch(cause => setError(cause instanceof ApiError ? cause.message : "Scheduled reports could not be loaded.")); }, [load]);
  useEffect(() => {
    if (!deliveryId) return;
    api<{ delivery: Delivery; run: DeliveryRun }>(`/api/chms/v1/reporting/deliveries/${encodeURIComponent(deliveryId)}`)
      .then(setDelivery)
      .catch(cause => setError(cause instanceof ApiError ? cause.message : "This report delivery is not available."));
  }, [deliveryId]);

  function toggleRecipient(id: string) { setRecipients(current => current.includes(id) ? current.filter(item => item !== id) : [...current, id]); }
  async function createSchedule() {
    const view = views.find(item => item.id === viewId);
    if (!view) return;
    setBusy(true); setError(null);
    try {
      await api("/api/chms/v1/reporting/schedules", { method: "POST", body: { savedViewId: view.id, savedViewVersion: view.version, name, cadence, format, recipientUserIds: recipients, nextRunAt: new Date(nextRunAt).toISOString() } });
      setName("");
      await load();
      showToast("Governed report schedule created.");
    } catch (cause) { setError(cause instanceof ApiError ? cause.message : "Schedule could not be created."); }
    finally { setBusy(false); }
  }
  async function downloadDelivery() {
    if (!deliveryId || !delivery) return;
    try { await apiDownload(`/api/chms/v1/reporting/deliveries/${encodeURIComponent(deliveryId)}/download`, delivery.run.fileName); }
    catch (cause) { showToast(cause instanceof ApiError ? cause.message : "Report could not be downloaded.", "error"); }
  }

  return <section className="board-pack-studio" aria-labelledby="board-pack-title">
    <header><div><span>Governed distribution</span><h2 id="board-pack-title">Board pack scheduler</h2><p>Turn a saved report into a recurring, permission-checked briefing. Every delivery expires after seven days.</p></div><b>{String(schedules.filter(item => item.state === "active").length).padStart(2, "0")} active</b></header>
    {delivery && <section className="board-pack-delivery"><div><span>Secure delivery</span><h3>{delivery.run.fileName}</h3><p>{delivery.run.rowCount} rows · {delivery.run.dictionaryVersion} · expires {new Date(delivery.delivery.expiresAt).toLocaleDateString("en-GH")}</p></div><code>{delivery.run.artifactHash.slice(0, 12)}</code><button type="button" className="admin-primary-button" onClick={() => void downloadDelivery()}>Download pack</button></section>}
    {error && <p className="board-pack-error" role="alert">{error}</p>}
    <div className="board-pack-layout"><div className="board-pack-form"><label><span>Schedule name</span><input value={name} onChange={event => setName(event.target.value)} placeholder="Monthly leadership pulse" /></label><label><span>Saved report</span><Select value={viewId} onChange={event => setViewId(event.target.value)}><option value="" disabled>Choose a saved view</option>{views.map(view => <option value={view.id} key={view.id}>{view.name} · v{view.version}</option>)}</Select></label><div className="board-pack-pair"><label><span>Cadence</span><Select value={cadence} onChange={event => setCadence(event.target.value)}><option value="weekly">Weekly</option><option value="monthly">Monthly</option></Select></label><label><span>Pack format</span><Select value={format} onChange={event => setFormat(event.target.value)}><option value="board-pack">ZIP board pack</option><option value="csv">CSV snapshot</option></Select></label></div><label><span>First delivery</span><DateTimePicker value={nextRunAt} onChange={setNextRunAt} required min={localDate(new Date())} /></label><fieldset><legend>Approved recipients <small>{recipients.length} selected</small></legend>{staffRestricted ? <p className="board-pack-restricted">Recipient management is reserved for super administrators. Existing schedules remain visible.</p> : staff.length ? <div className="board-pack-recipients">{staff.map(person => <button type="button" key={person.id} className={recipients.includes(person.id) ? "is-selected" : ""} aria-pressed={recipients.includes(person.id)} onClick={() => toggleRecipient(person.id)}><i aria-hidden="true">{recipients.includes(person.id) ? "✓" : "+"}</i><span><b>{person.name || person.email}</b><small>{person.email} · {person.role?.replaceAll("-", " ")}</small></span></button>)}</div> : <p className="board-pack-restricted">No active staff recipients are available.</p>}</fieldset><button type="button" className="admin-primary-button" disabled={busy || !name.trim() || !viewId || recipients.length === 0} onClick={() => void createSchedule()}>{busy ? "Scheduling…" : "Schedule board pack"}</button></div><aside><span>Delivery rhythm</span><h3>Upcoming schedules</h3>{schedules.length ? <div>{schedules.map(schedule => <article key={schedule.id} className={`is-${schedule.state}`}><header><b>{schedule.name}</b><em>{schedule.state}</em></header><p>{schedule.cadence} · {schedule.format.replace("-", " ")} · {schedule.recipientUserIds.length} recipient{schedule.recipientUserIds.length === 1 ? "" : "s"}</p><small>Next {new Date(schedule.nextRunAt).toLocaleString("en-GH", { dateStyle: "medium", timeStyle: "short" })} · view v{schedule.savedViewVersion}</small>{schedule.lastErrorCode && <code>{schedule.lastErrorCode.replaceAll("_", " ")}</code>}</article>)}</div> : <p>No recurring board packs yet. Save a report view first, then compose its delivery rhythm here.</p>}</aside></div>
  </section>;
}
