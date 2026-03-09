type PaginationProps = {
  page: number;
  totalPages: number;
  onPageChange: (page: number) => void;
};

function getPageNumbers(current: number, total: number): (number | "ellipsis")[] {
  if (total <= 7) return Array.from({ length: total }, (_, i) => i + 1);

  const pages: (number | "ellipsis")[] = [1];

  const rangeStart = Math.max(2, current - 1);
  const rangeEnd = Math.min(total - 1, current + 1);

  if (rangeStart > 2) pages.push("ellipsis");
  for (let i = rangeStart; i <= rangeEnd; i++) pages.push(i);
  if (rangeEnd < total - 1) pages.push("ellipsis");

  pages.push(total);
  return pages;
}

const btnBase =
  "inline-flex items-center justify-center min-w-[2rem] h-8 px-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors disabled:opacity-40 disabled:cursor-not-allowed";

export function Pagination({ page, totalPages, onPageChange }: PaginationProps) {
  if (totalPages <= 1) return null;

  const pages = getPageNumbers(page, totalPages);

  return (
    <nav className="flex items-center gap-1" aria-label="Pagination">
      <button
        className={`${btnBase} bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]`}
        disabled={page <= 1}
        onClick={() => onPageChange(page - 1)}
        aria-label="Previous page"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 19l-7-7 7-7" />
        </svg>
      </button>

      {pages.map((p, i) =>
        p === "ellipsis" ? (
          <span
            key={`e${i}`}
            className="inline-flex items-center justify-center min-w-[2rem] h-8 text-sm"
            style={{ color: "var(--text-muted)" }}
          >
            ...
          </span>
        ) : (
          <button
            key={p}
            onClick={() => onPageChange(p)}
            className={`${btnBase} ${
              p === page
                ? "bg-[var(--accent)] text-white"
                : "bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
            }`}
            aria-current={p === page ? "page" : undefined}
          >
            {p}
          </button>
        ),
      )}

      <button
        className={`${btnBase} bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]`}
        disabled={page >= totalPages}
        onClick={() => onPageChange(page + 1)}
        aria-label="Next page"
      >
        <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 5l7 7-7 7" />
        </svg>
      </button>
    </nav>
  );
}
