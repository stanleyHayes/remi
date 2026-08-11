"use client";

import {
  Children,
  isValidElement,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type KeyboardEvent,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

type OptionProps = {
  children?: ReactNode;
  disabled?: boolean;
  value?: string | number;
};

type SelectChangeEvent = {
  target: { value: string };
  currentTarget: { value: string };
};

export type SelectProps = {
  "aria-label"?: string;
  children: ReactNode;
  className?: string;
  defaultValue?: string | number;
  disabled?: boolean;
  id?: string;
  name?: string;
  onChange?: (event: SelectChangeEvent) => void;
  required?: boolean;
  title?: string;
  value?: string | number;
};

type SelectOption = {
  disabled: boolean;
  label: string;
  value: string;
};

function flattenOptions(children: ReactNode): SelectOption[] {
  const options: SelectOption[] = [];

  Children.forEach(children, (child) => {
    if (Array.isArray(child)) {
      options.push(...flattenOptions(child));
      return;
    }
    if (!isValidElement<OptionProps>(child)) return;
    if (child.type === "option") {
      const value = String(child.props.value ?? child.props.children ?? "");
      options.push({
        disabled: Boolean(child.props.disabled),
        label: Children.toArray(child.props.children).join(""),
        value,
      });
      return;
    }
    options.push(...flattenOptions(child.props.children));
  });

  return options;
}

export function Select({
  "aria-label": ariaLabel,
  children,
  className,
  defaultValue,
  disabled = false,
  id,
  name,
  onChange,
  required = false,
  title,
  value,
}: SelectProps) {
  const generatedID = useId();
  const listboxID = `${id ?? generatedID}-listbox`;
  const rootRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const options = useMemo(() => flattenOptions(children), [children]);
  const firstEnabled = options.find((option) => !option.disabled)?.value ?? "";
  const [internalValue, setInternalValue] = useState(String(defaultValue ?? value ?? firstEnabled));
  const [open, setOpen] = useState(false);
  const [menuStyle, setMenuStyle] = useState<CSSProperties>({});
  const controlled = value !== undefined;
  const selectedValue = controlled ? String(value) : internalValue;
  const selected = options.find((option) => option.value === selectedValue);

  useEffect(() => {
    if (!open) return;
    const trigger = triggerRef.current;
    if (trigger) {
      const rect = trigger.getBoundingClientRect();
      const menuHeight = Math.min(288, options.length * 46 + 14);
      const roomBelow = window.innerHeight - rect.bottom;
      const top =
        roomBelow >= menuHeight + 12 ? rect.bottom + 8 : Math.max(8, rect.top - menuHeight - 8);
      setMenuStyle({
        left: Math.min(rect.left, window.innerWidth - Math.max(rect.width, 208) - 8),
        top,
        width: Math.max(rect.width, 208),
      });
    }
    const close = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!rootRef.current?.contains(target) && !menuRef.current?.contains(target)) {
        setOpen(false);
      }
    };
    const closeForViewportChange = () => setOpen(false);
    window.addEventListener("pointerdown", close);
    window.addEventListener("resize", closeForViewportChange);
    window.addEventListener("scroll", closeForViewportChange, true);
    return () => {
      window.removeEventListener("pointerdown", close);
      window.removeEventListener("resize", closeForViewportChange);
      window.removeEventListener("scroll", closeForViewportChange, true);
    };
  }, [open, options.length]);

  function choose(nextValue: string) {
    if (!controlled) setInternalValue(nextValue);
    const target = { value: nextValue };
    onChange?.({ target, currentTarget: target });
    setOpen(false);
  }

  function move(direction: 1 | -1) {
    const enabled = options.filter((option) => !option.disabled);
    if (enabled.length === 0) return;
    const current = enabled.findIndex((option) => option.value === selectedValue);
    const next = current < 0 ? 0 : (current + direction + enabled.length) % enabled.length;
    choose(enabled[next].value);
  }

  function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) setOpen(true);
      else move(event.key === "ArrowDown" ? 1 : -1);
    } else if (event.key === "Escape") {
      setOpen(false);
    } else if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      setOpen((current) => !current);
    }
  }

  return (
    <div ref={rootRef} className={`lp-select w-full${className ? ` ${className}` : ""}`}>
      {name ? <input type="hidden" name={name} value={selectedValue} /> : null}
      <button
        ref={triggerRef}
        id={id}
        type="button"
        className="lp-select__trigger"
        aria-label={ariaLabel}
        aria-controls={listboxID}
        aria-expanded={open}
        aria-haspopup="listbox"
        disabled={disabled}
        title={title}
        onClick={() => setOpen((current) => !current)}
        onKeyDown={onKeyDown}
      >
        <span className={selectedValue === "" ? "text-zinc-400" : ""}>
          {selected?.label ?? "Select an option"}
        </span>
        <svg aria-hidden="true" viewBox="0 0 20 20" className="lp-select__chevron">
          <path d="m5 7.5 5 5 5-5" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.8" />
        </svg>
      </button>
      {open && typeof document !== "undefined"
        ? createPortal(
            <div ref={menuRef} id={listboxID} className="lp-select__menu" role="listbox" style={menuStyle}>
              {options.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  role="option"
                  aria-selected={option.value === selectedValue}
                  className="lp-select__option"
                  disabled={option.disabled}
                  onClick={() => choose(option.value)}
                >
                  <span>{option.label}</span>
                  {option.value === selectedValue ? (
                    <svg aria-hidden="true" viewBox="0 0 20 20">
                      <path d="m4.5 10 3.3 3.3L15.5 6" fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" />
                    </svg>
                  ) : null}
                </button>
              ))}
            </div>,
            document.body,
          )
        : null}
      {required && selectedValue === "" ? (
        <input className="pointer-events-none absolute h-px w-px opacity-0" required tabIndex={-1} aria-hidden="true" />
      ) : null}
    </div>
  );
}
