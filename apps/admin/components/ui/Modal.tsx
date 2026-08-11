"use client";

import { useEffect, type ReactNode } from "react";
import Portal from "./Portal";

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  className?: string;
}

function XIcon() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M18 6 6 18" />
      <path d="m6 6 12 12" />
    </svg>
  );
}

export function Modal({ open, onClose, title, children, className = "" }: ModalProps) {
  useEffect(() => {
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  return (
    <Portal>
      <div className="fixed inset-0 z-[150] flex items-center justify-center" role="dialog" aria-modal="true">
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm animate-fade-in" onClick={onClose} aria-hidden="true" />
        <div
          className={`admin-modal relative z-10 mx-4 max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-2xl border border-zinc-200 bg-white shadow-xl animate-scale-in ${className}`}
        >
          <div className="admin-modal-header sticky top-0 z-10 flex items-center justify-between border-b border-zinc-100 bg-white px-6 pb-4 pt-5">
            <h2 className="text-lg font-semibold text-zinc-900">{title}</h2>
            <button
              onClick={onClose}
              aria-label="Close"
              className="rounded-lg p-1 text-zinc-400 transition-colors hover:bg-zinc-100 hover:text-zinc-700"
            >
              <XIcon />
            </button>
          </div>
          <div className="px-6 py-5">{children}</div>
        </div>
      </div>
    </Portal>
  );
}
