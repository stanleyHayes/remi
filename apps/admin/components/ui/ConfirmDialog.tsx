"use client";

import Portal from "./Portal";

interface ConfirmDialogProps {
  isOpen: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  title: string;
  message: string;
  confirmLabel?: string;
  variant?: "danger" | "warning";
}

function AlertTriangleIcon({ className }: { className?: string }) {
  return (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className={className} aria-hidden="true">
      <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
      <line x1="12" x2="12" y1="9" y2="13" />
      <line x1="12" x2="12.01" y1="17" y2="17" />
    </svg>
  );
}

export default function ConfirmDialog({
  isOpen,
  onConfirm,
  onCancel,
  title,
  message,
  confirmLabel = "Confirm",
  variant = "danger",
}: ConfirmDialogProps) {
  if (!isOpen) return null;

  return (
    <Portal>
      <div className="fixed inset-0 z-[150] flex items-center justify-center p-4" role="dialog" aria-modal="true" aria-labelledby="confirm-title">
        <div className="fixed inset-0 bg-black/50 backdrop-blur-sm animate-fade-in" onClick={onCancel} aria-hidden="true" />
        <div className="admin-confirm-dialog relative w-full max-w-sm rounded-2xl border border-zinc-200 bg-white p-6 shadow-2xl animate-scale-in">
          <div className="mb-4 flex items-center gap-3">
            <div className={`rounded-full p-2 ${variant === "danger" ? "bg-rose-100" : "bg-[#f6ecce]"}`}>
              <AlertTriangleIcon className={variant === "danger" ? "text-rose-600" : "text-gold-dark"} />
            </div>
            <h3 id="confirm-title" className="text-lg font-semibold text-zinc-900">{title}</h3>
          </div>
          <p className="mb-6 text-sm text-zinc-600">{message}</p>
          <div className="flex justify-end gap-3">
            <button
              type="button"
              onClick={onCancel}
              className="rounded-md border border-zinc-300 px-4 py-2 text-sm font-medium text-zinc-600 transition hover:border-zinc-400"
            >
              Cancel
            </button>
            <button
              type="button"
              onClick={onConfirm}
              className={
                variant === "danger"
                  ? "rounded-md bg-rose-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-rose-700"
                  : "rounded-md bg-gold px-4 py-2 text-sm font-semibold text-sidebar transition hover:bg-gold-dark"
              }
            >
              {confirmLabel}
            </button>
          </div>
        </div>
      </div>
    </Portal>
  );
}
