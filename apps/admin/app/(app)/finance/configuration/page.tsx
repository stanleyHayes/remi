"use client";

import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Card, EmptyState, ErrorBox, PageHeader } from "@/components/ui";
import { Select } from "@/components/ui/Select";
import DatePicker from "@/components/ui/DatePicker";
import { api, ApiError } from "@/lib/api";

type Envelope = { id: string; version: number; branchId?: string };
type Fund = Envelope & {
  code: string;
  name: string;
  description?: string;
  restrictionType: string;
  activeFrom: string;
  activeUntil?: string;
  successorFundId?: string;
};
type Method = Envelope & {
  code: string;
  name: string;
  kind: string;
  provider?: string;
  active: boolean;
};
type Campus = Envelope & {
  branchId: string;
  name: string;
  timezone: string;
  currency: string;
};
type Period = Envelope & {
  code: string;
  name: string;
  startsAt: string;
  endsAt: string;
  status: string;
};
type Sequence = Envelope & {
  code: string;
  prefix: string;
  fiscalYear: number;
  padding: number;
  nextNumber: number;
};
type Mapping = Envelope & {
  fundId?: string;
  category: string;
  externalAccountCode: string;
  externalDimensionCode?: string;
  effectiveFrom: string;
  effectiveUntil?: string;
};
type PeriodRequest = {
  id: string;
  periodId: string;
  action: "close" | "reopen";
  state: string;
  reason: string;
  requestedBy: string;
  requestedAt: string;
  snapshot?: Record<string, unknown>;
};
type Branch = { _id?: string; id?: string; name?: string; title?: string };
type Tab =
  "funds" | "methods" | "campuses" | "periods" | "sequences" | "mappings";

const rows = <T,>(value: unknown): T[] =>
  Array.isArray(value)
    ? (value as T[])
    : value &&
        typeof value === "object" &&
        Array.isArray((value as { items?: unknown[] }).items)
      ? (value as { items: T[] }).items
      : [];
const dateOnly = (value?: string) => (value ? value.slice(0, 10) : "");
const iso = (value: string) =>
  value ? new Date(`${value}T00:00:00.000Z`).toISOString() : undefined;
const TABS: Array<{ id: Tab; label: string; copy: string }> = [
  { id: "funds", label: "Funds", copy: "Designations and restrictions" },
  { id: "methods", label: "Tenders", copy: "Accepted payment methods" },
  { id: "campuses", label: "Campuses", copy: "Branch finance settings" },
  { id: "periods", label: "Periods", copy: "Fiscal control windows" },
  { id: "sequences", label: "Receipts", copy: "Non-reusing number ranges" },
  { id: "mappings", label: "Mappings", copy: "External ledger codes" },
];

