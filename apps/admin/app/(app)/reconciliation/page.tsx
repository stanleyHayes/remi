"use client";

import {
  ChangeEvent,
  DragEvent,
  FormEvent,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { ApiError, api, asList } from "@/lib/api";
import { Card, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import DatePicker from "@/components/ui/DatePicker";

type Money = { amountMinor: number; currency: string };
type Period = {
  id: string;
  version: number;
  code: string;
  name: string;
  startsAt: string;
  endsAt: string;
  status: string;
};
type Campus = { id: string; branchId: string; name: string };
type Settlement = {
  id: string;
  branchId: string;
  sourceType: string;
  sourceName: string;
  fileName: string;
  reference: string;
  settledAt: string;
  grossAmountMinor: number;
  feeAmountMinor: number;
  netAmountMinor: number;
  matchedGrossMinor: number;
  approvedVarianceMinor: number;
  unexplainedVarianceMinor: number;
  itemCount: number;
  unresolvedCount: number;
  state: string;
};
type Item = {
  id: string;
  sourceRowId: string;
  reference: string;
  occurredAt: string;
  amountMinor: number;
  feeAmountMinor: number;
  description?: string;
  suggestedTargetType?: string;
  suggestedTargetId?: string;
  suggestedAmountMinor?: number;
  confidence: string;
  evidence: string[];
  resolution: string;
  varianceMinor: number;
  reason?: string;
};
type Contribution = {
  id: string;
  receiptNumber: string;
  providerReference?: string;
  total: Money;
};
type Batch = {
  id: string;
  state: string;
  enteredTotal: Money;
  receivedAt: string;
};
type ControlRequest = {
  id: string;
  periodId: string;
  action: "close" | "reopen";
  state: string;
  reason: string;
  requestedBy: string;
  requestedAt: string;
  snapshot: Record<string, number>;
};
type ReconciliationOwner = {
  id: string;
  name: string;
  email: string;
  role: string;
};
type ParsedRow = {
  sourceRowId: string;
  reference: string;
  occurredAt: string;
  amountMinor: number;
  feeAmountMinor: number;
  description: string;
};
type UploadSignature = {
  cloudName: string;
  apiKey: string;
  timestamp: number;
  folder: string;
  signature: string;
  type: string;
};
const money = (minor: number) =>
  new Intl.NumberFormat("en-GH", { style: "currency", currency: "GHS" }).format(
    minor / 100,
  );

function parseCSV(text: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [],
    cell = "",
    quoted = false;
  for (let i = 0; i < text.length; i++) {
    const char = text[i];
    if (char === '"') {
      if (quoted && text[i + 1] === '"') {
        cell += '"';
        i++;
      } else quoted = !quoted;
    } else if (char === "," && !quoted) {
      row.push(cell);
      cell = "";
    } else if ((char === "\n" || char === "\r") && !quoted) {
      if (char === "\r" && text[i + 1] === "\n") i++;
      row.push(cell);
      if (row.some((value) => value.trim())) rows.push(row);
      row = [];
      cell = "";
    } else cell += char;
  }
  row.push(cell);
  if (row.some((value) => value.trim())) rows.push(row);
  return rows;
}
function rowsFromFile(text: string): ParsedRow[] {
  const values = parseCSV(text);
  if (values.length < 2)
    throw new Error("The CSV needs a header and at least one transaction row.");
  const header = values[0].map((value) =>
    value
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9]/g, ""),
  );
  const aliases: Record<string, string[]> = {
    sourceRowId: ["sourcerowid", "rowid", "id"],
    reference: ["reference", "providerreference", "depositreference"],
    occurredAt: ["occurredat", "date", "transactiondate"],
    amount: ["amount", "gross"],
    fee: ["fee", "fees"],
    description: ["description", "narration", "memo"],
  };
  const column = (name: string, required = true) => {
    const index = header.findIndex((value) => aliases[name].includes(value));
    if (index < 0 && required) throw new Error(`Missing ${name} column.`);
    return index;
  };
  const ids = new Set<string>();
  return values.slice(1).map((row, index) => {
    const sourceRowId = row[column("sourceRowId")]?.trim();
    if (!sourceRowId || ids.has(sourceRowId))
      throw new Error(`Row ${index + 2} has a missing or duplicate source ID.`);
    ids.add(sourceRowId);
    const occurredAt = new Date(row[column("occurredAt")] || "");
    const amount = Math.round(Number(row[column("amount")]) * 100);
    const feeIndex = column("fee", false),
      descriptionIndex = column("description", false);
    const fee = feeIndex < 0 ? 0 : Math.round(Number(row[feeIndex] || 0) * 100);
    if (
      !Number.isFinite(amount) ||
      amount <= 0 ||
      !Number.isFinite(fee) ||
      fee < 0 ||
      Number.isNaN(occurredAt.getTime())
    )
      throw new Error(`Row ${index + 2} has an invalid date, amount or fee.`);
    return {
      sourceRowId,
      reference: row[column("reference", false)]?.trim() || "",
      occurredAt: occurredAt.toISOString(),
      amountMinor: amount,
      feeAmountMinor: fee,
      description:
        descriptionIndex < 0 ? "" : row[descriptionIndex]?.trim() || "",
    };
  });
}

