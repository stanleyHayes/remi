"use client";

import { Children, isValidElement, useEffect, useId, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent, type ReactNode } from "react";
import { createPortal } from "react-dom";

type OptionProps = { children?: ReactNode; disabled?: boolean; value?: string | number };
type ChangeEvent = { target: { value: string }; currentTarget: { value: string } };
type Option = { disabled: boolean; label: string; value: string };

export type BrandedSelectProps = {
  "aria-label"?: string;
  children: ReactNode;
  className?: string;
  disabled?: boolean;
  id?: string;
  name?: string;
  onChange?: (event: ChangeEvent) => void;
  required?: boolean;
  menuLabel?: string;
  value?: string | number;
};

function optionsFrom(children: ReactNode): Option[] {
  const result: Option[] = [];
  Children.forEach(children, (child) => {
    if (Array.isArray(child)) { result.push(...optionsFrom(child)); return; }
    if (!isValidElement<OptionProps>(child)) return;
    if (child.type === "option") {
      result.push({ value: String(child.props.value ?? child.props.children ?? ""), label: Children.toArray(child.props.children).join(""), disabled: Boolean(child.props.disabled) });
      return;
    }
    result.push(...optionsFrom(child.props.children));
  });
  return result;
}

export function BrandedSelect({ "aria-label": ariaLabel, children, className = "", disabled, id, menuLabel, name, onChange, required, value = "" }: BrandedSelectProps) {
  const generatedId = useId();
  const listboxId = `${id || generatedId}-listbox`;
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const typeaheadRef = useRef("");
  const typeaheadTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const options = useMemo(() => optionsFrom(children), [children]);
  const selectedValue = String(value);
  const selected = options.find((option) => option.value === selectedValue);
  const [open, setOpen] = useState(false);
  const [ready, setReady] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);
  const [style, setStyle] = useState<CSSProperties>({});

  function positionMenu() {
    const rect = triggerRef.current?.getBoundingClientRect();
    if (!rect) return;
    const height = Math.min(372, options.length * 52 + 62);
    const below = window.innerHeight - rect.bottom;
    const width = Math.min(Math.max(rect.width, 240), window.innerWidth - 24);
    setStyle({
      left: Math.max(12, Math.min(rect.left, window.innerWidth - width - 12)),
      top: below > height + 12 ? rect.bottom + 8 : Math.max(12, rect.top - height - 8),
      width,
    });
  }

  useEffect(() => {
    if (!open) return;
    positionMenu();
    setActiveIndex(Math.max(0, options.findIndex((option) => option.value === selectedValue)));
    const close = (event: PointerEvent) => { const target = event.target as Node; if (!triggerRef.current?.contains(target) && !menuRef.current?.contains(target)) setOpen(false); };
    const closeForViewport = () => setOpen(false);
    const repositionForScroll = () => positionMenu();
    window.addEventListener("pointerdown", close);
    window.addEventListener("resize", closeForViewport);
    window.addEventListener("scroll", repositionForScroll, true);
    return () => { window.removeEventListener("pointerdown", close); window.removeEventListener("resize", closeForViewport); window.removeEventListener("scroll", repositionForScroll, true); };
  }, [open, options, selectedValue]);

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    if (window.matchMedia("(max-width: 520px)").matches) document.body.style.overflow = "hidden";
    return () => { document.body.style.overflow = previousOverflow; };
  }, [open]);

  useEffect(() => () => { if (typeaheadTimerRef.current) clearTimeout(typeaheadTimerRef.current); }, []);
  useEffect(() => setReady(true), []);

  function choose(option: Option) {
    if (option.disabled) return;
    const target = { value: option.value };
    onChange?.({ target, currentTarget: target });
    setOpen(false);
    triggerRef.current?.focus();
  }

  function initialIndex() {
    const current = options.findIndex((option) => option.value === selectedValue && !option.disabled);
    if (current >= 0) return current;
    const firstAvailable = options.findIndex((option) => !option.disabled);
    return Math.max(0, firstAvailable);
  }

  function openMenu() {
    setActiveIndex(initialIndex());
    setOpen(true);
  }

  function keyDown(event: KeyboardEvent<HTMLButtonElement>) {
    if (event.key === "Escape") { setOpen(false); return; }
    if (!options.length) return;
    if (event.key === "Enter" || event.key === " ") { event.preventDefault(); if (open) choose(options[activeIndex]); else openMenu(); return; }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      if (!open) { openMenu(); return; }
      const direction = event.key === "ArrowDown" ? 1 : -1;
      let next = activeIndex;
      do { next = (next + direction + options.length) % options.length; } while (options[next]?.disabled && next !== activeIndex);
      setActiveIndex(next);
      return;
    }
    if (event.key.length === 1 && /[\p{L}\p{N}]/u.test(event.key)) {
      typeaheadRef.current += event.key.toLocaleLowerCase();
      if (typeaheadTimerRef.current) clearTimeout(typeaheadTimerRef.current);
      typeaheadTimerRef.current = setTimeout(() => { typeaheadRef.current = ""; }, 650);
      const match = options.findIndex((option) => !option.disabled && option.label.toLocaleLowerCase().startsWith(typeaheadRef.current));
      if (match >= 0) { event.preventDefault(); setOpen(true); setActiveIndex(match); }
    }
  }

  return <div className={`member-branded-select ${className}`}>
    {name && <input type="hidden" name={name} value={selectedValue} />}
    <button ref={triggerRef} id={id} type="button" role="combobox" disabled={disabled} data-ready={ready || undefined} aria-label={ariaLabel || selected?.label || "Choose an option"} aria-haspopup="listbox" aria-controls={listboxId} aria-expanded={open} aria-required={required || undefined} aria-invalid={required && !selectedValue ? true : undefined} aria-activedescendant={open ? `${listboxId}-option-${activeIndex}` : undefined} className="member-branded-select__trigger" onClick={(event) => { event.preventDefault(); event.stopPropagation(); if (open) setOpen(false); else openMenu(); }} onKeyDown={keyDown}>
      <span className={selected ? "" : "is-placeholder"}>{selected?.label || "Choose an option"}</span><i aria-hidden="true"><span /><span /></i>
    </button>
    {open && typeof document !== "undefined" && createPortal(<div className="member-branded-select__layer"><button type="button" className="member-branded-select__backdrop" aria-label="Dismiss choices" onClick={() => { setOpen(false); triggerRef.current?.focus(); }} /><div ref={menuRef} id={listboxId} role="listbox" aria-label={ariaLabel || menuLabel || "Choices"} className="member-branded-select__menu" style={style}>
      <div className="member-branded-select__menu-head"><span>{menuLabel || ariaLabel || "Choose"}</span><i aria-hidden="true" /><small>{options.filter((option) => !option.disabled).length} choices</small><button type="button" aria-label="Close choices" onClick={() => { setOpen(false); triggerRef.current?.focus(); }}><span aria-hidden="true" /></button></div>
      <div className="member-branded-select__options">{options.map((option, index) => <button id={`${listboxId}-option-${index}`} key={`${option.value}-${index}`} type="button" role="option" aria-selected={option.value === selectedValue} data-active={index === activeIndex} disabled={option.disabled} className="member-branded-select__option" onMouseEnter={() => setActiveIndex(index)} onClick={() => choose(option)}><em aria-hidden="true">{String(index + 1).padStart(2, "0")}</em><span>{option.label}</span><b aria-hidden="true">{option.value === selectedValue ? "✓" : ""}</b></button>)}</div>
    </div></div>, document.body)}
  </div>;
}