export default function FinanceConfigurationPage() {
  const [tab, setTab] = useState<Tab>("funds"),
    [funds, setFunds] = useState<Fund[]>([]),
    [methods, setMethods] = useState<Method[]>([]),
    [campuses, setCampuses] = useState<Campus[]>([]),
    [periods, setPeriods] = useState<Period[]>([]),
    [periodRequests, setPeriodRequests] = useState<PeriodRequest[]>([]),
    [sequences, setSequences] = useState<Sequence[]>([]),
    [mappings, setMappings] = useState<Mapping[]>([]),
    [branches, setBranches] = useState<Branch[]>([]);
  const [loading, setLoading] = useState(true),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string | null>(null),
    [notice, setNotice] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [fund, setFund] = useState({
    code: "",
    name: "",
    description: "",
    restrictionType: "unrestricted",
    activeFrom: dateOnly(new Date().toISOString()),
    activeUntil: "",
    successorFundId: "",
    expectedVersion: 0,
  });
  const [method, setMethod] = useState({
    code: "",
    name: "",
    kind: "cash",
    provider: "",
    active: true,
    expectedVersion: 0,
  });
  const [campus, setCampus] = useState({
    branchId: "",
    name: "",
    timezone: "Africa/Accra",
    currency: "GHS",
    expectedVersion: 0,
  });
  const [period, setPeriod] = useState({
    code: "",
    name: "",
    startsAt: "",
    endsAt: "",
    expectedVersion: 0,
  });
  const [sequence, setSequence] = useState({
    code: "",
    prefix: "REMI-",
    fiscalYear: new Date().getFullYear(),
    padding: 6,
    startingNumber: 1,
    expectedVersion: 0,
  });
  const [mapping, setMapping] = useState({
    branchId: "",
    fundId: "",
    category: "contribution",
    externalAccountCode: "",
    externalDimensionCode: "",
    effectiveFrom: dateOnly(new Date().toISOString()),
    effectiveUntil: "",
    expectedVersion: 0,
  });
  const [controlReason, setControlReason] = useState("");

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const [
        fundData,
        methodData,
        campusData,
        periodData,
        requestData,
        sequenceData,
        mappingData,
        branchData,
      ] = await Promise.all([
        api("/api/chms/v1/finance/funds"),
        api("/api/chms/v1/finance/payment-methods"),
        api("/api/chms/v1/finance/campuses"),
        api("/api/chms/v1/finance/periods"),
        api("/api/chms/v1/finance/period-control-requests"),
        api("/api/chms/v1/finance/receipt-sequences"),
        api("/api/chms/v1/finance/account-mappings"),
        api("/api/admin/branches?limit=100"),
      ]);
      const nextCampuses = rows<Campus>(campusData);
      setFunds(rows<Fund>(fundData));
      setMethods(rows<Method>(methodData));
      setCampuses(nextCampuses);
      setPeriods(rows<Period>(periodData));
      setPeriodRequests(rows<PeriodRequest>(requestData));
      setSequences(rows<Sequence>(sequenceData));
      setMappings(rows<Mapping>(mappingData));
      setBranches(rows<Branch>(branchData));
      setCampus((value) => ({
        ...value,
        branchId: value.branchId || nextCampuses[0]?.branchId || "",
      }));
      setMapping((value) => ({
        ...value,
        branchId: value.branchId || nextCampuses[0]?.branchId || "",
      }));
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "Finance configuration could not be loaded.",
      );
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void load();
  }, [load, notice]);

  async function save(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    setNotice("");
    const config: Record<Tab, { path: string; body: unknown }> = {
      funds: {
        path: "/api/chms/v1/finance/funds",
        body: {
          ...fund,
          activeFrom: iso(fund.activeFrom),
          activeUntil: iso(fund.activeUntil),
          successorFundId: fund.successorFundId || undefined,
        },
      },
      methods: { path: "/api/chms/v1/finance/payment-methods", body: method },
      campuses: { path: "/api/chms/v1/finance/campuses", body: campus },
      periods: {
        path: "/api/chms/v1/finance/periods",
        body: {
          ...period,
          startsAt: iso(period.startsAt),
          endsAt: iso(period.endsAt),
        },
      },
      sequences: {
        path: "/api/chms/v1/finance/receipt-sequences",
        body: sequence,
      },
      mappings: {
        path: "/api/chms/v1/finance/account-mappings",
        body: {
          ...mapping,
          fundId: mapping.fundId || undefined,
          effectiveFrom: iso(mapping.effectiveFrom),
          effectiveUntil: iso(mapping.effectiveUntil),
        },
      },
    };
    try {
      const target = config[tab];
      await api(`${target.path}${editing ? `/${editing}` : ""}`, {
        method: editing ? "PATCH" : "POST",
        body: target.body,
      });
      setNotice(
        `${TABS.find((item) => item.id === tab)?.label} configuration saved.`,
      );
      reset(tab);
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The configuration change could not be saved.",
      );
    } finally {
      setBusy(false);
    }
  }

  function reset(nextTab: Tab = tab) {
    setEditing(null);
    setControlReason("");
    if (nextTab === "funds")
      setFund({
        code: "",
        name: "",
        description: "",
        restrictionType: "unrestricted",
        activeFrom: dateOnly(new Date().toISOString()),
        activeUntil: "",
        successorFundId: "",
        expectedVersion: 0,
      });
    if (nextTab === "methods")
      setMethod({
        code: "",
        name: "",
        kind: "cash",
        provider: "",
        active: true,
        expectedVersion: 0,
      });
    if (nextTab === "campuses")
      setCampus({
        branchId: campuses[0]?.branchId || "",
        name: "",
        timezone: "Africa/Accra",
        currency: "GHS",
        expectedVersion: 0,
      });
    if (nextTab === "periods")
      setPeriod({
        code: "",
        name: "",
        startsAt: "",
        endsAt: "",
        expectedVersion: 0,
      });
    if (nextTab === "sequences")
      setSequence({
        code: "",
        prefix: "REMI-",
        fiscalYear: new Date().getFullYear(),
        padding: 6,
        startingNumber: 1,
        expectedVersion: 0,
      });
    if (nextTab === "mappings")
      setMapping({
        branchId: campuses[0]?.branchId || "",
        fundId: "",
        category: "contribution",
        externalAccountCode: "",
        externalDimensionCode: "",
        effectiveFrom: dateOnly(new Date().toISOString()),
        effectiveUntil: "",
        expectedVersion: 0,
      });
  }
  function edit(value: Fund | Method | Campus | Period | Sequence | Mapping) {
    setEditing(value.id);
    setControlReason("");
    if (tab === "funds") {
      const item = value as Fund;
      setFund({
        code: item.code,
        name: item.name,
        description: item.description || "",
        restrictionType: item.restrictionType,
        activeFrom: dateOnly(item.activeFrom),
        activeUntil: dateOnly(item.activeUntil),
        successorFundId: item.successorFundId || "",
        expectedVersion: item.version,
      });
    }
    if (tab === "methods") {
      const item = value as Method;
      setMethod({
        code: item.code,
        name: item.name,
        kind: item.kind,
        provider: item.provider || "",
        active: item.active,
        expectedVersion: item.version,
      });
    }
    if (tab === "campuses") {
      const item = value as Campus;
      setCampus({
        branchId: item.branchId,
        name: item.name,
        timezone: item.timezone,
        currency: item.currency,
        expectedVersion: item.version,
      });
    }
    if (tab === "periods") {
      const item = value as Period;
      setPeriod({
        code: item.code,
        name: item.name,
        startsAt: dateOnly(item.startsAt),
        endsAt: dateOnly(item.endsAt),
        expectedVersion: item.version,
      });
    }
    if (tab === "sequences") {
      const item = value as Sequence;
      setSequence({
        code: item.code,
        prefix: item.prefix,
        fiscalYear: item.fiscalYear,
        padding: item.padding,
        startingNumber: item.nextNumber,
        expectedVersion: item.version,
      });
    }
    if (tab === "mappings") {
      const item = value as Mapping;
      setMapping({
        branchId: item.branchId || campuses[0]?.branchId || "",
        fundId: item.fundId || "",
        category: item.category,
        externalAccountCode: item.externalAccountCode,
        externalDimensionCode: item.externalDimensionCode || "",
        effectiveFrom: dateOnly(item.effectiveFrom),
        effectiveUntil: dateOnly(item.effectiveUntil),
        expectedVersion: item.version,
      });
    }
    window.scrollTo({ top: 0, behavior: "smooth" });
  }
  async function deactivateFund(item: Fund) {
    if (controlReason.trim().length < 10) {
      setError(
        "Enter a specific reason of at least 10 characters before deactivating a fund.",
      );
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api(`/api/chms/v1/finance/funds/${item.id}/deactivate`, {
        method: "POST",
        body: {
          expectedVersion: item.version,
          reason: controlReason.trim(),
          successorFundId: fund.successorFundId || undefined,
        },
      });
      setNotice(`${item.name} deactivated with a retained audit reason.`);
      reset("funds");
    } catch (cause) {
      setError(
        cause instanceof ApiError
          ? cause.message
          : "The fund could not be deactivated.",
      );
    } finally {
      setBusy(false);
    }
  }
  async function controlPeriod(
    item: Period,
    action: "close" | "reopen",
    request?: PeriodRequest,
  ) {
    if (controlReason.trim().length < 10) {
      setError(
        "Enter a specific period-control reason of at least 10 characters.",
      );
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api(`/api/chms/v1/finance/periods/${item.id}/${action}`, {
        method: "POST",
        body: {
          action: request ? "approve" : "request",
          requestId: request?.id,
          expectedVersion: item.version,
          reason: controlReason.trim(),
        },
      });
      setNotice(
        request
          ? `${item.name} ${action} approved by an independent operator.`
          : `${action === "close" ? "Close" : "Reopen"} request recorded for ${item.name}; another authorized operator must approve it.`,
      );
      reset("periods");
    } catch (cause) {
      setError(
        cause instanceof ApiError ? cause.message : "Period control failed.",
      );
    } finally {
      setBusy(false);
    }
  }

  const currentRows = useMemo(
    () => ({ funds, methods, campuses, periods, sequences, mappings })[tab],
    [campuses, funds, mappings, methods, periods, sequences, tab],
  );
  return (
    <div>
      <PageHeader
        title="Finance configuration"
        subtitle="Control the accounting dimensions that make posting, receipts, reporting and external ledger exports deterministic."
      />
      {error && <ErrorBox message={error} onRetry={() => void load()} />}
      {notice && (
        <p
          role="status"
          className="mb-5 rounded-xl bg-emerald-500/10 px-4 py-3 text-sm font-semibold text-emerald-800"
        >
          {notice}
        </p>
      )}
      <section
        className="mb-6 grid gap-3 md:grid-cols-3 xl:grid-cols-6"
        aria-label="Configuration areas"
      >
        {TABS.map((item, index) => (
          <button
            key={item.id}
            type="button"
            aria-pressed={tab === item.id}
            onClick={() => {
              setTab(item.id);
              reset(item.id);
            }}
            className={`min-h-28 rounded-2xl border p-4 text-left transition ${tab === item.id ? "border-[var(--remi-gold)]/25 bg-[var(--remi-green)] text-white shadow-lg" : "border-black/[.055] bg-[var(--remi-surface)] hover:-translate-y-0.5"}`}
          >
            <span
              className={`font-mono text-[9px] ${tab === item.id ? "text-[var(--altar-mint)]" : "text-[var(--remi-gold-deep)]"}`}
            >
              {String(index + 1).padStart(2, "0")}
            </span>
            <b className="mt-4 block text-sm">{item.label}</b>
            <small
              className={`mt-1 block text-[10px] ${tab === item.id ? "text-white/45" : "text-[var(--remi-muted)]"}`}
            >
              {item.copy}
            </small>
          </button>
        ))}
      </section>
      <div className="grid gap-6 xl:grid-cols-[minmax(22rem,.8fr)_minmax(0,1.2fr)]">
        <Card className="self-start p-5">
          <header className="mb-5">
            <span className="font-mono text-[9px] uppercase tracking-wider text-[var(--remi-gold-deep)]">
              {editing ? "Editing controlled record" : "New controlled record"}
            </span>
            <h2 className="mt-1 text-xl font-bold">
              {TABS.find((item) => item.id === tab)?.label}
            </h2>
            <p className="mt-2 text-xs leading-5 text-[var(--remi-muted)]">
              Financial fields are permission-scoped and every successful
              mutation appends an audit event.
            </p>
          </header>
          <form className="grid gap-4" onSubmit={save}>
            {tab === "funds" && (
              <>
                <Text
                  label="Fund code"
                  value={fund.code}
                  onChange={(value) => setFund({ ...fund, code: value })}
                  placeholder="GENERAL"
                />
                <Text
                  label="Fund name"
                  value={fund.name}
                  onChange={(value) => setFund({ ...fund, name: value })}
                />
                <label className="admin-field">
                  <span>Restriction</span>
                  <Select
                    value={fund.restrictionType}
                    onChange={(event) =>
                      setFund({ ...fund, restrictionType: event.target.value })
                    }
                  >
                    <option value="unrestricted">Unrestricted</option>
                    <option value="temporarily-restricted">
                      Temporarily restricted
                    </option>
                    <option value="board-designated">Board designated</option>
                  </Select>
                </label>
                <label className="admin-field">
                  <span>Description</span>
                  <textarea
                    rows={3}
                    value={fund.description}
                    onChange={(event) =>
                      setFund({ ...fund, description: event.target.value })
                    }
                  />
                </label>
                <DateField
                  label="Active from"
                  value={fund.activeFrom}
                  onChange={(value) => setFund({ ...fund, activeFrom: value })}
                />
                <DateField
                  label="Active until (optional)"
                  value={fund.activeUntil}
                  onChange={(value) => setFund({ ...fund, activeUntil: value })}
                />
              </>
            )}
            {tab === "methods" && (
              <>
                <Text
                  label="Tender code"
                  value={method.code}
                  onChange={(value) => setMethod({ ...method, code: value })}
                  placeholder="MOMO"
                />
                <Text
                  label="Display name"
                  value={method.name}
                  onChange={(value) => setMethod({ ...method, name: value })}
                />
                <label className="admin-field">
                  <span>Kind</span>
                  <Select
                    value={method.kind}
                    onChange={(event) =>
                      setMethod({ ...method, kind: event.target.value })
                    }
                  >
                    {[
                      "cash",
                      "cheque",
                      "mobile-money",
                      "card",
                      "bank-transfer",
                      "in-kind",
                    ].map((kind) => (
                      <option value={kind} key={kind}>
                        {kind.replaceAll("-", " ")}
                      </option>
                    ))}
                  </Select>
                </label>
                <Text
                  label="Provider (optional)"
                  value={method.provider}
                  onChange={(value) =>
                    setMethod({ ...method, provider: value })
                  }
                  placeholder="Paystack"
                />
                <label className="flex items-center gap-3 rounded-xl bg-black/[.025] p-4 text-sm">
                  <input
                    type="checkbox"
                    checked={method.active}
                    onChange={(event) =>
                      setMethod({ ...method, active: event.target.checked })
                    }
                  />
                  <b>Accept this payment method</b>
                </label>
              </>
            )}
            {tab === "campuses" && (
              <>
                <label className="admin-field">
                  <span>Branch</span>
                  <Select
                    disabled={Boolean(editing)}
                    value={campus.branchId}
                    onChange={(event) => {
                      const branchId = event.target.value;
                      const branch = branches.find(
                        (item) => (item.id || item._id) === branchId,
                      );
                      setCampus({
                        ...campus,
                        branchId,
                        name: branch?.name || branch?.title || campus.name,
                      });
                    }}
                  >
                    <option value="">Choose branch</option>
                    {branches.map((branch) => (
                      <option
                        value={branch.id || branch._id}
                        key={branch.id || branch._id}
                      >
                        {branch.name || branch.title || branch.id}
                      </option>
                    ))}
                    {campuses
                      .filter(
                        (item) =>
                          !branches.some(
                            (branch) =>
                              (branch.id || branch._id) === item.branchId,
                          ),
                      )
                      .map((item) => (
                        <option value={item.branchId} key={item.branchId}>
                          {item.name}
                        </option>
                      ))}
                  </Select>
                </label>
                <Text
                  label="Finance display name"
                  value={campus.name}
                  onChange={(value) => setCampus({ ...campus, name: value })}
                />
                <label className="admin-field">
                  <span>Timezone</span>
                  <Select
                    value={campus.timezone}
                    onChange={(event) =>
                      setCampus({ ...campus, timezone: event.target.value })
                    }
                  >
                    {[
                      "Africa/Accra",
                      "Africa/Lagos",
                      "Africa/Nairobi",
                      "Europe/London",
                      "America/New_York",
                    ].map((zone) => (
                      <option value={zone} key={zone}>
                        {zone.replaceAll("_", " ")}
                      </option>
                    ))}
                  </Select>
                </label>
                <label className="admin-field">
                  <span>Currency</span>
                  <Select
                    value={campus.currency}
                    onChange={(event) =>
                      setCampus({ ...campus, currency: event.target.value })
                    }
                  >
                    <option value="GHS">Ghanaian cedi (GHS)</option>
                  </Select>
                </label>
              </>
            )}
            {tab === "periods" && (
              <>
                <Text
                  label="Period code"
                  value={period.code}
                  onChange={(value) => setPeriod({ ...period, code: value })}
                  placeholder="FY2026"
                />
                <Text
                  label="Period name"
                  value={period.name}
                  onChange={(value) => setPeriod({ ...period, name: value })}
                  placeholder="2026 fiscal year"
                />
                <DateField
                  label="Starts"
                  value={period.startsAt}
                  onChange={(value) =>
                    setPeriod({ ...period, startsAt: value })
                  }
                />
                <DateField
                  label="Ends (exclusive)"
                  value={period.endsAt}
                  onChange={(value) => setPeriod({ ...period, endsAt: value })}
                />
              </>
            )}
            {tab === "sequences" && (
              <>
                <Text
                  label="Sequence code"
                  value={sequence.code}
                  onChange={(value) =>
                    setSequence({ ...sequence, code: value })
                  }
                  placeholder="RECEIPT_2026"
                />
                <Text
                  label="Receipt prefix"
                  value={sequence.prefix}
                  onChange={(value) =>
                    setSequence({ ...sequence, prefix: value })
                  }
                />
                <NumberField
                  label="Fiscal year"
                  value={sequence.fiscalYear}
                  onChange={(value) =>
                    setSequence({ ...sequence, fiscalYear: value })
                  }
                  min={2000}
                  max={2200}
                />
                <NumberField
                  label={
                    editing
                      ? "Next number (read-only in effect)"
                      : "Starting number"
                  }
                  value={sequence.startingNumber}
                  onChange={(value) =>
                    setSequence({ ...sequence, startingNumber: value })
                  }
                  min={1}
                />
                <NumberField
                  label="Number padding"
                  value={sequence.padding}
                  onChange={(value) =>
                    setSequence({ ...sequence, padding: value })
                  }
                  min={4}
                  max={12}
                />
              </>
            )}
            {tab === "mappings" && (
              <>
                <label className="admin-field">
                  <span>Branch</span>
                  <Select
                    value={mapping.branchId}
                    onChange={(event) =>
                      setMapping({ ...mapping, branchId: event.target.value })
                    }
                  >
                    {campuses.map((item) => (
                      <option value={item.branchId} key={item.branchId}>
                        {item.name}
                      </option>
                    ))}
                  </Select>
                </label>
                <label className="admin-field">
                  <span>Fund (optional)</span>
                  <Select
                    value={mapping.fundId}
                    onChange={(event) =>
                      setMapping({ ...mapping, fundId: event.target.value })
                    }
                  >
                    <option value="">All funds for category</option>
                    {funds.map((item) => (
                      <option value={item.id} key={item.id}>
                        {item.name}
                      </option>
                    ))}
                  </Select>
                </label>
                <label className="admin-field">
                  <span>Category</span>
                  <Select
                    value={mapping.category}
                    onChange={(event) =>
                      setMapping({ ...mapping, category: event.target.value })
                    }
                  >
                    {[
                      "contribution",
                      "cash",
                      "cheque",
                      "mobile-money",
                      "card",
                      "bank-transfer",
                      "in-kind",
                      "fee",
                      "refund",
                    ].map((item) => (
                      <option value={item} key={item}>
                        {item.replaceAll("-", " ")}
                      </option>
                    ))}
                  </Select>
                </label>
                <Text
                  label="External account code"
                  value={mapping.externalAccountCode}
                  onChange={(value) =>
                    setMapping({ ...mapping, externalAccountCode: value })
                  }
                />
                <Text
                  label="External dimension code (optional)"
                  value={mapping.externalDimensionCode}
                  onChange={(value) =>
                    setMapping({ ...mapping, externalDimensionCode: value })
                  }
                />
                <DateField
                  label="Effective from"
                  value={mapping.effectiveFrom}
                  onChange={(value) =>
                    setMapping({ ...mapping, effectiveFrom: value })
                  }
                />
                <DateField
                  label="Effective until (optional)"
                  value={mapping.effectiveUntil}
                  onChange={(value) =>
                    setMapping({ ...mapping, effectiveUntil: value })
                  }
                />
              </>
            )}
            <div className="flex flex-wrap gap-2">
              <button
                disabled={busy || loading}
                className="admin-primary-button"
              >
                {busy
                  ? "Saving…"
                  : editing
                    ? "Save controlled update"
                    : "Create configuration"}
              </button>
              {editing && (
                <button
                  type="button"
                  onClick={() => reset()}
                  className="admin-secondary-button"
                >
                  Cancel
                </button>
              )}
            </div>
          </form>
        </Card>
        <Card className="overflow-hidden">
          <header className="flex items-end justify-between gap-4 border-b border-black/[.045] p-5">
            <div>
              <h2 className="font-bold">Configured records</h2>
              <p className="mt-1 text-xs text-[var(--remi-muted)]">
                Select a record to make an optimistic, version-checked update.
              </p>
            </div>
            <span className="font-mono text-xs text-[var(--remi-gold-deep)]">
              {currentRows.length}
            </span>
          </header>
          {currentRows.length ? (
            <div className="divide-y divide-black/[.045]">
              {currentRows.map((item) => (
                <ConfigRow
                  key={item.id}
                  tab={tab}
                  item={item}
                  funds={funds}
                  campuses={campuses}
                  onEdit={() => edit(item)}
                >
                  {tab === "funds" && editing === item.id && (
                    <ControlPanel
                      reason={controlReason}
                      onReason={setControlReason}
                    >
                      <label className="admin-field">
                        <span>Successor fund (optional)</span>
                        <Select
                          value={fund.successorFundId}
                          onChange={(event) =>
                            setFund({
                              ...fund,
                              successorFundId: event.target.value,
                            })
                          }
                        >
                          <option value="">No successor</option>
                          {funds
                            .filter((candidate) => candidate.id !== item.id)
                            .map((candidate) => (
                              <option value={candidate.id} key={candidate.id}>
                                {candidate.name}
                              </option>
                            ))}
                        </Select>
                      </label>
                      <button
                        type="button"
                        disabled={busy || controlReason.trim().length < 10}
                        onClick={() => void deactivateFund(item as Fund)}
                        className="admin-secondary-button text-rose-700"
                      >
                        Deactivate fund
                      </button>
                    </ControlPanel>
                  )}
                  {tab === "periods" && editing === item.id && (
                    <ControlPanel
                      reason={controlReason}
                      onReason={setControlReason}
                    >
                      {periodRequests
                        .filter(
                          (request) =>
                            request.periodId === item.id &&
                            request.state === "pending",
                        )
                        .map((request) => (
                          <div
                            key={request.id}
                            className="rounded-xl bg-[var(--remi-surface)] p-3 text-xs"
                          >
                            <b className="capitalize">
                              Pending {request.action} request
                            </b>
                            <p className="mt-1 text-[var(--remi-muted)]">
                              {request.reason}
                            </p>
                            <small className="mt-1 block font-mono text-[9px] text-[var(--remi-muted)]">
                              Requested{" "}
                              {new Date(request.requestedAt).toLocaleString(
                                "en-GH",
                              )}
                            </small>
                            <button
                              type="button"
                              disabled={
                                busy || controlReason.trim().length < 10
                              }
                              onClick={() =>
                                void controlPeriod(
                                  item as Period,
                                  request.action,
                                  request,
                                )
                              }
                              className="admin-primary-button mt-3"
                            >
                              Approve independently
                            </button>
                          </div>
                        ))}
                      <button
                        type="button"
                        disabled={
                          busy ||
                          controlReason.trim().length < 10 ||
                          periodRequests.some(
                            (request) =>
                              request.periodId === item.id &&
                              request.state === "pending",
                          )
                        }
                        onClick={() =>
                          void controlPeriod(
                            item as Period,
                            (item as Period).status === "open"
                              ? "close"
                              : "reopen",
                          )
                        }
                        className="admin-secondary-button"
                      >
                        {(item as Period).status === "open"
                          ? "Request fiscal-period close"
                          : "Request controlled reopen"}
                      </button>
                    </ControlPanel>
                  )}
                </ConfigRow>
              ))}
            </div>
          ) : (
            !loading && (
              <div className="p-5">
                <EmptyState
                  dense
                  title={`No ${TABS.find((item) => item.id === tab)?.label.toLowerCase()} configured`}
                  hint="Create the first controlled record using the form."
                />
              </div>
            )
          )}
        </Card>
      </div>
    </div>
  );
}

