"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { BrandedSelect } from "@/components/branded-select";

type Team = { id: string; name: string };
type Assignment = { id: string; status: string; startsAt: string; endsAt: string; version: number; reminder: { sentCount: number; lastSentAt?: string } };
type ScheduleItem = { name: string; positionName: string; assignment: Assignment };
type Detail = { team: Team; assignments: ScheduleItem[] };

async function request(path: string, method = "GET", body?: unknown) {
  const response = await fetch(path, { method, headers: body ? { "Content-Type": "application/json" } : undefined, body: body ? JSON.stringify(body) : undefined, cache: "no-store" });
  const data = await response.json();
  if (!response.ok) throw new Error(data.message || data.error?.message || data.error || "The request could not be completed.");
  return data;
}

export function LeaderTeamSchedule({ teams }: { teams: Team[] }) {
  const [teamId, setTeamId] = useState(teams[0]?.id || "");
  const [detail, setDetail] = useState<Detail | null>(null);
  const [reason, setReason] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState(""); const [error, setError] = useState(""); const [notice, setNotice] = useState("");
  const range = useMemo(() => { const from = new Date(); from.setDate(from.getDate() - 45); const to = new Date(); to.setFullYear(to.getFullYear() + 1); return `from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`; }, []);
  const load = useCallback(async () => { if (!teamId) return; setError(""); try { setDetail(await request(`/api/member/leader/leader-workspace/teams/${teamId}?${range}`)); } catch (cause) { setError(cause instanceof Error ? cause.message : "Team schedule could not be loaded."); } }, [range, teamId]);
  useEffect(() => { void load(); }, [load]);
  async function remind(item: ScheduleItem) { setBusy(item.assignment.id); setError(""); setNotice(""); try { await request(`/api/member/leader/assignments/${item.assignment.id}/reminders`, "POST", { expectedVersion: item.assignment.version, nextDueAt: null }); setNotice(`Reminder handoff recorded for ${item.name}.`); await load(); } catch (cause) { setError(cause instanceof Error ? cause.message : "Reminder could not be recorded."); } finally { setBusy(""); } }
  async function noShow(item: ScheduleItem) { const note = (reason[item.assignment.id] || "").trim(); if (note.length < 3) { setError("Add a short operational reason before recording a no-show."); return; } setBusy(item.assignment.id); setError(""); setNotice(""); try { await request(`/api/member/leader/assignments/${item.assignment.id}/no-show`, "POST", { expectedVersion: item.assignment.version, reason: note }); setNotice(`${item.name} was recorded as a no-show.`); setReason(values => ({ ...values, [item.assignment.id]: "" })); await load(); } catch (cause) { setError(cause instanceof Error ? cause.message : "No-show could not be recorded."); } finally { setBusy(""); } }
  if (!teams.length) return null;
  return <section className="member-leader-team-schedule"><header><div><span>05 · SERVING CARE</span><h2>Team schedule</h2></div><p>Coordinate only teams entrusted to you. Contact details and safeguarding records stay outside this workspace.</p></header>{error && <div role="alert" className="member-leader-notice error">{error}</div>}{notice && <div role="status" className="member-leader-notice">{notice}</div>}<label className="member-leader-team-picker"><span>Serving team</span><BrandedSelect value={teamId} onChange={event => setTeamId(event.target.value)}>{teams.map(team => <option value={team.id} key={team.id}>{team.name}</option>)}</BrandedSelect></label><div className="member-leader-team-list">{detail?.assignments.map(item => { const ended = new Date(item.assignment.endsAt).getTime() < Date.now(); const canNoShow = ended && item.assignment.status === "accepted"; return <article key={item.assignment.id}><time><b>{new Date(item.assignment.startsAt).toLocaleDateString("en-GH", { day: "2-digit" })}</b><span>{new Date(item.assignment.startsAt).toLocaleDateString("en-GH", { month: "short" })}</span></time><div><span>{item.positionName || "Serving position"}</span><h3>{item.name}</h3><p>{new Date(item.assignment.startsAt).toLocaleString("en-GH", { weekday: "short", hour: "numeric", minute: "2-digit" })} · <em>{item.assignment.status}</em></p></div><aside><button disabled={busy !== ""} onClick={() => void remind(item)}>Record reminder</button>{item.assignment.reminder.sentCount > 0 && <small>{item.assignment.reminder.sentCount} reminder{item.assignment.reminder.sentCount === 1 ? "" : "s"} logged</small>}</aside>{canNoShow && <footer><label><span>No-show reason</span><input value={reason[item.assignment.id] || ""} onChange={event => setReason(values => ({ ...values, [item.assignment.id]: event.target.value }))} placeholder="Brief operational note" /></label><button disabled={busy !== ""} onClick={() => void noShow(item)}>Mark no-show</button></footer>}</article>; })}{detail && !detail.assignments.length && <div className="member-leader-team-empty"><strong>No assignments in this range</strong><p>This owned team has no serving assignments from the last 45 days through the next year.</p></div>}</div></section>;
}
