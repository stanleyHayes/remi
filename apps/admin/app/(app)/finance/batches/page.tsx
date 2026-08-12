"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { api, ApiError } from "@/lib/api";
import { Select } from "@/components/ui/Select";
import { DateTimePicker } from "@/components/ui/TemporalPicker";

type Money = { amountMinor: number; currency: string };
type Campus = { branchId: string; name: string };
type Method = { id: string; name: string; kind: string; active: boolean };
type Fund = { id: string; name: string; activeUntil?: string };
type Owner = { id: string; name: string; email: string; role: string };
type Subject = {
  id: string;
  type: "person" | "household";
  name: string;
  branchId: string;
};
type Batch = {
  id: string;
  version: number;
  branchId: string;
  receivedAt: string;
  counterIds: string[];
  expectedPaymentMethodIds: string[];
  dualControlRequired: boolean;
  state: string;
  entryCount: number;
  enteredTotal: Money;
  declaredTotal: Money;
  variance: Money;
  confirmationCount: number;
};
type Entry = {
  id: string;
  donor: { type: string; personId?: string; householdId?: string };
  source: string;
  paymentMethodId: string;
  total: Money;
  splits: { fundId: string; amount: Money }[];
  provenance?: string;
};
type Confirmation = {
  id: string;
  counterId: string;
  declaredTotal: Money;
  confirmedAt: string;
};
type Event = { id: string; type: string; reason?: string; occurredAt: string };
type Detail = {
  batch: Batch;
  entries: Entry[];
  confirmations: Confirmation[];
  events: Event[];
};

const DENOMINATIONS = [
  20000, 10000, 5000, 2000, 1000, 500, 200, 100, 50, 20, 10, 5, 1,
];
const offlineKinds = new Set([
  "cash",
  "cheque",
  "mobile-money",
  "bank-transfer",
  "in-kind",
]);
const money = (minor = 0) =>
  new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS" }).format(
    minor / 100,
  );
const rows = <T,>(value: unknown): T[] =>
  Array.isArray(value)
    ? (value as T[])
    : value &&
        typeof value === "object" &&
        Array.isArray((value as { items?: unknown[] }).items)
      ? (value as { items: T[] }).items
      : [];