function ConfigRow({
  tab,
  item,
  funds,
  campuses,
  onEdit,
  children,
}: {
  tab: Tab;
  item: Fund | Method | Campus | Period | Sequence | Mapping;
  funds: Fund[];
  campuses: Campus[];
  onEdit: () => void;
  children?: React.ReactNode;
}) {
  let title = "",
    meta = "",
    state = "";
  if (tab === "funds") {
    const v = item as Fund;
    title = `${v.code} · ${v.name}`;
    meta = v.restrictionType.replaceAll("-", " ");
    state = v.activeUntil ? "ended" : "active";
  }
  if (tab === "methods") {
    const v = item as Method;
    title = `${v.code} · ${v.name}`;
    meta = `${v.kind.replaceAll("-", " ")}${v.provider ? ` · ${v.provider}` : ""}`;
    state = v.active ? "active" : "inactive";
  }
  if (tab === "campuses") {
    const v = item as Campus;
    title = v.name;
    meta = `${v.timezone} · ${v.currency}`;
    state = "configured";
  }
  if (tab === "periods") {
    const v = item as Period;
    title = `${v.code} · ${v.name}`;
    meta = `${dateOnly(v.startsAt)} → ${dateOnly(v.endsAt)}`;
    state = v.status;
  }
  if (tab === "sequences") {
    const v = item as Sequence;
    title = `${v.code} · ${v.prefix}${String(v.nextNumber).padStart(v.padding, "0")}`;
    meta = `FY ${v.fiscalYear} · ${v.padding} digits`;
    state = "non-reusing";
  }
  if (tab === "mappings") {
    const v = item as Mapping;
    title = `${v.category.replaceAll("-", " ")} → ${v.externalAccountCode}`;
    meta = `${campuses.find((campus) => campus.branchId === v.branchId)?.name || v.branchId}${v.fundId ? ` · ${funds.find((fund) => fund.id === v.fundId)?.name || v.fundId}` : " · all funds"}`;
    state = v.effectiveUntil ? "dated" : "current";
  }
  return (
    <article className="p-5">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="min-w-0">
          <b className="block break-words text-sm">{title}</b>
          <small className="mt-1 block capitalize text-[var(--remi-muted)]">
            {meta}
          </small>
        </div>
        <div className="flex items-center gap-2">
          <span className="rounded-full bg-[var(--remi-gold)]/10 px-2.5 py-1 font-mono text-[9px] uppercase tracking-wider text-[var(--remi-gold-deep)]">
            {state}
          </span>
          <button
            type="button"
            onClick={onEdit}
            className="admin-secondary-button !px-3 !py-2 text-xs"
          >
            Edit
          </button>
        </div>
      </div>
      {children}
    </article>
  );
}
function ControlPanel({
  reason,
  onReason,
  children,
}: {
  reason: string;
  onReason: (value: string) => void;
  children: React.ReactNode;
}) {
  return (
    <div className="mt-4 grid gap-3 rounded-xl bg-[var(--remi-gold)]/[.07] p-4">
      <label className="admin-field">
        <span>Required control reason · minimum 10 characters</span>
        <textarea
          rows={2}
          value={reason}
          onChange={(event) => onReason(event.target.value)}
          placeholder="Describe the evidence and approval for this control action."
        />
      </label>
      {children}
    </div>
  );
}
function Text({
  label,
  value,
  onChange,
  placeholder = "",
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <label className="admin-field">
      <span>{label}</span>
      <input
        required={!label.includes("optional")}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
      />
    </label>
  );
}
function DateField({
  label,
  value,
  onChange,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
}) {
  return (
    <label className="admin-field">
      <span>{label}</span>
      <DatePicker value={value} onChange={onChange} />
    </label>
  );
}
function NumberField({
  label,
  value,
  onChange,
  min,
  max,
}: {
  label: string;
  value: number;
  onChange: (value: number) => void;
  min: number;
  max?: number;
}) {
  return (
    <label className="admin-field">
      <span>{label}</span>
      <input
        required
        type="number"
        value={value}
        min={min}
        max={max}
        onChange={(event) => onChange(Number(event.target.value))}
      />
    </label>
  );
}
