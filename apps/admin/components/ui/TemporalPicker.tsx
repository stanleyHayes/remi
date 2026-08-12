"use client";

import DatePicker from "./DatePicker";
import { Select } from "./Select";

type CommonProps = {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  required?: boolean;
  className?: string;
};

function options(step = 15) {
  const values: string[] = [];
  for (let minute = 0; minute < 24 * 60; minute += step) {
    values.push(
      `${String(Math.floor(minute / 60)).padStart(2, "0")}:${String(minute % 60).padStart(2, "0")}`,
    );
  }
  return values;
}
const TIMES = options();

export function TimePicker({
  value,
  onChange,
  disabled,
  required,
  className = "",
  label = "Time",
}: CommonProps & { label?: string }) {
  const values =
    TIMES.includes(value) || !value ? TIMES : [value, ...TIMES].sort();
  return (
    <Select
      aria-label={label}
      value={value}
      onChange={(event) => onChange(event.target.value)}
      disabled={disabled}
      required={required}
      className={className}
    >
      <option value="">Choose time</option>
      {values.map((item) => (
        <option value={item} key={item}>
          {formatTime(item)}
        </option>
      ))}
    </Select>
  );
}

export function DateTimePicker({
  value,
  onChange,
  disabled,
  required,
  className = "",
  dateLabel = "Date",
  timeLabel = "Time",
  min,
  max,
}: CommonProps & {
  dateLabel?: string;
  timeLabel?: string;
  min?: string;
  max?: string;
}) {
  const [date, time] = value.includes("T") ? value.split("T") : [value, ""];
  const setDate = (next: string) =>
    onChange(next ? `${next}T${time || "09:00"}` : "");
  const setTime = (next: string) =>
    onChange(date && next ? `${date}T${next}` : date);
  return (
    <div className={`admin-temporal-pair ${className}`}>
      <DatePicker
        value={date || ""}
        onChange={setDate}
        disabled={disabled}
        clearable={!required}
        placeholder={dateLabel}
        min={min?.slice(0, 10)}
        max={max?.slice(0, 10)}
      />
      <TimePicker
        value={time?.slice(0, 5) || ""}
        onChange={setTime}
        disabled={disabled || !date}
        required={required}
        label={timeLabel}
      />
    </div>
  );
}

function formatTime(value: string) {
  const [rawHour, minute] = value.split(":");
  const hour = Number(rawHour);
  if (!Number.isFinite(hour)) return value;
  return `${hour % 12 || 12}:${minute} ${hour < 12 ? "AM" : "PM"}`;
}
