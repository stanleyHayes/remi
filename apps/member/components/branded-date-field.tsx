"use client";

import { useEffect, useId, useMemo, useState } from "react";
import { BrandedSelect } from "@/components/branded-select";

type BrandedDateFieldProps = { disabled?: boolean; maxYear?: number; minYear?: number; onChange: (value: string) => void; value?: string };

const MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
const parse = (value?: string) => { const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value || ""); return match ? { year: match[1], month: match[2], day: match[3] } : { year: "", month: "", day: "" }; };
const daysInMonth = (year: string, month: string) => month ? new Date(Number(year) || 2000, Number(month), 0).getDate() : 31;

export function BrandedDateField({ disabled, maxYear = new Date().getFullYear(), minYear = 1900, onChange, value }: BrandedDateFieldProps) {
  const id = useId();
  const [selected, setSelected] = useState(() => parse(value));
  useEffect(() => setSelected(parse(value)), [value]);
  const years = useMemo(() => Array.from({ length: maxYear - minYear + 1 }, (_, index) => maxYear - index), [maxYear, minYear]);
  const dayCount = daysInMonth(selected.year, selected.month);
  function update(key: "year" | "month" | "day", next: string) {
    const changed = { ...selected, [key]: next };
    const maxDay = daysInMonth(changed.year, changed.month);
    if (Number(changed.day) > maxDay) changed.day = String(maxDay).padStart(2, "0");
    setSelected(changed);
    onChange(changed.year && changed.month && changed.day ? `${changed.year}-${changed.month}-${changed.day}` : "");
  }
  return <fieldset className="member-branded-date" disabled={disabled} aria-label="Date of birth">
    <legend className="sr-only">Date of birth</legend>
    <BrandedSelect aria-label="Birth day" menuLabel="Day" id={`${id}-day`} value={selected.day} onChange={(event) => update("day", event.target.value)}><option value="">Day</option>{Array.from({ length: dayCount }, (_, index) => index + 1).map((day) => <option key={day} value={String(day).padStart(2, "0")}>{day}</option>)}</BrandedSelect>
    <BrandedSelect aria-label="Birth month" menuLabel="Month" id={`${id}-month`} value={selected.month} onChange={(event) => update("month", event.target.value)}><option value="">Month</option>{MONTHS.map((month, index) => <option key={month} value={String(index + 1).padStart(2, "0")}>{month}</option>)}</BrandedSelect>
    <BrandedSelect aria-label="Birth year" menuLabel="Year" id={`${id}-year`} value={selected.year} onChange={(event) => update("year", event.target.value)}><option value="">Year</option>{years.map((year) => <option key={year} value={year}>{year}</option>)}</BrandedSelect>
  </fieldset>;
}