export default function CountingBatchWorkspace() {
  const [campuses, setCampuses] = useState<Campus[]>([]),
    [methods, setMethods] = useState<Method[]>([]),
    [funds, setFunds] = useState<Fund[]>([]),
    [owners, setOwners] = useState<Owner[]>([]);
  const [branchId, setBranchId] = useState(""),
    [batches, setBatches] = useState<Batch[]>([]),
    [selectedId, setSelectedId] = useState(""),
    [detail, setDetail] = useState<Detail | null>(null);
  const [error, setError] = useState<string | null>(null),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(true);
  const [create, setCreate] = useState({
    receivedAt: "",
    counterOne: "",
    counterTwo: "",
    methods: [] as string[],
  });
  const [entry, setEntry] = useState({
    donorType: "anonymous",
    subject: null as Subject | null,
    query: "",
    paymentMethodId: "",
    fundId: "",
    amount: "",
    reference: "",
    provenance: "operator-entry",
  });
  const [subjects, setSubjects] = useState<Subject[]>([]),
    [searching, setSearching] = useState(false),
    [tenders, setTenders] = useState<Record<string, string>>({}),
    [denominations, setDenominations] = useState<Record<string, number>>({}),
    [reason, setReason] = useState("");

  const offlineMethods = useMemo(
    () => methods.filter((item) => item.active && offlineKinds.has(item.kind)),
    [methods],
  );
  const selectedMethods = useMemo(
    () =>
      (detail?.batch.expectedPaymentMethodIds
        .map((id) => methods.find((method) => method.id === id))
        .filter(Boolean) as Method[]) || [],
    [detail, methods],
  );

  const loadFoundation = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [campusData, methodData, fundData, ownerData] = await Promise.all([
        api("/api/chms/v1/finance/campuses"),
        api("/api/chms/v1/finance/payment-methods"),
        api("/api/chms/v1/finance/funds"),
        api("/api/chms/v1/finance/reconciliation-owners"),
      ]);
      const nextCampuses = rows<Campus>(campusData),
        nextMethods = rows<Method>(methodData).filter(
          (item) => item.active && offlineKinds.has(item.kind),
        ),
        nextFunds = rows<Fund>(fundData),
        nextOwners = rows<Owner>(ownerData).filter((owner) =>
          ["super-admin", "finance-admin", "finance-counter"].includes(
            owner.role,
          ),
        );
      setCampuses(nextCampuses);
      setMethods(nextMethods);
      setFunds(nextFunds);
      setOwners(nextOwners);
      setBranchId((value) => value || nextCampuses[0]?.branchId || "");
      setCreate((value) => ({
        ...value,
        counterOne: value.counterOne || nextOwners[0]?.id || "",
        counterTwo: value.counterTwo || nextOwners[1]?.id || "",
        methods: value.methods.length
          ? value.methods
          : nextMethods.slice(0, 1).map((item) => item.id),
      }));
      setEntry((value) => ({
        ...value,
        paymentMethodId: value.paymentMethodId || nextMethods[0]?.id || "",
        fundId: value.fundId || nextFunds[0]?.id || "",
      }));
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Counting controls could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, []);

  const loadBatches = useCallback(async () => {
    if (!branchId) return;
    try {
      const next = rows<Batch>(
        await api(
          `/api/chms/v1/finance/batches?branchId=${encodeURIComponent(branchId)}&limit=100`,
        ),
      );
      setBatches(next);
      setSelectedId((value) =>
        next.some((item) => item.id === value) ? value : next[0]?.id || "",
      );
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Counting batches could not be loaded.",
      );
    }
  }, [branchId]);

  const loadDetail = useCallback(async () => {
    if (!selectedId) {
      setDetail(null);
      return;
    }
    try {
      const next = await api<Detail>(
        `/api/chms/v1/finance/batches/${selectedId}`,
      );
      setDetail(next);
      setTenders(
        Object.fromEntries(
          next.batch.expectedPaymentMethodIds.map((id) => [
            id,
            String(
              next.entries
                .filter((item) => item.paymentMethodId === id)
                .reduce((sum, item) => sum + item.total.amountMinor, 0) / 100,
            ),
          ]),
        ),
      );
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Batch detail could not be loaded.",
      );
    }
  }, [selectedId]);

  useEffect(() => {
    void loadFoundation();
  }, [loadFoundation]);
  useEffect(() => {
    void loadBatches();
  }, [loadBatches, notice]);
  useEffect(() => {
    void loadDetail();
  }, [loadDetail, notice]);
  useEffect(() => {
    if (
      !entry.query.trim() ||
      entry.donorType === "anonymous" ||
      entry.subject
    ) {
      setSubjects([]);
      return;
    }
    const timer = window.setTimeout(async () => {
      setSearching(true);
      try {
        setSubjects(
          rows<Subject>(
            await api(
              `/api/chms/v1/finance/statement-subjects?q=${encodeURIComponent(entry.query.trim())}`,
            ),
          ).filter((item) => item.type === entry.donorType),
        );
      } catch {
        setSubjects([]);
      } finally {
        setSearching(false);
      }
    }, 250);
    return () => window.clearTimeout(timer);
  }, [entry.query, entry.donorType, entry.subject]);

  async function run(action: () => Promise<void>, success: string) {
    setBusy(true);
    setError(null);
    setNotice("");
    try {
      await action();
      setNotice(success);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The finance command could not be completed.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function createBatch(event: FormEvent) {
    event.preventDefault();
    await run(async () => {
      const value = await api<Batch>("/api/chms/v1/finance/batches", {
        method: "POST",
        body: {
          branchId,
          receivedAt: new Date(create.receivedAt).toISOString(),
          counterIds: [create.counterOne, create.counterTwo],
          dualControlRequired: true,
          expectedPaymentMethodIds: create.methods,
        },
      });
      setSelectedId(value.id);
    }, "Counting batch opened with dual control.");
  }
  async function transition(action: string) {
    if (!detail) return;
    await run(
      async () => {
        await api(
          `/api/chms/v1/finance/batches/${detail.batch.id}/transitions`,
          {
            method: "POST",
            body: { action, expectedVersion: detail.batch.version, reason },
          },
        );
        setReason("");
      },
      action === "post"
        ? "Batch posted to the immutable contribution ledger."
        : "Batch state updated.",
    );
  }
  async function addEntry(event: FormEvent) {
    event.preventDefault();
    if (!detail) return;
    const amountMinor = Math.round(Number(entry.amount) * 100);
    const donor =
      entry.donorType === "anonymous"
        ? { type: "anonymous" }
        : entry.subject
          ? {
              type: entry.subject.type,
              [entry.subject.type === "person" ? "personId" : "householdId"]:
                entry.subject.id,
            }
          : null;
    if (!donor) {
      setError("Choose a member or household from the suggestions.");
      return;
    }
    const method = methods.find((item) => item.id === entry.paymentMethodId);
    await run(async () => {
      await api(`/api/chms/v1/finance/batches/${detail.batch.id}/entries`, {
        method: "POST",
        body: {
          donor,
          source: method?.kind,
          paymentMethodId: entry.paymentMethodId,
          paymentMethodReference: entry.reference,
          total: { amountMinor, currency: "GHS" },
          splits: [
            { fundId: entry.fundId, amount: { amountMinor, currency: "GHS" } },
          ],
          provenance: entry.provenance,
        },
      });
      setEntry((value) => ({
        ...value,
        amount: "",
        reference: "",
        subject: null,
        query: "",
      }));
    }, "Contribution entry added to the count.");
  }
  async function confirmCount() {
    if (!detail) return;
    const tenderTotals = selectedMethods.map((method) => ({
      paymentMethodId: method.id,
      amount: {
        amountMinor:
          method.kind === "cash"
            ? cashTotal(method.id)
            : Math.round(Number(tenders[method.id] || 0) * 100),
        currency: "GHS",
      },
    }));
    const denominationRows = selectedMethods
      .filter((method) => method.kind === "cash")
      .flatMap((method) =>
        DENOMINATIONS.map((valueMinor) => ({
          paymentMethodId: method.id,
          valueMinor,
          count: denominations[`${method.id}:${valueMinor}`] || 0,
        })).filter((row) => row.count > 0),
      );
    await run(async () => {
      await api(
        `/api/chms/v1/finance/batches/${detail.batch.id}/confirmations`,
        {
          method: "POST",
          body: {
            expectedVersion: detail.batch.version,
            tenderTotals,
            denominations: denominationRows,
            attachments: [],
          },
        },
      );
    }, "Independent count confirmation recorded.");
  }
  function cashTotal(methodId: string) {
    return DENOMINATIONS.reduce(
      (sum, value) =>
        sum + value * (denominations[`${methodId}:${value}`] || 0),
      0,
    );
  }

  return (
    <div>
      <PageHeader
        title="Counting & approvals"
        subtitle="Capture offline offerings with assigned counters, independent declarations, locked entries and separation-of-duties posting."
      />
      {error && (
        <ErrorBox message={error} onRetry={() => void loadFoundation()} />
      )}
      {notice && (
        <p
          className="mb-5 rounded-xl bg-emerald-500/10 px-4 py-3 text-sm font-medium text-emerald-800"
          role="status"
        >
          {notice}
        </p>
      )}
      <div className="grid gap-6 xl:grid-cols-[22rem_minmax(0,1fr)]">
        <aside className="space-y-5">
          <Card className="p-5">
            <h2 className="font-bold">Open a counting batch</h2>
            <p className="mt-1 text-xs leading-5 text-[var(--remi-muted)]">
              Two distinct counters are required. They must independently
              confirm identical totals.
            </p>
            <form className="mt-5 grid gap-4" onSubmit={createBatch}>
              <label className="admin-field">
                <span>Branch</span>
                <Select
                  required
                  value={branchId}
                  onChange={(event) => setBranchId(event.target.value)}
                >
                  {campuses.map((campus) => (
                    <option value={campus.branchId} key={campus.branchId}>
                      {campus.name}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="admin-field">
                <span>Received date and time</span>
                <DateTimePicker
                  required
                  value={create.receivedAt}
                  onChange={(receivedAt) =>
                    setCreate({ ...create, receivedAt })
                  }
                />
              </label>
              <label className="admin-field">
                <span>First counter</span>
                <Select
                  required
                  value={create.counterOne}
                  onChange={(event) =>
                    setCreate({ ...create, counterOne: event.target.value })
                  }
                >
                  <option value="">Choose counter</option>
                  {owners.map((owner) => (
                    <option
                      disabled={owner.id === create.counterTwo}
                      value={owner.id}
                      key={owner.id}
                    >
                      {owner.name || owner.email} · {owner.role}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="admin-field">
                <span>Second counter</span>
                <Select
                  required
                  value={create.counterTwo}
                  onChange={(event) =>
                    setCreate({ ...create, counterTwo: event.target.value })
                  }
                >
                  <option value="">Choose counter</option>
                  {owners.map((owner) => (
                    <option
                      disabled={owner.id === create.counterOne}
                      value={owner.id}
                      key={owner.id}
                    >
                      {owner.name || owner.email} · {owner.role}
                    </option>
                  ))}
                </Select>
              </label>
              <fieldset>
                <legend className="text-xs font-bold">Expected tenders</legend>
                <div className="mt-2 grid gap-2">
                  {offlineMethods.map((method) => (
                    <label
                      className="flex items-center gap-3 rounded-xl bg-black/[.025] p-3 text-sm"
                      key={method.id}
                    >
                      <input
                        type="checkbox"
                        checked={create.methods.includes(method.id)}
                        onChange={(event) =>
                          setCreate({
                            ...create,
                            methods: event.target.checked
                              ? [...create.methods, method.id]
                              : create.methods.filter((id) => id !== method.id),
                          })
                        }
                      />
                      {method.name}
                      <small className="ml-auto text-[var(--remi-muted)]">
                        {method.kind}
                      </small>
                    </label>
                  ))}
                </div>
              </fieldset>
              <button
                disabled={
                  busy ||
                  loading ||
                  create.methods.length === 0 ||
                  !create.counterOne ||
                  !create.counterTwo ||
                  create.counterOne === create.counterTwo
                }
                className="admin-primary-button"
              >
                {busy ? "Opening…" : "Open dual-control batch"}
              </button>
            </form>
          </Card>
          <Card className="overflow-hidden">
            <header className="border-b border-black/[.045] p-5">
              <h2 className="font-bold">Recent batches</h2>
              <p className="mt-1 text-xs text-[var(--remi-muted)]">
                Scoped to the selected branch
              </p>
            </header>
            <div className="max-h-[34rem] overflow-auto p-2">
              {batches.map((batch) => (
                <button
                  key={batch.id}
                  onClick={() => setSelectedId(batch.id)}
                  className={`mb-1 w-full rounded-xl p-3 text-left ${selectedId === batch.id ? "bg-[var(--remi-gold)]/14" : "hover:bg-black/[.025]"}`}
                >
                  <div className="flex justify-between gap-3">
                    <strong className="text-sm">
                      {new Date(batch.receivedAt).toLocaleDateString("en-GH", {
                        day: "numeric",
                        month: "short",
                      })}
                    </strong>
                    <span className="font-mono text-[9px] uppercase tracking-wider text-[var(--remi-gold-deep)]">
                      {batch.state}
                    </span>
                  </div>
                  <small className="mt-1 block text-[var(--remi-muted)]">
                    {batch.entryCount} entries ·{" "}
                    {money(batch.enteredTotal.amountMinor)}
                  </small>
                </button>
              ))}
              {!loading && batches.length === 0 && (
                <div className="p-3">
                  <EmptyState
                    dense
                    title="No counting batches"
                    hint="Open the first dual-control count for this branch."
                  />
                </div>
              )}
            </div>
          </Card>
        </aside>

        <main className="min-w-0">
          {!detail ? (
            <EmptyState
              title="Choose or open a batch"
              hint="The entry register, independent counts and approval history will appear here."
            />
          ) : (
            <div className="space-y-5">
              <BatchHeader batch={detail.batch} methods={selectedMethods} />
              <div className="grid gap-5 lg:grid-cols-[1fr_.9fr]">
                <Card className="overflow-hidden">
                  <Panel
                    title="Contribution entries"
                    note={`${detail.entries.length} locked only after confirmation`}
                  />
                  {detail.batch.state === "open" && (
                    <div className="p-5">
                      <button
                        disabled={busy}
                        onClick={() => void transition("start-counting")}
                        className="admin-primary-button"
                      >
                        Start counting as an assigned counter
                      </button>
                    </div>
                  )}
                  {detail.batch.state === "counting" &&
                    detail.confirmations.length === 0 && (
                      <form className="grid gap-4 p-5" onSubmit={addEntry}>
                        <div
                          className="grid grid-cols-3 gap-2"
                          role="group"
                          aria-label="Donor attribution"
                        >
                          {["anonymous", "person", "household"].map((type) => (
                            <button
                              type="button"
                              aria-pressed={entry.donorType === type}
                              onClick={() =>
                                setEntry({
                                  ...entry,
                                  donorType: type,
                                  subject: null,
                                  query: "",
                                })
                              }
                              className={`rounded-xl px-3 py-2 text-xs font-bold capitalize ${entry.donorType === type ? "bg-[var(--remi-green)] text-white" : "bg-black/[.035]"}`}
                              key={type}
                            >
                              {type}
                            </button>
                          ))}
                        </div>
                        {entry.donorType !== "anonymous" && (
                          <label className="admin-field relative">
                            <span>
                              {entry.donorType === "person"
                                ? "Member"
                                : "Household"}
                            </span>
                            <input
                              autoComplete="off"
                              placeholder="Search by name or member number"
                              value={entry.subject?.name || entry.query}
                              onChange={(event) =>
                                setEntry({
                                  ...entry,
                                  subject: null,
                                  query: event.target.value,
                                })
                              }
                            />
                            {!entry.subject &&
                              (searching || subjects.length > 0) && (
                                <div className="absolute left-0 right-0 top-full z-20 mt-1 rounded-xl border border-black/[.06] bg-[var(--remi-surface)] p-1 shadow-xl">
                                  {searching ? (
                                    <p className="p-3 text-xs text-[var(--remi-muted)]">
                                      Searching…
                                    </p>
                                  ) : (
                                    subjects.map((subject) => (
                                      <button
                                        type="button"
                                        className="block w-full rounded-lg p-3 text-left text-sm hover:bg-black/[.035]"
                                        onClick={() =>
                                          setEntry({
                                            ...entry,
                                            subject,
                                            query: subject.name,
                                          })
                                        }
                                        key={subject.id}
                                      >
                                        {subject.name}
                                        <small className="ml-2 font-mono text-[9px] uppercase text-[var(--remi-muted)]">
                                          {subject.type}
                                        </small>
                                      </button>
                                    ))
                                  )}
                                </div>
                              )}
                          </label>
                        )}
                        <div className="grid gap-4 sm:grid-cols-2">
                          <label className="admin-field">
                            <span>Payment method</span>
                            <Select
                              required
                              value={entry.paymentMethodId}
                              onChange={(event) =>
                                setEntry({
                                  ...entry,
                                  paymentMethodId: event.target.value,
                                })
                              }
                            >
                              {selectedMethods.map((method) => (
                                <option value={method.id} key={method.id}>
                                  {method.name}
                                </option>
                              ))}
                            </Select>
                          </label>
                          <label className="admin-field">
                            <span>Amount (GHS)</span>
                            <input
                              required
                              type="number"
                              min="0.01"
                              step="0.01"
                              value={entry.amount}
                              onChange={(event) =>
                                setEntry({
                                  ...entry,
                                  amount: event.target.value,
                                })
                              }
                            />
                          </label>
                          <label className="admin-field">
                            <span>Fund</span>
                            <Select
                              required
                              value={entry.fundId}
                              onChange={(event) =>
                                setEntry({
                                  ...entry,
                                  fundId: event.target.value,
                                })
                              }
                            >
                              {funds.map((fund) => (
                                <option value={fund.id} key={fund.id}>
                                  {fund.name}
                                </option>
                              ))}
                            </Select>
                          </label>
                          <label className="admin-field">
                            <span>Tender reference</span>
                            <input
                              value={entry.reference}
                              onChange={(event) =>
                                setEntry({
                                  ...entry,
                                  reference: event.target.value,
                                })
                              }
                              placeholder="Cheque or transfer reference"
                            />
                          </label>
                        </div>
                        <button
                          disabled={
                            busy ||
                            !entry.amount ||
                            (entry.donorType !== "anonymous" && !entry.subject)
                          }
                          className="admin-primary-button"
                        >
                          Add contribution entry
                        </button>
                      </form>
                    )}
                  {detail.entries.length > 0 && (
                    <div className="divide-y divide-black/[.045] border-t border-black/[.045]">
                      {detail.entries.map((item) => (
                        <article
                          className="grid grid-cols-[1fr_auto] gap-3 p-4"
                          key={item.id}
                        >
                          <div>
                            <strong className="text-sm capitalize">
                              {item.donor.type} gift
                            </strong>
                            <small className="mt-1 block text-[var(--remi-muted)]">
                              {
                                methods.find(
                                  (method) =>
                                    method.id === item.paymentMethodId,
                                )?.name
                              }{" "}
                              ·{" "}
                              {
                                funds.find(
                                  (fund) => fund.id === item.splits[0]?.fundId,
                                )?.name
                              }
                            </small>
                          </div>
                          <b className="font-mono text-sm">
                            {money(item.total.amountMinor)}
                          </b>
                        </article>
                      ))}
                    </div>
                  )}
                </Card>

                <div className="space-y-5">
                  <Card className="overflow-hidden">
                    <Panel
                      title="Independent count"
                      note={`${detail.confirmations.length}/${detail.batch.dualControlRequired ? 2 : 1} confirmations`}
                    />
                    {detail.batch.state === "counting" && (
                      <div className="space-y-5 p-5">
                        {selectedMethods.map((method) => (
                          <section key={method.id}>
                            <div className="flex items-end justify-between gap-3">
                              <div>
                                <strong className="text-sm">
                                  {method.name}
                                </strong>
                                <small className="block text-[var(--remi-muted)]">
                                  {method.kind}
                                </small>
                              </div>
                              {method.kind === "cash" ? (
                                <b className="font-mono text-sm">
                                  {money(cashTotal(method.id))}
                                </b>
                              ) : (
                                <label className="admin-field max-w-40">
                                  <span>Declared GHS</span>
                                  <input
                                    type="number"
                                    min="0"
                                    step="0.01"
                                    value={tenders[method.id] || ""}
                                    onChange={(event) =>
                                      setTenders({
                                        ...tenders,
                                        [method.id]: event.target.value,
                                      })
                                    }
                                  />
                                </label>
                              )}
                            </div>
                            {method.kind === "cash" && (
                              <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
                                <span className="col-span-full text-[10px] font-bold uppercase tracking-wider text-[var(--remi-muted)]">
                                  Cash denominations
                                </span>
                                {DENOMINATIONS.map((value) => (
                                  <label
                                    className="rounded-lg bg-black/[.025] p-2"
                                    key={value}
                                  >
                                    <span className="block text-[10px] text-[var(--remi-muted)]">
                                      {money(value)}
                                    </span>
                                    <input
                                      aria-label={`${money(value)} note or coin count`}
                                      className="mt-1 w-full bg-transparent font-mono text-sm outline-none"
                                      type="number"
                                      min="0"
                                      step="1"
                                      value={
                                        denominations[
                                          `${method.id}:${value}`
                                        ] || ""
                                      }
                                      onChange={(event) =>
                                        setDenominations({
                                          ...denominations,
                                          [`${method.id}:${value}`]: Number(
                                            event.target.value,
                                          ),
                                        })
                                      }
                                    />
                                  </label>
                                ))}
                              </div>
                            )}
                          </section>
                        ))}
                        <button
                          disabled={busy || detail.entries.length === 0}
                          className="admin-primary-button w-full"
                          onClick={() => void confirmCount()}
                        >
                          Record my independent count
                        </button>
                        <p className="text-xs leading-5 text-[var(--remi-muted)]">
                          A different assigned counter must sign in and submit
                          the same tender totals. Entries lock after this
                          confirmation.
                        </p>
                      </div>
                    )}
                    {detail.confirmations.length > 0 && (
                      <div className="divide-y divide-black/[.045]">
                        {detail.confirmations.map((confirmation) => (
                          <div
                            className="flex justify-between gap-3 p-4"
                            key={confirmation.id}
                          >
                            <span>
                              <strong className="block text-sm">
                                {owners.find(
                                  (owner) =>
                                    owner.id === confirmation.counterId,
                                )?.name || "Assigned counter"}
                              </strong>
                              <small className="text-[var(--remi-muted)]">
                                {new Date(
                                  confirmation.confirmedAt,
                                ).toLocaleString("en-GH")}
                              </small>
                            </span>
                            <b className="font-mono text-sm">
                              {money(confirmation.declaredTotal.amountMinor)}
                            </b>
                          </div>
                        ))}
                      </div>
                    )}
                    {["counting", "counted"].includes(detail.batch.state) &&
                      detail.confirmations.length > 0 && (
                        <div className="border-t border-black/[.045] p-4">
                          <label className="admin-field">
                            <span>Reset reason</span>
                            <input
                              value={reason}
                              onChange={(event) =>
                                setReason(event.target.value)
                              }
                            />
                          </label>
                          <button
                            disabled={busy || reason.trim().length < 3}
                            onClick={() => void transition("reset-count")}
                            className="admin-secondary-button mt-3"
                          >
                            Reset confirmations
                          </button>
                        </div>
                      )}
                  </Card>

                  {detail.batch.state === "counted" && (
                    <Card className="p-5">
                      <h3 className="font-bold">Approval review</h3>
                      <p className="mt-2 text-sm text-[var(--remi-muted)]">
                        The approver must not be either counter. A variance
                        requires a documented reason.
                      </p>
                      <label className="admin-field mt-4">
                        <span>
                          {detail.batch.variance.amountMinor
                            ? "Required variance reason"
                            : "Approval note (optional)"}
                        </span>
                        <textarea
                          rows={3}
                          value={reason}
                          onChange={(event) => setReason(event.target.value)}
                        />
                      </label>
                      <button
                        disabled={
                          busy ||
                          (detail.batch.variance.amountMinor !== 0 &&
                            reason.trim().length < 3)
                        }
                        onClick={() => void transition("approve")}
                        className="admin-primary-button mt-4"
                      >
                        Approve independently
                      </button>
                    </Card>
                  )}
                  {detail.batch.state === "approved" && (
                    <Card className="border border-[var(--remi-gold)]/25 p-5">
                      <h3 className="font-bold">Post immutable gifts</h3>
                      <p className="mt-2 text-sm leading-6 text-[var(--remi-muted)]">
                        Posting allocates receipt numbers and creates one
                        immutable contribution per locked entry. Recent MFA and
                        a non-counter approver are required.
                      </p>
                      <button
                        disabled={busy}
                        onClick={() => void transition("post")}
                        className="admin-primary-button mt-4"
                      >
                        Post batch to ledger
                      </button>
                    </Card>
                  )}
                </div>
              </div>
              <Card className="overflow-hidden">
                <Panel
                  title="Audit trail"
                  note={`${detail.events.length} state events`}
                />
                <div className="divide-y divide-black/[.045]">
                  {detail.events.map((item) => (
                    <article
                      className="grid gap-1 p-4 sm:grid-cols-[1fr_auto]"
                      key={item.id}
                    >
                      <div>
                        <strong className="text-sm">
                          {item.type.replaceAll(".", " · ")}
                        </strong>
                        {item.reason && (
                          <p className="mt-1 text-xs text-[var(--remi-muted)]">
                            {item.reason}
                          </p>
                        )}
                      </div>
                      <time className="font-mono text-[10px] text-[var(--remi-muted)]">
                        {new Date(item.occurredAt).toLocaleString("en-GH")}
                      </time>
                    </article>
                  ))}
                </div>
              </Card>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

function Panel({ title, note }: { title: string; note: string }) {
  return (
    <header className="flex items-end justify-between gap-4 border-b border-black/[.045] p-5">
      <h2 className="font-bold">{title}</h2>
      <small className="text-right text-[var(--remi-muted)]">{note}</small>
    </header>
  );
}
function BatchHeader({ batch, methods }: { batch: Batch; methods: Method[] }) {
  return (
    <section className="grid gap-5 rounded-2xl border border-white/10 bg-[var(--remi-green)] p-6 text-white [background-image:radial-gradient(circle_at_90%_0%,rgba(209,173,85,.18),transparent_34%)] md:grid-cols-[1fr_auto]">
      <div>
        <span className="font-mono text-[10px] uppercase tracking-[.17em] text-[var(--altar-mint)]">
          {batch.state} · version {batch.version}
        </span>
        <h1 className="mt-2 text-2xl font-bold">
          Count received{" "}
          {new Date(batch.receivedAt).toLocaleString("en-GH", {
            dateStyle: "medium",
            timeStyle: "short",
          })}
        </h1>
        <p className="mt-2 text-sm text-white/50">
          {methods.map((method) => method.name).join(" · ")} · dual control
          required
        </p>
      </div>
      <div className="grid grid-cols-3 gap-px overflow-hidden rounded-xl border border-white/10 bg-white/10">
        <Stat
          label="Entries"
          value={String(batch.entryCount).padStart(2, "0")}
        />
        <Stat label="Entered" value={money(batch.enteredTotal.amountMinor)} />
        <Stat label="Variance" value={money(batch.variance.amountMinor)} />
      </div>
    </section>
  );
}
function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-24 bg-white/[.045] p-3 text-center">
      <b className="block font-mono text-sm">{value}</b>
      <span className="mt-1 block text-[8px] uppercase tracking-wider text-white/35">
        {label}
      </span>
    </div>
  );
}
