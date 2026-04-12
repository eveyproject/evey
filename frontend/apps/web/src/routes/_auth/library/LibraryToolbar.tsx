import { ArrowUpNarrowWide, ArrowDownWideNarrow, List, LayoutGrid } from "lucide-react";
import type { LibraryServiceListMediaItemsSortOrder } from "@evey/api";

export function SortOrderButton({
  order,
  onChange,
}: {
  order: LibraryServiceListMediaItemsSortOrder;
  onChange: (o: LibraryServiceListMediaItemsSortOrder) => void;
}) {
  const isAsc = order === "SORT_ORDER_ASC";
  return (
    <button
      type="button"
      onClick={() => onChange(isAsc ? "SORT_ORDER_DESC" : "SORT_ORDER_ASC")}
      className="p-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)] transition-colors-fast"
      title={isAsc ? "Ascending" : "Descending"}
      aria-label={isAsc ? "Sort ascending" : "Sort descending"}
    >
      {isAsc ? <ArrowUpNarrowWide size={16} /> : <ArrowDownWideNarrow size={16} />}
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
  return (
    <button
      type="button"
      onClick={() => onChange(mode === "grid" ? "list" : "grid")}
      className="p-2 rounded-[var(--radius-md)] bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text-muted)] hover:text-[var(--text)] hover:border-[var(--accent)] transition-colors-fast"
      title={mode === "grid" ? "Switch to list view" : "Switch to grid view"}
      aria-label={mode === "grid" ? "Switch to list view" : "Switch to grid view"}
      data-testid="view-toggle"
    >
      {mode === "grid" ? <List size={16} /> : <LayoutGrid size={16} />}
    </button>
  );
}
