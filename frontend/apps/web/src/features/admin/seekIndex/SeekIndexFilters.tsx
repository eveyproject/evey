import { useTranslation } from "react-i18next";
import { FilterChip } from "@/components/ui/FilterChip";
import { Input } from "@/components/ui/Input";
import { Select } from "@/components/ui/Select";
import { SEEK_VIEWS } from "@evey/api";
import type { SeekSort, SeekView } from "@evey/api";
import type { SeekIndexPageState } from "./hooks";

type SeekIndexFiltersProps = Pick<
  SeekIndexPageState,
  "view" | "setView" | "query" | "setQuery" | "sort" | "setSort" | "counts" | "result"
> & { result: NonNullable<SeekIndexPageState["result"]> };

const VIEW_KEYS: Record<SeekView, string> = {
  attention: "admin.seekIndex.viewAttention",
  repairable: "admin.seekIndex.viewRepairable",
  informational: "admin.seekIndex.viewInformational",
  acked: "admin.seekIndex.viewAcked",
  backups: "admin.seekIndex.viewBackups",
  all: "admin.seekIndex.viewAll",
};

// The triage bar above the table: one chip per view with its live count, a
// search on title/path, the sort, and when the scan was taken. Replaces the
// single "only files needing attention" checkbox, which could not say what
// it was hiding.
export function SeekIndexFilters({ view, setView, query, setQuery, sort, setSort, counts, result }: SeekIndexFiltersProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("admin.seekIndex.viewsLabel")}>
        {SEEK_VIEWS.map((v) => (
          <FilterChip
            key={v}
            active={view === v}
            onClick={() => setView(v)}
            size="md"
            data-testid={`seek-index-view-${v}`}
          >
            {t(VIEW_KEYS[v], { count: counts[v] })}
          </FilterChip>
        ))}
      </div>
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div className="flex flex-wrap items-end gap-3">
          <div className="w-72">
            <Input
              type="search"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder={t("admin.seekIndex.searchPlaceholder")}
              aria-label={t("admin.seekIndex.searchPlaceholder")}
              data-testid="seek-index-search"
            />
          </div>
          <div className="w-44">
            <Select
              label={t("admin.seekIndex.sortLabel")}
              value={sort}
              onChange={(v) => setSort(v as SeekSort)}
              options={[
                { value: "priority", label: t("admin.seekIndex.sortPriority") },
                { value: "size", label: t("admin.seekIndex.sortSize") },
                { value: "title", label: t("admin.seekIndex.sortTitle") },
              ]}
            />
          </div>
        </div>
        <span className="text-xs text-[var(--text-muted)]" data-testid="seek-index-scanned-at">
          {t("admin.seekIndex.scannedAt", {
            date: new Date(Number(result.scannedAtMs)).toLocaleString(),
          })}
        </span>
      </div>
    </div>
  );
}
