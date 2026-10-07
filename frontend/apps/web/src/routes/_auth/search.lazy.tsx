import { useState, useEffect, useRef } from "react";
import { Search, X, ListFilter, Music } from "lucide-react";
import { createLazyFileRoute, useNavigate, Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { libraryServiceSearchMedia, musicServiceSearchMusic } from "@evey/api";
import { toast } from "@evey/store";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { MediaGridSkeleton } from "@/components/ui/Skeleton";
import { MediaGrid } from "@/features/library/MediaGrid";
import { ICON } from "@/constants/iconSizes";
import { MediaCardWithModals as MediaCard } from "@/features/library/MediaCardWithModals";
import { useRecentSearches } from "@/hooks/useRecentSearches";
import { useDebounce } from "@/hooks/useDebounce";
import { FilterChip } from "@/components/ui/FilterChip";
import { coverArtSrc } from "../../lib/coverArtSrc";

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
  const { q, kind } = Route.useSearch();
  const navigate = useNavigate();
  const inputRef = useRef<HTMLInputElement>(null);

  // Top-level search scope: video catalog (default) or music.
  const [searchKind, setSearchKind] = useState<"media" | "music">(kind === "music" ? "music" : "media");

  const typeFilters: { label: string; value: MediaTypeFilter | undefined }[] = [
    { label: t("media.search.all"), value: undefined },
    { label: t("media.search.movies"), value: "MEDIA_ITEM_TYPE_MOVIE" },
    { label: t("media.search.series"), value: "MEDIA_ITEM_TYPE_SERIES" },
  ];

  const sortOptions = [
    { label: t("media.search.relevance"), field: undefined, order: undefined },
    { label: t("media.search.titleAZ"), field: "MEDIA_SORT_FIELD_TITLE" as const, order: "SORT_ORDER_ASC" as const },
    { label: t("media.search.yearNewest"), field: "MEDIA_SORT_FIELD_YEAR" as const, order: "SORT_ORDER_DESC" as const },
    { label: t("media.search.dateAdded"), field: "MEDIA_SORT_FIELD_DATE_ADDED" as const, order: "SORT_ORDER_DESC" as const },
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
    // The URL follows the DEBOUNCED input, so a q that equals it is this
    // page's own echo of what was typed: syncing it back erased the letters
    // typed after the debounce fired (and the trailing space before trim).
    // Only an outside change (back/forward, a link) reaches the field.
    if (q !== undefined && q !== inputValue && q !== debouncedQuery) {
      setInputValue(q);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- inputValue/debouncedQuery excluded to avoid loop: q → setInputValue → inputValue changes → re-run
  }, [q]);

  // Keep the scope in sync when the URL kind changes (e.g. arriving from the
  // search bar with music pre-selected).
  useEffect(() => {
    setSearchKind(kind === "music" ? "music" : "media");
  }, [kind]);

  // NOTE: recents are committed on an explicit action (blur / Enter), not here -
  // pushing on every debounced value would save every keystroke prefix ("in",
  // "inc", "ince", …) and evict real past searches (MAX 8).
  useEffect(() => {
    const urlKind = kind === "music" ? "music" : undefined;
    const nextKind = searchKind === "music" ? "music" : undefined;
    if ((q ?? "") !== debouncedQuery || urlKind !== nextKind) {
      void navigate({
        to: "/search",
        search: {
          ...(debouncedQuery ? { q: debouncedQuery } : {}),
          ...(nextKind ? { kind: nextKind } : {}),
        },
        replace: true,
      });
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- q/kind excluded: we only push to URL here (URL→state sync is above)
  }, [debouncedQuery, searchKind, navigate, pushRecent]);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  const sort = sortOptions[sortIdx];
  const parsedYearMin = yearMin ? parseInt(yearMin, 10) : undefined;
  const parsedYearMax = yearMax ? parseInt(yearMax, 10) : undefined;

  const { data, isLoading, isError, error, refetch } = useQuery({
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
    enabled: debouncedQuery.length > 1 && searchKind === "media",
  });

  useEffect(() => {
    if (isError) toast.error(t("media.search.searchFailed"));
  }, [isError, t]);

  const items = data?.items ?? [];
  const totalHits = data?.totalHits ?? items.length;


  // Music search (separate index, cross-library) - only when the music scope is
  // selected.
  const { data: musicData, isLoading: musicLoading, isError: musicIsError, error: musicError, refetch: refetchMusic } = useQuery({
    queryKey: ["/api/v1/music/search", debouncedQuery],
    queryFn: ({ signal }) => musicServiceSearchMusic({ query: debouncedQuery, limit: 40 }, signal),
    enabled: debouncedQuery.length > 1 && searchKind === "music",
  });
  const musicItems = musicData?.tracks ?? [];

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
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none text-[var(--text-muted)]" size={ICON.lg} />
          <input
            ref={inputRef}
            type="search"
            aria-label={t("media.search.placeholder")}
            placeholder={t("media.search.placeholder")}
            data-testid="search-page-input"
            value={inputValue}
            onChange={(e) => setInputValue(e.target.value)}
            // Commit to recents on a deliberate action (leaving the field - e.g.
            // clicking a result - or pressing Enter), not on every keystroke.
            onBlur={() => { const v = inputValue.trim(); if (v.length > 1) pushRecent(v); }}
            onKeyDown={(e) => {
              if (e.key === "Enter") { const v = inputValue.trim(); if (v.length > 1) pushRecent(v); }
            }}
            className="w-full pl-10 pr-4 py-3 rounded-[var(--radius-lg)] text-base bg-[var(--surface)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] transition-shadow"
          />
          {inputValue && (
            <button
              onClick={() => {
                setInputValue("");
                inputRef.current?.focus();
              }}
              className="absolute right-3 top-1/2 -translate-y-1/2 w-5 h-5 rounded-full flex items-center justify-center hover:bg-[var(--surface-raised)] transition-colors-fast cursor-pointer text-[var(--text-muted)]"
              aria-label={t("media.search.clearSearch")}
            >
              <X size={ICON.md}  aria-hidden="true" />
            </button>
          )}
        </div>
      </div>

      {hasQuery && (
        <div className="mb-4 flex items-center gap-1" data-testid="search-kind-toggle" role="radiogroup" aria-label={t("common.search")}>
          <FilterChip active={searchKind === "media"} onClick={() => setSearchKind("media")} size="md">
            {t("media.search.sectionMedia")}
          </FilterChip>
          <FilterChip active={searchKind === "music"} onClick={() => setSearchKind("music")} size="md">
            {t("media.search.sectionMusic")}
          </FilterChip>
        </div>
      )}

      {hasQuery && searchKind === "media" && (
        <div className="mb-5 space-y-3">
          <div className="flex items-center gap-2 flex-wrap">
            <div className="flex items-center gap-1" data-testid="search-type-filters" role="radiogroup" aria-label={t("media.search.filterByType")}>
              {typeFilters.map((f) => (
                <FilterChip
                  key={f.value ?? "all"}
                  active={typeFilter === f.value}
                  onClick={() => setTypeFilter(f.value)}
                  size="md"
                  data-testid={`search-filter-${f.value === "MEDIA_ITEM_TYPE_MOVIE" ? "movies" : f.value === "MEDIA_ITEM_TYPE_SERIES" ? "series" : "all"}`}
                >
                  {f.label}
                </FilterChip>
              ))}
            </div>

            <FilterChip
              active={showFilters}
              onClick={() => setShowFilters((v) => !v)}
              size="md"
              icon={<ListFilter size={ICON.sm} />}
            >
              {t("media.search.filters")}
              {hasActiveFilters && !showFilters && (
                <span
                  className="w-2 h-2 rounded-full bg-[var(--accent)]"
                />
              )}
            </FilterChip>

            {hasActiveFilters && (
              <button
                onClick={clearAllFilters}
                className="text-xs hover:underline cursor-pointer text-[var(--text-muted)]"
              >
                {t("media.search.clearFilters")}
              </button>
            )}
          </div>

          {showFilters && (
            <div
              data-testid="search-filters-panel"
              className="flex flex-wrap gap-4 p-4 rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
            >
              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium text-[var(--text-muted)]">
                  {t("media.search.genre")}
                </label>
                <select
                  data-testid="search-filter-genre"
                  aria-label={t("media.search.genre")}
                  value={genre}
                  onChange={(e) => setGenre(e.target.value)}
                  className="px-3 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] cursor-pointer"
                >
                  <option value="">{t("media.search.allGenres")}</option>
                  {COMMON_GENRES.map((g) => (
                    <option key={g} value={g}>{t(`media.genres.${g.toLowerCase().replace(/[^a-z]/g, "")}`, { defaultValue: g })}</option>
                  ))}
                </select>
              </div>

              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium text-[var(--text-muted)]">
                  {t("media.search.year")}
                </label>
                <div className="flex items-center gap-2">
                  <input
                    data-testid="search-filter-year-min"
                    type="number"
                    aria-label={t("media.search.fromYear")}
                    placeholder={t("media.search.fromYear")}
                    value={yearMin}
                    onChange={(e) => setYearMin(e.target.value)}
                    className="w-20 px-2 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
                    min={1900}
                    max={2099}
                  />
                  <span className="text-xs text-[var(--text-muted)]">-</span>
                  <input
                    data-testid="search-filter-year-max"
                    type="number"
                    aria-label={t("media.search.toYear")}
                    placeholder={t("media.search.toYear")}
                    value={yearMax}
                    onChange={(e) => setYearMax(e.target.value)}
                    className="w-20 px-2 py-1.5 rounded-[var(--radius-md)] text-sm bg-[var(--surface-raised)] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
                    min={1900}
                    max={2099}
                  />
                </div>
              </div>

              <div className="flex flex-col gap-1.5">
                <label className="text-xs font-medium text-[var(--text-muted)]">
                  {t("media.search.sortBy")}
                </label>
                <select
                  data-testid="search-filter-sort"
                  aria-label={t("media.search.sortBy")}
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

      {hasQuery && (isLoading || musicLoading) && (
        <div>
          <div
            className="h-4 w-32 mb-4 animate-pulse rounded bg-[var(--surface-raised)]"
          />
          <MediaGridSkeleton count={12} />
        </div>
      )}

      {hasQuery && !isLoading && !musicLoading && items.length === 0 && musicItems.length === 0 && !musicIsError && !isError && (
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <EmptyState
            icon={<Search size={ICON["4xl"]} strokeWidth={1.5} />}
            title={t("media.search.noResultsFor", { query: debouncedQuery })}
            description={t("media.search.noResultsDesc")}
          />
        </div>
      )}

      {!hasQuery && !isLoading && recents.length > 0 && (
        <div className="mb-6">
          <div className="flex items-center justify-between mb-3">
            <span className="text-sm font-semibold text-[var(--text-muted)]">{t("media.search.recentSearches")}</span>
            <button
              onClick={clearRecents}
              className="text-xs hover:underline cursor-pointer text-[var(--text-muted)]"
            >
              {t("media.search.clearAll")}
            </button>
          </div>
          <div className="flex flex-wrap gap-2">
            {recents.map((r) => (
              <div
                key={r}
                className="flex items-center gap-1 px-3 py-1.5 rounded-full text-sm bg-[var(--surface)] border border-[var(--border)] text-[var(--text)]"
              >
                <button
                  className="hover:text-[var(--accent)] cursor-pointer transition-colors-fast"
                  onClick={() => setInputValue(r)}
                >
                  {r}
                </button>
                <button
                  onClick={() => removeRecent(r)}
                  className="ml-1 cursor-pointer transition-colors-fast hover:opacity-80 text-[var(--text-muted)]"
                  aria-label={t("common.removeItem", { item: r })}
                >
                  <X size={ICON.xs}  aria-hidden="true" />
                </button>
              </div>
            ))}
          </div>
        </div>
      )}

      {!hasQuery && !isLoading && recents.length === 0 && (
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <EmptyState
            icon={<Search size={ICON["4xl"]} strokeWidth={1.5} />}
            title={t("media.search.searchLibrary")}
            description={t("media.search.searchLibraryDesc")}
          />
        </div>
      )}

      {searchKind === "media" && items.length > 0 && (
        <>
          <p className="text-sm mb-4 text-[var(--text-muted)]">
            {t("media.search.resultCount", { count: totalHits })} {t("media.search.resultFor", { query: debouncedQuery })}
          </p>
          <MediaGrid>
            {items.map((item) => (
              <MediaCard key={item.id} item={item} showType lite />
            ))}
          </MediaGrid>
        </>
      )}

      {searchKind === "music" && hasQuery && !musicLoading && musicIsError && (
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <FetchErrorState error={musicError} onRetry={() => void refetchMusic()} />
        </div>
      )}

      {searchKind === "media" && hasQuery && !isLoading && isError && (
        <div
          className="rounded-[var(--radius-lg)] bg-[var(--surface)] border border-[var(--border)]"
        >
          <FetchErrorState error={error} onRetry={() => void refetch()} />
        </div>
      )}

      {searchKind === "music" && musicItems.length > 0 && (
        <div>
          <p className="text-sm mb-4 text-[var(--text-muted)]">
            {t("media.search.resultCount", { count: musicItems.length })} {t("media.search.resultFor", { query: debouncedQuery })}
          </p>
          <ul className="flex flex-col gap-1">
            {musicItems.map((track) => (
              <li key={track.id}>
                <Link
                  to="/music"
                  search={{ tab: "tracks", album: track.albumId || undefined, libraryId: track.libraryId || undefined }}
                  className="flex items-center gap-3 px-3 py-2 rounded-[var(--radius-md)] hover:bg-[var(--surface-raised)] transition-colors-fast"
                >
                  <div className="w-10 h-10 rounded overflow-hidden shrink-0 flex items-center justify-center bg-[var(--surface-raised)]">
                    {track.coverArtUrl ? (
                      <img src={coverArtSrc(track.coverArtUrl, "", 80)} alt="" aria-hidden="true" className="w-full h-full object-cover" onError={(e) => { e.currentTarget.style.display = "none"; }} />
                    ) : (
                      <Music size={ICON.lg} className="text-[var(--text-muted)]" aria-hidden="true" />
                    )}
                  </div>
                  <div className="flex-1 min-w-0">
                    <p className="text-sm font-medium truncate text-[var(--text)]">{track.title}</p>
                    <p className="text-xs text-[var(--text-muted)] truncate">
                      {track.artist}{track.album ? ` · ${track.album}` : ""}
                    </p>
                  </div>
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
