"use client";

import { useCallback, useEffect, useState, type FormEvent } from "react";
import { api, ApiError, asList } from "@/lib/api";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import DatePicker from "@/components/ui/DatePicker";
import { TimePicker } from "@/components/ui/TemporalPicker";
import { SkeletonTable } from "@/components/ui/Skeleton";
import { AttendanceWorkspace } from "@/components/attendance/AttendanceWorkspace";
import { AttendanceAnalytics } from "@/components/attendance/AttendanceAnalytics";

interface Branch {
  id: string;
  name: string;
}
interface Definition {
  id: string;
  version: number;
  name: string;
  homeBranchId: string;
  timezone: string;
  defaultDurationMinutes: number;
  defaultCapacity: number;
  recurrence?: {
    daysOfWeek: string[];
    localStart: string;
    interval: number;
    startsOn: string;
  };
  status: string;
}
interface Occurrence {
  id: string;
  version: number;
  name: string;
  startsAt: string;
  endsAt: string;
  capacity: number;
  status: string;
  occurrenceKey: string;
}
const DAYS = [
  "monday",
  "tuesday",
  "wednesday",
  "thursday",
  "friday",
  "saturday",
  "sunday",
];
const EMPTY = {
  name: "",
  timezone: "Africa/Accra",
  duration: "120",
  capacity: "",
  localStart: "09:00",
  startsOn: new Date().toISOString().slice(0, 10),
  days: ["sunday"] as string[],
};

