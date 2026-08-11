interface SkeletonProps {
  className?: string;
  style?: React.CSSProperties;
}

export function Skeleton({ className = "", style }: SkeletonProps) {
  return <div className={`admin-skeleton animate-pulse rounded-md bg-zinc-200 ${className}`} style={style} aria-hidden="true" />;
}

export function SkeletonText({ lines = 3, className = "" }: { lines?: number; className?: string }) {
  return (
    <div className={`space-y-2 ${className}`}>
      {Array.from({ length: lines }).map((_, i) => (
        <Skeleton key={i} className={`h-3.5 ${i === lines - 1 ? "w-2/3" : "w-full"}`} />
      ))}
    </div>
  );
}

export function SkeletonCard({ className = "" }: SkeletonProps) {
  return (
    <div className={`admin-skeleton-surface space-y-4 rounded-lg border border-zinc-200 bg-white p-5 shadow-sm ${className}`}>
      <div className="flex items-center gap-3">
        <Skeleton className="h-10 w-10 rounded-full" />
        <div className="flex-1 space-y-2">
          <Skeleton className="h-4 w-1/3" />
          <Skeleton className="h-3 w-1/2" />
        </div>
      </div>
      <SkeletonText lines={2} />
    </div>
  );
}

export function SkeletonTable({ rows = 5, cols = 4 }: { rows?: number; cols?: number }) {
  return (
    <div className="admin-skeleton-surface overflow-hidden rounded-lg border border-zinc-200 bg-white shadow-sm">
      <div className="admin-skeleton-header flex gap-4 border-b border-zinc-200 bg-zinc-50 p-4">
        {Array.from({ length: cols }).map((_, i) => (
          <Skeleton key={i} className={`h-3.5 shrink-0 ${i === 0 ? "w-32" : "w-20"}`} />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, r) => (
        <div key={r} className="flex gap-4 border-b border-zinc-100 p-4 last:border-0">
          {Array.from({ length: cols }).map((_, c) => (
            <Skeleton key={c} className={`h-3.5 shrink-0 ${c === 0 ? "w-36" : "w-16"}`} />
          ))}
        </div>
      ))}
    </div>
  );
}

/** Stat-card grid placeholder for the dashboard. */
export function SkeletonStats({ count = 8 }: { count?: number }) {
  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="admin-skeleton-surface space-y-3 rounded-lg border border-zinc-200 bg-white p-5 shadow-sm">
          <Skeleton className="h-7 w-16" />
          <Skeleton className="h-3.5 w-24" />
        </div>
      ))}
    </div>
  );
}

export function SkeletonDashboard() {
  return <div role="status" aria-label="Loading ministry intelligence" aria-busy="true" className="space-y-5"><SkeletonStats count={8}/><div className="grid grid-cols-1 gap-5 xl:grid-cols-[1.45fr_.55fr]"><div className="admin-skeleton-surface rounded-2xl border border-zinc-200 bg-white p-7"><Skeleton className="h-5 w-44"/><Skeleton className="mt-8 h-56 w-full rounded-xl"/></div><div className="admin-skeleton-surface rounded-2xl border border-zinc-200 bg-white p-7"><Skeleton className="h-5 w-36"/><Skeleton className="mx-auto mt-8 size-44 rounded-full"/></div></div><div className="grid grid-cols-1 gap-5 xl:grid-cols-2">{[0,1].map(i=><div key={i} className="admin-skeleton-surface rounded-2xl border border-zinc-200 bg-white p-7"><Skeleton className="h-5 w-40"/><div className="mt-7 space-y-4">{Array.from({length:5},(_,j)=><Skeleton key={j} className="h-7 w-full rounded-full"/>)}</div></div>)}</div></div>;
}

/** Stacked submission-card placeholder for the inbox. */
export function SkeletonCards({ count = 3 }: { count?: number }) {
  return (
    <div className="space-y-3">
      {Array.from({ length: count }).map((_, i) => (
        <SkeletonCard key={i} />
      ))}
    </div>
  );
}
