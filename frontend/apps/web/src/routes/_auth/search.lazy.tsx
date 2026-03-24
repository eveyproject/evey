import { useState, useEffect, useRef } from "react";
import { Search, X, ListFilter } from "lucide-react";
import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { libraryServiceSearchMedia } from "@evey/api";
import { toast } from "@evey/store";
import { EmptyState } from "@/components/ui/EmptyState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";
import { MediaGrid } from "@/features/library/MediaGrid";
import { MediaCard } from "@/features/library/MediaCard";
import { useRecentSearches } from "@/hooks/useRecentSearches";
import { useDebounce } from "@/hooks/useDebounce";

export const Route = createLazyFileRoute("/_auth/search")({
  component: SearchPage,
});

type MediaTypeFilter = "MEDIA_ITEM_TYPE_MOVIE" | "MEDIA_ITEM_TYPE_SERIES";

const COMMON_GENRES = [
  "Action", "Adventure", "Animation", "Comedy", "Crime", "Documentary",
  "Drama", "Fantasy", "Horror", "Mystery", "Romance", "Sci-Fi",
  "Thriller", "War", "Western",
];

function SearchPage() {
  const { t } = useTranslation();
  const { q } = Route.useSearch();
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);

  const typeFilters: { label: string; value: MediaTypeFilter | undefined }[] = [
    { label: t("search.all"), value: undefined },
    { label: t("search.movies"), value: "MEDIA_ITEM_TYPE_MOVIE" },
    { label: t("search.series"), value: "MEDIA_ITEM_TYPE_SERIES" },
  ];

  const sortOptions = [
    { label: t("search.relevance"), field: undefined, order: undefined },
    { label: t("search.titleAZ"), field: "MEDIA_SORT_FIELD_TITLE" as const, order: "SORT_ORDER_ASC" as const },
    { label: t("search.yearNewest"), field: "MEDIA_SORT_FIELD_YEAR" as const, order: "SORT_ORDER_DESC" as const },
    { label: t("search.dateAdded"), field: "MEDIA_SORT_FIELD_DATE_ADDED" as const, order: "SORT_ORDER_DESC" as const },
  ];

  const [inputValue, setInputValue] = useState(q ?? "");
  const debouncedQuery = useDebounce(inputValue.trim(), 300);
  const [typeFilter, setTypeFilter] = useState<MediaTypeFilter | undefined>(undefined);
  const [genre, setGenre] = useState("");
  const [yearMin, setYearMin] = useState("");
  const [yearMax, setYearMax] = useState("");
  const [sortIdx, setSortIdx] = useState(0);
  const [showFilters, setShowFilters] = useState(false);
  const { recents, push: pushRecent, remove: removeRecent, clear: clearRecents } = useRecentSearches();

  useEffect(() => {
    if (q !== undefined && q !== inputValue) {
      setInputValue(q);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- inputValue excluded to avoid loop: q → setInputValue → inputValue changes → re-run
  }, [q]);

  useEffect(() => {
    if (debouncedQuery.length > 1) pushRecent(debouncedQuery);
    void navigate({
      to: "/search",
      search: debouncedQuery ? { q: debouncedQuery } : {},
      replace: true,
    });
  }, [debouncedQuery, navigate, pushRecent]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const sort = sortOptions[sortIdx];
  const parsedYearMin = yearMin ? parseInt(yearMin, 10) : undefined;
  const parsedYearMax = yearMax ? parseInt(yearMax, 10) : undefined;

  const { data, isLoading, isError } = useQuery({
    queryKey: ["/api/v1/media/search", debouncedQuery, typeFilter, genre, parsedYearMin, parsedYearMax, sort.field, sort.order],
    queryFn: ({ signal }) =>
      libraryServiceSearchMedia(
        {
          query: debouncedQuery,
          ...(typeFilter ? { type: typeFilter } : {}),
          ...(genre ? { genre } : {}),
          ...(parsedYearMin ? { yearMin: parsedYearMin } : {}),
          ...(parsedYearMax ? { yearMax: parsedYearMax } : {}),
          ...(sort.field ? { sortField: sort.field, sortOrder: sort.order } : {}),
          limit: 40,
        },
        signal,
      ),
    enabled: debouncedQuery.length > 1,
  });

  useEffect(() => {
    if (isError) toast.error(t("search.searchFailed"));
  }, [isError, t]);

  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? items.length;

  const hasQuery = debouncedQuery.length > 1;
  const hasActiveFilters = !!typeFilter || !!genre || !!yearMin || !!yearMax || sortIdx !== 0;

  const clearAllFilters = () => {
    setTypeFilter(undefined);
    setGenre("");
    setYearMin("");
    setYearMax("");
    setSortIdx(0);
  };

  return (
    <div>
      <div className="mb-6">
        <div className="relative max-w-xl">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none" size={20} style={{ color: "var(--text-muted)" }} />
          <input
            ref={inputRef}
            type="search"
            placeholder={t("search.placeholder")}
            data-testid="search-page-input"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            className="w-full pl-10 pr-4 py-3 rounded-[var(--radius-lg)] text-base bg-[var(--surface)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] transition-shadow"
          />
          {inputValue && (
            <button
              onClick={() => {
                setInputValue("");
                inputRef.current?.focus();
              }}
              className="absolute right-3 top-1/2 -translate-y-1/2 w-5 h-5 rounded-full flex items-center justify-center hover:bg-[var(--surface-raised)] transition-colors cursor-pointer"
              style={{ color: "var(--text-muted)" }}
              aria-label={t("search.clearSearch")}
            >
              <X size={16} />
            </button>
          )}
        </div>
      </div>

      {hasQuery && (
        <div className="mb-5 space-y-3">
          <div className="flex items-center gap-2 flex-wrap">
            <div className="flex items-center gap-1" data-testid="search-type-filters" role="radiogroup" aria-label={t("search.filterByType")}>
              {typeFilters.map((f) => (
                <button
                  key={f.value ?? "all"}
                  data-testid={`search-filter-${f.value === undefined ? "all" : f.value === "MEDIA_ITEM_TYPE_MOVIE" ? "movies" : "series"}`}
                  role="radio"
                  aria-checked={typeFilter === f.value}
                  onClick={() => setTypeFilter(f.value)}
                  onKeyDown={(e) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); setTypeFilter(f.value); } }}
                  className="px-3 py-1.5 rounded-full text-sm font-medium transition-colors cursor-pointer"
                  style={{
                    backgroundColor:
                      typeFilter === f.value ? "var(--accent)" : "var(--surface)",
                    color:
                      typeFilter === f.value ? "var(--filter-active-text)" : "var(--text-secondary)",
                    border: `1px solid ${typeFilter === f.value ? "var(--accent)" : "var(--border)"}`,
                  }}
                >
                  {f.label}
                </button>
              ))}
            </div>

            <button
              data-testid="search-toggle-filters"
              aria-label={t("search.filters")}
              aria-expanded={showFilters}
              onClick={() => setShowFilters((v) => !v)}
              className="px-3 py-1.5 rounded-full text-sm font-medium transition-colors cursor-pointer flex items-center gap-1.5"
              style={{
                backgroundColor: showFilters ? "var(--accent)" : "var(--surface)",
                color: showFilters ? "var(--filter-active-text)" : "var(--text-secondary)",
                border: `1px solid ${showFilters ? "var(--accent)" : "var(--border)"}`,
              }}
            >
              <ListFilter size={14} />
              {t("search.filters")}
              {hasActiveFilters && !showFilters && (
                <span
                  className="w-2 h-2 rounded-full"
                  style={{ backgroundColor: "var(--accent)" }}
                />
              )}
            </button>

            {hasActiveFilters && (
              <button
                onClick={clearAllFilters}
                className="text-xs hover:underline cursor-pointer"
                style={{ color: "var(--text-muted)" }}
              >
                {t("search.clearFilters")}
              </button>
            )}
          </div>

          {showFilters && (
            <div
              data-testid="search-filters-panel"
              className="flex flex-wrap gap-4 p-4 rounded-[var(--radius-lg)]"
              style={{
                backgroundColor: "var(--surface)",
                border: "1px solid var(--border)",
              }}
            >
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium" style={{ color: "var(--text-muted)" }}>
                  {t("search.genre")}
                </label>
                <select
                  data-testid="search-filter-genre"
                  aria-label={t("search.genre")}
                  value={genre}
                  onChange={(e) => setGenre(e.target.value)}
                  className="px-3 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] cursor-pointer"
                >
                  <option value="">{t("search.allGenres")}</option>
                  {COMMON_GENRES.map((g) => (
                    <option key={g} value={g}>{g}</option>
                  ))}
                </select>
              </div>

              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium" style={{ color: "var(--text-muted)" }}>
                  {t("search.year")}
                </label>
                <div className="flex items-center gap-2">
                  <input
                    data-testid="search-filter-year-min"
                    type="number"
                    aria-label={t("search.fromYear")}
                    placeholder={t("search.fromYear")}
                    value={yearMin}
                    onChange={(e) => setYearMin(e.target.value)}
                    className="w-20 px-2 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
                    min={1900}
                    max={2099}
                  />
                  <span className="text-xs" style={{ color: "var(--text-muted)" }}>-</span>
                  <input
                    data-testid="search-filter-year-max"
                    type="number"
                    aria-label={t("search.toYear")}
                    placeholder={t("search.toYear")}
                    value={yearMax}
                    onChange={(e) => setYearMax(e.target.value)}
                    className="w-20 px-2 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
                    min={1900}
                    max={2099}
                  />
                </div>
              </div>

              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium" style={{ color: "var(--text-muted)" }}>
                  {t("search.sortBy")}
                </label>
                <select
                  data-testid="search-filter-sort"
                  aria-label={t("search.sortBy")}
                  value={sortIdx}
                  onChange={(e) => setSortIdx(Number(e.target.value))}
                  className="px-3 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] cursor-pointer"
                >
                  {sortOptions.map((opt, i) => (
                    <option key={i} value={i}>{opt.label}</option>
                  ))}
                </select>
              </div>
            </div>
          )}
        </div>
      )}

      {hasQuery && isLoading && (
        <div>
          <div
            className="h-4 w-32 mb-4 animate-pulse rounded"
            style={{ backgroundColor: "var(--surface-raised)" }}
          />
          <MediaGridSkeleton count={12} />
        </div>
      )}

      {hasQuery && !isLoading && items.length === 0 && (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{
            backgroundColor: "var(--surface)",
            border: "1px solid var(--border)",
          }}
        >
          <EmptyState
            icon={<Search size={48} strokeWidth={1.5} />}
            title={t("search.noResultsFor", { query: debouncedQuery })}
            description={t("search.noResultsDesc")}
          />
        </div>
      )}

      {!hasQuery && !isLoading && recents.length > 0 && (
        <div className="mb-6">
          <div className="flex items-center justify-between mb-3">
            <span className="text-sm font-semibold" style={{ color: "var(--text-muted)" }}>{t("search.recentSearches")}</span>
            <button
              onClick={clearRecents}
              className="text-xs hover:underline cursor-pointer"
              style={{ color: "var(--text-muted)" }}
            >
              {t("search.clearAll")}
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            {recents.map((r) => (
              <div
                key={r}
                className="flex items-center gap-1 px-3 py-1.5 rounded-full text-sm"
                style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)", color: "var(--text)" }}
              >
                <button
                  className="hover:text-[var(--accent)] cursor-pointer transition-colors"
                  onClick={() => setInputValue(r)}
                >
                  {r}
                </button>
                <button
                  onClick={() => removeRecent(r)}
                  className="ml-1 cursor-pointer transition-colors hover:opacity-80"
                  style={{ color: "var(--text-muted)" }}
                  aria-label={`Remove ${r}`}
                >
                  <X size={12} />
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {!hasQuery && !isLoading && recents.length === 0 && (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{
            backgroundColor: "var(--surface)",
            border: "1px solid var(--border)",
          }}
        >
          <EmptyState
            icon={<Search size={48} strokeWidth={1.5} />}
            title={t("search.searchLibrary")}
            description={t("search.searchLibraryDesc")}
          />
        </div>
      )}

      {items.length > 0 && (
        <>
          <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
            {t("search.resultCount", { count: totalHits })} {t("search.resultFor", { query: debouncedQuery })}
          </p>
          <MediaGrid>
            {items.map((item) => (
              <MediaCard key={item.id} item={item} showType lite />
            ))}
          </MediaGrid>
        </>
      )}
    </div>
  );
}
