export function PageHeader({
  title,
  subtitle,
  children,
}: {
  title: string;
  subtitle?: string;
  children?: React.ReactNode;
}) {
  return (
    <header className="admin-page-header">
      <div className="admin-page-heading">
        <span className="admin-page-eyebrow">Ministry operations / {title}</span>
        <h1>{title}</h1>
        {subtitle && <p>{subtitle}</p>}
      </div>
      {children && <div className="admin-page-actions">{children}</div>}
    </header>
  );
}

export function Loading({ label = "Loading…" }: { label?: string }) {
  return (
    <div className="admin-loading-surface flex items-center gap-3 rounded-lg border border-zinc-200 bg-white p-8 text-sm text-zinc-500">
      <span className="h-4 w-4 animate-spin rounded-full border-2 border-zinc-300 border-t-gold" />
      {label}
    </div>
  );
}

export function ErrorBox({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div className="admin-error-surface rounded-lg border border-rose-200 bg-rose-50 p-4 text-sm text-rose-800">
      <p className="font-medium">Something went wrong</p>
      <p className="mt-1">{message}</p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="mt-3 rounded-md bg-rose-100 px-3 py-1.5 text-xs font-medium text-rose-800 hover:bg-rose-200"
        >
          Retry
        </button>
      )}
    </div>
  );
}

function EmptyStateIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className="size-5"
      aria-hidden="true"
    >
      <polyline points="22 12 16 12 14 15 10 15 8 12 2 12" />
      <path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z" />
    </svg>
  );
}

export function EmptyState({
  title,
  hint,
  icon,
  action,
  dense = false,
}: {
  title: string;
  hint?: string;
  icon?: React.ReactNode;
  action?: React.ReactNode;
  dense?: boolean;
}) {
  return (
    <div
      className={`admin-empty-surface rounded-lg border border-zinc-200 bg-white text-center shadow-sm ${dense ? "px-5 py-8" : "px-6 py-14"}`}
    >
      <div className="remi-empty-state-icon mx-auto mb-5 grid size-16 place-items-center rounded-[22px] bg-gold/10 text-gold-dark">
        <span className="remi-empty-state-icon__halo" aria-hidden="true" />
        <span className="relative grid size-10 place-items-center rounded-2xl bg-white shadow-sm">
          {icon ?? <EmptyStateIcon />}
        </span>
      </div>
      <p className="text-base font-semibold text-zinc-900">{title}</p>
      {hint && <p className="mx-auto mt-2 max-w-md text-sm text-zinc-500">{hint}</p>}
      {action && <div className="mt-5 flex justify-center gap-2">{action}</div>}
    </div>
  );
}

export function Card({ children, className = "" }: { children: React.ReactNode; className?: string }) {
  return <section className={`admin-card ${className}`}>{children}</section>;
}
