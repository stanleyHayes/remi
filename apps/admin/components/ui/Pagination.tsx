"use client";

interface PaginationProps {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
  totalItems?: number;
  pageSize?: number;
  className?: string;
}

function ChevronLeftIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m15 18-6-6 6-6" />
    </svg>
  );
}

function ChevronRightIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m9 18 6-6-6-6" />
    </svg>
  );
}

function ChevronsLeftIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m11 17-5-5 5-5" />
      <path d="m18 17-5-5 5-5" />
    </svg>
  );
}

function ChevronsRightIcon() {
  return (
    <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="m6 17 5-5-5-5" />
      <path d="m13 17 5-5-5-5" />
    </svg>
  );
}

export default function Pagination({ page, totalPages, onPageChange, totalItems, pageSize, className = "" }: PaginationProps) {
  // With a single page, still surface a count footer if we have item totals.
  if (totalPages <= 1) {
    if (totalItems == null) return null;
    return (
      <div className={`flex items-center justify-end border-t border-zinc-100 px-4 py-3 ${className}`}>
        <p className="text-xs text-zinc-500">
          Showing all <span className="font-medium text-zinc-700">{totalItems.toLocaleString()}</span>{" "}
          {totalItems === 1 ? "item" : "items"}
        </p>
      </div>
    );
  }

  const getVisiblePages = () => {
    const pages: (number | "ellipsis")[] = [];
    const maxVisible = 5;

    if (totalPages <= maxVisible + 2) {
      for (let i = 1; i <= totalPages; i++) pages.push(i);
      return pages;
    }

    pages.push(1);

    let start = Math.max(2, page - 1);
    let end = Math.min(totalPages - 1, page + 1);

    if (page <= 3) {
      start = 2;
      end = Math.min(maxVisible, totalPages - 1);
    } else if (page >= totalPages - 2) {
      start = Math.max(2, totalPages - maxVisible + 1);
      end = totalPages - 1;
    }

    if (start > 2) pages.push("ellipsis");
    for (let i = start; i <= end; i++) pages.push(i);
    if (end < totalPages - 1) pages.push("ellipsis");

    pages.push(totalPages);
    return pages;
  };

  const visiblePages = getVisiblePages();

  const startItem = totalItems && pageSize ? (page - 1) * pageSize + 1 : null;
  const endItem = totalItems && pageSize ? Math.min(page * pageSize, totalItems) : null;

  const navBtn =
    "rounded-lg p-1.5 text-zinc-400 transition-colors hover:bg-zinc-100 disabled:cursor-not-allowed disabled:opacity-30";

  return (
    <div className={`flex items-center justify-between gap-4 border-t border-zinc-100 px-4 py-3 ${className}`}>
      {totalItems != null && startItem != null && endItem != null ? (
        <p className="text-xs text-zinc-500">
          Showing <span className="font-medium text-zinc-700">{startItem}-{endItem}</span> of{" "}
          <span className="font-medium text-zinc-700">{totalItems.toLocaleString()}</span>
        </p>
      ) : (
        <p className="text-xs text-zinc-500">
          Page <span className="font-medium text-zinc-700">{page}</span> of{" "}
          <span className="font-medium text-zinc-700">{totalPages}</span>
        </p>
      )}

      <div className="flex items-center gap-1">
        <button onClick={() => onPageChange(1)} disabled={page === 1} className={navBtn} title="First page">
          <ChevronsLeftIcon />
        </button>
        <button onClick={() => onPageChange(page - 1)} disabled={page === 1} className={navBtn} title="Previous page">
          <ChevronLeftIcon />
        </button>

        {visiblePages.map((p, i) =>
          p === "ellipsis" ? (
            <span key={`e${i}`} className="px-1 text-xs text-zinc-400">…</span>
          ) : (
            <button
              key={p}
              onClick={() => onPageChange(p)}
              className={`h-8 min-w-[32px] rounded-lg text-xs font-medium transition-colors ${
                p === page ? "bg-gold text-sidebar shadow-sm" : "text-zinc-600 hover:bg-zinc-100"
              }`}
            >
              {p}
            </button>
          ),
        )}

        <button onClick={() => onPageChange(page + 1)} disabled={page === totalPages} className={navBtn} title="Next page">
          <ChevronRightIcon />
        </button>
        <button onClick={() => onPageChange(totalPages)} disabled={page === totalPages} className={navBtn} title="Last page">
          <ChevronsRightIcon />
        </button>
      </div>
    </div>
  );
}
