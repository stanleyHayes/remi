"use client";

import { useState } from "react";
import { Icon } from "@/components/icon";

export interface MemberServingAssignment {
  id: string;
  version: number;
  startsAt: string;
  status: string;
  teamName?: string;
  positionName?: string;
}

export function ServingRequest({ assignment: initialAssignment }: { assignment?: MemberServingAssignment | null }) {
  const [assignment, setAssignment] = useState<MemberServingAssignment | null>(initialAssignment || null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");

  async function respond(response: "accepted" | "declined") {
    if (!assignment) return;
    setBusy(true);
    setMessage("");
    try {
      const result = await fetch(`/api/member/serving/${encodeURIComponent(assignment.id)}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ expectedVersion: assignment.version, response, reasonCode: response === "declined" ? "unavailable" : "" }),
      });
      const data = await result.json();
      const apiMessage = typeof data?.error === "string" ? data.error : data?.error?.message;
      if (!result.ok) throw new Error(data.message || apiMessage || "Response could not be saved.");
      setAssignment(null);
      setMessage(response === "accepted" ? "You are confirmed. Thank you for serving." : "Your coordinator has been notified.");
    } catch (error) {
      setMessage(error instanceof Error ? error.message : "Response could not be saved.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <aside className="member-action-card" aria-live="polite">
      <span>{assignment ? "NEEDS YOUR RESPONSE" : "SERVING SCHEDULE"}</span>
      <div className="member-action-icon"><Icon name="heart" /></div>
      {assignment ? (
        <>
          <p>Serving request</p>
          <h2>{assignment.teamName || "Serving team"}</h2>
          <small>{assignment.positionName ? `${assignment.positionName} · ` : ""}{new Intl.DateTimeFormat("en-GH", { weekday: "long", day: "numeric", month: "long", hour: "numeric", minute: "2-digit" }).format(new Date(assignment.startsAt))}</small>
          <div>
            <button disabled={busy} onClick={() => respond("accepted")}><Icon name="check" /> Accept</button>
            <button disabled={busy} onClick={() => respond("declined")}>Decline</button>
          </div>
        </>
      ) : (
        <><p>You are up to date</p><h2>No invitations waiting</h2><small>{message || "New serving requests will appear here."}</small></>
      )}
      {assignment && message && <small className="member-serving-message">{message}</small>}
      <a href="/serving">View schedule</a>
    </aside>
  );
}
