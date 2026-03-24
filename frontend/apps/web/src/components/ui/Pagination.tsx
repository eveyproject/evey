import { ChevronLeft, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  if (totalPages <= 1) return null;

  const pages = getPageNumbers(page, totalPages);

  return (
    <nav className="flex items-center gap-1" aria-label={t("common.aria.pagination")}>
      <button
        className={`${btnBase} bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]`}
        disabled={page <= 1}
        onClick={() => onPageChange(page - 1)}
        aria-label={t("common.aria.previousPage")}
      >
        <ChevronLeft size={16} />
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
                ? "bg-[var(--accent)] text-[var(--filter-active-text)]"
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
        aria-label={t("common.aria.nextPage")}
      >
        <ChevronRight size={16} />
      </button>
    </nav>
  );
}
