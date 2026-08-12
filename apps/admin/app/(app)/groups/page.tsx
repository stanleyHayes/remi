"use client";

import { FormEvent, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { ApiError, api, asList } from "@/lib/api";
import {
  displayMemberName,
  loadMemberProfile,
  loadPeople,
  type PeoplePage,
} from "@/lib/chms";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import { TimePicker } from "@/components/ui/TemporalPicker";
import { SkeletonTable } from "@/components/ui/Skeleton";

interface Branch {
  id: string;
  name: string;
}
interface Ministry { id: string; name: string }
interface Pattern {
  frequency: string;
  weekday: string;
  localStart: string;
  durationMinutes: number;
  timezone: string;
  location: string;
}
interface Group {
  id: string;
  version: number;
  name: string;
  type: string;
  description?: string;
  homeBranchId: string;
  ministryId?: string;
  leaderPersonIds: string[];
  capacity: number;
  meetingPattern?: Pattern;
  privacy: string;
  discoverability: string;
  status: string;
  closureReason?: string;
}
type Person = PeoplePage["items"][number];
const EMPTY = {
  name: "",
  type: "community",
  ministryId: "",
  description: "",
  capacity: "",
  privacy: "request",
  discoverability: "members",
  status: "draft",
  reason: "",
  hasMeeting: true,
  frequency: "weekly",
  weekday: "friday",
  localStart: "18:30",
  duration: "90",
  timezone: "Africa/Accra",
  location: "",
};
const TYPES = [
  ["community", "Community group"],
  ["class", "Class"],
  ["ministry", "Ministry"],
  ["support", "Support group"],
  ["next-step", "Next step"],
  ["youth", "Youth"],
  ["children", "Children"],
  ["other", "Other"],
];
const DAYS = [
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
  "sunday",
];

export default function GroupsPage() {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [ministries, setMinistries] = useState<Ministry[]>([]);
  const [branchId, setBranchId] = useState("");
  const [groups, setGroups] = useState<Group[]>([]);
  const [selected, setSelected] = useState<Group | null>(null);
  const [form, setForm] = useState(EMPTY);
  const [leaders, setLeaders] = useState<Person[]>([]);
  const [candidates, setCandidates] = useState<Person[]>([]);
  const [leaderQuery, setLeaderQuery] = useState("");
  const [includeClosed, setIncludeClosed] = useState(false);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  useEffect(() => {
    Promise.all([api("/api/branches"), api("/api/ministries")])
      .then(([branchValue, ministryValue]) => {
        const items = asList<Branch>(branchValue);
        setBranches(items);
        setBranchId(items[0]?.id || "");
        setMinistries(asList<Ministry>(ministryValue));
      })
      .catch(() => { setBranches([]); setMinistries([]); });
  }, []);
  const load = useCallback(async () => {
    if (!branchId) {
      setGroups([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    try {
      const value = await api<{ items: Group[] }>(
        `/api/chms/v1/groups?branchId=${encodeURIComponent(branchId)}&includeClosed=${includeClosed}`,
      );
      setGroups(value.items || []);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Groups could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [branchId, includeClosed]);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (leaderQuery.trim().length < 2) {
      setCandidates([]);
      return;
    }
    const handle = window.setTimeout(
      () =>
        loadPeople(leaderQuery, branchId)
          .then((value) => setCandidates(value.items || []))
          .catch(() => setCandidates([])),
      250,
    );
    return () => window.clearTimeout(handle);
  }, [branchId, leaderQuery]);
  function choose(group: Group) {
    setSelected(group);
    setLeaders(
      group.leaderPersonIds.map((id) => ({
        id,
        names: { given: `Leader ${id.slice(-5)}` },
        personNumber: "",
        membershipStage: "member",
        homeBranchId: group.homeBranchId,
        version: 1,
      })),
    );
    void Promise.all(group.leaderPersonIds.map((id) => loadMemberProfile(id)))
      .then((profiles) =>
        setLeaders(
          profiles.map(({ person }) => ({
            id: person.id,
            version: person.version,
            personNumber: person.personNumber,
            names: person.names,
            photoUrl: person.photoUrl,
            homeBranchId: person.homeBranchId,
            membershipStage: person.membershipStage,
            tags: person.tags,
            updatedAt: person.updatedAt,
          })),
        ),
      )
      .catch(() => {});
    const pattern = group.meetingPattern;
    setForm({
      name: group.name,
      type: group.type,
      ministryId: group.ministryId || "",
      description: group.description || "",
      capacity: group.capacity ? String(group.capacity) : "",
      privacy: group.privacy,
      discoverability: group.discoverability,
      status: group.status,
      reason: "",
      hasMeeting: !!pattern,
      frequency: pattern?.frequency || "weekly",
      weekday: pattern?.weekday || "friday",
      localStart: pattern?.localStart || "18:30",
      duration: String(pattern?.durationMinutes || 90),
      timezone: pattern?.timezone || "Africa/Accra",
      location: pattern?.location || "",
    });
  }
  function reset() {
    setSelected(null);
    setForm(EMPTY);
    setLeaders([]);
    setLeaderQuery("");
    setCandidates([]);
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    if (!branchId) return;
    setBusy(true);
    setError(null);
    const body = {
      homeBranchId: branchId,
      name: form.name,
      type: form.type,
      ministryId: form.ministryId || undefined,
      description: form.description,
      leaderPersonIds: leaders.map((item) => item.id),
      capacity: Number(form.capacity || 0),
      privacy: form.privacy,
      discoverability: form.discoverability,
      status: form.status,
      reason: form.reason,
      meetingPattern: form.hasMeeting
        ? {
            frequency: form.frequency,
            weekday: form.weekday,
            localStart: form.localStart,
            durationMinutes: Number(form.duration),
            timezone: form.timezone,
            location: form.location,
          }
        : null,
      ...(selected ? { expectedVersion: selected.version } : {}),
    };
    try {
      const value = await api<Group>(
        selected ? `/api/chms/v1/groups/${selected.id}` : "/api/chms/v1/groups",
        { method: selected ? "PATCH" : "POST", body },
      );
      await load();
      choose(value);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The group could not be saved.",
      );
    } finally {
      setBusy(false);
    }
  }
  const needsReason =
    selected &&
    (form.status === "paused" || form.status === "closed") &&
    form.status !== selected.status;
  return (
    <div>
      <PageHeader
        title="Groups & ministries"
        subtitle="Shape communities, classes and ministry rosters with clear ownership, privacy and meeting rhythms."
      >
        <Link
          href="/groups/insights"
          className="admin-secondary-action rounded-lg px-4 py-2.5 text-sm font-semibold"
        >
          Ministry insights
        </Link>
        <Select
          aria-label="Branch"
          value={branchId}
          onChange={(event) => {
            setBranchId(event.target.value);
            reset();
          }}
        >
          <option value="">Choose branch</option>
          {branches.map((branch) => (
            <option key={branch.id} value={branch.id}>
              {branch.name}
            </option>
          ))}
        </Select>
        <button
          type="button"
          className="admin-primary-button rounded-lg px-4 py-2.5 text-sm font-semibold"
          onClick={reset}
        >
          New group
        </button>
      </PageHeader>
      {error && (
        <div className="mb-5">
          <ErrorBox message={error} onRetry={load} />
        </div>
      )}
      {loading ? (
        <SkeletonTable rows={6} cols={4} />
      ) : !branchId ? (
        <EmptyState
          title="Choose a branch"
          hint="Groups and their leader scope are branch-owned."
        />
      ) : (
        <div className="grid gap-5 xl:grid-cols-[minmax(18rem,.65fr)_minmax(0,1.35fr)]">
          <Card className="member-panel">
            <div className="member-panel-title">
              <div>
                <h3>Community directory</h3>
                <p>{groups.length} configured groups and ministries.</p>
              </div>
              <label className="flex items-center gap-2 text-xs text-[var(--remi-muted)]">
                <input
                  type="checkbox"
                  checked={includeClosed}
                  onChange={(event) => setIncludeClosed(event.target.checked)}
                />{" "}
                Closed
              </label>
            </div>
            {groups.length ? (
              <div className="member-row-list mt-4">
                {groups.map((group) => (
                  <button
                    type="button"
                    key={group.id}
                    onClick={() => choose(group)}
                    className={`w-full text-left ${selected?.id === group.id ? "is-active" : ""}`}
                  >
                    <span className="member-row-icon">
                      {group.status === "active"
                        ? "●"
                        : group.status === "paused"
                          ? "Ⅱ"
                          : "○"}
                    </span>
                    <span>
                      <b>{group.name}</b>
                      <small>
                        {ministries.find((item) => item.id === group.ministryId)?.name || "General ministry"} · {group.type.replaceAll("-", " ")} · {group.privacy} ·{" "}
                        {group.capacity
                          ? `${group.capacity} places`
                          : "open capacity"}
                      </small>
                    </span>
                    <em>{group.status}</em>
                  </button>
                ))}
              </div>
            ) : (
              <EmptyState
                title="No groups yet"
                hint="Create a community, class, ministry or next-step group."
              />
            )}
          </Card>
          <Card className="member-panel">
            <div className="member-panel-title">
              <div>
                <h3>{selected ? `Edit ${selected.name}` : "Create a group"}</h3>
                <p>
                  Public discoverability never exposes the internal roster or
                  leader contact data.
                </p>
              </div>
              {selected && (
                <div className="flex flex-wrap gap-2">
                  <Link
                    href={`/groups/${selected.id}`}
                    className="admin-primary-button rounded-lg px-3 py-2 text-xs font-semibold"
                  >
                    Manage roster
                  </Link>
                  <button
                    type="button"
                    className="admin-secondary-action rounded-lg px-3 py-2 text-xs font-semibold"
                    onClick={reset}
                  >
                    Cancel edit
                  </button>
                </div>
              )}
            </div>
            <form onSubmit={save} className="mt-5 grid gap-4 md:grid-cols-2">
              <label className="person-field">
                <span>Name</span>
                <input
                  required
                  minLength={2}
                  maxLength={150}
                  value={form.name}
                  onChange={(event) =>
                    setForm((value) => ({ ...value, name: event.target.value }))
                  }
                />
              </label>
              <label className="person-field">
                <span>Type</span>
                <Select
                  value={form.type}
                  onChange={(event) =>
                    setForm((value) => ({ ...value, type: event.target.value }))
                  }
                >
                  {TYPES.map(([value, label]) => (
                    <option key={value} value={value}>
                      {label}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="person-field md:col-span-2">
                <span>Owning ministry</span>
                <Select value={form.ministryId} onChange={(event) => setForm((value) => ({ ...value, ministryId: event.target.value }))}>
                  <option value="">General branch ministry</option>
                  {ministries.map((ministry) => <option key={ministry.id} value={ministry.id}>{ministry.name}</option>)}
                </Select>
                <small>Ministry-scoped staff can only see and manage groups assigned to one of their ministries.</small>
              </label>
              <label className="person-field md:col-span-2">
                <span>Description</span>
                <textarea
                  rows={3}
                  maxLength={2000}
                  value={form.description}
                  onChange={(event) =>
                    setForm((value) => ({
                      ...value,
                      description: event.target.value,
                    }))
                  }
                />
              </label>
              <label className="person-field">
                <span>Joining policy</span>
                <Select
                  value={form.privacy}
                  onChange={(event) =>
                    setForm((value) => ({
                      ...value,
                      privacy: event.target.value,
                      discoverability:
                        event.target.value === "invite-only" &&
                        value.discoverability === "public"
                          ? "staff"
                          : value.discoverability,
                    }))
                  }
                >
                  <option value="open">Open</option>
                  <option value="request">Request to join</option>
                  <option value="invite-only">Invite only</option>
                </Select>
              </label>
              <label className="person-field">
                <span>Discoverable to</span>
                <Select
                  value={form.discoverability}
                  onChange={(event) =>
                    setForm((value) => ({
                      ...value,
                      discoverability: event.target.value,
                    }))
                  }
                >
                  <option value="staff">Staff only</option>
                  <option value="members">Signed-in members</option>
                  {form.privacy !== "invite-only" && (
                    <option value="public">Public directory</option>
                  )}
                </Select>
              </label>
              <label className="person-field">
                <span>Capacity</span>
                <input
                  type="number"
                  min="0"
                  max="100000"
                  inputMode="numeric"
                  value={form.capacity}
                  onChange={(event) =>
                    setForm((value) => ({
                      ...value,
                      capacity: event.target.value,
                    }))
                  }
                  placeholder="Unlimited"
                />
              </label>
              <label className="person-field">
                <span>Status</span>
                <Select
                  value={form.status}
                  onChange={(event) =>
                    setForm((value) => ({
                      ...value,
                      status: event.target.value,
                    }))
                  }
                  disabled={selected?.status === "closed"}
                >
                  <option value="draft">Draft</option>
                  <option value="active">Active</option>
                  {selected?.status !== "draft" && (
                    <option value="paused">Paused</option>
                  )}
                  <option value="closed">Closed</option>
                </Select>
              </label>
              <div className="md:col-span-2">
                <label className="text-sm font-semibold text-[var(--remi-ink)]">
                  Leaders
                </label>
                <div className="mt-2 flex flex-wrap gap-2">
                  {leaders.map((leader) => (
                    <button
                      type="button"
                      key={leader.id}
                      className="member-tag"
                      onClick={() =>
                        setLeaders((items) =>
                          items.filter((item) => item.id !== leader.id),
                        )
                      }
                    >
                      {displayMemberName(leader.names)} ×
                    </button>
                  ))}
                </div>
                <label className="person-field mt-3">
                  <span>Find an active person</span>
                  <input
                    type="search"
                    value={leaderQuery}
                    onChange={(event) => setLeaderQuery(event.target.value)}
                    placeholder="Search name or contact"
                  />
                </label>
                {candidates.length > 0 && (
                  <div className="mt-2 rounded-xl border border-[var(--remi-line)] bg-[var(--remi-surface)] p-2">
                    {candidates
                      .filter(
                        (person) =>
                          !leaders.some((leader) => leader.id === person.id),
                      )
                      .slice(0, 6)
                      .map((person) => (
                        <button
                          type="button"
                          key={person.id}
                          className="block w-full rounded-lg px-3 py-2 text-left text-sm text-[var(--remi-ink)] hover:bg-[var(--remi-surface-soft)]"
                          onClick={() => {
                            setLeaders((items) => [...items, person]);
                            setLeaderQuery("");
                            setCandidates([]);
                          }}
                        >
                          {displayMemberName(person.names)}{" "}
                          <small className="text-[var(--remi-muted)]">
                            · {person.membershipStage.replaceAll("-", " ")}
                          </small>
                        </button>
                      ))}
                  </div>
                )}
              </div>
              <label className="md:col-span-2 flex items-center gap-3 rounded-xl border border-[var(--remi-line)] p-3 text-sm font-semibold text-[var(--remi-ink)]">
                <input
                  type="checkbox"
                  checked={form.hasMeeting}
                  onChange={(event) =>
                    setForm((value) => ({
                      ...value,
                      hasMeeting: event.target.checked,
                    }))
                  }
                />{" "}
                This group has a regular meeting rhythm
              </label>
              {form.hasMeeting && (
                <>
                  <label className="person-field">
                    <span>Frequency</span>
                    <Select
                      value={form.frequency}
                      onChange={(event) =>
                        setForm((value) => ({
                          ...value,
                          frequency: event.target.value,
                        }))
                      }
                    >
                      <option value="weekly">Weekly</option>
                      <option value="biweekly">Every two weeks</option>
                      <option value="monthly">Monthly</option>
                    </Select>
                  </label>
                  <label className="person-field">
                    <span>Weekday</span>
                    <Select
                      value={form.weekday}
                      onChange={(event) =>
                        setForm((value) => ({
                          ...value,
                          weekday: event.target.value,
                        }))
                      }
                    >
                      {DAYS.map((day) => (
                        <option key={day} value={day}>
                          {day[0].toUpperCase() + day.slice(1)}
                        </option>
                      ))}
                    </Select>
                  </label>
                  <label className="person-field">
                    <span>Start time</span>
                    <TimePicker
                      value={form.localStart}
                      onChange={(localStart) =>
                        setForm((value) => ({
                          ...value,
                          localStart,
                        }))
                      }
                    />
                  </label>
                  <label className="person-field">
                    <span>Duration</span>
                    <Select
                      value={form.duration}
                      onChange={(event) =>
                        setForm((value) => ({
                          ...value,
                          duration: event.target.value,
                        }))
                      }
                    >
                      <option value="60">1 hour</option>
                      <option value="90">1½ hours</option>
                      <option value="120">2 hours</option>
                      <option value="180">3 hours</option>
                    </Select>
                  </label>
                  <label className="person-field">
                    <span>Timezone</span>
                    <Select
                      value={form.timezone}
                      onChange={(event) =>
                        setForm((value) => ({
                          ...value,
                          timezone: event.target.value,
                        }))
                      }
                    >
                      <option value="Africa/Accra">Africa/Accra (GMT)</option>
                      <option value="Africa/Lagos">Africa/Lagos</option>
                      <option value="Europe/London">Europe/London</option>
                    </Select>
                  </label>
                  <label className="person-field">
                    <span>Location</span>
                    <input
                      value={form.location}
                      onChange={(event) =>
                        setForm((value) => ({
                          ...value,
                          location: event.target.value,
                        }))
                      }
                      placeholder="Room, home or online"
                    />
                  </label>
                </>
              )}
              {needsReason && (
                <label className="person-field md:col-span-2">
                  <span>Lifecycle reason</span>
                  <input
                    required
                    minLength={3}
                    value={form.reason}
                    onChange={(event) =>
                      setForm((value) => ({
                        ...value,
                        reason: event.target.value,
                      }))
                    }
                    placeholder={`Why is this group being ${form.status}?`}
                  />
                </label>
              )}
              <div className="md:col-span-2">
                <button
                  disabled={busy || selected?.status === "closed"}
                  className="admin-primary-button rounded-lg px-5 py-3 text-sm font-semibold"
                >
                  {busy ? "Saving…" : selected ? "Save group" : "Create group"}
                </button>
              </div>
            </form>
          </Card>
        </div>
      )}
    </div>
  );
}