export default function ServicesPage() {
  const [branches, setBranches] = useState<Branch[]>([]);
  const [branchId, setBranchId] = useState("");
  const [definitions, setDefinitions] = useState<Definition[]>([]);
  const [occurrences, setOccurrences] = useState<Occurrence[]>([]);
  const [selectedOccurrence, setSelectedOccurrence] =
    useState<Occurrence | null>(null);
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState(EMPTY);
  useEffect(() => {
    api("/api/branches")
      .then((value) => {
        const items = asList<Branch>(value);
        setBranches(items);
        setBranchId(items[0]?.id || "");
      })
      .catch(() => setBranches([]));
  }, []);
  const load = useCallback(async () => {
    if (!branchId) {
      setDefinitions([]);
      setOccurrences([]);
      setLoading(false);
      return;
    }
    setLoading(true);
    setError(null);
    const from = new Date();
    from.setDate(from.getDate() - 7);
    const to = new Date();
    to.setDate(to.getDate() + 120);
    try {
      const [definitionData, occurrenceData] = await Promise.all([
        api<{ items: Definition[] }>(
          `/api/chms/v1/service-definitions?branchId=${encodeURIComponent(branchId)}&includeInactive=true`,
        ),
        api<{ items: Occurrence[] }>(
          `/api/chms/v1/occurrences?branchId=${encodeURIComponent(branchId)}&from=${encodeURIComponent(from.toISOString())}&to=${encodeURIComponent(to.toISOString())}`,
        ),
      ]);
      setDefinitions(definitionData.items || []);
      setOccurrences(occurrenceData.items || []);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Service operations could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, [branchId]);
  useEffect(() => {
    load();
  }, [load]);
  async function create(event: FormEvent) {
    event.preventDefault();
    if (!branchId) return;
    setBusy(true);
    setError(null);
    try {
      await api("/api/chms/v1/service-definitions", {
        method: "POST",
        body: {
          homeBranchId: branchId,
          name: form.name,
          timezone: form.timezone,
          defaultDurationMinutes: Number(form.duration),
          defaultRoomIds: [],
          defaultCapacity: Number(form.capacity || 0),
          recurrence: {
            frequency: "weekly",
            daysOfWeek: form.days,
            localStart: form.localStart,
            interval: 1,
            startsOn: form.startsOn,
          },
          status: "active",
        },
      });
      setForm(EMPTY);
      setShowCreate(false);
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The service definition could not be created.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function generate(definition: Definition) {
    setBusy(true);
    setError(null);
    const from = new Date();
    const to = new Date();
    to.setDate(to.getDate() + 120);
    try {
      await api(
        `/api/chms/v1/service-definitions/${definition.id}/occurrences:generate`,
        {
          method: "POST",
          body: { from: from.toISOString(), to: to.toISOString() },
        },
      );
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Upcoming occurrences could not be generated.",
      );
    } finally {
      setBusy(false);
    }
  }
  return (
    <div>
      <PageHeader
        title="Services & attendance"
        subtitle="Shape recurring ministry rhythms, review dated occurrences and prepare each gathering for attendance."
      >
        <Select
          aria-label="Branch"
          value={branchId}
          onChange={(event) => setBranchId(event.target.value)}
        >
          <option value="">Choose branch</option>
          {branches.map((branch) => (
            <option key={branch.id} value={branch.id}>
              {branch.name}
            </option>
          ))}
        </Select>
        <button
          className="admin-primary-button rounded-lg px-4 py-2.5 text-sm font-semibold"
          onClick={() => setShowCreate((value) => !value)}
        >
          {showCreate ? "Close form" : "New service"}
        </button>
      </PageHeader>
      {error && (
        <div className="mb-5">
          <ErrorBox message={error} onRetry={load} />
        </div>
      )}
      <AttendanceAnalytics branchId={branchId} />
      {showCreate && (
        <Card className="member-panel mb-5">
          <div className="member-panel-title">
            <div>
              <h3>New recurring service</h3>
              <p>
                Local calendar intent is retained; generated occurrences are
                immutable dated records.
              </p>
            </div>
          </div>
          <form
            onSubmit={create}
            className="grid gap-4 md:grid-cols-2 xl:grid-cols-4"
          >
            <label className="person-field">
              <span>Service name</span>
              <input
                required
                value={form.name}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    name: event.target.value,
                  }))
                }
              />
            </label>
            <label className="person-field">
              <span>Timezone</span>
              <Select
                value={form.timezone}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    timezone: event.target.value,
                  }))
                }
              >
                <option value="Africa/Accra">Africa/Accra (GMT)</option>
                <option value="Africa/Lagos">Africa/Lagos</option>
                <option value="Europe/London">Europe/London</option>
                <option value="America/New_York">America/New York</option>
              </Select>
            </label>
            <label className="person-field">
              <span>Local start</span>
              <TimePicker
                required
                label="Local start"
                value={form.localStart}
                onChange={(localStart) =>
                  setForm((current) => ({ ...current, localStart }))
                }
              />
            </label>
            <label className="person-field">
              <span>Recurrence begins</span>
              <DatePicker
                value={form.startsOn}
                onChange={(startsOn) =>
                  setForm((current) => ({ ...current, startsOn }))
                }
              />
            </label>
            <label className="person-field">
              <span>Duration</span>
              <Select
                value={form.duration}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    duration: event.target.value,
                  }))
                }
              >
                <option value="60">1 hour</option>
                <option value="90">1½ hours</option>
                <option value="120">2 hours</option>
                <option value="150">2½ hours</option>
                <option value="180">3 hours</option>
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
                  setForm((current) => ({
                    ...current,
                    capacity: event.target.value,
                  }))
                }
                placeholder="No fixed limit"
              />
            </label>
            <fieldset className="md:col-span-2">
              <legend className="mb-2 text-sm font-semibold text-[var(--remi-ink)]">
                Days of week
              </legend>
              <div className="flex flex-wrap gap-2">
                {DAYS.map((day) => (
                  <label key={day} className="member-tag cursor-pointer">
                    <input
                      className="mr-2"
                      type="checkbox"
                      checked={form.days.includes(day)}
                      onChange={(event) =>
                        setForm((current) => ({
                          ...current,
                          days: event.target.checked
                            ? [...current.days, day]
                            : current.days.filter((value) => value !== day),
                        }))
                      }
                    />
                    {day.slice(0, 3)}
                  </label>
                ))}
              </div>
            </fieldset>
            <div className="flex items-end md:col-span-2 xl:col-span-4">
              <button
                disabled={busy || !form.days.length}
                className="admin-primary-button rounded-lg px-5 py-3 text-sm font-semibold"
              >
                {busy ? "Saving…" : "Create recurring service"}
              </button>
            </div>
          </form>
        </Card>
      )}
      {loading ? (
        <SkeletonTable rows={6} cols={4} />
      ) : !branchId ? (
        <EmptyState
          title="Choose a branch"
          hint="Service definitions and occurrences are always scoped to a branch."
        />
      ) : (
        <>
          <div className="grid gap-5 xl:grid-cols-[minmax(0,.9fr)_minmax(0,1.35fr)]">
            <Card className="member-panel">
              <div className="member-panel-title">
                <div>
                  <h3>Service definitions</h3>
                  <p>Recurring intent and operational defaults.</p>
                </div>
              </div>
              {definitions.length ? (
                <div className="member-row-list">
                  {definitions.map((definition) => (
                    <div key={definition.id}>
                      <span className="member-row-icon">◫</span>
                      <span>
                        <b>{definition.name}</b>
                        <small>
                          {definition.recurrence
                            ? `${definition.recurrence.daysOfWeek.join(", ")} · ${definition.recurrence.localStart}`
                            : "No recurrence"}{" "}
                          · {definition.defaultDurationMinutes} min
                        </small>
                      </span>
                      <button
                        disabled={busy || definition.status !== "active"}
                        onClick={() => generate(definition)}
                        className="admin-secondary-action rounded-lg px-3 py-2 text-xs font-semibold"
                      >
                        Generate dates
                      </button>
                    </div>
                  ))}
                </div>
              ) : (
                <EmptyState
                  title="No service definitions"
                  hint="Create the recurring service rhythm for this branch."
                />
              )}
            </Card>
            <Card className="member-panel">
              <div className="member-panel-title">
                <div>
                  <h3>Upcoming occurrences</h3>
                  <p>
                    Concrete dated gatherings used by check-in and attendance.
                  </p>
                </div>
              </div>
              {occurrences.length ? (
                <div className="member-row-list">
                  {occurrences.map((occurrence) => (
                    <div key={occurrence.id}>
                      <span className="member-row-icon">
                        {occurrence.status === "cancelled" ? "×" : "→"}
                      </span>
                      <span>
                        <b>{occurrence.name}</b>
                        <small>
                          {formatDate(occurrence.startsAt)} · capacity{" "}
                          {occurrence.capacity || "open"}
                        </small>
                      </span>
                      {occurrence.status === "scheduled" ? (
                        <button
                          type="button"
                          onClick={() => setSelectedOccurrence(occurrence)}
                          className="admin-secondary-action rounded-lg px-3 py-2 text-xs font-semibold"
                        >
                          Open attendance
                        </button>
                      ) : (
                        <em>{occurrence.status}</em>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <EmptyState
                  title="No dated occurrences"
                  hint="Generate upcoming dates from a recurring definition."
                />
              )}
            </Card>
          </div>
          {selectedOccurrence && (
            <AttendanceWorkspace
              occurrence={selectedOccurrence}
              branchId={branchId}
              onClose={() => setSelectedOccurrence(null)}
            />
          )}
        </>
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