export default function ReconciliationPage() {
  const [periods, setPeriods] = useState<Period[]>([]),
    [campuses, setCampuses] = useState<Campus[]>([]),
    [settlements, setSettlements] = useState<Settlement[]>([]),
    [items, setItems] = useState<Item[]>([]),
    [contributions, setContributions] = useState<Contribution[]>([]),
    [batches, setBatches] = useState<Batch[]>([]),
    [owners, setOwners] = useState<ReconciliationOwner[]>([]),
    [requests, setRequests] = useState<ControlRequest[]>([]),
    [selected, setSelected] = useState(""),
    [error, setError] = useState<string | null>(null),
    [notice, setNotice] = useState(""),
    [busy, setBusy] = useState(false),
    [dragging, setDragging] = useState(false),
    [file, setFile] = useState<File | null>(null),
    [rows, setRows] = useState<ParsedRow[]>([]),
    [hash, setHash] = useState(""),
    [resolution, setResolution] = useState<
      Record<string, { target: string; reason: string; ownerId: string }>
    >({});
  const inputRef = useRef<HTMLInputElement>(null);
  const [form, setForm] = useState({
    branchId: "",
    periodId: "",
    sourceType: "paystack",
    sourceName: "Paystack Ghana",
    reference: "",
    settledAt: new Date().toISOString().slice(0, 10),
    otherDeduction: "0",
  });
  const [controlReason, setControlReason] = useState("");
  const load = useCallback(async () => {
    setError(null);
    try {
      const [p, c, s, o] = await Promise.all([
        api("/api/chms/v1/finance/periods"),
        api("/api/chms/v1/finance/campuses"),
        api("/api/chms/v1/finance/settlements"),
        api("/api/chms/v1/finance/reconciliation-owners"),
      ]);
      const nextPeriods = asList<Period>(p),
        nextCampuses = asList<Campus>(c),
        nextSettlements = asList<Settlement>(s);
      setPeriods(nextPeriods);
      setCampuses(nextCampuses);
      setSettlements(nextSettlements);
      setOwners(asList<ReconciliationOwner>(o));
      setSelected((value) => value || nextSettlements[0]?.id || "");
      setForm((value) => ({
        ...value,
        periodId:
          value.periodId ||
          nextPeriods.find((item) => item.status === "open")?.id ||
          nextPeriods[0]?.id ||
          "",
        branchId: value.branchId || nextCampuses[0]?.branchId || "",
      }));
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Reconciliation could not be loaded.",
      );
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load]);
  useEffect(() => {
    if (!selected) {
      setItems([]);
      return;
    }
    const branch =
      settlements.find((value) => value.id === selected)?.branchId ||
      form.branchId;
    if (!branch) return;
    void Promise.all([
      api(`/api/chms/v1/finance/settlements/${selected}/items`),
      api(
        `/api/chms/v1/finance/contributions?branchId=${encodeURIComponent(branch)}&limit=100`,
      ),
      api(
        `/api/chms/v1/finance/batches?branchId=${encodeURIComponent(branch)}&limit=100`,
      ),
    ])
      .then(([i, c, b]) => {
        setItems(asList<Item>(i));
        setContributions(asList<Contribution>(c));
        setBatches(asList<Batch>(b));
      })
      .catch((cause) =>
        setError(
          cause instanceof ApiError
            ? cause.message
            : "Settlement detail could not be loaded.",
        ),
      );
  }, [selected, settlements, form.branchId]);
  useEffect(() => {
    if (!form.periodId) {
      setRequests([]);
      return;
    }
    void api(
      `/api/chms/v1/finance/period-control-requests?periodId=${form.periodId}`,
    )
      .then((value) => setRequests(asList<ControlRequest>(value)))
      .catch(() => setRequests([]));
  }, [form.periodId, notice]);
  const totals = useMemo(
    () => ({
      gross: rows.reduce((sum, row) => sum + row.amountMinor, 0),
      fees: rows.reduce((sum, row) => sum + row.feeAmountMinor, 0),
    }),
    [rows],
  );
  async function acceptFile(next?: File) {
    if (!next) return;
    if (
      next.size > 10 * 1024 * 1024 ||
      (!next.name.toLowerCase().endsWith(".csv") && !next.type.includes("csv"))
    ) {
      setError("Choose a CSV file smaller than 10 MB.");
      return;
    }
    try {
      const text = await next.text();
      const parsed = rowsFromFile(text);
      const digest = await crypto.subtle.digest(
        "SHA-256",
        await next.arrayBuffer(),
      );
      setFile(next);
      setRows(parsed);
      setHash(
        Array.from(new Uint8Array(digest), (value) =>
          value.toString(16).padStart(2, "0"),
        ).join(""),
      );
      setError(null);
    } catch (cause) {
      setFile(null);
      setRows([]);
      setError(
        cause instanceof Error ? cause.message : "CSV could not be parsed.",
      );
    }
  }
  async function uploadPrivate(file: File) {
    const signed = await api<UploadSignature>("/api/admin/uploads/signature", {
      method: "POST",
      body: { purpose: "finance-settlement" },
    });
    const body = new FormData();
    body.append("file", file);
    body.append("api_key", signed.apiKey);
    body.append("timestamp", String(signed.timestamp));
    body.append("folder", signed.folder);
    body.append("signature", signed.signature);
    body.append("type", signed.type);
    const response = await fetch(
      `https://api.cloudinary.com/v1_1/${signed.cloudName}/raw/upload`,
      { method: "POST", body },
    );
    const result = (await response.json()) as {
      public_id?: string;
      error?: { message?: string };
    };
    if (!response.ok || !result.public_id)
      throw new Error(
        result.error?.message || "Private evidence upload failed.",
      );
    return result.public_id;
  }
  async function submit(event: FormEvent) {
    event.preventDefault();
    if (!file || !rows.length) return;
    setBusy(true);
    setError(null);
    try {
      const privateAssetId = await uploadPrivate(file);
      const other = Math.round(Number(form.otherDeduction) * 100);
      const created = await api<Settlement>(
        "/api/chms/v1/finance/settlements",
        {
          method: "POST",
          body: {
            ...form,
            settledAt: new Date(`${form.settledAt}T12:00:00Z`).toISOString(),
            fileName: file.name,
            fileHash: hash,
            privateAssetId,
            currency: "GHS",
            grossAmountMinor: totals.gross,
            feeAmountMinor: totals.fees,
            otherDeductionMinor: other,
            netAmountMinor: totals.gross - totals.fees - other,
            rows,
          },
        },
      );
      setNotice("Settlement imported with deterministic match suggestions.");
      setFile(null);
      setRows([]);
      setSelected(created.id);
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError || cause instanceof Error
          ? cause.message
          : "Settlement could not be imported.",
      );
    } finally {
      setBusy(false);
    }
  }
  const targets = [
    ...contributions.map((value) => ({
      value: `contribution:${value.id}`,
      label: `Receipt ${value.receiptNumber} · ${money(value.total.amountMinor)}`,
    })),
    ...batches.map((value) => ({
      value: `batch:${value.id}`,
      label: `Batch ${value.id.slice(-7)} · ${money(value.enteredTotal.amountMinor)}`,
    })),
  ];
  async function resolve(item: Item, exact = false) {
    const draft = resolution[item.id] || {
      target:
        item.suggestedTargetType && item.suggestedTargetId
          ? `${item.suggestedTargetType}:${item.suggestedTargetId}`
          : "",
      reason: "",
      ownerId: "",
    };
    const [targetType, targetId] = draft.target.split(":");
    if (!targetId) return;
    const isSuggestion =
      targetType === item.suggestedTargetType &&
      targetId === item.suggestedTargetId;
    const targetAmount =
      isSuggestion && item.suggestedAmountMinor !== undefined
        ? item.suggestedAmountMinor
        : targetType === "contribution"
          ? contributions.find((value) => value.id === targetId)?.total
              .amountMinor
          : batches.find((value) => value.id === targetId)?.enteredTotal
              .amountMinor;
    const action =
      exact || targetAmount === item.amountMinor ? "match" : "approve-variance";
    setBusy(true);
    setError(null);
    try {
      await api(
        `/api/chms/v1/finance/reconciliation-items/${item.id}/resolve`,
        {
          method: "POST",
          body: { action, targetType, targetId, reason: draft.reason },
        },
      );
      setNotice(
        action === "match"
          ? "Exact settlement match recorded."
          : "Variance resolution recorded with its reason.",
      );
      const value = await api(
        `/api/chms/v1/finance/settlements/${selected}/items`,
      );
      setItems(asList<Item>(value));
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Resolution could not be recorded.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function assignException(item: Item) {
    const draft = resolution[item.id] || {
      target: "",
      reason: "",
      ownerId: "",
    };
    if (!draft.ownerId || draft.reason.trim().length < 10) {
      setError(
        "Choose an active finance owner and enter a detailed exception reason.",
      );
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api(
        `/api/chms/v1/finance/reconciliation-items/${item.id}/resolve`,
        {
          method: "POST",
          body: {
            action: "assign-exception",
            ownerId: draft.ownerId,
            reason: draft.reason,
          },
        },
      );
      setNotice(
        "Exception assigned with an accountable owner and audit reason.",
      );
      const value = await api(
        `/api/chms/v1/finance/settlements/${selected}/items`,
      );
      setItems(asList<Item>(value));
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Exception could not be assigned.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function control(
    period: Period,
    action: "close" | "reopen",
    requestId?: string,
  ) {
    if (controlReason.trim().length < 10) {
      setError("Enter a detailed control reason of at least 10 characters.");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api(`/api/chms/v1/finance/periods/${period.id}/${action}`, {
        method: "POST",
        body: {
          action: requestId ? "approve" : "request",
          requestId,
          reason: controlReason,
          expectedVersion: requestId ? period.version : undefined,
        },
      });
      setNotice(
        requestId
          ? `Period ${action} approved.`
          : `Period ${action} request submitted for independent approval.`,
      );
      setControlReason("");
      await load();
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Period control could not be completed.",
      );
    } finally {
      setBusy(false);
    }
  }
  const active = settlements.find((value) => value.id === selected),
    period = periods.find((value) => value.id === form.periodId);
  return (
    <div>
      <PageHeader
        title="Settlement reconciliation"
        subtitle="Import private bank or processor evidence, resolve every variance and close periods with independent approval."
      />
      {error && <ErrorBox message={error} onRetry={load} />}{" "}
      {notice && (
        <div
          className="mb-5 rounded-xl bg-emerald-500/10 p-4 text-sm text-emerald-800"
          role="status"
        >
          {notice}
        </div>
      )}
      <div className="grid gap-6 xl:grid-cols-[.82fr_1.18fr]">
        <Card className="p-5">
          <span className="font-mono text-[10px] uppercase tracking-[.18em] text-amber-700">
            Private import
          </span>
          <h2 className="mt-2 text-lg font-semibold">Stage a settlement</h2>
          <p className="mt-1 text-sm text-zinc-500">
            CSV headers: rowId, reference, date, amount, fee, description.
            Amounts use GHS.
          </p>
          <form onSubmit={submit} className="mt-5 grid gap-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <label className="admin-field">
                <span>Source</span>
                <Select
                  value={form.sourceType}
                  onChange={(event) =>
                    setForm({
                      ...form,
                      sourceType: event.target.value,
                      sourceName:
                        event.target.value === "paystack"
                          ? "Paystack Ghana"
                          : event.target.value === "bank"
                            ? "Church bank"
                            : "Physical deposit",
                    })
                  }
                >
                  <option value="paystack">Paystack</option>
                  <option value="bank">Bank statement</option>
                  <option value="deposit">Physical deposit</option>
                </Select>
              </label>
              <label className="admin-field">
                <span>Source name</span>
                <input
                  required
                  value={form.sourceName}
                  onChange={(event) =>
                    setForm({ ...form, sourceName: event.target.value })
                  }
                />
              </label>
              <label className="admin-field">
                <span>Branch</span>
                <Select
                  required
                  value={form.branchId}
                  onChange={(event) =>
                    setForm({ ...form, branchId: event.target.value })
                  }
                >
                  {campuses.map((value) => (
                    <option value={value.branchId} key={value.id}>
                      {value.name}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="admin-field">
                <span>Fiscal period</span>
                <Select
                  required
                  value={form.periodId}
                  onChange={(event) =>
                    setForm({ ...form, periodId: event.target.value })
                  }
                >
                  {periods.map((value) => (
                    <option value={value.id} key={value.id}>
                      {value.name} · {value.status}
                    </option>
                  ))}
                </Select>
              </label>
              <label className="admin-field">
                <span>Settlement reference</span>
                <input
                  required
                  value={form.reference}
                  onChange={(event) =>
                    setForm({ ...form, reference: event.target.value })
                  }
                />
              </label>
              <label className="admin-field">
                <span>Settlement date</span>
                <DatePicker
                  value={form.settledAt}
                  onChange={(settledAt) => setForm({ ...form, settledAt })}
                />
              </label>
              <label className="admin-field">
                <span>Other deductions (GHS)</span>
                <input
                  min="0"
                  step="0.01"
                  type="number"
                  value={form.otherDeduction}
                  onChange={(event) =>
                    setForm({ ...form, otherDeduction: event.target.value })
                  }
                />
              </label>
            </div>
            <div
              className={`admin-upload-zone rounded-xl border border-dashed p-5 ${dragging ? "is-dragging" : ""}`}
              onDragOver={(event: DragEvent) => {
                event.preventDefault();
                setDragging(true);
              }}
              onDragLeave={() => setDragging(false)}
              onDrop={(event: DragEvent) => {
                event.preventDefault();
                setDragging(false);
                void acceptFile(event.dataTransfer.files[0]);
              }}
            >
              <input
                ref={inputRef}
                className="sr-only"
                type="file"
                accept=".csv,text/csv"
                onChange={(event: ChangeEvent<HTMLInputElement>) =>
                  void acceptFile(event.target.files?.[0])
                }
              />
              <button
                type="button"
                className="admin-secondary-button"
                onClick={() => inputRef.current?.click()}
              >
                {file ? "Replace CSV" : "Choose CSV"}
              </button>
              <strong className="ml-3 text-sm">
                {file?.name || "or drag and drop here"}
              </strong>
              {rows.length > 0 && (
                <p className="mt-3 text-xs text-zinc-500">
                  {rows.length} rows · {money(totals.gross)} gross ·{" "}
                  {money(totals.fees)} fees · SHA-256 verified
                </p>
              )}
            </div>
            <button
              disabled={busy || !rows.length || !form.reference}
              className="admin-primary-button"
            >
              {busy ? "Encrypting evidence…" : "Import and suggest matches"}
            </button>
          </form>
        </Card>
        <Card className="overflow-hidden">
          <div className="border-b border-black/[.045] p-5">
            <span className="font-mono text-[10px] uppercase tracking-[.18em] text-amber-700">
              Review queue
            </span>
            <div className="mt-2 flex flex-wrap items-end justify-between gap-3">
              <div>
                <h2 className="text-lg font-semibold">Settlement evidence</h2>
                <p className="mt-1 text-sm text-zinc-500">
                  Suggestions are evidence, never automatic decisions.
                </p>
              </div>
              <Select
                className="admin-select min-w-56"
                value={selected}
                onChange={(event) => setSelected(event.target.value)}
              >
                {settlements.map((value) => (
                  <option value={value.id} key={value.id}>
                    {value.reference} · {value.state}
                  </option>
                ))}
              </Select>
            </div>
            {active && (
              <div className="mt-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
                <Metric label="Gross" value={money(active.grossAmountMinor)} />
                <Metric
                  label="Matched"
                  value={money(active.matchedGrossMinor)}
                />
                <Metric
                  label="Unexplained"
                  value={money(active.unexplainedVarianceMinor)}
                />
                <Metric
                  label="Open rows"
                  value={String(active.unresolvedCount)}
                />
              </div>
            )}
          </div>
          <div className="divide-y divide-black/[.045]">
            {items.length ? (
              items.map((item) => {
                const suggested =
                  item.suggestedTargetType && item.suggestedTargetId
                    ? `${item.suggestedTargetType}:${item.suggestedTargetId}`
                    : "";
                const draft = resolution[item.id] || {
                  target: suggested,
                  reason: "",
                  ownerId: "",
                };
                return (
                  <article className="grid gap-3 p-5" key={item.id}>
                    <div className="flex flex-wrap justify-between gap-3">
                      <div>
                        <span
                          className={`rounded-full px-2 py-1 font-mono text-[9px] uppercase tracking-wider ${item.confidence === "exact" ? "bg-emerald-500/10 text-emerald-700" : "bg-amber-500/10 text-amber-700"}`}
                        >
                          {item.resolution === "unresolved"
                            ? `${item.confidence} suggestion`
                            : item.resolution}
                        </span>
                        <strong className="mt-2 block">
                          {item.reference || item.sourceRowId}
                        </strong>
                        <small className="text-zinc-500">
                          {new Date(item.occurredAt).toLocaleDateString(
                            "en-GH",
                          )}{" "}
                          ·{" "}
                          {item.evidence.join(" + ") ||
                            "No deterministic evidence"}
                        </small>
                      </div>
                      <b className="font-mono">{money(item.amountMinor)}</b>
                    </div>
                    {item.resolution === "unresolved" && (
                      <div className="grid gap-3">
                        <div className="grid gap-2 sm:grid-cols-2">
                          <Select
                            value={draft.target}
                            onChange={(event) =>
                              setResolution({
                                ...resolution,
                                [item.id]: {
                                  ...draft,
                                  target: event.target.value,
                                },
                              })
                            }
                          >
                            <option value="">Choose ledger target</option>
                            {targets.map((target) => (
                              <option key={target.value} value={target.value}>
                                {target.label}
                              </option>
                            ))}
                          </Select>
                          <Select
                            aria-label="Exception owner"
                            value={draft.ownerId}
                            onChange={(event) =>
                              setResolution({
                                ...resolution,
                                [item.id]: {
                                  ...draft,
                                  ownerId: event.target.value,
                                },
                              })
                            }
                          >
                            <option value="">Choose exception owner</option>
                            {owners.map((owner) => (
                              <option value={owner.id} key={owner.id}>
                                {owner.name || owner.email} · {owner.role}
                              </option>
                            ))}
                          </Select>
                        </div>
                        <input
                          placeholder="Required variance or exception reason"
                          value={draft.reason}
                          onChange={(event) =>
                            setResolution({
                              ...resolution,
                              [item.id]: {
                                ...draft,
                                reason: event.target.value,
                              },
                            })
                          }
                        />
                        <div className="flex flex-wrap gap-2">
                          <button
                            disabled={busy || !draft.target}
                            className="admin-secondary-button"
                            onClick={() =>
                              void resolve(item, item.confidence === "exact")
                            }
                          >
                            {item.confidence === "exact"
                              ? "Accept exact match"
                              : "Resolve variance"}
                          </button>
                          <button
                            disabled={
                              busy ||
                              !draft.ownerId ||
                              draft.reason.trim().length < 10
                            }
                            className="admin-secondary-button"
                            onClick={() => void assignException(item)}
                          >
                            Assign exception
                          </button>
                        </div>
                      </div>
                    )}
                  </article>
                );
              })
            ) : (
              <p className="p-8 text-sm text-zinc-500">
                Choose or import a settlement to review its rows.
              </p>
            )}
          </div>
        </Card>
      </div>
      <Card className="mt-6 p-5">
        <span className="font-mono text-[10px] uppercase tracking-[.18em] text-amber-700">
          Period control
        </span>
        <div className="mt-2 grid gap-5 lg:grid-cols-[1fr_.8fr]">
          <div>
            <h2 className="text-lg font-semibold">
              Close and reopen with dual control
            </h2>
            <p className="mt-1 text-sm text-zinc-500">
              The requester cannot approve the same action. Close is blocked by
              unexplained reconciliation work.
            </p>
            <label className="admin-field mt-4">
              <span>Control reason</span>
              <textarea
                rows={2}
                value={controlReason}
                onChange={(event) => setControlReason(event.target.value)}
                placeholder="Document the review, impact or late evidence…"
              />
            </label>
            <div className="mt-3 flex flex-wrap gap-2">
              {period && (
                <button
                  className="admin-primary-button"
                  disabled={busy}
                  onClick={() =>
                    void control(
                      period,
                      period.status === "open" ? "close" : "reopen",
                    )
                  }
                >
                  Request {period.status === "open" ? "close" : "reopen"}
                </button>
              )}
            </div>
          </div>
          <div className="grid gap-2">
            {requests
              .filter((value) => value.state === "pending")
              .map((value) => (
                <article
                  className="rounded-xl border border-black/[.05] p-4"
                  key={value.id}
                >
                  <div className="flex justify-between gap-3">
                    <strong className="capitalize">
                      {value.action} requested
                    </strong>
                    <small>
                      {new Date(value.requestedAt).toLocaleDateString("en-GH")}
                    </small>
                  </div>
                  <p className="mt-2 text-sm text-zinc-500">{value.reason}</p>
                  {period && (
                    <button
                      className="admin-secondary-button mt-3"
                      disabled={busy}
                      onClick={() =>
                        void control(period, value.action, value.id)
                      }
                    >
                      Approve independently
                    </button>
                  )}
                </article>
              ))}
            {!requests.some((value) => value.state === "pending") && (
              <p className="text-sm text-zinc-500">
                No pending close or reopen approval.
              </p>
            )}
          </div>
        </div>
      </Card>
    </div>
  );
}
function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl bg-black/[.025] p-3">
      <small className="text-zinc-500">{label}</small>
      <strong className="mt-1 block font-mono text-sm">{value}</strong>
    </div>
  );
}
