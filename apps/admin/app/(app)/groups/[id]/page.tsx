"use client";

import Link from "next/link";
import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { useParams } from "next/navigation";
import { ApiError, api } from "@/lib/api";
import {
  displayMemberName,
  loadMemberProfile,
  loadPeople,
  type PeoplePage,
} from "@/lib/chms";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import { DateTimePicker } from "@/components/ui/TemporalPicker";
import { SkeletonTable } from "@/components/ui/Skeleton";

interface Group {
  id: string;
  name: string;
  type: string;
  status: string;
  homeBranchId: string;
  capacity: number;
  activeMemberCount: number;
  privacy: string;
  discoverability: string;
  meetingPattern?: { weekday: string; localStart: string; location?: string };
}
interface Membership {
  id: string;
  version: number;
  personId: string;
  role: string;
  status: string;
  leaderNote?: string;
  directoryVisibility: "hidden" | "members";
  requestedAt?: string;
  invitedAt?: string;
  joinedAt?: string;
  waitlistedAt?: string;
}
interface Meeting {
  id: string;
  topic: string;
  startsAt: string;
  endsAt: string;
  location?: string;
  status: string;
}
interface Attendance {
  id: string;
  version: number;
  personId: string;
  status: string;
  confidence: number;
}
type Person = PeoplePage["items"][number];

