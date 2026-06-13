import { ArrowUpNarrowWide, ArrowDownWideNarrow, List, LayoutGrid } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { LibraryServiceListMediaItemsSortOrder } from "@evey/api";

export function SortOrderButton({
  order,
  onChange,
}: {
  order: LibraryServiceListMediaItemsSortOrder;
  onChange: (o: LibraryServiceListMediaItemsSortOrder) => void;
}) {
  const { t } = useTranslation();
  const isAsc = order === "SORT_ORDER_ASC";
  return (
    <button
      type="button"
      onClick={() => onChange(isAsc ? "SORT_ORDER_DESC" : "SORT_ORDER_ASC")}
      className="p-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)] transition-colors-fast"
      title={isAsc ? t("common.sortAscending") : t("common.sortDescending")}
      aria-label={isAsc ? t("common.sortAscending") : t("common.sortDescending")}
    >
      {isAsc ? <ArrowUpNarrowWide size={16}  aria-hidden="true" /> : <ArrowDownWideNarrow size={16}  aria-hidden="true" />}
    </button>
  );
}

export function ViewToggleButton({
  mode,
  onChange,
}: {
  mode: "grid" | "list";
  onChange: (m: "grid" | "list") => void;
}) {
  const { t } = useTranslation();
  return (
    <button
      type="button"
      onClick={() => onChange(mode === "grid" ? "list" : "grid")}
      className="p-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)] transition-colors-fast"
      title={mode === "grid" ? t("common.switchToListView") : t("common.switchToGridView")}
      aria-label={mode === "grid" ? t("common.switchToListView") : t("common.switchToGridView")}
      data-testid="view-toggle"
    >
      {mode === "grid" ? <List size={16}  aria-hidden="true" /> : <LayoutGrid size={16}  aria-hidden="true" />}
    </button>
  );
}
