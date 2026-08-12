"use client";

import {
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
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
import DatePicker from "@/components/ui/DatePicker";
import { DateTimePicker } from "@/components/ui/TemporalPicker";
import { SkeletonTable } from "@/components/ui/Skeleton";

interface Branch {
  id: string;
  name: string;
}
interface Team {
  id: string;
  version: number;
  name: string;
  description?: string;
  homeBranchId: string;
  leaderPersonIds: string[];
  status: "active" | "inactive";
}
interface Eligibility {
  minimumAgeYears?: number;
  membershipStages?: string[];
  requiredSkills?: string[];
  backgroundCheckRequired?: boolean;
  backgroundCheckMaxAgeDays?: number;
  safeguardingTrainingRequired?: boolean;
}
interface Position {
  id: string;
  version: number;
  teamId: string;
  name: string;
  description?: string;
  eligibility: Eligibility;
  status: "active" | "inactive";
}
interface Screening {
  backgroundCheckStatus: string;
  backgroundCheckedAt?: string;
  backgroundCheckExpiresAt?: string;
  backgroundCheckReference?: string;
  safeguardingTrainingAt?: string;
  safeguardingTrainingExpiresAt?: string;
}
interface Profile {
  id: string;
  version: number;
  personId: string;
  skills: string[];
  preferredTeamIds: string[];
  preferredPositionIds: string[];
  status: string;
  eligibility: Screening;
}
interface Availability {
  id: string;
  startsAt: string;
  endsAt: string;
  state: string;
  source: string;
}
type Person = PeoplePage["items"][number];

const TEAM_EMPTY = {
  name: "",
  description: "",
  status: "active" as "active" | "inactive",
};
const POSITION_EMPTY = {
  name: "",
  description: "",
  status: "active" as "active" | "inactive",
  minimumAge: "",
  membershipStage: "any",
  backgroundRequired: false,
  backgroundMaxAge: "365",
  trainingRequired: false,
  requiredSkills: [] as string[],
};
const PROFILE_EMPTY = {
  version: 0,
  status: "active",
  skills: [] as string[],
  preferredTeamIds: [] as string[],
  preferredPositionIds: [] as string[],
  backgroundCheckStatus: "not-required",
  backgroundCheckedAt: "",
  backgroundCheckExpiresAt: "",
  backgroundCheckReference: "",
  safeguardingTrainingAt: "",
  safeguardingTrainingExpiresAt: "",
};
const AVAILABILITY_EMPTY = {
  startsAt: "",
  endsAt: "",
  state: "available",
  source: "staff",
};
const SKILLS = [
  "audio",
  "children",
  "first-aid",
  "hospitality",
  "livestream",
  "music",
  "photography",
  "prayer",
  "production",
  "security",
  "teaching",
  "transport",
  "ushering",
  "video",
  "worship",
];
const MEMBERSHIP_STAGES = ["member", "serving-member", "leader"];

export default function VolunteersPage() {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branchId, setBranchId] = useState("");
  const [teams, setTeams] = useState<Team[]>([]);
  const [selectedTeam, setSelectedTeam] = useState<Team | null>(null);
  const [teamForm, setTeamForm] = useState(TEAM_EMPTY);
  const [leaders, setLeaders] = useState<Person[]>([]);
  const [leaderQuery, setLeaderQuery] = useState("");
  const [leaderCandidates, setLeaderCandidates] = useState<Person[]>([]);
  const [positions, setPositions] = useState<Position[]>([]);
  const [selectedPosition, setSelectedPosition] = useState<Position | null>(
    null,
  );
  const [positionForm, setPositionForm] = useState(POSITION_EMPTY);
  const [personQuery, setPersonQuery] = useState("");
  const [people, setPeople] = useState<Person[]>([]);
  const [person, setPerson] = useState<Person | null>(null);
  const [profile, setProfile] = useState(PROFILE_EMPTY);
  const [availability, setAvailability] = useState<Availability[]>([]);
  const [availabilityForm, setAvailabilityForm] = useState(AVAILABILITY_EMPTY);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    api("/api/branches")
      .then((value) => {
        const items = asList<Branch>(value);
        setBranches(items);
        setBranchId(items[0]?.id || "");
      })
      .catch(() => setBranches([]));
  }, []);

  const loadTeams = useCallback(async () => {
    if (!branchId) {
      setTeams([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const value = await api<{ items: Team[] }>(
        `/api/chms/v1/teams?branchId=${encodeURIComponent(branchId)}&includeInactive=true`,
      );
      setTeams(value.items || []);
    } catch (cause) {
      setError(message(cause, "Volunteer teams could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [branchId]);
  useEffect(() => {
    void loadTeams();
  }, [loadTeams]);

  useEffect(() => {
    if (leaderQuery.trim().length < 2 || !branchId) {
      setLeaderCandidates([]);
      return;
    }
    const handle = window.setTimeout(
      () =>
        loadPeople(leaderQuery, branchId)
          .then((value) => setLeaderCandidates(value.items || []))
          .catch(() => setLeaderCandidates([])),
      250,
    );
    return () => window.clearTimeout(handle);
  }, [branchId, leaderQuery]);
  useEffect(() => {
    if (personQuery.trim().length < 2 || !branchId) {
      setPeople([]);
      return;
    }
    const handle = window.setTimeout(
      () =>
        loadPeople(personQuery, branchId)
          .then((value) => setPeople(value.items || []))
          .catch(() => setPeople([])),
      250,
    );
    return () => window.clearTimeout(handle);
  }, [branchId, personQuery]);

  async function chooseTeam(value: Team) {
    setSelectedTeam(value);
    setSelectedPosition(null);
    setPositionForm(POSITION_EMPTY);
    setTeamForm({
      name: value.name,
      description: value.description || "",
      status: value.status,
    });
    setLeaders(
      value.leaderPersonIds.map((id) => personStub(id, value.homeBranchId)),
    );
    setBusy("positions");
    try {
      const [positionValue, leaderProfiles] = await Promise.all([
        api<{ items: Position[] }>(
          `/api/chms/v1/teams/${encodeURIComponent(value.id)}/positions?includeInactive=true`,
        ),
        Promise.all(
          value.leaderPersonIds.map((id) => loadMemberProfile(id)),
        ).catch(() => []),
      ]);
      setPositions(positionValue.items || []);
      if (leaderProfiles.length)
        setLeaders(
          leaderProfiles.map(({ person: profilePerson }) => ({
            id: profilePerson.id,
            version: profilePerson.version,
            personNumber: profilePerson.personNumber,
            names: profilePerson.names,
            photoUrl: profilePerson.photoUrl,
            homeBranchId: profilePerson.homeBranchId,
            membershipStage: profilePerson.membershipStage,
          })),
        );
    } catch (cause) {
      setError(message(cause, "Team positions could not be loaded."));
    } finally {
      setBusy("");
    }
  }

  function resetTeam() {
    setSelectedTeam(null);
    setTeamForm(TEAM_EMPTY);
    setLeaders([]);
    setLeaderQuery("");
    setPositions([]);
    setSelectedPosition(null);
    setPositionForm(POSITION_EMPTY);
  }
  async function saveTeam(event: FormEvent) {
    event.preventDefault();
    if (!branchId) return;
    setBusy("team");
    setError(null);
    const body = {
      ...teamForm,
      homeBranchId: branchId,
      leaderPersonIds: leaders.map((item) => item.id),
      ...(selectedTeam ? { expectedVersion: selectedTeam.version } : {}),
    };
    try {
      const value = await api<Team>(
        selectedTeam
          ? `/api/chms/v1/teams/${encodeURIComponent(selectedTeam.id)}`
          : "/api/chms/v1/teams",
        { method: selectedTeam ? "PATCH" : "POST", body },
      );
      await loadTeams();
      await chooseTeam(value);
    } catch (cause) {
      setError(message(cause, "The volunteer team could not be saved."));
    } finally {
      setBusy("");
    }
  }

  function choosePosition(value: Position) {
    setSelectedPosition(value);
    setPositionForm({
      name: value.name,
      description: value.description || "",
      status: value.status,
      minimumAge: value.eligibility.minimumAgeYears
        ? String(value.eligibility.minimumAgeYears)
        : "",
      membershipStage: value.eligibility.membershipStages?.[0] || "any",
      backgroundRequired: !!value.eligibility.backgroundCheckRequired,
      backgroundMaxAge: String(
        value.eligibility.backgroundCheckMaxAgeDays || 365,
      ),
      trainingRequired: !!value.eligibility.safeguardingTrainingRequired,
      requiredSkills: value.eligibility.requiredSkills || [],
    });
  }
  async function savePosition(event: FormEvent) {
    event.preventDefault();
    if (!selectedTeam) return;
    setBusy("position");
    setError(null);
    const body = {
      name: positionForm.name,
      description: positionForm.description,
      status: positionForm.status,
      eligibility: {
        minimumAgeYears: Number(positionForm.minimumAge || 0),
        membershipStages:
          positionForm.membershipStage === "any"
            ? []
            : [positionForm.membershipStage],
        requiredSkills: positionForm.requiredSkills,
        backgroundCheckRequired: positionForm.backgroundRequired,
        backgroundCheckMaxAgeDays: positionForm.backgroundRequired
          ? Number(positionForm.backgroundMaxAge)
          : 0,
        safeguardingTrainingRequired: positionForm.trainingRequired,
      },
      ...(selectedPosition
        ? { expectedVersion: selectedPosition.version }
        : {}),
    };
    try {
      await api(
        selectedPosition
          ? `/api/chms/v1/teams/${encodeURIComponent(selectedTeam.id)}/positions/${encodeURIComponent(selectedPosition.id)}`
          : `/api/chms/v1/teams/${encodeURIComponent(selectedTeam.id)}/positions`,
        { method: selectedPosition ? "PATCH" : "POST", body },
      );
      const value = await api<{ items: Position[] }>(
        `/api/chms/v1/teams/${encodeURIComponent(selectedTeam.id)}/positions?includeInactive=true`,
      );
      setPositions(value.items || []);
      setSelectedPosition(null);
      setPositionForm(POSITION_EMPTY);
    } catch (cause) {
      setError(message(cause, "The serving position could not be saved."));
    } finally {
      setBusy("");
    }
  }

  async function choosePerson(value: Person) {
    setPerson(value);
    setPersonQuery("");
    setPeople([]);
    setBusy("profile");
    setError(null);
    try {
      const existing = await api<Profile>(
        `/api/chms/v1/people/${encodeURIComponent(value.id)}/volunteer-profile`,
      );
      setProfile({
        version: existing.version,
        status: existing.status,
        skills: existing.skills || [],
        preferredTeamIds: existing.preferredTeamIds || [],
        preferredPositionIds: existing.preferredPositionIds || [],
        backgroundCheckStatus:
          existing.eligibility?.backgroundCheckStatus || "not-required",
        backgroundCheckedAt: dateOnly(
          existing.eligibility?.backgroundCheckedAt,
        ),
        backgroundCheckExpiresAt: dateOnly(
          existing.eligibility?.backgroundCheckExpiresAt,
        ),
        backgroundCheckReference:
          existing.eligibility?.backgroundCheckReference || "",
        safeguardingTrainingAt: dateOnly(
          existing.eligibility?.safeguardingTrainingAt,
        ),
        safeguardingTrainingExpiresAt: dateOnly(
          existing.eligibility?.safeguardingTrainingExpiresAt,
        ),
      });
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 404)
        setProfile(PROFILE_EMPTY);
      else
        setError(
          message(
            cause,
            "The volunteer profile could not be loaded. Screening metadata requires restricted access.",
          ),
        );
    } finally {
      setBusy("");
    }
    await loadAvailability(value.id);
  }
  async function loadAvailability(personId: string) {
    const from = new Date();
    from.setMonth(from.getMonth() - 1);
    const to = new Date();
    to.setFullYear(to.getFullYear() + 1);
    try {
      const value = await api<{ items: Availability[] }>(
        `/api/chms/v1/people/${encodeURIComponent(personId)}/availability?from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`,
      );
      setAvailability(value.items || []);
    } catch {
      setAvailability([]);
    }
  }
  async function saveProfile(event: FormEvent) {
    event.preventDefault();
    if (!person) return;
    setBusy("profile-save");
    setError(null);
    const eligibility = {
      backgroundCheckStatus: profile.backgroundCheckStatus,
      backgroundCheckedAt: isoDate(profile.backgroundCheckedAt),
      backgroundCheckExpiresAt: isoDate(profile.backgroundCheckExpiresAt),
      backgroundCheckReference: profile.backgroundCheckReference,
      safeguardingTrainingAt: isoDate(profile.safeguardingTrainingAt),
      safeguardingTrainingExpiresAt: isoDate(
        profile.safeguardingTrainingExpiresAt,
      ),
    };
    try {
      const value = await api<Profile>(
        `/api/chms/v1/people/${encodeURIComponent(person.id)}/volunteer-profile`,
        {
          method: "PUT",
          body: {
            expectedVersion: profile.version,
            status: profile.status,
            skills: profile.skills,
            preferredTeamIds: profile.preferredTeamIds,
            preferredPositionIds: profile.preferredPositionIds,
            eligibility,
          },
        },
      );
      setProfile((current) => ({ ...current, version: value.version }));
    } catch (cause) {
      setError(message(cause, "The volunteer profile could not be saved."));
    } finally {
      setBusy("");
    }
  }
  async function addAvailability(event: FormEvent) {
    event.preventDefault();
    if (!person) return;
    setBusy("availability");
    setError(null);
    try {
      await api(
        `/api/chms/v1/people/${encodeURIComponent(person.id)}/availability`,
        {
          method: "POST",
          body: {
            startsAt: new Date(availabilityForm.startsAt).toISOString(),
            endsAt: new Date(availabilityForm.endsAt).toISOString(),
            state: availabilityForm.state,
            source: availabilityForm.source,
          },
        },
      );
      setAvailabilityForm(AVAILABILITY_EMPTY);
      await loadAvailability(person.id);
    } catch (cause) {
      setError(message(cause, "Availability could not be saved."));
    } finally {
      setBusy("");
    }
  }

  const availablePositions = useMemo(
    () => positions.filter((item) => item.status === "active"),
    [positions],
  );
  if (loading) return <SkeletonTable rows={7} cols={4} />;
  return (
    <div>
      <PageHeader
        title="Volunteer operations"
        subtitle="Build serving teams, define transparent eligibility, and coordinate preferences and availability without storing screening reports."
      >
        <Select
          aria-label="Branch"
          value={branchId}
          onChange={(event) => {
            setBranchId(event.target.value);
            resetTeam();
            setPerson(null);
          }}
        >
          {branches.map((branch) => (
            <option key={branch.id} value={branch.id}>
              {branch.name}
            </option>
          ))}
        </Select>
        <button
          type="button"
          onClick={resetTeam}
          className="admin-secondary-action rounded-lg px-4 py-2.5 text-sm font-semibold"
        >
          New team
        </button>
        <Link
          href="/volunteers/schedule"
          className="admin-primary-button rounded-lg px-4 py-2.5 text-sm font-semibold"
        >
          Open schedule
        </Link>
      </PageHeader>
      {error && (
        <div className="mb-5">
          <ErrorBox message={error} onRetry={loadTeams} />
        </div>
      )}
      <div className="grid gap-5 xl:grid-cols-[minmax(18rem,.72fr)_minmax(0,1.28fr)]">
        <Card className="member-panel">
          <PanelTitle
            title="Teams"
            text="Branch-owned serving structures and accountable leaders."
          />
          <div className="member-row-list mt-3">
            {teams.map((team) => (
              <button
                type="button"
                key={team.id}
                onClick={() => void chooseTeam(team)}
                className="w-full text-left"
              >
                <span className="member-row-icon">
                  {team.status === "active" ? "✦" : "–"}
                </span>
                <span>
                  <b>{team.name}</b>
                  <small>
                    {team.leaderPersonIds.length} leader
                    {team.leaderPersonIds.length === 1 ? "" : "s"} ·{" "}
                    {team.status}
                  </small>
                </span>
                <em>{selectedTeam?.id === team.id ? "Selected" : "Open"}</em>
              </button>
            ))}
          </div>
          {!teams.length && (
            <EmptyState
              title="No volunteer teams"
              hint="Create the first serving team for this branch."
            />
          )}
        </Card>
        <Card className="member-panel">
          <PanelTitle
            title={selectedTeam ? "Edit team" : "Create team"}
            text="Names, leadership and lifecycle stay versioned and auditable."
          />
          <form onSubmit={saveTeam} className="mt-4 space-y-3">
            <div className="grid gap-3 sm:grid-cols-[1fr_10rem]">
              <Field label="Team name">
                <input
                  required
                  minLength={2}
                  value={teamForm.name}
                  onChange={(event) =>
                    setTeamForm((current) => ({
                      ...current,
                      name: event.target.value,
                    }))
                  }
                />
              </Field>
              <Field label="Status">
                <Select
                  value={teamForm.status}
                  onChange={(event) =>
                    setTeamForm((current) => ({
                      ...current,
                      status: event.target.value as "active" | "inactive",
                    }))
                  }
                >
                  <option value="active">Active</option>
                  <option value="inactive">Inactive</option>
                </Select>
              </Field>
            </div>
            <Field label="Purpose">
              <input
                maxLength={2000}
                value={teamForm.description}
                onChange={(event) =>
                  setTeamForm((current) => ({
                    ...current,
                    description: event.target.value,
                  }))
                }
                placeholder="What this team owns and how it serves"
              />
            </Field>
            <Field label="Find team leaders">
              <input
                type="search"
                value={leaderQuery}
                onChange={(event) => setLeaderQuery(event.target.value)}
                placeholder="Search active people"
              />
            </Field>
            <ChoiceResults
              people={leaderCandidates.filter(
                (candidate) =>
                  !leaders.some((leader) => leader.id === candidate.id),
              )}
              onChoose={(candidate) => {
                setLeaders((current) => [...current, candidate]);
                setLeaderQuery("");
                setLeaderCandidates([]);
              }}
            />
            <div className="flex flex-wrap gap-2">
              {leaders.map((leader) => (
                <button
                  type="button"
                  key={leader.id}
                  onClick={() =>
                    setLeaders((current) =>
                      current.filter((item) => item.id !== leader.id),
                    )
                  }
                  className="member-tag"
                >
                  {displayMemberName(leader.names)} ×
                </button>
              ))}
            </div>
            <button
              disabled={busy === "team"}
              className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold"
            >
              {busy === "team"
                ? "Saving…"
                : selectedTeam
                  ? "Save team"
                  : "Create team"}
            </button>
          </form>
        </Card>
      </div>
      {selectedTeam && (
        <div className="mt-5 grid gap-5 xl:grid-cols-[minmax(18rem,.72fr)_minmax(0,1.28fr)]">
          <Card className="member-panel">
            <PanelTitle
              title="Serving positions"
              text="Eligibility is explicit so scheduling decisions can be explained."
            />
            <div className="member-row-list mt-3">
              {positions.map((position) => (
                <button
                  type="button"
                  key={position.id}
                  onClick={() => choosePosition(position)}
                  className="w-full text-left"
                >
                  <span className="member-row-icon">
                    {position.status === "active" ? "◎" : "–"}
                  </span>
                  <span>
                    <b>{position.name}</b>
                    <small>
                      {position.eligibility.requiredSkills?.join(", ") ||
                        "No required skills"}
                    </small>
                  </span>
                  <em>
                    {selectedPosition?.id === position.id
                      ? "Editing"
                      : position.status}
                  </em>
                </button>
              ))}
            </div>
            <button
              type="button"
              onClick={() => {
                setSelectedPosition(null);
                setPositionForm(POSITION_EMPTY);
              }}
              className="admin-secondary-action mt-3 w-full rounded-lg px-3 py-2 text-xs font-semibold"
            >
              New position
            </button>
          </Card>
          <Card className="member-panel">
            <PanelTitle
              title={selectedPosition ? "Edit position" : "Add position"}
              text="Only eligibility metadata is stored here—never screening reports."
            />
            <form onSubmit={savePosition} className="mt-4 space-y-3">
              <div className="grid gap-3 sm:grid-cols-[1fr_10rem]">
                <Field label="Position name">
                  <input
                    required
                    minLength={2}
                    value={positionForm.name}
                    onChange={(event) =>
                      setPositionForm((current) => ({
                        ...current,
                        name: event.target.value,
                      }))
                    }
                  />
                </Field>
                <Field label="Status">
                  <Select
                    value={positionForm.status}
                    onChange={(event) =>
                      setPositionForm((current) => ({
                        ...current,
                        status: event.target.value as "active" | "inactive",
                      }))
                    }
                  >
                    <option value="active">Active</option>
                    <option value="inactive">Inactive</option>
                  </Select>
                </Field>
              </div>
              <Field label="Description">
                <input
                  maxLength={2000}
                  value={positionForm.description}
                  onChange={(event) =>
                    setPositionForm((current) => ({
                      ...current,
                      description: event.target.value,
                    }))
                  }
                />
              </Field>
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label="Minimum age">
                  <Select
                    value={positionForm.minimumAge || "0"}
                    onChange={(event) =>
                      setPositionForm((current) => ({
                        ...current,
                        minimumAge: event.target.value,
                      }))
                    }
                  >
                    <option value="0">No minimum</option>
                    <option value="16">16+</option>
                    <option value="18">18+</option>
                    <option value="21">21+</option>
                  </Select>
                </Field>
                <Field label="Membership requirement">
                  <Select
                    value={positionForm.membershipStage}
                    onChange={(event) =>
                      setPositionForm((current) => ({
                        ...current,
                        membershipStage: event.target.value,
                      }))
                    }
                  >
                    <option value="any">No stage requirement</option>
                    {MEMBERSHIP_STAGES.map((stage) => (
                      <option key={stage} value={stage}>
                        {stage.replaceAll("-", " ")}
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>
              <SkillPicker
                selected={positionForm.requiredSkills}
                onChange={(requiredSkills) =>
                  setPositionForm((current) => ({ ...current, requiredSkills }))
                }
                label="Required skills"
              />
              <div className="grid gap-3 sm:grid-cols-2">
                <ChoiceCard
                  checked={positionForm.backgroundRequired}
                  title="Background check"
                  text="Require current clearance metadata"
                  onChange={(backgroundRequired) =>
                    setPositionForm((current) => ({
                      ...current,
                      backgroundRequired,
                    }))
                  }
                />
                <ChoiceCard
                  checked={positionForm.trainingRequired}
                  title="Safeguarding training"
                  text="Require current training metadata"
                  onChange={(trainingRequired) =>
                    setPositionForm((current) => ({
                      ...current,
                      trainingRequired,
                    }))
                  }
                />
              </div>
              {positionForm.backgroundRequired && (
                <Field label="Clearance renewal">
                  <Select
                    value={positionForm.backgroundMaxAge}
                    onChange={(event) =>
                      setPositionForm((current) => ({
                        ...current,
                        backgroundMaxAge: event.target.value,
                      }))
                    }
                  >
                    <option value="180">Every 6 months</option>
                    <option value="365">Every year</option>
                    <option value="730">Every 2 years</option>
                    <option value="1095">Every 3 years</option>
                  </Select>
                </Field>
              )}
              <button
                disabled={busy === "position"}
                className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold"
              >
                {busy === "position"
                  ? "Saving…"
                  : selectedPosition
                    ? "Save position"
                    : "Add position"}
              </button>
            </form>
          </Card>
        </div>
      )}
      <Card className="member-panel mt-5">
        <PanelTitle
          title="Volunteer profile & availability"
          text="Choose a person, record serving preferences, and keep restricted clearance metadata separate from ordinary scheduling."
        />
        <div className="mt-4 max-w-2xl">
          <Field label="Find a volunteer">
            <input
              type="search"
              value={personQuery}
              onChange={(event) => setPersonQuery(event.target.value)}
              placeholder="Search by name or contact"
            />
          </Field>
          <ChoiceResults
            people={people}
            onChoose={(value) => void choosePerson(value)}
          />
        </div>
        {person && (
          <div className="mt-5 grid gap-5 xl:grid-cols-2">
            <form
              onSubmit={saveProfile}
              className="space-y-4 rounded-2xl bg-[var(--remi-surface-soft)] p-4 sm:p-5"
            >
              <div>
                <h3 className="text-sm font-bold text-[var(--remi-ink)]">
                  {displayMemberName(person.names)}
                </h3>
                <p className="mt-1 text-xs text-[var(--remi-muted)]">
                  Skills, preferences and restricted eligibility metadata
                </p>
              </div>
              <Field label="Volunteer status">
                <Select
                  value={profile.status}
                  onChange={(event) =>
                    setProfile((current) => ({
                      ...current,
                      status: event.target.value,
                    }))
                  }
                >
                  <option value="active">Active</option>
                  <option value="paused">Paused</option>
                  <option value="inactive">Inactive</option>
                </Select>
              </Field>
              <SkillPicker
                selected={profile.skills}
                onChange={(skills) =>
                  setProfile((current) => ({ ...current, skills }))
                }
                label="Skills"
              />
              <PreferencePicker
                label="Preferred teams"
                items={teams
                  .filter((item) => item.status === "active")
                  .map((item) => ({ id: item.id, label: item.name }))}
                selected={profile.preferredTeamIds}
                onChange={(preferredTeamIds) =>
                  setProfile((current) => ({ ...current, preferredTeamIds }))
                }
              />
              <PreferencePicker
                label="Preferred positions in selected team"
                items={availablePositions.map((item) => ({
                  id: item.id,
                  label: item.name,
                }))}
                selected={profile.preferredPositionIds}
                onChange={(preferredPositionIds) =>
                  setProfile((current) => ({
                    ...current,
                    preferredPositionIds,
                  }))
                }
              />
              <div className="rounded-xl border border-[var(--remi-line)] p-4">
                <p className="text-xs font-bold text-[var(--remi-ink)]">
                  Restricted eligibility metadata
                </p>
                <p className="mt-1 text-[10px] leading-relaxed text-[var(--remi-muted)]">
                  Record status, dates and a provider reference only. Do not
                  paste findings, reports, identity documents or safeguarding
                  notes.
                </p>
                <div className="mt-3 space-y-3">
                  <Field label="Background-check status">
                    <Select
                      value={profile.backgroundCheckStatus}
                      onChange={(event) =>
                        setProfile((current) => ({
                          ...current,
                          backgroundCheckStatus: event.target.value,
                        }))
                      }
                    >
                      <option value="not-required">Not required</option>
                      <option value="pending">Pending</option>
                      <option value="cleared">Cleared</option>
                      <option value="expired">Expired</option>
                      <option value="restricted">Restricted</option>
                    </Select>
                  </Field>
                  {profile.backgroundCheckStatus === "cleared" && (
                    <>
                      <div className="grid gap-3 sm:grid-cols-2">
                        <Field label="Checked on">
                          <DatePicker
                            value={profile.backgroundCheckedAt}
                            onChange={(backgroundCheckedAt) =>
                              setProfile((current) => ({
                                ...current,
                                backgroundCheckedAt,
                              }))
                            }
                          />
                        </Field>
                        <Field label="Expires on">
                          <DatePicker
                            min={profile.backgroundCheckedAt}
                            value={profile.backgroundCheckExpiresAt}
                            onChange={(backgroundCheckExpiresAt) =>
                              setProfile((current) => ({
                                ...current,
                                backgroundCheckExpiresAt,
                              }))
                            }
                          />
                        </Field>
                      </div>
                      <Field label="Provider reference">
                        <input
                          required
                          maxLength={120}
                          value={profile.backgroundCheckReference}
                          onChange={(event) =>
                            setProfile((current) => ({
                              ...current,
                              backgroundCheckReference: event.target.value,
                            }))
                          }
                        />
                      </Field>
                    </>
                  )}
                  <div className="grid gap-3 sm:grid-cols-2">
                    <Field label="Training completed">
                      <DatePicker
                        value={profile.safeguardingTrainingAt}
                        onChange={(safeguardingTrainingAt) =>
                          setProfile((current) => ({
                            ...current,
                            safeguardingTrainingAt,
                          }))
                        }
                      />
                    </Field>
                    <Field label="Training expires">
                      <DatePicker
                        min={profile.safeguardingTrainingAt}
                        value={profile.safeguardingTrainingExpiresAt}
                        onChange={(safeguardingTrainingExpiresAt) =>
                          setProfile((current) => ({
                            ...current,
                            safeguardingTrainingExpiresAt,
                          }))
                        }
                      />
                    </Field>
                  </div>
                </div>
              </div>
              <button
                disabled={busy === "profile-save"}
                className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold"
              >
                {busy === "profile-save" ? "Saving…" : "Save volunteer profile"}
              </button>
            </form>
            <div className="rounded-2xl bg-[var(--remi-surface-soft)] p-4 sm:p-5">
              <h3 className="text-sm font-bold text-[var(--remi-ink)]">
                Availability
              </h3>
              <p className="mt-1 text-xs text-[var(--remi-muted)]">
                Bounded windows feed scheduling conflicts without guessing
                intent.
              </p>
              <form onSubmit={addAvailability} className="mt-4 space-y-3">
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field label="Starts">
                    <DateTimePicker
                      required
                      value={availabilityForm.startsAt}
                      onChange={(startsAt) =>
                        setAvailabilityForm((current) => ({
                          ...current,
                          startsAt,
                        }))
                      }
                    />
                  </Field>
                  <Field label="Ends">
                    <DateTimePicker
                      required
                      min={availabilityForm.startsAt}
                      value={availabilityForm.endsAt}
                      onChange={(endsAt) =>
                        setAvailabilityForm((current) => ({
                          ...current,
                          endsAt,
                        }))
                      }
                    />
                  </Field>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <Field label="Availability">
                    <Select
                      value={availabilityForm.state}
                      onChange={(event) =>
                        setAvailabilityForm((current) => ({
                          ...current,
                          state: event.target.value,
                        }))
                      }
                    >
                      <option value="available">Available</option>
                      <option value="preferred">Preferred</option>
                      <option value="unavailable">Unavailable</option>
                    </Select>
                  </Field>
                  <Field label="Source">
                    <Select
                      value={availabilityForm.source}
                      onChange={(event) =>
                        setAvailabilityForm((current) => ({
                          ...current,
                          source: event.target.value,
                        }))
                      }
                    >
                      <option value="staff">Recorded by staff</option>
                      <option value="member">Provided by member</option>
                      <option value="import">Imported</option>
                    </Select>
                  </Field>
                </div>
                <button
                  disabled={busy === "availability"}
                  className="admin-primary-button w-full rounded-lg px-4 py-2.5 text-sm font-semibold"
                >
                  Add availability window
                </button>
              </form>
              <div className="member-row-list mt-4">
                {availability.map((item) => (
                  <div key={item.id}>
                    <span className="member-row-icon">
                      {item.state === "unavailable"
                        ? "×"
                        : item.state === "preferred"
                          ? "★"
                          : "✓"}
                    </span>
                    <span>
                      <b>{item.state}</b>
                      <small>
                        {formatDate(item.startsAt)} → {formatDate(item.endsAt)}
                      </small>
                    </span>
                    <em>{item.source}</em>
                  </div>
                ))}
              </div>
              {!availability.length && (
                <EmptyState
                  title="No availability windows"
                  hint="Add availability, preference or unavailable time for this volunteer."
                />
              )}
            </div>
          </div>
        )}
        {!person && (
          <EmptyState
            title="Choose a volunteer"
            hint="Search for an active person to manage skills, preferences, eligibility and availability."
          />
        )}
      </Card>
    </div>
  );
}

function PanelTitle({ title, text }: { title: string; text: string }) {
  return (
    <div className="member-panel-title">
      <div>
        <h3>{title}</h3>
        <p>{text}</p>
      </div>
    </div>
  );
}
function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="person-field">
      <span>{label}</span>
      {children}
    </label>
  );
}
function ChoiceResults({
  people,
  onChoose,
}: {
  people: Person[];
  onChoose: (person: Person) => void;
}) {
  if (!people.length) return null;
  return (
    <div className="mt-2 rounded-xl border border-[var(--remi-line)] p-2">
      {people.slice(0, 6).map((person) => (
        <button
          type="button"
          key={person.id}
          onClick={() => onChoose(person)}
          className="block w-full rounded-lg px-3 py-2 text-left text-sm text-[var(--remi-ink)] hover:bg-[var(--remi-surface-soft)]"
        >
          {displayMemberName(person.names)}{" "}
          <small className="text-[var(--remi-muted)]">
            · {person.membershipStage.replaceAll("-", " ")}
          </small>
        </button>
      ))}
    </div>
  );
}
function SkillPicker({
  selected,
  onChange,
  label,
}: {
  selected: string[];
  onChange: (skills: string[]) => void;
  label: string;
}) {
  return (
    <fieldset>
      <legend className="mb-2 text-[.68rem] font-bold text-[var(--remi-ink)]">
        {label}
      </legend>
      <div className="flex flex-wrap gap-2">
        {SKILLS.map((skill) => {
          const active = selected.includes(skill);
          return (
            <button
              type="button"
              key={skill}
              aria-pressed={active}
              onClick={() =>
                onChange(
                  active
                    ? selected.filter((item) => item !== skill)
                    : [...selected, skill],
                )
              }
              className={
                active
                  ? "admin-primary-button rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
                  : "admin-secondary-action rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
              }
            >
              {skill.replaceAll("-", " ")}
            </button>
          );
        })}
      </div>
    </fieldset>
  );
}
function PreferencePicker({
  label,
  items,
  selected,
  onChange,
}: {
  label: string;
  items: Array<{ id: string; label: string }>;
  selected: string[];
  onChange: (ids: string[]) => void;
}) {
  return (
    <fieldset>
      <legend className="mb-2 text-[.68rem] font-bold text-[var(--remi-ink)]">
        {label}
      </legend>
      <div className="flex flex-wrap gap-2">
        {items.map((item) => {
          const active = selected.includes(item.id);
          return (
            <button
              type="button"
              key={item.id}
              aria-pressed={active}
              onClick={() =>
                onChange(
                  active
                    ? selected.filter((id) => id !== item.id)
                    : [...selected, item.id],
                )
              }
              className={
                active
                  ? "admin-primary-button rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
                  : "admin-secondary-action rounded-lg px-2.5 py-1.5 text-[11px] font-semibold"
              }
            >
              {item.label}
            </button>
          );
        })}
        {!items.length && (
          <small className="text-xs text-[var(--remi-muted)]">
            Create an active option first.
          </small>
        )}
      </div>
    </fieldset>
  );
}
function ChoiceCard({
  checked,
  title,
  text,
  onChange,
}: {
  checked: boolean;
  title: string;
  text: string;
  onChange: (value: boolean) => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={checked}
      onClick={() => onChange(!checked)}
      className={`rounded-xl border p-3 text-left transition ${checked ? "border-[rgb(209_173_85/.35)] bg-[rgb(209_173_85/.1)]" : "border-[var(--remi-line)] bg-transparent"}`}
    >
      <b className="block text-xs text-[var(--remi-ink)]">
        {checked ? "✓ " : ""}
        {title}
      </b>
      <small className="mt-1 block text-[10px] text-[var(--remi-muted)]">
        {text}
      </small>
    </button>
  );
}
function personStub(id: string, homeBranchId: string): Person {
  return {
    id,
    version: 1,
    personNumber: "",
    names: { given: `Leader ${id.slice(-5)}` },
    homeBranchId,
    membershipStage: "member",
  };
}
function message(cause: unknown, fallback: string) {
  return cause instanceof ApiError ? cause.message : fallback;
}
function dateOnly(value?: string) {
  return value ? value.slice(0, 10) : "";
}
function isoDate(value: string) {
  return value ? new Date(`${value}T12:00:00Z`).toISOString() : null;
}
function formatDate(value: string) {
  return new Intl.DateTimeFormat(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(new Date(value));
}