export default function GroupOperationsPage() {
  const { id } = useParams<{ id: string }>();
  const [group, setGroup] = useState<Group | null>(null);
  const [roster, setRoster] = useState<Membership[]>([]);
  const [meetings, setMeetings] = useState<Meeting[]>([]);
  const [names, setNames] = useState<Record<string, string>>({});
  const [selectedMeeting, setSelectedMeeting] = useState<Meeting | null>(null);
  const [attendance, setAttendance] = useState<Attendance[]>([]);
  const [attendanceReason, setAttendanceReason] = useState("");
  const [query, setQuery] = useState("");
  const [candidates, setCandidates] = useState<Person[]>([]);
  const [mode, setMode] = useState("add");
  const [role, setRole] = useState("member");
  const [directoryVisibility, setDirectoryVisibility] = useState<
    "hidden" | "members"
  >("hidden");
  const [note, setNote] = useState("");
  const [pending, setPending] = useState<Membership | null>(null);
  const [reason, setReason] = useState("");
  const [meeting, setMeeting] = useState({
    topic: "",
    startsAt: "",
    duration: "90",
    timezone: "Africa/Accra",
    location: "",
  });
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<string | null>(null);
  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    const from = new Date();
    from.setFullYear(from.getFullYear() - 1);
    const to = new Date();
    to.setFullYear(to.getFullYear() + 1);
    try {
      const [groupValue, rosterValue, meetingValue] = await Promise.all([
        api<Group>(`/api/chms/v1/groups/${encodeURIComponent(id)}`),
        api<{ items: Membership[] }>(
          `/api/chms/v1/groups/${encodeURIComponent(id)}/members?includeInactive=true`,
        ),
        api<{ items: Meeting[] }>(
          `/api/chms/v1/groups/${encodeURIComponent(id)}/meetings?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`,
        ),
      ]);
      setGroup(groupValue);
      setRoster(rosterValue.items || []);
      setMeetings(meetingValue.items || []);
      const ids = [
        ...new Set((rosterValue.items || []).map((item) => item.personId)),
      ];
      void Promise.all(ids.map((personId) => loadMemberProfile(personId)))
        .then((values) =>
          setNames(
            Object.fromEntries(
              values.map((value) => [
                value.person.id,
                displayMemberName(value.person.names),
              ]),
            ),
          ),
        )
        .catch(() => {});
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Group operations could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [id]);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (query.trim().length < 2 || !group) {
      setCandidates([]);
      return;
    }
    const handle = window.setTimeout(
      () =>
        loadPeople(query, group.homeBranchId)
          .then((value) => setCandidates(value.items || []))
          .catch(() => setCandidates([])),
      250,
    );
    return () => window.clearTimeout(handle);
  }, [group, query]);
  async function add(person: Person) {
    setBusy(`add-${person.id}`);
    setError(null);
    try {
      await api(`/api/chms/v1/groups/${encodeURIComponent(id)}/members`, {
        method: "POST",
        body: {
          personId: person.id,
          mode,
          role,
          source: "staff",
          leaderNote: note,
          directoryVisibility,
        },
      });
      setQuery("");
      setCandidates([]);
      setNote("");
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Roster membership could not be created.",
      );
    } finally {
      setBusy("");
    }
  }
  async function transition(
    member: Membership,
    status: string,
    transitionReason = "",
  ) {
    setBusy(member.id);
    setError(null);
    try {
      await api(
        `/api/chms/v1/groups/${encodeURIComponent(id)}/members/${encodeURIComponent(member.personId)}`,
        {
          method: "PATCH",
          body: {
            expectedVersion: member.version,
            status,
            role: member.role,
            leaderNote: member.leaderNote || "",
            directoryVisibility: member.directoryVisibility || "hidden",
            reason: transitionReason,
          },
        },
      );
      setPending(null);
      setReason("");
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Membership could not be updated.",
      );
    } finally {
      setBusy("");
    }
  }
  async function schedule(event: FormEvent) {
    event.preventDefault();
    const startsAt = new Date(meeting.startsAt);
    const endsAt = new Date(
      startsAt.getTime() + Number(meeting.duration) * 60000,
    );
    setBusy("meeting");
    setError(null);
    try {
      await api(`/api/chms/v1/groups/${encodeURIComponent(id)}/meetings`, {
        method: "POST",
        body: {
          topic: meeting.topic,
          startsAt: startsAt.toISOString(),
          endsAt: endsAt.toISOString(),
          timezone: meeting.timezone,
          location: meeting.location,
        },
      });
      setMeeting((value) => ({
        ...value,
        topic: "",
        startsAt: "",
        location: "",
      }));
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Meeting could not be scheduled.",
      );
    } finally {
      setBusy("");
    }
  }
  async function openMeeting(value: Meeting) {
    setSelectedMeeting(value);
    setAttendanceReason("");
    setBusy("attendance-load");
    try {
      const result = await api<{ items: Attendance[] }>(
        `/api/chms/v1/groups/${encodeURIComponent(id)}/meetings/${encodeURIComponent(value.id)}/attendance`,
      );
      setAttendance(result.items || []);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Meeting attendance could not be loaded.",
      );
    } finally {
      setBusy("");
    }
  }
  const facts = useMemo(
    () => new Map(attendance.map((item) => [item.personId, item])),
    [attendance],
  );
  async function mark(member: Membership, status: string) {
    if (!selectedMeeting) return;
    const isLate = new Date() > new Date(selectedMeeting.endsAt);
    if (isLate && attendanceReason.trim().length < 3) {
      setError(
        "Enter a specific correction reason before changing attendance after the meeting.",
      );
      return;
    }
    const current = facts.get(member.personId);
    setBusy(`mark-${member.personId}`);
    try {
      const value = await api<Attendance>(
        `/api/chms/v1/groups/${encodeURIComponent(id)}/meetings/${encodeURIComponent(selectedMeeting.id)}/attendance/${encodeURIComponent(member.personId)}`,
        {
          method: "PUT",
          body: {
            expectedVersion: current?.version || 0,
            status,
            confidence: 100,
            reason: isLate ? attendanceReason.trim() : "",
          },
        },
      );
      setAttendance((items) => [
        ...items.filter((item) => item.personId !== member.personId),
        value,
      ]);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Attendance could not be recorded.",
      );
    } finally {
      setBusy("");
    }
  }
  if (loading) return <SkeletonTable rows={7} cols={4} />;
  if (!group)
    return <ErrorBox message={error || "Group not found."} onRetry={load} />;
  const active = roster.filter((item) => item.status === "active");
  return (
    <div>
      <PageHeader
        title={group.name}
        subtitle={`${group.type.replaceAll("-", " ")} · ${group.privacy} · ${group.discoverability}`}
      >
        <Link
          href="/groups"
          className="admin-secondary-action rounded-lg px-4 py-2.5 text-sm font-semibold"
        >
          Back to groups
        </Link>
        <span className="member-tag">
          {group.activeMemberCount ?? 0}/{group.capacity || "∞"} active
        </span>
      </PageHeader>
      {error && (
        <div className="mb-5">
          <ErrorBox message={error} onRetry={load} />
        </div>
      )}
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.15fr)_minmax(19rem,.85fr)]">
        <Card className="member-panel">
          <div className="member-panel-title">
            <div>
              <h3>Roster & joining queue</h3>
              <p>
                Applications, invitations, waitlists and active seats keep their
                own lifecycle.
              </p>
            </div>
          </div>
          <div className="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-[1fr_9rem_9rem_11rem]">
            <label className="person-field">
              <span>Find a person</span>
              <input
                type="search"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search name or contact"
              />
            </label>
            <label className="person-field">
              <span>Entry mode</span>
              <Select
                value={mode}
                onChange={(event) => setMode(event.target.value)}
              >
                <option value="add">Add now</option>
                <option value="invite">Invite</option>
                <option value="apply">Application</option>
              </Select>
            </label>
            <label className="person-field">
              <span>Roster role</span>
              <Select
                value={role}
                onChange={(event) => setRole(event.target.value)}
              >
                <option value="member">Member</option>
                <option value="participant">Participant</option>
                <option value="facilitator">Facilitator</option>
              </Select>
            </label>
            <label className="person-field">
              <span>Member directory</span>
              <Select
                value={directoryVisibility}
                onChange={(event) =>
                  setDirectoryVisibility(
                    event.target.value as "hidden" | "members",
                  )
                }
              >
                <option value="hidden">Keep private</option>
                <option value="members">Show to members</option>
              </Select>
            </label>
          </div>
          <label className="person-field mt-3">
            <span>
              Leader note <small>(staff-only operational context)</small>
            </span>
            <input
              maxLength={500}
              value={note}
              onChange={(event) => setNote(event.target.value)}
              placeholder="Optional; do not place pastoral or safeguarding detail here"
            />
          </label>
          {candidates.length > 0 && (
            <div className="mt-2 rounded-xl border border-[var(--remi-line)] p-2">
              {candidates
                .filter(
                  (person) =>
                    !roster.some(
                      (item) =>
                        item.personId === person.id &&
                        !["ended", "declined"].includes(item.status),
                    ),
                )
                .slice(0, 6)
                .map((person) => (
                  <button
                    type="button"
                    key={person.id}
                    disabled={busy === `add-${person.id}`}
                    onClick={() => add(person)}
                    className="block w-full rounded-lg px-3 py-2 text-left text-sm text-[var(--remi-ink)] hover:bg-[var(--remi-surface-soft)]"
                  >
                    {displayMemberName(person.names)}{" "}
                    <small className="text-[var(--remi-muted)]">
                      · {person.membershipStage.replaceAll("-", " ")}
                    </small>
                  </button>
                ))}
            </div>
          )}
          <div className="member-row-list mt-5">
            {roster.map((member) => (
              <div key={member.id}>
                <span className="member-row-icon">
                  {member.status === "active"
                    ? "✓"
                    : member.status === "waitlisted"
                      ? "…"
                      : member.status === "invited"
                        ? "→"
                        : "○"}
                </span>
                <span>
                  <b>
                    {names[member.personId] ||
                      `Person ${member.personId.slice(-5)}`}
                  </b>
                  <small>
                    {member.role} · {member.status}
                    {member.leaderNote ? ` · ${member.leaderNote}` : ""}
                  </small>
                </span>
                <div className="flex flex-wrap justify-end gap-1.5">
                  {["applied", "invited", "waitlisted"].includes(
                    member.status,
                  ) && (
                    <button
                      type="button"
                      disabled={busy === member.id}
                      onClick={() => transition(member, "active")}
                      className="admin-primary-button rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
                    >
                      Activate
                    </button>
                  )}
                  {member.status === "applied" && (
                    <button
                      type="button"
                      onClick={() => {
                        setPending(member);
                        setReason("");
                      }}
                      className="admin-secondary-action rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
                    >
                      Decline
                    </button>
                  )}
                  {member.status === "active" && (
                    <button
                      type="button"
                      onClick={() => {
                        setPending(member);
                        setReason("");
                      }}
                      className="admin-secondary-action rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
                    >
                      End
                    </button>
                  )}
                </div>
              </div>
            ))}
          </div>
          {!roster.length && (
            <EmptyState
              title="No roster entries"
              hint="Find a person to add, invite or place into the application queue."
            />
          )}
          {pending && (
            <div className="mt-4 rounded-2xl border border-[var(--remi-line)] bg-[var(--remi-surface-soft)] p-4">
              <b className="text-sm text-[var(--remi-ink)]">
                {pending.status === "active"
                  ? "End membership"
                  : "Decline application"}
              </b>
              <label className="person-field mt-3">
                <span>Required reason</span>
                <input
                  autoFocus
                  minLength={3}
                  value={reason}
                  onChange={(event) => setReason(event.target.value)}
                />
              </label>
              <div className="mt-3 flex gap-2">
                <button
                  type="button"
                  disabled={reason.trim().length < 3}
                  onClick={() =>
                    transition(
                      pending,
                      pending.status === "active" ? "ended" : "declined",
                      reason,
                    )
                  }
                  className="admin-primary-button rounded-lg px-3 py-2 text-xs font-semibold"
                >
                  Confirm
                </button>
                <button
                  type="button"
                  onClick={() => setPending(null)}
                  className="admin-secondary-action rounded-lg px-3 py-2 text-xs font-semibold"
                >
                  Cancel
                </button>
              </div>
            </div>
          )}
        </Card>
        <Card className="member-panel">
          <div className="member-panel-title">
            <div>
              <h3>Meeting rhythm</h3>
              <p>
                Schedule dated gatherings and reconcile only active roster
                members.
              </p>
            </div>
          </div>
          <form onSubmit={schedule} className="mt-4 space-y-3">
            <label className="person-field">
              <span>Topic</span>
              <input
                required
                minLength={2}
                value={meeting.topic}
                onChange={(event) =>
                  setMeeting((value) => ({
                    ...value,
                    topic: event.target.value,
                  }))
                }
              />
            </label>
            <label className="person-field">
              <span>Starts</span>
              <DateTimePicker
                required
                value={meeting.startsAt}
                onChange={(startsAt) =>
                  setMeeting((value) => ({
                    ...value,
                    startsAt,
                  }))
                }
              />
            </label>
            <div className="grid grid-cols-2 gap-3">
              <label className="person-field">
                <span>Duration</span>
                <Select
                  value={meeting.duration}
                  onChange={(event) =>
                    setMeeting((value) => ({
                      ...value,
                      duration: event.target.value,
                    }))
                  }
                >
                  <option value="60">1 hour</option>
                  <option value="90">1½ hours</option>
                  <option value="120">2 hours</option>
                </Select>
              </label>
              <label className="person-field">
                <span>Timezone</span>
                <Select
                  value={meeting.timezone}
                  onChange={(event) =>
                    setMeeting((value) => ({
                      ...value,
                      timezone: event.target.value,
                    }))
                  }
                >
                  <option value="Africa/Accra">Africa/Accra</option>
                  <option value="Africa/Lagos">Africa/Lagos</option>
                  <option value="Europe/London">Europe/London</option>
                </Select>
              </label>
            </div>
            <label className="person-field">
              <span>Location</span>
              <input
                value={meeting.location}
                onChange={(event) =>
                  setMeeting((value) => ({
                    ...value,
                    location: event.target.value,
                  }))
                }
              />
            </label>
            <button
              disabled={busy === "meeting" || group.status !== "active"}
              className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold"
            >
              {busy === "meeting" ? "Scheduling…" : "Schedule meeting"}
            </button>
          </form>
          <div className="member-row-list mt-5">
            {meetings.map((value) => (
              <button
                type="button"
                key={value.id}
                onClick={() => openMeeting(value)}
                className="w-full text-left"
              >
                <span className="member-row-icon">□</span>
                <span>
                  <b>{value.topic}</b>
                  <small>
                    {formatDate(value.startsAt)} ·{" "}
                    {value.location || "Location pending"}
                  </small>
                </span>
                <em>Open</em>
              </button>
            ))}
          </div>
        </Card>
      </div>
      {selectedMeeting && (
        <Card className="member-panel mt-5">
          <div className="member-panel-title">
            <div>
              <h3>{selectedMeeting.topic} attendance</h3>
              <p>
                {formatDate(selectedMeeting.startsAt)} · corrections after the
                meeting carry a reason.
              </p>
            </div>
            <button
              type="button"
              className="admin-secondary-action rounded-lg px-3 py-2 text-xs font-semibold"
              onClick={() => setSelectedMeeting(null)}
            >
              Close desk
            </button>
          </div>
          {new Date() > new Date(selectedMeeting.endsAt) && (
            <label className="person-field mt-4 max-w-2xl">
              <span>Correction reason</span>
              <input
                required
                minLength={3}
                maxLength={500}
                value={attendanceReason}
                onChange={(event) => setAttendanceReason(event.target.value)}
                placeholder="Explain why this attendance record is changing"
              />
              <small>
                This reason is attached to every change made after the meeting.
              </small>
            </label>
          )}
          <div className="member-row-list mt-4">
            {active.map((member) => {
              const fact = facts.get(member.personId);
              return (
                <div key={member.id}>
                  <span className="member-row-icon">
                    {fact?.status === "present"
                      ? "✓"
                      : fact?.status === "excused"
                        ? "–"
                        : "○"}
                  </span>
                  <span>
                    <b>
                      {names[member.personId] ||
                        `Person ${member.personId.slice(-5)}`}
                    </b>
                    <small>
                      {fact
                        ? `${fact.status} · ${fact.confidence}% confidence`
                        : "Not marked"}
                    </small>
                  </span>
                  <div className="flex gap-1.5">
                    {["present", "absent", "excused"].map((status) => (
                      <button
                        type="button"
                        key={status}
                        disabled={busy === `mark-${member.personId}`}
                        onClick={() => mark(member, status)}
                        className={
                          fact?.status === status
                            ? "admin-primary-button rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
                            : "admin-secondary-action rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
                        }
                      >
                        {status}
                      </button>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>
          {!active.length && (
            <EmptyState
              title="No active roster members"
              hint="Activate a membership before recording meeting attendance."
            />
          )}
        </Card>
      )}
    </div>
  );
}
function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    weekday: "short",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
}
