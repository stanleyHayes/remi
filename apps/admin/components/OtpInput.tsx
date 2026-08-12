"use client";

import { useRef, type ClipboardEvent, type KeyboardEvent } from "react";

type OtpInputProps = {
  value: string;
  onChange: (value: string) => void;
  length?: number;
  label?: string;
  autoFocus?: boolean;
  disabled?: boolean;
};

export function OtpInput({
  value,
  onChange,
  length = 6,
  label = "Verification code",
  autoFocus = false,
  disabled = false,
}: OtpInputProps) {
  const refs = useRef<Array<HTMLInputElement | null>>([]);
  const digits = Array.from({ length }, (_, index) => value[index] ?? "");

  function commit(index: number, raw: string) {
    const incoming = raw.replace(/\D/g, "");
    if (!incoming) {
      const next = [...digits];
      next[index] = "";
      onChange(next.join(""));
      return;
    }

    const next = [...digits];
    incoming.slice(0, length - index).split("").forEach((digit, offset) => {
      next[index + offset] = digit;
    });
    onChange(next.join(""));
    refs.current[Math.min(index + incoming.length, length - 1)]?.focus();
  }

  function onKeyDown(event: KeyboardEvent<HTMLInputElement>, index: number) {
    if (event.key === "Backspace" && !digits[index] && index > 0) {
      event.preventDefault();
      const next = [...digits];
      next[index - 1] = "";
      onChange(next.join(""));
      refs.current[index - 1]?.focus();
    } else if (event.key === "ArrowLeft" && index > 0) {
      event.preventDefault();
      refs.current[index - 1]?.focus();
    } else if (event.key === "ArrowRight" && index < length - 1) {
      event.preventDefault();
      refs.current[index + 1]?.focus();
    }
  }

  function onPaste(event: ClipboardEvent<HTMLDivElement>) {
    const pasted = event.clipboardData.getData("text").replace(/\D/g, "").slice(0, length);
    if (!pasted) return;
    event.preventDefault();
    onChange(pasted);
    refs.current[Math.min(pasted.length, length) - 1]?.focus();
  }

  return (
    <div className="admin-otp" role="group" aria-label={label} onPaste={onPaste}>
      {digits.map((digit, index) => (
        <input
          key={index}
          ref={(node) => { refs.current[index] = node; }}
          value={digit}
          onChange={(event) => commit(index, event.target.value)}
          onKeyDown={(event) => onKeyDown(event, index)}
          onFocus={(event) => event.currentTarget.select()}
          aria-label={`${label}, digit ${index + 1} of ${length}`}
          autoComplete={index === 0 ? "one-time-code" : "off"}
          autoFocus={autoFocus && index === 0}
          disabled={disabled}
          inputMode="numeric"
          pattern="[0-9]*"
          maxLength={length}
          className="admin-otp-slot"
        />
      ))}
    </div>
  );
}
